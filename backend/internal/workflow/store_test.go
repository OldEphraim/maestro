package workflow_test

import (
	"context"
	"testing"

	agentpkg "github.com/oldephraim/maestro/backend/internal/agent"
	"github.com/oldephraim/maestro/backend/internal/workflow"
)

func TestGetFullEdgeOrdering(t *testing.T) {
	pool := testPool(t)
	agentStore := agentpkg.NewStore(pool)
	wfStore := workflow.NewStore(pool)
	ctx := context.Background()

	a1 := createTestAgent(t, agentStore, "Agent1", "role")
	a2 := createTestAgent(t, agentStore, "Agent2", "role")
	a3 := createTestAgent(t, agentStore, "Agent3", "role")

	wf, err := wfStore.CreateWorkflow(ctx, workflow.Workflow{Name: "ordering-test", Status: "active"})
	if err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}

	n1, _ := wfStore.CreateNode(ctx, workflow.WorkflowNode{
		WorkflowID: wf.ID, AgentID: a1.ID, Label: "Source", IsEntry: true,
	})
	n2, _ := wfStore.CreateNode(ctx, workflow.WorkflowNode{
		WorkflowID: wf.ID, AgentID: a2.ID, Label: "Target A",
	})
	n3, _ := wfStore.CreateNode(ctx, workflow.WorkflowNode{
		WorkflowID: wf.ID, AgentID: a3.ID, Label: "Target B",
	})

	// Insert edges OUT OF ORDER: priority 2 first, then 0, then 1
	wfStore.CreateEdge(ctx, workflow.WorkflowEdge{
		WorkflowID: wf.ID, SourceNodeID: n1.ID, TargetNodeID: n3.ID,
		Condition: "fallback", Priority: 2,
	})
	wfStore.CreateEdge(ctx, workflow.WorkflowEdge{
		WorkflowID: wf.ID, SourceNodeID: n1.ID, TargetNodeID: n2.ID,
		Condition: "rejected", Priority: 0,
	})
	wfStore.CreateEdge(ctx, workflow.WorkflowEdge{
		WorkflowID: wf.ID, SourceNodeID: n1.ID, TargetNodeID: n2.ID,
		Condition: "approved", Priority: 1,
	})

	fw, err := wfStore.GetFull(ctx, wf.ID)
	if err != nil {
		t.Fatalf("GetFull: %v", err)
	}

	if len(fw.Edges) != 3 {
		t.Fatalf("expected 3 edges, got %d", len(fw.Edges))
	}

	// Edges should be sorted by priority ASC regardless of insertion order
	for i := 0; i < len(fw.Edges)-1; i++ {
		if fw.Edges[i].Priority > fw.Edges[i+1].Priority {
			t.Fatalf("edges not sorted by priority: edge[%d].priority=%d > edge[%d].priority=%d",
				i, fw.Edges[i].Priority, i+1, fw.Edges[i+1].Priority)
		}
	}

	if fw.Edges[0].Priority != 0 {
		t.Fatalf("first edge should have priority 0, got %d", fw.Edges[0].Priority)
	}
	if fw.Edges[2].Priority != 2 {
		t.Fatalf("last edge should have priority 2, got %d", fw.Edges[2].Priority)
	}
}
