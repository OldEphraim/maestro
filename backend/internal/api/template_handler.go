package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/oldephraim/maestro/backend/internal/agent"
	"github.com/oldephraim/maestro/backend/internal/workflow"
)

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
