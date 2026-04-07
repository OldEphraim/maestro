package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/oldephraim/maestro/backend/internal/workflow"
)

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

func workflowUpdateEdgeHandler(store *workflow.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		edgeID, err := uuid.Parse(chi.URLParam(r, "edgeId"))
		if err != nil {
			http.Error(w, "invalid edge id", http.StatusBadRequest)
			return
		}
		var e workflow.WorkflowEdge
		if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		e.ID = edgeID
		updated, err := store.UpdateEdge(r.Context(), e)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, updated)
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
