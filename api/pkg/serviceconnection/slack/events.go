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

// Event is the transport-neutral inbound Slack event. Both ingress
// sources (REST Events API, Socket Mode) normalise their
// provider-specific payloads into this shape, so the downstream
// processing path is identical across modes.
type Event struct {
	// Channel is the Slack channel id the message landed in.
	Channel string
	// User is the Slack user id of the poster (carried as Message.From).
	User string
	// Text is the message body.
	Text string
	// TS is the Slack message timestamp ("1700000000.000100"), unique
	// per message (carried as Message.MessageID).
	TS string
	// ThreadTS is the parent message ts when the message is in a thread;
	// empty for top-level messages (carried as Message.ThreadID so
	// outbound replies can preserve threading).
	ThreadTS string
	// BotID is non-empty when the message was posted by a bot (including
	// our own shared bot). The self-echo guard: any bot-authored event
	// is dropped so a Worker's own posts never re-enter as inbound.
	BotID string
}

// SigningSecretFunc resolves every configured Slack app's signing secret.
// A request is accepted if it verifies against any of them, so multiple
// REST apps can share the one Events endpoint. An empty slice means "no
// app configured" — the handler treats that as inert.
type SigningSecretFunc func(ctx context.Context) ([]string, error)

// EventHandler consumes a normalised inbound event for one workspace
// (identified by teamID). Returning an error is logged but never
// changes the HTTP status Slack sees — Slack must get a prompt 2xx or
// it retries, and a retry won't fix an internal failure.
type EventHandler func(ctx context.Context, teamID string, ev Event) error

// maxBody caps the inbound body size. Slack event payloads are small;
// 1 MiB is generous.
const maxBody = 1 << 20

// EventsAPIHandler returns the http.Handler Slack POSTs events to. The
// url_verification handshake is answered immediately; every real event is
// verified against the global app's signing secret and routed to onEvent
// by team_id. One handler serves every per-org install.
//
// Status codes:
//   - 405 on non-POST
//   - 200 on the url_verification challenge (always — see below)
//   - 503 when a real event arrives but no signing secret is configured
//   - 401 on a real event with a missing, stale, or mismatched signature
//   - 200 on every accepted event
func EventsAPIHandler(signingSecret SigningSecretFunc, onEvent EventHandler, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
		if err != nil {
			http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
			return
		}

		// Answer the url_verification handshake BEFORE the signature gate.
		// Slack pings a Request URL with this challenge the moment an app
		// is created from a manifest that declares the URL — which is
		// before the operator has copied the new app's signing secret into
		// Helix, so gating the handshake on the secret would make a
		// manifest-driven REST install impossible to verify. The challenge
		// is non-sensitive (we echo back exactly what Slack sent); every
		// real event below is still signature-verified.
		var probe struct {
			Type      string `json:"type"`
			Challenge string `json:"challenge"`
		}
		_ = json.Unmarshal(body, &probe)
		if probe.Type == slackevents.URLVerification {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(probe.Challenge))
			return
		}

		// A real delivery: require a configured signing secret and a valid
		// signature. NewSecretsVerifier also enforces the 5-minute
		// timestamp window, so a replayed (stale) delivery is rejected too.
		secrets, err := signingSecret(r.Context())
		if err != nil || len(secrets) == 0 {
			logger.Info("slack.events: no signing secret — inert", "err", err)
			http.Error(w, "slack not configured", http.StatusServiceUnavailable)
			return
		}
		if !verifyAny(r.Header, body, secrets) {
			logger.Warn("slack.events: bad signature")
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}

		parsed, err := slackevents.ParseEvent(body, slackevents.OptionNoVerifyToken())
		if err != nil {
			logger.Warn("slack.events: parse event", "err", err)
			http.Error(w, "parse event: "+err.Error(), http.StatusBadRequest)
			return
		}
		if parsed.Type == slackevents.CallbackEvent {
			if ev, ok := ToEvent(parsed.InnerEvent.Data); ok {
				if err := onEvent(r.Context(), parsed.TeamID, ev); err != nil {
					logger.Error("slack.events: handle", "team", parsed.TeamID, "err", err)
				}
			}
		}
		w.WriteHeader(http.StatusOK)
	})
}

// verifyAny reports whether the request verifies against any of the
// candidate signing secrets. Each secret gets its own verifier (Write
// consumes the body).
func verifyAny(header http.Header, body []byte, secrets []string) bool {
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		v, err := slack.NewSecretsVerifier(header, secret)
		if err != nil {
			continue
		}
		if _, err := v.Write(body); err != nil {
			continue
		}
		if v.Ensure() == nil {
			return true
		}
	}
	return false
}

// ToEvent normalises a parsed inner event into the transport-neutral
// Event. Only message-bearing events (message, app_mention) map;
// everything else is ignored (ok=false).
func ToEvent(inner any) (Event, bool) {
	switch m := inner.(type) {
	case *slackevents.MessageEvent:
		return Event{Channel: m.Channel, User: m.User, Text: m.Text, TS: m.TimeStamp, ThreadTS: m.ThreadTimeStamp, BotID: m.BotID}, true
	case *slackevents.AppMentionEvent:
		return Event{Channel: m.Channel, User: m.User, Text: m.Text, TS: m.TimeStamp, ThreadTS: m.ThreadTimeStamp, BotID: m.BotID}, true
	default:
		return Event{}, false
	}
}
