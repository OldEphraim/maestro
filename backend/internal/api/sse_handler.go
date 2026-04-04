package api

import (
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/oldephraim/maestro/backend/internal/sse"
)

func SSEHandler(broadcaster *sse.Broadcaster) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		executionFilter := r.URL.Query().Get("executionId")

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		clientID := uuid.New().String()
		events := broadcaster.Subscribe(clientID)
		defer broadcaster.Unsubscribe(clientID)

		for {
			select {
			case <-r.Context().Done():
				return
			case event, ok := <-events:
				if !ok {
					return
				}
				// Server-side filtering
				if executionFilter != "" && event.ExecutionID != executionFilter {
					continue
				}
				fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, event.JSON())
				flusher.Flush()
			}
		}
	}
}
