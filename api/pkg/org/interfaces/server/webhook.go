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

// webhookHandler accepts inbound POSTs on /webhooks/<topicID> and
// turns each request body into an Event on that Topic. The Topic
// must exist and have transport.kind == webhook; otherwise 404.
//
// Source attribution on the resulting Event is empty (system-emitted,
// per streaming.NewEvent's contract). The dispatcher is invoked so Bots
// subscribed to the Topic are activated; the broadcaster is notified so
// any long-poll observer wakes.
func (s *Server) webhookHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		topicID := streaming.TopicID(r.PathValue("topicID"))
		if topicID == "" {
			http.Error(w, "missing topicID", http.StatusNotFound)
			return
		}
		// Webhook URL shape: /webhooks/{org}/{topicID}. The org segment
		// is required under composite (id, org_id) PKs — topic IDs are
		// not globally unique across helix tenants.
		orgID := r.PathValue("org")
		if orgID == "" {
			orgID = OrgIDFromContext(r.Context())
		}
		if orgID == "" {
			http.Error(w, "missing org", http.StatusNotFound)
			return
		}

		topic, err := s.queries.GetTopic(r.Context(), orgID, topicID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				http.Error(w, fmt.Sprintf("topic %q: not found", topicID), http.StatusNotFound)
				return
			}
			s.logger.Error("webhook: lookup topic", "topic", topicID, "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if topic.Transport.Kind != transport.KindWebhook {
			http.Error(w, fmt.Sprintf("topic %q is not a webhook topic", topicID), http.StatusNotFound)
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
			s.logger.Error("webhook: publishing service not wired", "topic", topicID)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		// Append → notify → dispatch through the publishing service.
		// From is empty — webhook callers are arbitrary external systems
		// with no helix Bot identity; routing decisions about "who
		// sent this" belong in the receiving Bot's prompt.
		event, err := s.publishing.Publish(r.Context(), orgID, topicID, "", streaming.Message{Body: string(body)})
		if err != nil {
			s.logger.Error("webhook: publish event", "topic", topicID, "err", err)
			http.Error(w, "append event", http.StatusInternalServerError)
			return
		}

		ack, _ := json.Marshal(map[string]string{
			"id":       string(event.ID),
			"topicId": string(topicID),
		})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(ack)
	})
}
