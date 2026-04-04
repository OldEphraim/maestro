package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/oldephraim/maestro/backend/internal/agent"
	"github.com/oldephraim/maestro/backend/internal/workflow"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func parseUUID(r *http.Request, param string) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, param))
}

// --- Agent handlers ---

func agentListHandler(store *agent.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agents, err := store.List(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if agents == nil {
			agents = []agent.Agent{}
		}
		writeJSON(w, http.StatusOK, agents)
	}
}

func agentCreateHandler(store *agent.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var a agent.Agent
		if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if a.Model == "" {
			a.Model = "claude-sonnet-4-5-20250929"
		}
		created, err := store.Create(r.Context(), a)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, created)
	}
}

func agentGetHandler(store *agent.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseUUID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		a, err := store.GetWithMemory(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, a)
	}
}

func agentUpdateHandler(store *agent.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseUUID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		var a agent.Agent
		if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		a.ID = id
		updated, err := store.Update(r.Context(), a)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, updated)
	}
}

func agentDeleteHandler(store *agent.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseUUID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		if err := store.Delete(r.Context(), id); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func agentGetMemoryHandler(store *agent.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseUUID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		mem, err := store.GetMemory(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, mem)
	}
}

func agentSetMemoryHandler(store *agent.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseUUID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		var body struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := store.SetMemory(r.Context(), id, body.Key, body.Value); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func agentDeleteMemoryHandler(store *agent.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseUUID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		key := chi.URLParam(r, "key")
		if err := store.DeleteMemoryKey(r.Context(), id, key); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func agentGetSkillsHandler(store *agent.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseUUID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		skills, err := store.GetSkills(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if skills == nil {
			skills = []agent.Skill{}
		}
		writeJSON(w, http.StatusOK, skills)
	}
}

func agentAddSkillHandler(store *agent.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseUUID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		var body struct {
			Name        string   `json:"name"`
			Description string   `json:"description"`
			Steps       []string `json:"steps"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		skill, err := store.AddSkill(r.Context(), id, body.Name, body.Description, body.Steps)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, skill)
	}
}

func agentDeleteSkillHandler(store *agent.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		skillID, err := uuid.Parse(chi.URLParam(r, "skillId"))
		if err != nil {
			http.Error(w, "invalid skill id", http.StatusBadRequest)
			return
		}
		if err := store.DeleteSkill(r.Context(), skillID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func agentGetSchedulesHandler(store *agent.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseUUID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		schedules, err := store.GetSchedules(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if schedules == nil {
			schedules = []agent.Schedule{}
		}
		writeJSON(w, http.StatusOK, schedules)
	}
}

func agentUpsertScheduleHandler(store *agent.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseUUID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		var body struct {
			CronExpr   string `json:"cron_expr"`
			TaskPrompt string `json:"task_prompt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		sch, err := store.UpsertSchedule(r.Context(), id, body.CronExpr, body.TaskPrompt)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, sch)
	}
}

// --- Workflow handlers ---

func workflowListHandler(store *workflow.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		wfs, err := store.ListWorkflows(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if wfs == nil {
			wfs = []workflow.Workflow{}
		}
		writeJSON(w, http.StatusOK, wfs)
	}
}

func workflowCreateHandler(store *workflow.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var wf workflow.Workflow
		if err := json.NewDecoder(r.Body).Decode(&wf); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if wf.Status == "" {
			wf.Status = "draft"
		}
		created, err := store.CreateWorkflow(r.Context(), wf)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, created)
	}
}

func workflowGetHandler(store *workflow.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseUUID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		fw, err := store.GetFull(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, fw)
	}
}

func workflowUpdateHandler(store *workflow.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseUUID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		var wf workflow.Workflow
		if err := json.NewDecoder(r.Body).Decode(&wf); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		wf.ID = id
		updated, err := store.UpdateWorkflow(r.Context(), wf)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, updated)
	}
}

func workflowDeleteHandler(store *workflow.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseUUID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		if err := store.DeleteWorkflow(r.Context(), id); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func workflowExecuteHandler(engine *workflow.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseUUID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		var body struct {
			Trigger string `json:"trigger"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.Trigger == "" {
			body.Trigger = "manual"
		}
		// Use background context — the engine runs asynchronously after the HTTP response is sent.
		// The request context would cancel the engine goroutine when the response completes.
		execID, err := engine.Execute(context.Background(), id, body.Trigger)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"execution_id": execID.String()})
	}
}

func workflowCreateNodeHandler(store *workflow.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		wfID, err := parseUUID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		var n workflow.WorkflowNode
		if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		n.WorkflowID = wfID
		created, err := store.CreateNode(r.Context(), n)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, created)
	}
}

func workflowDeleteNodeHandler(store *workflow.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nodeID, err := uuid.Parse(chi.URLParam(r, "nodeId"))
		if err != nil {
			http.Error(w, "invalid node id", http.StatusBadRequest)
			return
		}
		if err := store.DeleteNode(r.Context(), nodeID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func workflowCreateEdgeHandler(store *workflow.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		wfID, err := parseUUID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		var e workflow.WorkflowEdge
		if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		e.WorkflowID = wfID
		created, err := store.CreateEdge(r.Context(), e)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, created)
	}
}

func workflowDeleteEdgeHandler(store *workflow.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		edgeID, err := uuid.Parse(chi.URLParam(r, "edgeId"))
		if err != nil {
			http.Error(w, "invalid edge id", http.StatusBadRequest)
			return
		}
		if err := store.DeleteEdge(r.Context(), edgeID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

// --- Execution handlers ---

func executionListHandler(store *workflow.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		execs, err := store.ListExecutions(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if execs == nil {
			execs = []workflow.Execution{}
		}
		writeJSON(w, http.StatusOK, execs)
	}
}

func executionGetHandler(store *workflow.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseUUID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		exec, err := store.GetExecution(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, exec)
	}
}

func executionMessagesHandler(store *workflow.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseUUID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		msgs, err := store.GetMessages(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if msgs == nil {
			msgs = []workflow.Message{}
		}
		writeJSON(w, http.StatusOK, msgs)
	}
}

func executionLogsHandler(store *workflow.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseUUID(r, "id")
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		logs, err := store.GetLogs(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if logs == nil {
			logs = []workflow.Log{}
		}
		writeJSON(w, http.StatusOK, logs)
	}
}

func executionCancelHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Stub — cancel support requires per-execution cancel contexts (stretch goal)
		writeJSON(w, http.StatusOK, map[string]string{"status": "stub"})
	}
}

// --- Template handlers ---

type templateInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type templateFile struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Agents      []struct {
		TempID       string   `json:"temp_id"`
		Name         string   `json:"name"`
		Role         string   `json:"role"`
		SystemPrompt string   `json:"system_prompt"`
		Model        string   `json:"model"`
		Tools        []string `json:"tools"`
		Channels     []string `json:"channels"`
	} `json:"agents"`
	Nodes []struct {
		TempID   string  `json:"temp_id"`
		AgentRef string  `json:"agent_ref"`
		Label    string  `json:"label"`
		PosX     float64 `json:"position_x"`
		PosY     float64 `json:"position_y"`
		IsEntry  bool    `json:"is_entry"`
	} `json:"nodes"`
	Edges []struct {
		SourceRef string `json:"source_ref"`
		TargetRef string `json:"target_ref"`
		Condition string `json:"condition"`
		Priority  int    `json:"priority"`
	} `json:"edges"`
}

func templateListHandler(templatesDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		entries, err := os.ReadDir(templatesDir)
		if err != nil {
			writeJSON(w, http.StatusOK, []any{})
			return
		}
		var templates []templateInfo
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(templatesDir, entry.Name()))
			if err != nil {
				continue
			}
			var tf templateFile
			if err := json.Unmarshal(data, &tf); err != nil {
				continue
			}
			templates = append(templates, templateInfo{ID: tf.ID, Name: tf.Name, Description: tf.Description})
		}
		if templates == nil {
			templates = []templateInfo{}
		}
		writeJSON(w, http.StatusOK, templates)
	}
}

func templateLoadHandler(templatesDir string, agents *agent.Store, workflows *workflow.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		templateID := chi.URLParam(r, "id")

		// Find template file
		entries, err := os.ReadDir(templatesDir)
		if err != nil {
			http.Error(w, "templates directory not found", http.StatusInternalServerError)
			return
		}

		var tf templateFile
		found := false
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(templatesDir, entry.Name()))
			if err != nil {
				continue
			}
			var candidate templateFile
			if err := json.Unmarshal(data, &candidate); err != nil {
				continue
			}
			if candidate.ID == templateID {
				tf = candidate
				found = true
				break
			}
		}
		if !found {
			http.Error(w, "template not found", http.StatusNotFound)
			return
		}

		ctx := r.Context()

		// Create agents, mapping temp_id → real UUID
		agentMap := make(map[string]uuid.UUID)
		for _, ta := range tf.Agents {
			a := agent.Agent{
				Name:         ta.Name,
				Role:         ta.Role,
				SystemPrompt: ta.SystemPrompt,
				Model:        ta.Model,
				Tools:        ta.Tools,
				Channels:     ta.Channels,
			}
			if a.Model == "" {
				a.Model = "claude-sonnet-4-5-20250929"
			}
			if a.Tools == nil {
				a.Tools = []string{}
			}
			if a.Channels == nil {
				a.Channels = []string{}
			}
			created, err := agents.Create(ctx, a)
			if err != nil {
				http.Error(w, fmt.Sprintf("create agent %s: %v", ta.Name, err), http.StatusInternalServerError)
				return
			}
			agentMap[ta.TempID] = created.ID
		}

		// Create workflow
		wf := workflow.Workflow{
			Name:        tf.Name,
			Description: tf.Description,
			TemplateID:  tf.ID,
			Status:      "active",
		}
		createdWf, err := workflows.CreateWorkflow(ctx, wf)
		if err != nil {
			http.Error(w, fmt.Sprintf("create workflow: %v", err), http.StatusInternalServerError)
			return
		}

		// Create nodes, mapping temp_id → real UUID
		nodeMap := make(map[string]uuid.UUID)
		for _, tn := range tf.Nodes {
			agentID, ok := agentMap[tn.AgentRef]
			if !ok {
				http.Error(w, fmt.Sprintf("agent ref %s not found", tn.AgentRef), http.StatusInternalServerError)
				return
			}
			n := workflow.WorkflowNode{
				WorkflowID: createdWf.ID,
				AgentID:    agentID,
				Label:      tn.Label,
				PositionX:  tn.PosX,
				PositionY:  tn.PosY,
				IsEntry:    tn.IsEntry,
			}
			createdNode, err := workflows.CreateNode(ctx, n)
			if err != nil {
				http.Error(w, fmt.Sprintf("create node %s: %v", tn.Label, err), http.StatusInternalServerError)
				return
			}
			nodeMap[tn.TempID] = createdNode.ID
		}

		// Create edges
		for _, te := range tf.Edges {
			sourceID, ok := nodeMap[te.SourceRef]
			if !ok {
				http.Error(w, fmt.Sprintf("source ref %s not found", te.SourceRef), http.StatusInternalServerError)
				return
			}
			targetID, ok := nodeMap[te.TargetRef]
			if !ok {
				http.Error(w, fmt.Sprintf("target ref %s not found", te.TargetRef), http.StatusInternalServerError)
				return
			}
			e := workflow.WorkflowEdge{
				WorkflowID:   createdWf.ID,
				SourceNodeID: sourceID,
				TargetNodeID: targetID,
				Condition:    te.Condition,
				Priority:     te.Priority,
			}
			if _, err := workflows.CreateEdge(ctx, e); err != nil {
				http.Error(w, fmt.Sprintf("create edge: %v", err), http.StatusInternalServerError)
				return
			}
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"workflow_id": createdWf.ID.String(),
			"template_id": tf.ID,
			"status":      "loaded",
		})
	}
}

// --- Webhook handlers ---

func whatsappWebhookHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// TODO: validate X-Twilio-Signature in production
		// Stub — will be wired in Phase 3
		w.WriteHeader(http.StatusOK)
	}
}

// --- Mock data ---

func mockFailedTransactionsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		txns := []map[string]any{
			{"id": "txn_001", "amount": 99.99, "currency": "USD", "customer_phone": "+14405239475", "failure_reason": "insufficient_funds", "provider": "stripe", "failed_at": now.Add(-1 * time.Hour).Format(time.RFC3339)},
			{"id": "txn_002", "amount": 249.00, "currency": "USD", "customer_phone": "+14405239475", "failure_reason": "card_declined", "provider": "adyen", "failed_at": now.Add(-55 * time.Minute).Format(time.RFC3339)},
			{"id": "txn_003", "amount": 15.50, "currency": "EUR", "customer_phone": "+14405239475", "failure_reason": "expired_card", "provider": "checkout", "failed_at": now.Add(-30 * time.Minute).Format(time.RFC3339)},
		}
		writeJSON(w, http.StatusOK, txns)
	}
}
