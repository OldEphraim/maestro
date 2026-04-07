package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/oldephraim/maestro/backend/internal/agent"
)

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

func agentDeleteScheduleHandler(store *agent.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scheduleID, err := uuid.Parse(chi.URLParam(r, "scheduleId"))
		if err != nil {
			http.Error(w, "invalid schedule id", http.StatusBadRequest)
			return
		}
		if err := store.DeleteSchedule(r.Context(), scheduleID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func agentToggleScheduleHandler(store *agent.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scheduleID, err := uuid.Parse(chi.URLParam(r, "scheduleId"))
		if err != nil {
			http.Error(w, "invalid schedule id", http.StatusBadRequest)
			return
		}
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := store.SetEnabled(r.Context(), scheduleID, body.Enabled); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}
