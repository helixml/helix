package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/helixml/helix/api/pkg/org/event"
	"github.com/helixml/helix/api/pkg/org/message"
	"github.com/helixml/helix/api/pkg/org/store"
	"github.com/helixml/helix/api/pkg/org/stream"
	"github.com/helixml/helix/api/pkg/org/transport"
	"github.com/helixml/helix/helix-org/domain"
)

// nowUTC returns the current wall-clock time in UTC. Kept as a package
// helper so handlers stay short and the time source is easy to audit.
func nowUTC() time.Time { return time.Now().UTC() }

// maxWebhookBody caps the body size we'll accept on a webhook POST.
// 1 MiB is comfortable for text payloads and prevents an obvious DoS.
const maxWebhookBody = 1 << 20

// webhookHandler accepts inbound POSTs on /webhooks/<streamID> and
// turns each request body into an Event on that Stream. The Stream
// must exist and have transport.kind == webhook; otherwise 404.
//
// Source attribution on the resulting Event is empty (system-emitted,
// per domain.NewEvent's contract). The dispatcher is invoked so AI
// Workers subscribed to the Stream are activated; the broadcaster is
// notified so any long-poll observer wakes.
func (s *Server) webhookHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		streamID := stream.ID(r.PathValue("streamID"))
		if streamID == "" {
			http.Error(w, "missing streamID", http.StatusNotFound)
			return
		}

		stream, err := s.store.Streams.Get(r.Context(), streamID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				http.Error(w, fmt.Sprintf("stream %q: not found", streamID), http.StatusNotFound)
				return
			}
			s.logger.Error("webhook: lookup stream", "stream", streamID, "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if stream.Transport.Kind != transport.KindWebhook {
			http.Error(w, fmt.Sprintf("stream %q is not a webhook stream", streamID), http.StatusNotFound)
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

		// Wrap the inbound bytes into the canonical Message envelope.
		// From is empty — webhook callers are arbitrary external systems
		// with no helix Worker identity; routing decisions about "who
		// sent this" belong in the receiving Role's prompt.
		event, err := domain.NewMessageEvent(
			event.ID("e-"+uuid.NewString()),
			streamID,
			"", // system-emitted; webhooks have no Worker source
			message.Message{Body: string(body)},
			nowUTC(),
		)
		if err != nil {
			http.Error(w, "build event: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.store.Events.Append(r.Context(), event); err != nil {
			s.logger.Error("webhook: append event", "stream", streamID, "err", err)
			http.Error(w, "append event", http.StatusInternalServerError)
			return
		}

		if s.broadcaster != nil {
			s.broadcaster.Notify(streamID)
		}
		if s.dispatcher != nil {
			s.dispatcher.Dispatch(r.Context(), event)
		}

		ack, _ := json.Marshal(map[string]string{
			"id":       string(event.ID),
			"streamId": string(streamID),
		})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(ack)
	})
}
