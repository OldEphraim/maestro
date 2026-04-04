package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/oldephraim/maestro/backend/internal/agent"
	"github.com/oldephraim/maestro/backend/internal/sse"
	"github.com/oldephraim/maestro/backend/internal/workflow"
)

func NewRouter(agents *agent.Store, workflows *workflow.Store, broadcaster *sse.Broadcaster) http.Handler {
	r := chi.NewRouter()

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3000"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "Authorization"},
		AllowCredentials: false,
	}))
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Health
	r.Get("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Agents
	r.Route("/api/agents", func(r chi.Router) {
		r.Get("/", agentListHandler(agents))
		r.Post("/", agentCreateHandler(agents))
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", agentGetHandler(agents))
			r.Put("/", agentUpdateHandler(agents))
			r.Delete("/", agentDeleteHandler(agents))
			r.Get("/memory", agentGetMemoryHandler(agents))
			r.Put("/memory", agentSetMemoryHandler(agents))
			r.Delete("/memory/{key}", agentDeleteMemoryHandler(agents))
			r.Get("/skills", agentGetSkillsHandler(agents))
			r.Post("/skills", agentAddSkillHandler(agents))
			r.Delete("/skills/{skillId}", agentDeleteSkillHandler(agents))
			r.Get("/schedules", agentGetSchedulesHandler(agents))
			r.Post("/schedules", agentUpsertScheduleHandler(agents))
		})
	})

	// Workflows
	r.Route("/api/workflows", func(r chi.Router) {
		r.Get("/", workflowListHandler(workflows))
		r.Post("/", workflowCreateHandler(workflows))
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", workflowGetHandler(workflows))
			r.Put("/", workflowUpdateHandler(workflows))
			r.Delete("/", workflowDeleteHandler(workflows))
			r.Post("/execute", workflowExecuteHandler())
			r.Post("/nodes", workflowCreateNodeHandler(workflows))
			r.Delete("/nodes/{nodeId}", workflowDeleteNodeHandler(workflows))
			r.Post("/edges", workflowCreateEdgeHandler(workflows))
			r.Delete("/edges/{edgeId}", workflowDeleteEdgeHandler(workflows))
		})
	})

	// Executions
	r.Route("/api/executions", func(r chi.Router) {
		r.Get("/", executionListHandler(workflows))
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", executionGetHandler(workflows))
			r.Get("/messages", executionMessagesHandler(workflows))
			r.Get("/logs", executionLogsHandler(workflows))
			r.Delete("/", executionCancelHandler())
		})
	})

	// Templates
	r.Get("/api/templates", templateListHandler())
	r.Post("/api/templates/{id}/load", templateLoadHandler())

	// SSE
	r.Get("/api/events", SSEHandler(broadcaster))

	// Webhooks
	r.Post("/api/webhooks/whatsapp", whatsappWebhookHandler())

	// Mock data
	r.Get("/api/mock/failed-transactions", mockFailedTransactionsHandler())

	return r
}
