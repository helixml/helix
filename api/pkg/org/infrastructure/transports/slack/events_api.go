package slack

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
)

// SigningSecretFunc resolves the global Slack app's signing secret (the
// REST-mode request-authenticity key, NFR-4). Backed by the GlobalApp
// port at the composition root. Returning empty string + nil error
// means "no app configured" — the handler treats that as inert and
// rejects deliveries (FR-3).
type SigningSecretFunc func(ctx context.Context) (string, error)

// EventsAPI is the REST ingress source (Slack Events API). It verifies
// the request signature against the global app's signing secret,
// answers the url_verification handshake, and forwards real events to
// the shared ingest path. One handler serves every per-org install;
// routing to the right org happens downstream in the ingest, keyed on
// team_id (FR-17).
type EventsAPI struct {
	receiver      Receiver
	signingSecret SigningSecretFunc
	logger        *slog.Logger
}

// NewEventsAPI builds the REST source.
func NewEventsAPI(r Receiver, signingSecret SigningSecretFunc, logger *slog.Logger) *EventsAPI {
	if logger == nil {
		logger = slog.Default()
	}
	return &EventsAPI{receiver: r, signingSecret: signingSecret, logger: logger}
}

// maxBody caps the inbound body size. Slack event payloads are small;
// 1 MiB is generous.
const maxBody = 1 << 20

// Handler returns the http.Handler Slack POSTs events to.
//
// Status codes:
//   - 405 on non-POST
//   - 503 when no global app / signing secret is configured (inert; FR-3)
//   - 401 on missing, malformed, stale, or mismatched signature (NFR-4)
//   - 400 on an unparseable body
//   - 200 on the url_verification challenge (body = challenge) and on
//     every accepted event (Slack needs a prompt 2xx or it retries)
func (e *EventsAPI) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		secret, err := e.signingSecret(r.Context())
		if err != nil || secret == "" {
			// No global app configured: the subsystem is inert. Reject so
			// a misconfigured Slack app subscription doesn't look healthy.
			e.logger.Info("slack.events: no signing secret — inert", "err", err)
			http.Error(w, "slack not configured", http.StatusServiceUnavailable)
			return
		}

		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
		if err != nil {
			http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
			return
		}

		// Signature first — fail closed before parsing an adversarial body.
		// NewSecretsVerifier also enforces the 5-minute timestamp window,
		// so a replayed (stale) delivery is rejected here too.
		verifier, err := slack.NewSecretsVerifier(r.Header, secret)
		if err != nil {
			e.logger.Warn("slack.events: build verifier", "err", err)
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
		if _, err := verifier.Write(body); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if err := verifier.Ensure(); err != nil {
			e.logger.Warn("slack.events: bad signature", "err", err)
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}

		// url_verification handshake — Slack signs it too, so we only
		// reach here after the signature check. Echo the challenge.
		var probe struct {
			Type      string `json:"type"`
			Challenge string `json:"challenge"`
		}
		if err := json.Unmarshal(body, &probe); err != nil {
			http.Error(w, "parse body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if probe.Type == slackevents.URLVerification {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(probe.Challenge))
			return
		}

		parsed, err := slackevents.ParseEvent(body, slackevents.OptionNoVerifyToken())
		if err != nil {
			e.logger.Warn("slack.events: parse event", "err", err)
			http.Error(w, "parse event: "+err.Error(), http.StatusBadRequest)
			return
		}
		if parsed.Type == slackevents.CallbackEvent {
			if ev, ok := toIngestEvent(parsed.InnerEvent.Data); ok {
				if err := e.receiver.Receive(r.Context(), parsed.TeamID, ev); err != nil {
					e.logger.Error("slack.events: ingest", "team", parsed.TeamID, "err", err)
					// Still answer 2xx so Slack doesn't retry — the failure
					// is on our side and retrying won't fix it.
				}
			}
		}
		w.WriteHeader(http.StatusOK)
	})
}

// toIngestEvent normalises a parsed inner event into the transport-
// neutral Event the ingest consumes. Only message-bearing events
// (message, app_mention) map; everything else is ignored (ok=false).
func toIngestEvent(inner any) (Event, bool) {
	switch m := inner.(type) {
	case *slackevents.MessageEvent:
		return Event{
			Channel:  m.Channel,
			User:     m.User,
			Text:     m.Text,
			TS:       m.TimeStamp,
			ThreadTS: m.ThreadTimeStamp,
			BotID:    m.BotID,
		}, true
	case *slackevents.AppMentionEvent:
		return Event{
			Channel:  m.Channel,
			User:     m.User,
			Text:     m.Text,
			TS:       m.TimeStamp,
			ThreadTS: m.ThreadTimeStamp,
			BotID:    m.BotID,
		}, true
	default:
		return Event{}, false
	}
}
