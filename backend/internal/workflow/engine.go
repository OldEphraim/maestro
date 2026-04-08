package workflow

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/oldephraim/maestro/backend/internal/agent"
	"github.com/oldephraim/maestro/backend/internal/channels"
	"github.com/oldephraim/maestro/backend/internal/runtime"
	"github.com/oldephraim/maestro/backend/internal/sse"
)

const DefaultMaxIterations = 5

type Engine struct {
	Agents    *agent.Store
	Workflows *Store
	Runtime   runtime.Runner
	SSE       *sse.Broadcaster
	WhatsApp  channels.WhatsAppClient
}

func NewEngine(agents *agent.Store, workflows *Store, runner runtime.Runner, broadcaster *sse.Broadcaster, whatsapp channels.WhatsAppClient) *Engine {
	return &Engine{
		Agents:    agents,
		Workflows: workflows,
		Runtime:   runner,
		SSE:       broadcaster,
		WhatsApp:  whatsapp,
	}
}

func (e *Engine) Execute(ctx context.Context, workflowID uuid.UUID, trigger string) (uuid.UUID, error) {
	wf, err := e.Workflows.GetFull(ctx, workflowID)
	if err != nil {
		return uuid.Nil, err
	}

	exec := &Execution{
		ID:            uuid.New(),
		WorkflowID:    &workflowID,
		ExecutionType: "workflow",
		Status:        "running",
		TriggeredBy:   trigger,
	}
	if err := e.Workflows.CreateExecution(ctx, exec); err != nil {
		return uuid.Nil, fmt.Errorf("engine.Execute: create execution: %w", err)
	}

	entry := wf.EntryNode()
	if entry == nil {
		e.failExecution(ctx, exec, "no entry node found")
		return uuid.Nil, errors.New("no entry node found")
	}

	e.SSE.Publish(sse.Event{Type: "ExecutionStarted", ExecutionID: exec.ID.String(), AgentID: entry.AgentID.String()})

	go e.runNode(ctx, exec, wf, entry, trigger)
	return exec.ID, nil
}

func (e *Engine) ExecuteAdhoc(ctx context.Context, agentID uuid.UUID, trigger, input string) (uuid.UUID, error) {
	exec := &Execution{
		ID:            uuid.New(),
		ExecutionType: "adhoc",
		Status:        "running",
		TriggeredBy:   trigger,
	}
	if err := e.Workflows.CreateExecution(ctx, exec); err != nil {
		return uuid.Nil, fmt.Errorf("engine.ExecuteAdhoc: %w", err)
	}

	e.SSE.Publish(sse.Event{Type: "ExecutionStarted", ExecutionID: exec.ID.String()})

	go func() {
		ag, err := e.Agents.GetWithMemory(ctx, agentID)
		if err != nil {
			e.failExecution(ctx, exec, fmt.Sprintf("load agent: %v", err))
			return
		}

		e.SSE.Publish(sse.Event{Type: "AgentStarted", ExecutionID: exec.ID.String(), AgentID: agentID.String()})

		output, usage, err := e.Runtime.Run(ctx, ag, input)
		if err != nil {
			e.failExecution(ctx, exec, err.Error())
			return
		}

		e.Workflows.CreateMessage(ctx, exec.ID, agentID, nil, output, "internal")
		e.Workflows.RecordCost(ctx, exec.ID, agentID, Usage{
			TokensIn: usage.TokensIn, TokensOut: usage.TokensOut,
			EstimatedCostUSD: usage.EstimatedCostUSD, Source: usage.Source,
		})

		e.SSE.Publish(sse.Event{Type: "AgentCompleted", ExecutionID: exec.ID.String(), AgentID: agentID.String()})
		e.Workflows.SetStatus(ctx, exec.ID, "completed")
		e.Workflows.SetCompletedAt(ctx, exec.ID, time.Now())
		e.SSE.Publish(sse.Event{Type: "ExecutionCompleted", ExecutionID: exec.ID.String()})
	}()

	return exec.ID, nil
}

func (e *Engine) runNode(ctx context.Context, exec *Execution, wf *FullWorkflow, node *WorkflowNode, input string) {
	count, err := e.Workflows.IncrementIterationCount(ctx, exec.ID)
	if err != nil {
		e.failExecution(ctx, exec, fmt.Sprintf("increment iteration: %v", err))
		return
	}
	if count > maxIterations() {
		e.failExecution(ctx, exec, "max iterations exceeded")
		return
	}

	ag, err := e.Agents.GetWithMemory(ctx, node.AgentID)
	if err != nil {
		e.failExecution(ctx, exec, fmt.Sprintf("load agent %s: %v", node.AgentID, err))
		return
	}

	if err := e.Workflows.CheckGuardrails(ctx, ag.ID, ag.Guardrails); err != nil {
		e.failExecution(ctx, exec, fmt.Sprintf("guardrails: %v", err))
		return
	}

	stepCtx, cancel := context.WithTimeout(ctx, stepTimeout())
	defer cancel()

	log.Printf("[engine] exec=%s agent=%s (%s) starting", exec.ID, ag.Name, node.Label)
	e.SSE.Publish(sse.Event{Type: "AgentStarted", ExecutionID: exec.ID.String(), AgentID: node.AgentID.String()})

	output, usage, err := e.Runtime.Run(stepCtx, ag, input)
	if errors.Is(err, runtime.ErrStepTimeout) {
		log.Printf("[engine] exec=%s agent=%s timed out", exec.ID, ag.Name)
		e.Workflows.SetStatus(ctx, exec.ID, "timed_out")
		e.SSE.Publish(sse.Event{Type: "StepTimedOut", ExecutionID: exec.ID.String(), AgentID: node.AgentID.String()})
		return
	}
	if err != nil {
		e.failExecution(ctx, exec, fmt.Sprintf("agent %s: %v", ag.Name, err))
		return
	}

	log.Printf("[engine] exec=%s agent=%s completed (tokens: %d in, %d out)", exec.ID, ag.Name, usage.TokensIn, usage.TokensOut)

	// Handle WhatsApp actions
	if ag.HasChannel("whatsapp") {
		e.handleWhatsAppAction(ctx, exec, ag, output)
	}

	// Persist message and cost
	e.Workflows.CreateMessage(ctx, exec.ID, node.AgentID, nil, output, "internal")
	e.Workflows.RecordCost(ctx, exec.ID, node.AgentID, Usage{
		TokensIn: usage.TokensIn, TokensOut: usage.TokensOut,
		EstimatedCostUSD: usage.EstimatedCostUSD, Source: usage.Source,
	})

	e.SSE.Publish(sse.Event{Type: "AgentCompleted", ExecutionID: exec.ID.String(), AgentID: node.AgentID.String()})

	// First-match edge evaluation
	for _, edge := range wf.OutgoingEdges(node.ID) {
		if evaluateCondition(output, edge.Condition) {
			target := wf.Node(edge.TargetNodeID)
			if target == nil {
				e.failExecution(ctx, exec, fmt.Sprintf("edge target node %s not found", edge.TargetNodeID))
				return
			}
			log.Printf("[engine] exec=%s routing %s → %s (condition: %q)", exec.ID, node.Label, target.Label, edge.Condition)
			e.SSE.Publish(sse.Event{
				Type:        "MessageDispatched",
				ExecutionID: exec.ID.String(),
				From:        node.AgentID.String(),
				To:          target.AgentID.String(),
			})
			go e.runNode(ctx, exec, wf, target, output)
			return
		}
	}

	// No matching edge = terminal node
	log.Printf("[engine] exec=%s completed (terminal node: %s)", exec.ID, node.Label)
	e.Workflows.SetStatus(ctx, exec.ID, "completed")
	e.Workflows.SetCompletedAt(ctx, exec.ID, time.Now())
	e.SSE.Publish(sse.Event{Type: "ExecutionCompleted", ExecutionID: exec.ID.String()})
}

func (e *Engine) handleWhatsAppAction(ctx context.Context, exec *Execution, ag agent.AgentWithMemory, output string) {
	for _, line := range strings.Split(output, "\n") {
		to, msg, ok := parseWhatsAppAction(line)
		if !ok {
			continue
		}
		if err := e.WhatsApp.Send(ctx, to, msg); err != nil {
			e.Workflows.LogError(ctx, exec.ID, ag.ID, "WhatsApp send failed: "+err.Error())
		} else {
			log.Printf("[engine] exec=%s WhatsApp sent to %s", exec.ID, to)
			e.Workflows.CreateMessage(ctx, exec.ID, ag.ID, nil, msg, "whatsapp")
			e.SSE.Publish(sse.Event{
				Type:        "WhatsAppSent",
				ExecutionID: exec.ID.String(),
				AgentID:     ag.ID.String(),
				To:          to,
			})
		}
	}
}

func (e *Engine) failExecution(ctx context.Context, exec *Execution, reason string) {
	log.Printf("[engine] exec=%s FAILED: %s", exec.ID, reason)
	e.Workflows.SetStatus(ctx, exec.ID, "failed")
	e.Workflows.SetCompletedAt(ctx, exec.ID, time.Now())
	e.Workflows.LogEvent(ctx, exec.ID, uuid.Nil, "error", reason, nil)
	e.SSE.Publish(sse.Event{Type: "ExecutionFailed", ExecutionID: exec.ID.String(), Payload: reason})
}

func maxIterations() int {
	if v := os.Getenv("MAX_ITERATIONS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return DefaultMaxIterations
}

func stepTimeout() time.Duration {
	if v := os.Getenv("AGENT_STEP_TIMEOUT_SECS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return time.Duration(n) * time.Second
		}
	}
	return 60 * time.Second
}
