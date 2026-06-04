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
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/application/streamhub"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
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

// Validate checks the Config has the fields the transport needs for
// inbound delivery handling. webhook_secret is mandatory — without it
// we can't verify HMAC signatures and would have to drop every
// delivery. token is optional here: the inbound path never uses it;
// downstream consumers (worker environments calling `gh`) resolve it
// via Transport.Token, which falls back to the operator-provided
// TokenResolver when transport.github.token isn't set. Reusing a
// per-user GitHub OAuth connection through TokenResolver is the
// recommended path so ops don't have to paste a PAT into
// transport.github.
func (c Config) Validate() error {
	if c.WebhookSecret == "" {
		return errors.New("webhook_secret is empty")
	}
	return nil
}

// Dispatcher is the subset of the dispatcher this transport needs:
// fan an Event out to subscribed AI Workers after appending it.
// Defining the interface here keeps the import edge one-directional.
type Dispatcher interface {
	Dispatch(ctx context.Context, event streaming.Event)
}

// TokenResolver returns the GitHub access token to use for outbound
// actions in the org's Worker environments — typically pulled from an
// existing helix OAuth connection so operators don't have to paste a
// separate PAT into transport.github.
//
// Returning empty string + nil error is treated as "no token
// available right now"; callers decide whether that's fatal.
type TokenResolver func(ctx context.Context, orgID string) (string, error)

// Transport is the long-lived inbound webhook handler. One instance
// per running helix-org server.
type Transport struct {
	orgID         string
	registry      *configregistry.Registry
	store         *store.Store
	broadcaster   *streamhub.Hub
	dispatcher    Dispatcher
	tokenResolver TokenResolver
	logger        *slog.Logger
}

// New returns a Transport bound to the given config registry, store,
// broadcaster (for waking long-poll observers on inbound) and
// dispatcher (for activating subscribed Workers on inbound).
// dispatcher and broadcaster may be nil for tests that don't
// exercise those paths.
func New(orgID string, reg *configregistry.Registry, st *store.Store, bc *streamhub.Hub, d Dispatcher, logger *slog.Logger) *Transport {
	return &Transport{
		orgID:       orgID,
		registry:    reg,
		store:       st,
		broadcaster: bc,
		dispatcher:  d,
		logger:      logger,
	}
}

// WithTokenResolver installs the operator-side hook for resolving
// GitHub access tokens when transport.github.token is empty. Returns
// the same Transport for fluent wiring at construction time. Passing
// nil clears any previously-installed resolver.
func (t *Transport) WithTokenResolver(r TokenResolver) *Transport {
	t.tokenResolver = r
	return t
}

// Token returns the GitHub access token the transport's downstream
// consumers (worker environments running `gh`) should use. Order of
// precedence:
//
//  1. transport.github.token from the operational config registry —
//     wins so ops can pin a specific PAT.
//  2. TokenResolver — the recommended path; in production wired to
//     the helix OAuth manager so the user's existing GitHub login is
//     reused.
//
// Empty string + nil error means "no token configured". Callers
// decide whether that's fatal.
func (t *Transport) Token(ctx context.Context) (string, error) {
	var c Config
	if err := t.registry.GetObject(ctx, t.orgID, "transport.github", &c); err == nil && c.Token != "" {
		return c.Token, nil
	}
	if t.tokenResolver == nil {
		return "", nil
	}
	return t.tokenResolver(ctx, t.orgID)
}

func (t *Transport) config(ctx context.Context) (Config, error) {
	var c Config
	if err := t.registry.GetObject(ctx, t.orgID, "transport.github", &c); err != nil {
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

// decodeWebhookPayload normalises the raw POST body into a typed
// map regardless of the content type GitHub used. GitHub's
// webhook UI offers two options:
//   - `application/json` — body is the JSON payload directly.
//   - `application/x-www-form-urlencoded` — body is
//     `payload=<urlencoded-json>`, the form variant the GH UI
//     defaults to in some flows.
//
// helix-org's install code always asks for `application/json`,
// but operators who set the webhook up manually before the
// install endpoint shipped (or whose existing hook was adopted
// idempotently with a stale content type) can end up sending us
// form-encoded deliveries. Honour both so deliveries never drop
// on the content-type axis alone.
func decodeWebhookPayload(contentType string, body []byte) (map[string]any, error) {
	ct := contentType
	if i := strings.Index(ct, ";"); i >= 0 {
		ct = ct[:i]
	}
	ct = strings.TrimSpace(strings.ToLower(ct))
	var payload map[string]any
	switch ct {
	case "application/x-www-form-urlencoded":
		// GitHub form-encodes the JSON as `payload=<json>`.
		vals, err := url.ParseQuery(string(body))
		if err != nil {
			return nil, fmt.Errorf("parse form body: %w", err)
		}
		raw := vals.Get("payload")
		if raw == "" {
			return nil, errors.New("form body missing `payload` field")
		}
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return nil, fmt.Errorf("parse form payload json: %w", err)
		}
		return payload, nil
	case "", "application/json":
		// Default to JSON when the content type is empty (some
		// proxies strip the header) or explicitly JSON.
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("parse json body: %w", err)
		}
		return payload, nil
	default:
		// Try JSON anyway — GitHub doesn't use other types, but
		// being permissive avoids breakage on rare proxies.
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("parse body (content-type %q): %w", ct, err)
		}
		return payload, nil
	}
}

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

		payload, err := decodeWebhookPayload(r.Header.Get("Content-Type"), body)
		if err != nil {
			t.logger.Warn("github.inbound: decode body", "err", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
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

		msg := streaming.Message{
			From:      sender(payload),
			Subject:   subjectFor(eventType, payload),
			Body:      bodyFor(eventType, payload),
			ThreadID:  threadIDFor(payload),
			MessageID: deliveryID,
			Extra:     extraJSON,
		}

		now := nowUTC()
		for _, s := range streams {
			event, err := streaming.NewMessageEvent(
				streaming.EventID("e-"+uuid.NewString()),
				s.ID,
				"", // system-emitted: external sender, no helix Worker source
				msg,
				now,
				t.orgID,
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

// HandleInboundForStream is the per-stream variant of HandleInbound.
// It HMAC-verifies the delivery the same way, but pins fanout to a
// single Stream identified by streamID rather than scanning every
// github stream in the org. The stream's own (repo, events)
// configuration still applies — a delivery for the wrong repo or
// for an event the stream doesn't whitelist returns 204 (dropped)
// so GitHub stops retrying. Use this handler when you want one
// GitHub webhook → one helix stream (the recommended setup on the
// detail page); the org-level handler stays for fan-out delivery
// across every matching github stream.
func (t *Transport) HandleInboundForStream(streamID streaming.StreamID) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		cfg, err := t.config(r.Context())
		if err != nil {
			t.logger.Error("github.inbound.stream: config", "err", err)
			http.Error(w, "transport not configured", http.StatusServiceUnavailable)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
		if err != nil {
			http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if !verifySignature(cfg.WebhookSecret, body, r.Header.Get("X-Hub-Signature-256")) {
			t.logger.Warn("github.inbound.stream: bad signature",
				"stream", streamID,
				"delivery", r.Header.Get("X-GitHub-Delivery"),
				"event", r.Header.Get("X-GitHub-Event"))
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
		payload, err := decodeWebhookPayload(r.Header.Get("Content-Type"), body)
		if err != nil {
			t.logger.Warn("github.inbound.stream: decode body", "stream", streamID, "err", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		eventType := r.Header.Get("X-GitHub-Event")
		deliveryID := r.Header.Get("X-GitHub-Delivery")
		stream, err := t.store.Streams.Get(r.Context(), t.orgID, streamID)
		if err != nil {
			t.logger.Warn("github.inbound.stream: lookup", "stream", streamID, "err", err)
			http.Error(w, "stream not found", http.StatusNotFound)
			return
		}
		if stream.Transport.Kind != transport.KindGitHub {
			http.Error(w, "stream is not a github transport", http.StatusBadRequest)
			return
		}
		streamCfg, err := stream.Transport.GitHubConfig()
		if err != nil {
			t.logger.Error("github.inbound.stream: parse stream config", "stream", streamID, "err", err)
			http.Error(w, "stream config invalid", http.StatusInternalServerError)
			return
		}
		repo := repoFullName(payload)
		if repo == "" {
			t.logger.Info("github.inbound.stream: no repository in payload",
				"stream", streamID, "event", eventType, "delivery", deliveryID)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		// Stream-level filters: drop on repo or event-whitelist
		// mismatch with 204 so GitHub stops retrying. This is the
		// same drop semantics as the org-level handler.
		if !strings.EqualFold(streamCfg.Repo, repo) {
			t.logger.Info("github.inbound.stream: repo mismatch",
				"stream", streamID, "stream_repo", streamCfg.Repo, "payload_repo", repo,
				"event", eventType, "delivery", deliveryID)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if !contains(streamCfg.Events, eventType) {
			t.logger.Info("github.inbound.stream: event not whitelisted",
				"stream", streamID, "event", eventType, "delivery", deliveryID)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		// Inject the event header into the body's top level for
		// roles, same as HandleInbound.
		payload["event"] = eventType
		extraJSON, err := json.Marshal(payload)
		if err != nil {
			t.logger.Error("github.inbound.stream: re-marshal", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		msg := streaming.Message{
			From:      sender(payload),
			Subject:   subjectFor(eventType, payload),
			Body:      bodyFor(eventType, payload),
			ThreadID:  threadIDFor(payload),
			MessageID: deliveryID,
			Extra:     extraJSON,
		}
		now := nowUTC()
		event, err := streaming.NewMessageEvent(
			streaming.EventID("e-"+uuid.NewString()),
			stream.ID,
			"",
			msg,
			now,
			t.orgID,
		)
		if err != nil {
			t.logger.Error("github.inbound.stream: build event", "stream", streamID, "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if err := t.store.Events.Append(r.Context(), event); err != nil {
			t.logger.Error("github.inbound.stream: append", "stream", streamID, "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if t.broadcaster != nil {
			t.broadcaster.Notify(stream.ID)
		}
		if t.dispatcher != nil {
			t.dispatcher.Dispatch(r.Context(), event)
		}
		t.logger.Info("github.inbound.stream",
			"stream", stream.ID, "repo", repo, "event", eventType,
			"delivery", deliveryID, "from", msg.From)
		w.WriteHeader(http.StatusNoContent)
	})
}

// matchingStreams returns every github-transport Stream whose repo
// matches `repo` (case-insensitive) and whose events whitelist
// contains `eventType`. Linear scan is fine at the scale we expect;
// indexed lookups are an obvious follow-on if installations ever
// grow many github streams.
func (t *Transport) matchingStreams(ctx context.Context, repo, eventType string) ([]streaming.Stream, error) {
	all, err := t.store.Streams.List(ctx, t.orgID)
	if err != nil {
		return nil, fmt.Errorf("list streams: %w", err)
	}
	var matched []streaming.Stream
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

// contains is the events-whitelist check. "*" anywhere in the list
// is a wildcard meaning "deliver every event the repo emits" — the
// same convention GitHub's webhook API uses for `events: ["*"]`.
// Otherwise an exact string match.
func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == "*" || s == needle {
			return true
		}
	}
	return false
}

// nowUTC returns the current wall-clock time in UTC. Matches the
// helper convention used elsewhere; trivial to override in a test
// if a deterministic timestamp is ever needed.
func nowUTC() time.Time { return time.Now().UTC() }
