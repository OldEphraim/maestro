package api

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/oldephraim/maestro/backend/internal/agent"
	"github.com/oldephraim/maestro/backend/internal/channels"
	"github.com/oldephraim/maestro/backend/internal/sse"
	"github.com/oldephraim/maestro/backend/internal/workflow"
)

func whatsappWebhookHandler(agents *agent.Store, engine *workflow.Engine, broadcaster *sse.Broadcaster) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// TODO: validate X-Twilio-Signature in production
		from, body, err := channels.ParseTwilioWebhook(r)
		if err != nil {
			log.Printf("[whatsapp] parse error: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		log.Printf("[whatsapp] inbound from %s: %s", from, body)

		// Find agent configured for whatsapp channel
		ag, err := agents.FindByChannel(r.Context(), "whatsapp")
		if err != nil {
			log.Printf("[whatsapp] no agent with whatsapp channel: %v", err)
			http.Error(w, "no whatsapp agent configured", http.StatusNotFound)
			return
		}

		// Publish SSE event for inbound message
		broadcaster.Publish(sse.Event{
			Type:    "ExternalMessageReceived",
			From:    from,
			Payload: body,
		})

		// Run ad-hoc execution in background
		go func() {
			ctx := context.Background()
			execID, err := engine.ExecuteAdhoc(ctx, ag.ID, "whatsapp", body)
			if err != nil {
				log.Printf("[whatsapp] execute adhoc failed: %v", err)
				return
			}

			// Wait for execution to complete, then send reply
			// Poll for completion (engine runs async)
			for i := 0; i < 120; i++ {
				time.Sleep(1 * time.Second)
				exec, err := engine.Workflows.GetExecution(ctx, execID)
				if err != nil {
					continue
				}
				if exec.Status == "completed" || exec.Status == "failed" || exec.Status == "timed_out" {
					// Get the agent's response message
					msgs, err := engine.Workflows.GetMessages(ctx, execID)
					if err != nil || len(msgs) == 0 {
						log.Printf("[whatsapp] no response messages for exec %s", execID)
						return
					}
					reply := msgs[len(msgs)-1].Content
					// Truncate if too long for WhatsApp (1600 char limit)
					if len(reply) > 1500 {
						reply = reply[:1500] + "..."
					}
					if err := engine.WhatsApp.Send(ctx, from, reply); err != nil {
						log.Printf("[whatsapp] reply failed: %v", err)
					} else {
						log.Printf("[whatsapp] replied to %s (exec=%s)", from, execID)
					}
					return
				}
			}
			log.Printf("[whatsapp] exec %s did not complete within 120s", execID)
		}()

		// Return 200 immediately — Twilio expects a fast response
		w.WriteHeader(http.StatusOK)
	}
}
