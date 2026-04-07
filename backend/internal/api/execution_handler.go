package api

import (
	"net/http"
	"time"

	"github.com/oldephraim/maestro/backend/internal/workflow"
)

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

type executionWithCosts struct {
	workflow.Execution
	CostSummary *workflow.CostSummary `json:"cost_summary,omitempty"`
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
		costs, err := store.GetCostSummary(r.Context(), id)
		if err != nil {
			// Non-fatal: return execution without costs
			writeJSON(w, http.StatusOK, executionWithCosts{Execution: exec})
			return
		}
		writeJSON(w, http.StatusOK, executionWithCosts{Execution: exec, CostSummary: &costs})
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
