// These tests pin down the github transport's behaviour as specified
// in design/github-transport.md. They cover:
//
//   - HMAC-SHA256 signature verification on /github/webhook (good /
//     bad / missing).
//   - Repo routing: deliveries route to every Stream whose
//     Transport.Config.Repo equals payload.repository.full_name.
//   - Per-stream event filter: only event types listed in the
//     stream's `events` whitelist become Events; others are accepted
//     (200) but dropped.
//   - Envelope mapping: From=sender.login, Subject=upstream title
//     verbatim, Body=upstream user-typed text verbatim,
//     ThreadID=#<number>, MessageID=X-GitHub-Delivery, Extra is the
//     full webhook body verbatim with one synthetic top-level key
//     (`event`) injected from the X-GitHub-Event header.
//   - Inbound-only: dispatcher fires; broadcaster wakes; helix's
//     Source on the resulting Event is empty.
//   - Method/body validation: GET → 405, malformed JSON → 400.
//   - Domain transport validation: stream config requires repo +
//     non-empty events whitelist; event names must match
//     ^[a-z][a-z0-9_]+$ (so typos are caught) but are not limited to
//     a fixed set.
package github_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	githubtransport "github.com/helixml/helix/api/pkg/org/infrastructure/transports/github"
	"github.com/helixml/helix/api/pkg/org/infrastructure/wakebus"
	"github.com/helixml/helix/api/pkg/pubsub"
)

const testWebhookSecret = "abc123" // shared secret used in HMAC computations

// recordingDispatcher captures Dispatch calls so tests can assert
// the dispatcher was woken (and how many times) for a given inbound
// delivery.
type recordingDispatcher struct {
	mu     sync.Mutex
	events []streaming.Event
}

func (d *recordingDispatcher) Dispatch(_ context.Context, e streaming.Event) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.events = append(d.events, e)
}

func (d *recordingDispatcher) snapshot() []streaming.Event {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]streaming.Event, len(d.events))
	copy(out, d.events)
	return out
}

func newTestTransport(t *testing.T) (*githubtransport.Transport, *store.Store, *recordingDispatcher, *wakebus.Bus, *configregistry.Registry) {
	t.Helper()
	st := orggorm.GetOrgTestDB(t)
	ps, err := pubsub.NewInMemoryNats()
	if err != nil {
		t.Fatalf("NewInMemoryNats: %v", err)
	}
	bc := wakebus.New(ps)
	rd := &recordingDispatcher{}
	reg := configregistry.New(st.Configs)
	reg.Register(configregistry.Spec{
		Key:     "transport.github",
		Type:    configregistry.TypeObject,
		Secrets: []string{"token", "webhook_secret"},
	})
	tp := githubtransport.New("org-test", reg, st, bc, rd, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return tp, st, rd, bc, reg
}

func setGitHubConfig(t *testing.T, reg *configregistry.Registry, token, secret string) {
	t.Helper()
	val, _ := json.Marshal(map[string]string{"token": token, "webhook_secret": secret})
	if err := reg.Set(context.Background(), "org-test", "transport.github", string(val)); err != nil {
		t.Fatalf("set config: %v", err)
	}
}

// seedGitHubStream creates a github-transport Stream with the given
// repo and event whitelist. Returns the persisted Stream.
func seedGitHubStream(t *testing.T, st *store.Store, id streaming.StreamID, repo string, events []string) streaming.Stream {
	t.Helper()
	cfg, _ := json.Marshal(map[string]any{"repo": repo, "events": events})
	stream, err := streaming.NewStream(id, string(id), "", "w-owner", time.Now().UTC(),
		transport.Transport{Kind: transport.KindGitHub, Config: cfg}, "org-test")
	if err != nil {
		t.Fatalf("new stream: %v", err)
	}
	if err := st.Streams.Create(context.Background(), stream); err != nil {
		t.Fatalf("create stream: %v", err)
	}
	return stream
}

// signBody returns the value of the X-Hub-Signature-256 header for
// the given body and secret. GitHub's exact format: "sha256=<hex>".
func signBody(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// postResult is what `post` returns — closing the response body
// inside the helper keeps the bodyclose linter satisfied at every
// call site.
type postResult struct {
	StatusCode int
	Body       []byte
}

// post POSTs body to the test transport's HandleInbound, optionally
// setting the GitHub headers. Sig defaults to a correct HMAC of body
// using testWebhookSecret if empty; "-" sends no signature header at
// all (for the missing-signature test).
func post(t *testing.T, tp http.Handler, body []byte, eventType, deliveryID, sig string) postResult {
	t.Helper()
	srv := httptest.NewServer(tp)
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodPost, srv.URL, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if eventType != "" {
		req.Header.Set("X-GitHub-Event", eventType)
	}
	if deliveryID != "" {
		req.Header.Set("X-GitHub-Delivery", deliveryID)
	}
	if sig == "" {
		sig = signBody(testWebhookSecret, body)
	}
	if sig != "-" {
		req.Header.Set("X-Hub-Signature-256", sig)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)
	return postResult{StatusCode: resp.StatusCode, Body: respBody}
}

// issuesOpenedPayload returns a representative `issues.opened`
// webhook body for repo `owner/name`.
func issuesOpenedPayload(repo string) map[string]any {
	return map[string]any{
		"action": "opened",
		"issue": map[string]any{
			"id":     12345,
			"number": 42,
			"title":  "README setup steps mention an env var that no longer exists",
			"body":   "Step 3 references HELIX_FOO; the code reads HELIX_BAR now.",
		},
		"sender":     map[string]any{"login": "philwinder"},
		"repository": map[string]any{"full_name": repo},
	}
}

// pullRequestLabeledPayload returns a representative
// `pull_request.labeled` body for repo `owner/name` with `label.name`
// = labelName.
func pullRequestLabeledPayload(repo, labelName string) map[string]any {
	return map[string]any{
		"action": "labeled",
		"number": 7,
		"pull_request": map[string]any{
			"id":     987,
			"number": 7,
			"title":  "Fix typo in setup section",
			"body":   "Spotted while running through README.",
		},
		"label":      map[string]any{"name": labelName, "color": "0075ca"},
		"sender":     map[string]any{"login": "octocat"},
		"repository": map[string]any{"full_name": repo},
	}
}

// issueCommentCreatedPayload returns an `issue_comment.created` body
// — note `comment.body` is the user text, while the parent `issue`
// object carries the title that goes into Subject.
func issueCommentCreatedPayload(repo string) map[string]any {
	return map[string]any{
		"action": "created",
		"issue": map[string]any{
			"id":     999,
			"number": 42,
			"title":  "README setup steps mention an env var that no longer exists",
		},
		"comment": map[string]any{
			"id":   55,
			"body": "I hit the same thing — happy to send a PR.",
		},
		"sender":     map[string]any{"login": "alice"},
		"repository": map[string]any{"full_name": repo},
	}
}

// TestInboundIssuesOpened: full envelope mapping for a representative
// event. Subject = issue title, Body = issue body, ThreadID = #42,
// MessageID = delivery UUID, From = sender.login, Source on the
// stored Event is empty (system-emitted), and Extra is the body
// verbatim with one synthetic top-level key (`event`).
func TestInboundIssuesOpened(t *testing.T) {
	t.Parallel()
	tp, st, rd, _, reg := newTestTransport(t)
	setGitHubConfig(t, reg, "tok", testWebhookSecret)
	seedGitHubStream(t, st, "s-github", "helixml/helix-org",
		[]string{"issues", "issue_comment", "pull_request", "pull_request_review", "pull_request_review_comment"})

	body, _ := json.Marshal(issuesOpenedPayload("helixml/helix-org"))

	resp := post(t, tp.HandleInbound(), body, "issues", "delivery-uuid-1", "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, body = %q, want 204", resp.StatusCode, resp.Body)
	}

	events, _ := st.Events.ListForStream(context.Background(), "org-test", "s-github", 10)
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	ev := events[0]
	if ev.Source != "" {
		t.Fatalf("Source = %q, want empty", ev.Source)
	}

	msg, err := ev.Message()
	if err != nil {
		t.Fatalf("parse message: %v", err)
	}
	if msg.From != "philwinder" {
		t.Fatalf("From = %q, want philwinder", msg.From)
	}
	if msg.Subject != "README setup steps mention an env var that no longer exists" {
		t.Fatalf("Subject = %q", msg.Subject)
	}
	if !strings.Contains(msg.Body, "HELIX_BAR") {
		t.Fatalf("Body = %q", msg.Body)
	}
	if msg.ThreadID != "#42" {
		t.Fatalf("ThreadID = %q, want #42", msg.ThreadID)
	}
	if msg.MessageID != "delivery-uuid-1" {
		t.Fatalf("MessageID = %q, want delivery-uuid-1", msg.MessageID)
	}
	if len(msg.Extra) == 0 {
		t.Fatalf("Extra is empty")
	}

	// Extra is the full body with `event` injected at the top level.
	var extra map[string]any
	if err := json.Unmarshal(msg.Extra, &extra); err != nil {
		t.Fatalf("Extra not JSON: %v", err)
	}
	if extra["event"] != "issues" {
		t.Fatalf("Extra.event = %v, want issues", extra["event"])
	}
	if extra["action"] != "opened" {
		t.Fatalf("Extra.action = %v, want opened (preserved from upstream body)", extra["action"])
	}
	if extra["issue"] == nil {
		t.Fatalf("Extra.issue missing — body should be passed through verbatim")
	}
	repo, _ := extra["repository"].(map[string]any)
	if repo == nil || repo["full_name"] != "helixml/helix-org" {
		t.Fatalf("Extra.repository.full_name = %v, want helixml/helix-org", repo)
	}

	// Dispatcher fired exactly once.
	if got := len(rd.snapshot()); got != 1 {
		t.Fatalf("dispatcher fired %d times, want 1", got)
	}
}

// TestInboundPullRequestLabeled: action / sender / title / number all
// flow through; the role's `Extra.label.name` lookup works because
// Extra is the body verbatim.
func TestInboundPullRequestLabeled(t *testing.T) {
	t.Parallel()
	tp, st, _, _, reg := newTestTransport(t)
	setGitHubConfig(t, reg, "tok", testWebhookSecret)
	seedGitHubStream(t, st, "s-github", "helixml/helix-org", []string{"pull_request"})

	body, _ := json.Marshal(pullRequestLabeledPayload("helixml/helix-org", "docs"))
	resp := post(t, tp.HandleInbound(), body, "pull_request", "d-2", "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	events, _ := st.Events.ListForStream(context.Background(), "org-test", "s-github", 10)
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	msg, _ := events[0].Message()
	if msg.From != "octocat" {
		t.Fatalf("From = %q", msg.From)
	}
	if msg.Subject != "Fix typo in setup section" {
		t.Fatalf("Subject = %q (want PR title verbatim)", msg.Subject)
	}
	if !strings.Contains(msg.Body, "Spotted while") {
		t.Fatalf("Body = %q (want PR body verbatim)", msg.Body)
	}
	if msg.ThreadID != "#7" {
		t.Fatalf("ThreadID = %q", msg.ThreadID)
	}
	var extra map[string]any
	_ = json.Unmarshal(msg.Extra, &extra)
	if extra["event"] != "pull_request" || extra["action"] != "labeled" {
		t.Fatalf("Extra event/action = %v / %v", extra["event"], extra["action"])
	}
	label, _ := extra["label"].(map[string]any)
	if label == nil || label["name"] != "docs" {
		t.Fatalf("Extra.label.name = %v, want docs", label)
	}
}

// TestInboundIssueCommentMapsBodyToCommentBody: for comment events,
// Body is `comment.body` (the user-typed text), Subject is the
// parent issue's title (so a reader skimming the stream sees what
// thread the comment is on).
func TestInboundIssueCommentMapsBodyToCommentBody(t *testing.T) {
	t.Parallel()
	tp, st, _, _, reg := newTestTransport(t)
	setGitHubConfig(t, reg, "tok", testWebhookSecret)
	seedGitHubStream(t, st, "s-github", "helixml/helix-org", []string{"issue_comment"})

	body, _ := json.Marshal(issueCommentCreatedPayload("helixml/helix-org"))
	resp := post(t, tp.HandleInbound(), body, "issue_comment", "d-3", "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	events, _ := st.Events.ListForStream(context.Background(), "org-test", "s-github", 10)
	msg, _ := events[0].Message()
	if msg.Body != "I hit the same thing — happy to send a PR." {
		t.Fatalf("Body = %q (want comment.body verbatim)", msg.Body)
	}
	if msg.Subject != "README setup steps mention an env var that no longer exists" {
		t.Fatalf("Subject = %q (want parent issue title)", msg.Subject)
	}
	if msg.ThreadID != "#42" {
		t.Fatalf("ThreadID = %q", msg.ThreadID)
	}
}

// TestInboundBadSignatureReturns401: HMAC mismatch is rejected with
// 401; no event is appended; dispatcher is not called.
func TestInboundBadSignatureReturns401(t *testing.T) {
	t.Parallel()
	tp, st, rd, _, reg := newTestTransport(t)
	setGitHubConfig(t, reg, "tok", testWebhookSecret)
	seedGitHubStream(t, st, "s-github", "helixml/helix-org", []string{"issues"})

	body, _ := json.Marshal(issuesOpenedPayload("helixml/helix-org"))
	resp := post(t, tp.HandleInbound(), body, "issues", "d-1", "sha256=deadbeef")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	events, _ := st.Events.ListForStream(context.Background(), "org-test", "s-github", 10)
	if len(events) != 0 {
		t.Fatalf("events = %d, want 0 (bad sig must not append)", len(events))
	}
	if got := len(rd.snapshot()); got != 0 {
		t.Fatalf("dispatcher fired %d times, want 0", got)
	}
}

// TestInboundMissingSignatureReturns401: a request without
// X-Hub-Signature-256 is rejected. We fail closed; never trust an
// unsigned webhook.
func TestInboundMissingSignatureReturns401(t *testing.T) {
	t.Parallel()
	tp, st, _, _, reg := newTestTransport(t)
	setGitHubConfig(t, reg, "tok", testWebhookSecret)
	seedGitHubStream(t, st, "s-github", "helixml/helix-org", []string{"issues"})

	body, _ := json.Marshal(issuesOpenedPayload("helixml/helix-org"))
	resp := post(t, tp.HandleInbound(), body, "issues", "d-1", "-")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

// TestInboundUnknownRepoReturns200NoAppend: a delivery for a repo
// that no Stream is configured for is accepted (200) so GitHub stops
// retrying, but no event is appended. Operators should see this in
// the logs; tests just assert the no-op.
func TestInboundUnknownRepoReturns200NoAppend(t *testing.T) {
	t.Parallel()
	tp, st, rd, _, reg := newTestTransport(t)
	setGitHubConfig(t, reg, "tok", testWebhookSecret)
	seedGitHubStream(t, st, "s-github", "helixml/helix-org", []string{"issues"})

	body, _ := json.Marshal(issuesOpenedPayload("someone-else/their-repo"))
	resp := post(t, tp.HandleInbound(), body, "issues", "d-1", "")
	if resp.StatusCode/100 != 2 {
		t.Fatalf("status = %d, want 2xx", resp.StatusCode)
	}
	events, _ := st.Events.ListForStream(context.Background(), "org-test", "s-github", 10)
	if len(events) != 0 {
		t.Fatalf("events = %d, want 0", len(events))
	}
	if got := len(rd.snapshot()); got != 0 {
		t.Fatalf("dispatcher fired %d times, want 0", got)
	}
}

// TestInboundEventTypeFilterDrops: a delivery for an event type not
// in the stream's `events` whitelist is accepted (so GitHub stops
// retrying) but does not become an Event.
func TestInboundEventTypeFilterDrops(t *testing.T) {
	t.Parallel()
	tp, st, rd, _, reg := newTestTransport(t)
	setGitHubConfig(t, reg, "tok", testWebhookSecret)
	// Stream wants only `issues`; we'll send a `pull_request`.
	seedGitHubStream(t, st, "s-github", "helixml/helix-org", []string{"issues"})

	body, _ := json.Marshal(pullRequestLabeledPayload("helixml/helix-org", "docs"))
	resp := post(t, tp.HandleInbound(), body, "pull_request", "d-1", "")
	if resp.StatusCode/100 != 2 {
		t.Fatalf("status = %d, want 2xx", resp.StatusCode)
	}
	events, _ := st.Events.ListForStream(context.Background(), "org-test", "s-github", 10)
	if len(events) != 0 {
		t.Fatalf("events = %d, want 0 (filtered out)", len(events))
	}
	if got := len(rd.snapshot()); got != 0 {
		t.Fatalf("dispatcher fired %d times, want 0", got)
	}
}

// TestInboundFanOutToMultipleStreams: two streams configured for the
// same repo with overlapping event whitelists both receive a copy of
// the event.
func TestInboundFanOutToMultipleStreams(t *testing.T) {
	t.Parallel()
	tp, st, rd, _, reg := newTestTransport(t)
	setGitHubConfig(t, reg, "tok", testWebhookSecret)
	seedGitHubStream(t, st, "s-docs", "helixml/helix-org", []string{"issues", "pull_request"})
	seedGitHubStream(t, st, "s-triage", "helixml/helix-org", []string{"issues"})

	body, _ := json.Marshal(issuesOpenedPayload("helixml/helix-org"))
	resp := post(t, tp.HandleInbound(), body, "issues", "d-1", "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	docsEv, _ := st.Events.ListForStream(context.Background(), "org-test", "s-docs", 10)
	triageEv, _ := st.Events.ListForStream(context.Background(), "org-test", "s-triage", 10)
	if len(docsEv) != 1 || len(triageEv) != 1 {
		t.Fatalf("fan-out = %d / %d, want 1 / 1", len(docsEv), len(triageEv))
	}
	if got := len(rd.snapshot()); got != 2 {
		t.Fatalf("dispatcher fired %d times, want 2 (one per stream)", got)
	}
}

// TestInboundMethodNotAllowed: GET (and other non-POSTs) → 405.
func TestInboundMethodNotAllowed(t *testing.T) {
	t.Parallel()
	tp, _, _, _, reg := newTestTransport(t)
	setGitHubConfig(t, reg, "tok", testWebhookSecret)

	srv := httptest.NewServer(tp.HandleInbound())
	t.Cleanup(srv.Close)
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
}

// TestInboundMalformedJSONReturns400: a syntactically broken body is
// rejected. (Note: signature must still verify, since HMAC is over
// raw bytes.)
func TestInboundMalformedJSONReturns400(t *testing.T) {
	t.Parallel()
	tp, _, _, _, reg := newTestTransport(t)
	setGitHubConfig(t, reg, "tok", testWebhookSecret)

	body := []byte(`{not valid json`)
	resp := post(t, tp.HandleInbound(), body, "issues", "d-1", "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

// TestInboundDeliveryIDIsMessageID: the X-GitHub-Delivery header
// value lands in Message.MessageID, mirroring email's MessageID
// preservation.
func TestInboundDeliveryIDIsMessageID(t *testing.T) {
	t.Parallel()
	tp, st, _, _, reg := newTestTransport(t)
	setGitHubConfig(t, reg, "tok", testWebhookSecret)
	seedGitHubStream(t, st, "s-github", "helixml/helix-org", []string{"issues"})

	body, _ := json.Marshal(issuesOpenedPayload("helixml/helix-org"))
	resp := post(t, tp.HandleInbound(), body, "issues", "particular-uuid-here", "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	events, _ := st.Events.ListForStream(context.Background(), "org-test", "s-github", 10)
	msg, _ := events[0].Message()
	if msg.MessageID != "particular-uuid-here" {
		t.Fatalf("MessageID = %q, want particular-uuid-here", msg.MessageID)
	}
}

// TestInboundEmptySenderTolerated: events without a sender (some
// system events) leave Message.From empty rather than erroring.
func TestInboundEmptySenderTolerated(t *testing.T) {
	t.Parallel()
	tp, st, _, _, reg := newTestTransport(t)
	setGitHubConfig(t, reg, "tok", testWebhookSecret)
	seedGitHubStream(t, st, "s-github", "helixml/helix-org", []string{"issues"})

	payload := issuesOpenedPayload("helixml/helix-org")
	delete(payload, "sender")
	body, _ := json.Marshal(payload)

	resp := post(t, tp.HandleInbound(), body, "issues", "d-1", "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	events, _ := st.Events.ListForStream(context.Background(), "org-test", "s-github", 10)
	msg, _ := events[0].Message()
	if msg.From != "" {
		t.Fatalf("From = %q, want empty for sender-less event", msg.From)
	}
}

// TestTransportValidateGitHub: stream-config validation. Required:
// non-empty repo of form "owner/name", non-empty events list. Event
// names must match ^[a-z][a-z0-9_]+$ — curated names (issues,
// pull_request, …) work, custom event names from new GitHub event
// types are accepted, malformed names are rejected.
func TestTransportValidateGitHub(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		cfg     string
		wantErr string // substring; "" = no error
	}{
		{
			name: "valid",
			cfg:  `{"repo":"helixml/helix-org","events":["issues","pull_request"]}`,
		},
		{
			name: "valid all known events",
			cfg: `{"repo":"helixml/helix-org","events":["issues","issue_comment","pull_request",` +
				`"pull_request_review","pull_request_review_comment"]}`,
		},
		{
			name:    "missing repo",
			cfg:     `{"events":["issues"]}`,
			wantErr: "repo",
		},
		{
			name:    "repo without slash",
			cfg:     `{"repo":"helix-org","events":["issues"]}`,
			wantErr: "owner/name",
		},
		{
			name:    "repo with extra path segment",
			cfg:     `{"repo":"helixml/helix-org/extra","events":["issues"]}`,
			wantErr: "owner/name",
		},
		{
			name:    "missing events",
			cfg:     `{"repo":"helixml/helix-org"}`,
			wantErr: "events",
		},
		{
			name:    "empty events",
			cfg:     `{"repo":"helixml/helix-org","events":[]}`,
			wantErr: "events",
		},
		{
			// Custom event names are accepted — the format check
			// catches typos, but operators can subscribe to event
			// types GitHub ships that we don't yet curate.
			name: "custom event name",
			cfg:  `{"repo":"helixml/helix-org","events":["workflow_run"]}`,
		},
		{
			name:    "malformed event name",
			cfg:     `{"repo":"helixml/helix-org","events":["Bad-Name"]}`,
			wantErr: "invalid event",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tr := transport.Transport{Kind: transport.KindGitHub, Config: json.RawMessage(tc.cfg)}
			err := tr.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate() = %q, want error containing %q", err, tc.wantErr)
			}
		})
	}
}

// TestGitHubConfigRoundTrip: parse Transport.Config back into a
// GitHubConfig with all fields populated.
func TestGitHubConfigRoundTrip(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{"repo":"helixml/helix-org","events":["issues","pull_request"]}`)
	c, err := transport.Transport{Kind: transport.KindGitHub, Config: raw}.GitHubConfig()
	if err != nil {
		t.Fatalf("GitHubConfig() = %v", err)
	}
	if c.Repo != "helixml/helix-org" {
		t.Fatalf("Repo = %q", c.Repo)
	}
	if len(c.Events) != 2 || c.Events[0] != "issues" || c.Events[1] != "pull_request" {
		t.Fatalf("Events = %v", c.Events)
	}
}

// TestTokenFallsBackToOAuthResolver pins the user-facing contract
// "reinstate the GitHub stream and reuse the existing GitHub
// integration for Auth". Operators previously had to paste a GitHub
// PAT into transport.github.token; the recommended path is now to
// connect a GitHub OAuth provider via Settings → Connected Services
// and let the transport pick up the token at activation time. The
// TokenResolver hook is what production wiring uses to plug in the
// helix OAuth manager.
//
// This test covers three cases:
//
//  1. transport.github.token set → wins, resolver not consulted (ops
//     can pin a specific PAT if they really want to).
//  2. transport.github.token empty + resolver returns a value →
//     resolver's value is returned.
//  3. transport.github.token empty + no resolver → empty string,
//     no error.
func TestTokenFallsBackToOAuthResolver(t *testing.T) {
	t.Parallel()
	t.Run("config token wins over resolver", func(t *testing.T) {
		tp, _, _, _, reg := newTestTransport(t)
		setGitHubConfig(t, reg, "from-config", testWebhookSecret)
		tp.WithTokenResolver(func(_ context.Context, _ string) (string, error) {
			t.Errorf("resolver consulted despite config token being set")
			return "from-resolver", nil
		})
		got, err := tp.Token(context.Background())
		if err != nil {
			t.Fatalf("Token: %v", err)
		}
		if got != "from-config" {
			t.Errorf("Token = %q, want %q", got, "from-config")
		}
	})

	t.Run("empty config falls through to resolver", func(t *testing.T) {
		tp, _, _, _, reg := newTestTransport(t)
		// Only the webhook_secret is set; token is intentionally empty
		// to mimic an operator who connected GitHub via OAuth instead
		// of pasting a PAT.
		setGitHubConfig(t, reg, "", testWebhookSecret)
		var called bool
		tp.WithTokenResolver(func(_ context.Context, orgID string) (string, error) {
			called = true
			if orgID != "org-test" {
				t.Errorf("resolver called with orgID = %q, want %q", orgID, "org-test")
			}
			return "from-oauth", nil
		})
		got, err := tp.Token(context.Background())
		if err != nil {
			t.Fatalf("Token: %v", err)
		}
		if !called {
			t.Error("resolver not consulted")
		}
		if got != "from-oauth" {
			t.Errorf("Token = %q, want %q", got, "from-oauth")
		}
	})

	t.Run("no config no resolver returns empty without error", func(t *testing.T) {
		tp, _, _, _, _ := newTestTransport(t)
		got, err := tp.Token(context.Background())
		if err != nil {
			t.Fatalf("Token: %v", err)
		}
		if got != "" {
			t.Errorf("Token = %q, want empty", got)
		}
	})
}

// TestInboundWildcardEvents pins the "send me everything" default.
// GitHub honours `events: ["*"]` as a wildcard meaning every event
// type; helix's transport mirrors that semantic in contains() so a
// stream configured with ["*"] receives every delivery regardless of
// the X-GitHub-Event header. Regression guard: before the wildcard
// landed, ["*"] would have been treated as a literal event name and
// no deliveries would ever match.
func TestInboundWildcardEvents(t *testing.T) {
	t.Parallel()

	tp, st, _, _, reg := newTestTransport(t)
	setGitHubConfig(t, reg, "", testWebhookSecret)
	seedGitHubStream(t, st, streaming.StreamID("s-everything"), "helixml/helix", []string{"*"})

	cases := []struct {
		name    string
		event   string
		payload map[string]any
	}{
		{"issues opened", "issues", issuesOpenedPayload("helixml/helix")},
		{"pull_request labeled", "pull_request", pullRequestLabeledPayload("helixml/helix", "ready-for-review")},
		{
			// An event type NOT in the curated whitelist (push,
			// release, workflow_run, etc) — wildcard should still
			// accept it.
			name:  "push (custom event)",
			event: "push",
			payload: map[string]any{
				"ref":        "refs/heads/main",
				"repository": map[string]any{"full_name": "helixml/helix"},
				"sender":     map[string]any{"login": "octocat"},
			},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(tc.payload)
			res := post(t, tp.HandleInbound(), body, tc.event, "del-"+tc.event, "")
			if res.StatusCode != http.StatusNoContent {
				t.Fatalf("status = %d, want 204 (wildcard should accept %q): body=%s", res.StatusCode, tc.event, res.Body)
			}
			events, err := st.Events.ListForStream(context.Background(), "org-test", streaming.StreamID("s-everything"), 50)
			if err != nil {
				t.Fatalf("list events: %v", err)
			}
			if len(events) == 0 {
				t.Fatalf("expected at least 1 event after wildcard delivery, got 0")
			}
		})
	}
}

// TestInboundFormEncodedBody covers the regression where GitHub's
// `application/x-www-form-urlencoded` deliveries (where the body is
// `payload=<urlencoded-json>`) used to 400 because helix tried to
// json.Unmarshal a form body. Before the decodeWebhookPayload
// helper landed, every form-encoded delivery from an existing
// hook that helix's UpsertWebhook had adopted (instead of editing)
// failed with `parse json: invalid character 'p' looking for
// beginning of value`.
func TestInboundFormEncodedBody(t *testing.T) {
	t.Parallel()

	tp, st, _, _, reg := newTestTransport(t)
	setGitHubConfig(t, reg, "", testWebhookSecret)
	seedGitHubStream(t, st, streaming.StreamID("s-form"), "helixml/helix", []string{"issues"})

	payload, _ := json.Marshal(issuesOpenedPayload("helixml/helix"))
	// GitHub form-encoding: percent-encode the JSON, prefix with `payload=`.
	formBody := []byte("payload=" + urlEncodeForTest(payload))

	srv := httptest.NewServer(tp.HandleInbound())
	t.Cleanup(srv.Close)
	req, _ := http.NewRequest(http.MethodPost, srv.URL, bytes.NewReader(formBody))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("X-GitHub-Delivery", "form-del-1")
	// HMAC is over the RAW body — GitHub signs the form-encoded
	// bytes, not the inner JSON. helix verifies the same.
	req.Header.Set("X-Hub-Signature-256", signBody(testWebhookSecret, formBody))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 204; body=%s", resp.StatusCode, body)
	}
	events, err := st.Events.ListForStream(context.Background(), "org-test", streaming.StreamID("s-form"), 50)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
}

// urlEncodeForTest percent-encodes raw bytes the same way GitHub
// form-encodes the JSON payload — the `payload=…` form field
// uses standard URL query-escape semantics.
func urlEncodeForTest(raw []byte) string {
	return url.QueryEscape(string(raw))
}

// TestInboundForStreamPinnedRouting pins the per-stream variant
// of the inbound handler. HandleInboundForStream(streamID) routes
// deliveries to ONLY that stream — even when another github stream
// in the same org would match by repo + events. Operators get a 1:1
// "GitHub webhook → helix stream" mapping with the per-stream URL,
// instead of the fan-out the org-level handler does.
func TestInboundForStreamPinnedRouting(t *testing.T) {
	t.Parallel()

	tp, st, _, _, reg := newTestTransport(t)
	setGitHubConfig(t, reg, "", testWebhookSecret)
	// Two streams for the SAME repo + same events — the only thing
	// that differs is which one the operator pasted the URL into.
	seedGitHubStream(t, st, streaming.StreamID("s-target"), "helixml/helix", []string{"issues"})
	seedGitHubStream(t, st, streaming.StreamID("s-bystander"), "helixml/helix", []string{"issues"})

	body, _ := json.Marshal(issuesOpenedPayload("helixml/helix"))
	res := post(t, tp.HandleInboundForStream(streaming.StreamID("s-target")), body, "issues", "del-pin-1", "")
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", res.StatusCode, res.Body)
	}

	// Targeted stream got the event.
	target, err := st.Events.ListForStream(context.Background(), "org-test", streaming.StreamID("s-target"), 50)
	if err != nil {
		t.Fatalf("list target events: %v", err)
	}
	if len(target) != 1 {
		t.Fatalf("target stream events = %d, want 1", len(target))
	}
	// Bystander stream did NOT — even though its (repo, events)
	// config matched the delivery, the per-stream handler pinned
	// fan-out to s-target only.
	by, err := st.Events.ListForStream(context.Background(), "org-test", streaming.StreamID("s-bystander"), 50)
	if err != nil {
		t.Fatalf("list bystander events: %v", err)
	}
	if len(by) != 0 {
		t.Fatalf("bystander stream events = %d, want 0 (per-stream handler should not fan out)", len(by))
	}
}

// TestInboundForStreamAppliesRepoFilter pins that the per-stream
// handler still honours the stream's own repo whitelist. A delivery
// for a different repo lands on the pinned URL but is dropped
// (with 204 so GitHub stops retrying) without becoming an event.
func TestInboundForStreamAppliesRepoFilter(t *testing.T) {
	t.Parallel()

	tp, st, _, _, reg := newTestTransport(t)
	setGitHubConfig(t, reg, "", testWebhookSecret)
	seedGitHubStream(t, st, streaming.StreamID("s-helix"), "helixml/helix", []string{"issues"})

	body, _ := json.Marshal(issuesOpenedPayload("other-owner/other-repo"))
	res := post(t, tp.HandleInboundForStream(streaming.StreamID("s-helix")), body, "issues", "del-wrong-repo", "")
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", res.StatusCode, res.Body)
	}
	events, err := st.Events.ListForStream(context.Background(), "org-test", streaming.StreamID("s-helix"), 50)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("events = %d, want 0 (wrong repo should drop)", len(events))
	}
}

// TestInboundForStreamAppliesEventFilter pins the event-whitelist
// filter for the per-stream handler. A delivery for an event NOT
// in the stream's `events` list drops with 204.
func TestInboundForStreamAppliesEventFilter(t *testing.T) {
	t.Parallel()

	tp, st, _, _, reg := newTestTransport(t)
	setGitHubConfig(t, reg, "", testWebhookSecret)
	seedGitHubStream(t, st, streaming.StreamID("s-issues-only"), "helixml/helix", []string{"issues"})

	// `pull_request` is NOT in the whitelist.
	body, _ := json.Marshal(pullRequestLabeledPayload("helixml/helix", "ready"))
	res := post(t, tp.HandleInboundForStream(streaming.StreamID("s-issues-only")), body, "pull_request", "del-wrong-event", "")
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", res.StatusCode, res.Body)
	}
	events, err := st.Events.ListForStream(context.Background(), "org-test", streaming.StreamID("s-issues-only"), 50)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("events = %d, want 0 (off-whitelist event should drop)", len(events))
	}
}

// TestInboundForStreamUnknownStreamReturns404 pins that POSTing to
// the per-stream URL for a non-existent stream returns 404. (The
// org-level handler returns 204 for unknown repos; the per-stream
// handler is stricter because the stream ID is in the URL.)
func TestInboundForStreamUnknownStreamReturns404(t *testing.T) {
	t.Parallel()

	tp, _, _, _, reg := newTestTransport(t)
	setGitHubConfig(t, reg, "", testWebhookSecret)

	body, _ := json.Marshal(issuesOpenedPayload("helixml/helix"))
	res := post(t, tp.HandleInboundForStream(streaming.StreamID("s-does-not-exist")), body, "issues", "del-missing", "")
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", res.StatusCode, res.Body)
	}
}
