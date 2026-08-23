package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

// maxWebhookBody caps the body size we'll accept on a webhook POST.
// 1 MiB is comfortable for text payloads and prevents an obvious DoS.
const maxWebhookBody = 1 << 20

// webhookHandler accepts inbound POSTs on /webhooks/<triggerID> and
// turns each request body into an event on that Trigger. The Trigger
// must exist and have kind == webhook; otherwise 404.
//
// Source attribution on the resulting event is empty (system-emitted,
// per streaming.NewEvent's contract). Publishing routes it to every
// Worker attached to the Trigger and wakes any long-poll observer.
func (s *Server) webhookHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		triggerID := r.PathValue("triggerID")
		if triggerID == "" {
			http.Error(w, "missing triggerID", http.StatusNotFound)
			return
		}
		// Webhook URL shape: /webhooks/{org}/{triggerID}. The org segment
		// is required under composite (id, org_id) PKs — Trigger IDs are
		// not globally unique across helix tenants.
		orgID := r.PathValue("org")
		if orgID == "" {
			orgID = OrgIDFromContext(r.Context())
		}
		if orgID == "" {
			http.Error(w, "missing org", http.StatusNotFound)
			return
		}

		trg, err := s.queries.GetTrigger(r.Context(), orgID, triggerID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				http.Error(w, fmt.Sprintf("trigger %q: not found", triggerID), http.StatusNotFound)
				return
			}
			s.logger.Error("webhook: lookup trigger", "trigger", triggerID, "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if trg.Kind != transport.KindWebhook {
			http.Error(w, fmt.Sprintf("trigger %q is not a webhook trigger", triggerID), http.StatusNotFound)
			return
		}

		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBody))
		if err != nil {
			http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if len(body) == 0 {
			http.Error(w, "body is empty", http.StatusBadRequest)
			return
		}

		if s.publishing == nil {
			s.logger.Error("webhook: publishing service not wired", "trigger", triggerID)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		// Append → notify → route through the publishing service.
		// From is empty — webhook callers are arbitrary external systems
		// with no helix Bot identity; routing decisions about "who
		// sent this" belong in the receiving Bot's prompt.
		event, err := s.publishing.PublishToTrigger(r.Context(), orgID, triggerID, "", streaming.Message{Body: string(body)})
		if err != nil {
			s.logger.Error("webhook: publish event", "trigger", triggerID, "err", err)
			http.Error(w, "append event", http.StatusInternalServerError)
			return
		}

		ack, _ := json.Marshal(map[string]string{
			"id":        string(event.ID),
			"triggerId": triggerID,
		})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(ack)
	})
}
