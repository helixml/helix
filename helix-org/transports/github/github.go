// Package github implements helix-org's inbound GitHub webhooks
// transport. A single HTTP handler at /github/webhook turns every
// signed delivery into Events on the Streams configured for that
// repo.
//
// Server-level configuration lives in the operational config
// registry under `transport.github`:
//
//	{
//	  "token":          "<gh-token>",          // PAT used by Workers' gh
//	  "webhook_secret": "<random hex>"         // HMAC-SHA256 over body
//	}
//
// Streams declare `{"repo":"owner/name","events":[...]}`. The
// transport HMAC-verifies the delivery, fans it out to every Stream
// whose `repo` matches `payload.repository.full_name` and whose
// `events` whitelist contains the X-GitHub-Event header value, and
// builds a canonical Message envelope per the design doc.
//
// Outbound is intentionally not supported. Workers act on the repo
// via `gh` in their Environment; publish to a github stream is
// rejected at the publish tool with an explanatory error. See
// design/github-transport.md.
package github

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/helixml/helix/api/pkg/org/broadcast"
	"github.com/helixml/helix/api/pkg/org/event"
	"github.com/helixml/helix/api/pkg/org/message"
	"github.com/helixml/helix/api/pkg/org/transport"
	"github.com/helixml/helix/helix-org/config"
	"github.com/helixml/helix/helix-org/domain"
	"github.com/helixml/helix/helix-org/store"
)

// Config is the parsed shape of the operational-config row
// `transport.github`. Read on every delivery so live updates via
// `helix-org config set` apply without a restart.
type Config struct {
	// Token is the GitHub PAT (or `gh auth token`) that Workers
	// provision into their Environment's `gh` for outbound actions.
	// Opaque to this transport; we never call the GitHub API
	// ourselves on the inbound path.
	Token string `json:"token"`

	// WebhookSecret is the shared secret GitHub HMAC-signs each
	// delivery with. The transport verifies X-Hub-Signature-256
	// against this on every request; a mismatch is a 401 and a
	// dropped delivery.
	WebhookSecret string `json:"webhook_secret"`
}

// Validate checks the Config has the fields the transport needs.
// Token validity is opaque (we don't call GitHub from here), but a
// missing token is a misconfiguration worth surfacing early.
func (c Config) Validate() error {
	if c.Token == "" {
		return errors.New("token is empty")
	}
	if c.WebhookSecret == "" {
		return errors.New("webhook_secret is empty")
	}
	return nil
}

// Dispatcher is the subset of the dispatcher this transport needs:
// fan an Event out to subscribed AI Workers after appending it.
// Defining the interface here keeps the import edge one-directional.
type Dispatcher interface {
	Dispatch(ctx context.Context, event domain.Event)
}

// Transport is the long-lived inbound webhook handler. One instance
// per running helix-org server.
type Transport struct {
	registry    *config.Registry
	store       *store.Store
	broadcaster *broadcast.Hub
	dispatcher  Dispatcher
	logger      *slog.Logger
}

// New returns a Transport bound to the given config registry, store,
// broadcaster (for waking long-poll observers on inbound) and
// dispatcher (for activating subscribed Workers on inbound).
// dispatcher and broadcaster may be nil for tests that don't
// exercise those paths.
func New(reg *config.Registry, st *store.Store, bc *broadcast.Hub, d Dispatcher, logger *slog.Logger) *Transport {
	return &Transport{
		registry:    reg,
		store:       st,
		broadcaster: bc,
		dispatcher:  d,
		logger:      logger,
	}
}

func (t *Transport) config(ctx context.Context) (Config, error) {
	var c Config
	if err := t.registry.GetObject(ctx, "transport.github", &c); err != nil {
		return Config{}, err
	}
	if err := c.Validate(); err != nil {
		return Config{}, fmt.Errorf("transport.github: %w", err)
	}
	return c, nil
}

// maxBody caps webhook body size. GitHub's hard limit is 25 MiB; we
// match that for safety.
const maxBody = 25 << 20

// HandleInbound is the http.Handler GitHub POSTs each signed
// delivery to. It HMAC-verifies the body, then fans the parsed
// payload out to every Stream whose repo + events whitelist
// matches.
//
// Status codes:
//   - 401 on missing or mismatched signature
//   - 400 on unparseable body
//   - 405 on non-POST
//   - 204 on success (event appended) and on no-op (delivery for a
//     repo we have no streams for, or for an event type no stream
//     wants — both 2xx so GitHub stops retrying)
func (t *Transport) HandleInbound() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		cfg, err := t.config(r.Context())
		if err != nil {
			t.logger.Error("github.inbound: config", "err", err)
			http.Error(w, "transport not configured", http.StatusServiceUnavailable)
			return
		}

		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
		if err != nil {
			http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
			return
		}

		// Signature first. Fail closed on missing or bad signature —
		// we don't want to parse adversarial bodies before we know
		// they came from GitHub.
		if !verifySignature(cfg.WebhookSecret, body, r.Header.Get("X-Hub-Signature-256")) {
			t.logger.Warn("github.inbound: bad signature",
				"delivery", r.Header.Get("X-GitHub-Delivery"),
				"event", r.Header.Get("X-GitHub-Event"))
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}

		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			http.Error(w, "parse json: "+err.Error(), http.StatusBadRequest)
			return
		}

		eventType := r.Header.Get("X-GitHub-Event")
		deliveryID := r.Header.Get("X-GitHub-Delivery")
		repo := repoFullName(payload)
		if repo == "" {
			// Some legitimate event types (`ping`, `meta`) carry no
			// repository field. Accept and log, but nothing to route to.
			t.logger.Info("github.inbound: no repository in payload", "event", eventType, "delivery", deliveryID)
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Find every stream this delivery should fan out to.
		streams, err := t.matchingStreams(r.Context(), repo, eventType)
		if err != nil {
			t.logger.Error("github.inbound: match streams", "repo", repo, "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if len(streams) == 0 {
			// Either no Stream is configured for this repo, or none of
			// them want this event type. Log so misconfigurations are
			// visible; respond 2xx so GitHub stops retrying.
			t.logger.Info("github.inbound: no matching streams", "repo", repo, "event", eventType, "delivery", deliveryID)
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Inject the event header into the body's top level — GitHub
		// puts the event type in X-GitHub-Event rather than the JSON,
		// and we want roles to read it from one place. Safe: GitHub
		// payloads do not have a top-level `event` field of their own.
		payload["event"] = eventType
		extraJSON, err := json.Marshal(payload)
		if err != nil {
			t.logger.Error("github.inbound: re-marshal payload", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		msg := message.Message{
			From:      sender(payload),
			Subject:   subjectFor(eventType, payload),
			Body:      bodyFor(eventType, payload),
			ThreadID:  threadIDFor(payload),
			MessageID: deliveryID,
			Extra:     extraJSON,
		}

		now := nowUTC()
		for _, s := range streams {
			event, err := domain.NewMessageEvent(
				event.ID("e-"+uuid.NewString()),
				s.ID,
				"", // system-emitted: external sender, no helix Worker source
				msg,
				now,
			)
			if err != nil {
				t.logger.Error("github.inbound: build event", "stream", s.ID, "err", err)
				continue
			}
			if err := t.store.Events.Append(r.Context(), event); err != nil {
				t.logger.Error("github.inbound: append", "stream", s.ID, "err", err)
				continue
			}
			if t.broadcaster != nil {
				t.broadcaster.Notify(s.ID)
			}
			if t.dispatcher != nil {
				t.dispatcher.Dispatch(r.Context(), event)
			}
			t.logger.Info("github.inbound",
				"stream", s.ID, "repo", repo, "event", eventType,
				"delivery", deliveryID, "from", msg.From)
		}

		w.WriteHeader(http.StatusNoContent)
	})
}

// matchingStreams returns every github-transport Stream whose repo
// matches `repo` (case-insensitive) and whose events whitelist
// contains `eventType`. Linear scan is fine at the scale we expect;
// indexed lookups are an obvious follow-on if installations ever
// grow many github streams.
func (t *Transport) matchingStreams(ctx context.Context, repo, eventType string) ([]domain.Stream, error) {
	all, err := t.store.Streams.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list streams: %w", err)
	}
	var matched []domain.Stream
	for _, s := range all {
		if s.Transport.Kind != transport.KindGitHub {
			continue
		}
		cfg, err := s.Transport.GitHubConfig()
		if err != nil {
			t.logger.Warn("github.inbound: stream config parse", "stream", s.ID, "err", err)
			continue
		}
		if !strings.EqualFold(cfg.Repo, repo) {
			continue
		}
		if !contains(cfg.Events, eventType) {
			continue
		}
		matched = append(matched, s)
	}
	return matched, nil
}

// verifySignature compares X-Hub-Signature-256 ("sha256=<hex>")
// against an HMAC-SHA256 of body keyed by secret. Constant-time;
// returns false on any malformed input.
func verifySignature(secret string, body []byte, header string) bool {
	if header == "" {
		return false
	}
	const prefix = "sha256="
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	want, err := hex.DecodeString(header[len(prefix):])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(want, mac.Sum(nil))
}

// repoFullName extracts repository.full_name from a webhook payload.
func repoFullName(p map[string]any) string {
	repo, _ := p["repository"].(map[string]any)
	if repo == nil {
		return ""
	}
	full, _ := repo["full_name"].(string)
	return full
}

// sender returns sender.login or empty.
func sender(p map[string]any) string {
	s, _ := p["sender"].(map[string]any)
	if s == nil {
		return ""
	}
	login, _ := s["login"].(string)
	return login
}

// subjectFor picks the natural "title" field of the event. Issue
// events use `issue.title`; PR events (including PR-comment and
// PR-review variants) use `pull_request.title`; release events use
// `release.name`. Comment-on-issue events use the parent issue's
// title.
//
// Falls back to empty on events with no natural title (push, label
// changes that don't carry the parent's title in the payload, etc).
//
// Disambiguation by payload shape rather than by event type: payload
// objects (`pull_request`, `issue`, `release`) are mutually exclusive
// per event in practice, with the one exception that PR-comment
// events carry both `pull_request` and `issue` (the latter is a
// shim) — we want the PR title there, so check pull_request first.
func subjectFor(_ string, p map[string]any) string {
	// Prefer pull_request.title for PR-shaped events, even if `issue`
	// is also present (PR-comment events have both).
	if pr, ok := p["pull_request"].(map[string]any); ok {
		if title, _ := pr["title"].(string); title != "" {
			return title
		}
	}
	if iss, ok := p["issue"].(map[string]any); ok {
		if title, _ := iss["title"].(string); title != "" {
			return title
		}
	}
	if rel, ok := p["release"].(map[string]any); ok {
		if name, _ := rel["name"].(string); name != "" {
			return name
		}
	}
	return ""
}

// bodyFor picks the natural "user-typed text" of the event.
//   - issues.opened / issues.edited     → issue.body
//   - pull_request.opened / .edited     → pull_request.body
//   - issue_comment / pull_request_review_comment → comment.body
//   - pull_request_review.submitted     → review.body
//
// For events that carry no user text (label, assigned, sync, push,
// …), returns empty.
func bodyFor(eventType string, p map[string]any) string {
	switch eventType {
	case "issue_comment", "pull_request_review_comment":
		if c, ok := p["comment"].(map[string]any); ok {
			if b, _ := c["body"].(string); b != "" {
				return b
			}
		}
	case "pull_request_review":
		if rev, ok := p["review"].(map[string]any); ok {
			if b, _ := rev["body"].(string); b != "" {
				return b
			}
		}
	case "issues":
		if iss, ok := p["issue"].(map[string]any); ok {
			if b, _ := iss["body"].(string); b != "" {
				return b
			}
		}
	case "pull_request":
		if pr, ok := p["pull_request"].(map[string]any); ok {
			if b, _ := pr["body"].(string); b != "" {
				return b
			}
		}
	}
	return ""
}

// threadIDFor returns "#<number>" for events scoped to one issue or
// PR (or one of their comments/reviews); empty for repo-level
// events. Lets a role read all events for one PR via `read_events`
// filtered by ThreadID.
//
// Number resolution prefers payload.number (set on PR events,
// payload-level), then pull_request.number, then issue.number.
func threadIDFor(p map[string]any) string {
	if n, ok := numberFromAny(p["number"]); ok {
		return fmt.Sprintf("#%d", n)
	}
	if pr, ok := p["pull_request"].(map[string]any); ok {
		if n, ok := numberFromAny(pr["number"]); ok {
			return fmt.Sprintf("#%d", n)
		}
	}
	if iss, ok := p["issue"].(map[string]any); ok {
		if n, ok := numberFromAny(iss["number"]); ok {
			return fmt.Sprintf("#%d", n)
		}
	}
	return ""
}

// numberFromAny coerces JSON-decoded numeric values (which come in
// as float64) into an int64. Returns false for anything else.
func numberFromAny(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int:
		return int64(n), true
	case int64:
		return n, true
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	}
	return 0, false
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// nowUTC returns the current wall-clock time in UTC. Matches the
// helper convention used elsewhere; trivial to override in a test
// if a deterministic timestamp is ever needed.
func nowUTC() time.Time { return time.Now().UTC() }
