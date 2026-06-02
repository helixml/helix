package postmark_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/config"
	"github.com/helixml/helix/api/pkg/org/event"
	"github.com/helixml/helix/api/pkg/org/message"
	"github.com/helixml/helix/api/pkg/org/store"
	orggorm "github.com/helixml/helix/api/pkg/org/store/gorm"
	"github.com/helixml/helix/api/pkg/org/stream"
	"github.com/helixml/helix/api/pkg/org/streamhub"
	"github.com/helixml/helix/api/pkg/org/transport"
	"github.com/helixml/helix/api/pkg/org/domain"
	"github.com/helixml/helix/api/pkg/org/transports/postmark"
	"github.com/helixml/helix/api/pkg/org/worker"
	"github.com/helixml/helix/api/pkg/pubsub"
)

// recordingDispatcher captures Dispatch calls for assertion.
type recordingDispatcher struct {
	mu     sync.Mutex
	events []domain.Event
}

func (d *recordingDispatcher) Dispatch(_ context.Context, e domain.Event) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.events = append(d.events, e)
}

func (d *recordingDispatcher) snapshot() []domain.Event {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]domain.Event, len(d.events))
	copy(out, d.events)
	return out
}

func newTestTransport(t *testing.T) (*postmark.Transport, *store.Store, *recordingDispatcher, *streamhub.Hub, *config.Registry) {
	t.Helper()
	st := orggorm.GetOrgTestDB(t)
	ps, err := pubsub.NewInMemoryNats()
	if err != nil {
		t.Fatalf("NewInMemoryNats: %v", err)
	}
	bc := streamhub.New(ps)
	rd := &recordingDispatcher{}
	reg := config.New(st.Configs)
	reg.Register(config.Spec{
		Key:     "transport.postmark",
		Type:    config.TypeObject,
		Secrets: []string{"token"},
	})
	tp := postmark.New("org-test", reg, st, bc, rd, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return tp, st, rd, bc, reg
}

func setPostmarkConfig(t *testing.T, reg *config.Registry, token, inbound, from string) {
	t.Helper()
	val, _ := json.Marshal(map[string]string{"token": token, "inbound": inbound, "from": from})
	if err := reg.Set(context.Background(), "org-test", "transport.postmark", string(val), worker.ID("")); err != nil {
		t.Fatalf("set config: %v", err)
	}
}

func seedEmailStream(t *testing.T, st *store.Store, id stream.ID, alias string) domain.Stream {
	t.Helper()
	cfg, _ := json.Marshal(transport.EmailConfig{Alias: alias})
	stream, err := domain.NewStream(id, string(id), "", "w-owner", time.Now().UTC(),
		transport.Transport{Kind: transport.KindEmail, Config: cfg}, "org-test")
	if err != nil {
		t.Fatalf("new stream: %v", err)
	}
	if err := st.Streams.Create(context.Background(), stream); err != nil {
		t.Fatalf("create stream: %v", err)
	}
	return stream
}

// TestInboundHappyPath: a Postmark inbound POST with `+sam` alias
// lands as an Event on the s-support stream, with all envelope
// fields populated and the dispatcher fired.
func TestInboundHappyPath(t *testing.T) {
	t.Parallel()
	tp, st, rd, _, reg := newTestTransport(t)
	setPostmarkConfig(t, reg, "tok", "abc123@inbound.postmarkapp.com", "you@gmail.com")
	seedEmailStream(t, st, "s-support", "sam")

	srv := httptest.NewServer(tp.HandleInbound())
	t.Cleanup(srv.Close)

	payload := map[string]any{
		"From":              "alice@example.com",
		"OriginalRecipient": "abc123+sam@inbound.postmarkapp.com",
		"To":                "abc123+sam@inbound.postmarkapp.com",
		"Subject":           "Webhook stream isn't firing",
		"MessageID":         "<msg-1@example.com>",
		"TextBody":          "I've got a stream set up but POSTs don't wake the worker.",
		"Headers": []map[string]string{
			{"Name": "In-Reply-To", "Value": ""},
		},
	}
	body, _ := json.Marshal(payload)
	resp, err := http.Post(srv.URL, "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		got, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body = %q", resp.StatusCode, got)
	}

	events, _ := st.Events.ListForStream(context.Background(), "org-test", "s-support", 10)
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	msg, err := events[0].Message()
	if err != nil {
		t.Fatalf("parse message: %v", err)
	}
	if msg.From != "alice@example.com" {
		t.Fatalf("From = %q", msg.From)
	}
	if msg.Subject != "Webhook stream isn't firing" {
		t.Fatalf("Subject = %q", msg.Subject)
	}
	if !strings.Contains(msg.Body, "POSTs don't wake the worker") {
		t.Fatalf("Body = %q", msg.Body)
	}
	if msg.MessageID != "<msg-1@example.com>" {
		t.Fatalf("MessageID = %q", msg.MessageID)
	}
	if events[0].Source != "" {
		t.Fatalf("Source should be empty for inbound webhook events, got %q", events[0].Source)
	}
	if len(rd.snapshot()) != 1 {
		t.Fatalf("dispatcher fired %d times, want 1", len(rd.snapshot()))
	}
}

func TestInboundNoAliasReturns400(t *testing.T) {
	t.Parallel()
	tp, st, _, _, reg := newTestTransport(t)
	setPostmarkConfig(t, reg, "tok", "abc123@inbound.postmarkapp.com", "you@gmail.com")
	seedEmailStream(t, st, "s-support", "sam")
	srv := httptest.NewServer(tp.HandleInbound())
	t.Cleanup(srv.Close)

	body, _ := json.Marshal(map[string]any{
		"From":              "alice@example.com",
		"OriginalRecipient": "abc123@inbound.postmarkapp.com", // no +alias
		"Subject":           "...",
		"TextBody":          "...",
	})
	resp, _ := http.Post(srv.URL, "application/json", strings.NewReader(string(body)))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestInboundUnknownAliasReturns404(t *testing.T) {
	t.Parallel()
	tp, st, _, _, reg := newTestTransport(t)
	setPostmarkConfig(t, reg, "tok", "abc123@inbound.postmarkapp.com", "you@gmail.com")
	seedEmailStream(t, st, "s-support", "sam") // alias=sam exists
	srv := httptest.NewServer(tp.HandleInbound())
	t.Cleanup(srv.Close)

	body, _ := json.Marshal(map[string]any{
		"From":              "alice@example.com",
		"OriginalRecipient": "abc123+marketing@inbound.postmarkapp.com", // alias=marketing missing
		"Subject":           "...",
		"TextBody":          "...",
	})
	resp, _ := http.Post(srv.URL, "application/json", strings.NewReader(string(body)))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestInboundMethodNotAllowed(t *testing.T) {
	t.Parallel()
	tp, _, _, _, _ := newTestTransport(t)
	srv := httptest.NewServer(tp.HandleInbound())
	t.Cleanup(srv.Close)

	resp, _ := http.Get(srv.URL)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
}

func TestInboundReplyPopulatesInReplyTo(t *testing.T) {
	t.Parallel()
	tp, st, _, _, reg := newTestTransport(t)
	setPostmarkConfig(t, reg, "tok", "abc123@inbound.postmarkapp.com", "you@gmail.com")
	seedEmailStream(t, st, "s-support", "sam")
	srv := httptest.NewServer(tp.HandleInbound())
	t.Cleanup(srv.Close)

	body, _ := json.Marshal(map[string]any{
		"From":              "alice@example.com",
		"OriginalRecipient": "abc123+sam@inbound.postmarkapp.com",
		"Subject":           "Re: Webhook stream isn't firing",
		"MessageID":         "<msg-2@example.com>",
		"TextBody":          "tried that, still broken",
		"Headers": []map[string]string{
			{"Name": "In-Reply-To", "Value": "<original@example.com>"},
			{"Name": "References", "Value": "<root@example.com> <intermediate@example.com>"},
		},
	})
	resp, _ := http.Post(srv.URL, "application/json", strings.NewReader(string(body)))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d", resp.StatusCode)
	}

	events, _ := st.Events.ListForStream(context.Background(), "org-test", "s-support", 10)
	msg, _ := events[0].Message()
	if msg.InReplyTo != "<original@example.com>" {
		t.Fatalf("InReplyTo = %q", msg.InReplyTo)
	}
	// References has multiple IDs space-separated; ThreadID = root.
	if msg.ThreadID != "<root@example.com>" {
		t.Fatalf("ThreadID = %q, want <root@example.com>", msg.ThreadID)
	}
}

// fakePostmark records the inbound /email POSTs (the reverse direction
// from the transport's perspective — we *send* outbound, Postmark
// receives). Tests use this to assert outbound payload shape without
// hitting the real API.
type fakePostmark struct {
	mu       sync.Mutex
	requests []fakePostmarkRequest
	status   int
}
type fakePostmarkRequest struct {
	headers http.Header
	payload map[string]any
}

func newFakePostmark(t *testing.T) (*httptest.Server, *fakePostmark) {
	t.Helper()
	fp := &fakePostmark{status: http.StatusOK}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		var p map[string]any
		_ = json.Unmarshal(body, &p)
		fp.mu.Lock()
		fp.requests = append(fp.requests, fakePostmarkRequest{headers: r.Header.Clone(), payload: p})
		s := fp.status
		fp.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(s)
		_, _ = w.Write([]byte(`{"ErrorCode":0,"Message":"OK","MessageID":"abc-fake"}`))
	}))
	t.Cleanup(srv.Close)
	return srv, fp
}

func (fp *fakePostmark) snapshot() []fakePostmarkRequest {
	fp.mu.Lock()
	defer fp.mu.Unlock()
	out := make([]fakePostmarkRequest, len(fp.requests))
	copy(out, fp.requests)
	return out
}

// TestEmitOutbound: a Message published to an email stream POSTs to
// Postmark with all the right fields — From from server config,
// To/Subject/Body from the Message, ReplyTo derived from alias,
// InReplyTo / References headers when threading.
func TestEmitOutbound(t *testing.T) {
	t.Parallel()
	tp, st, _, _, reg := newTestTransport(t)
	setPostmarkConfig(t, reg, "secret-token", "abc123@inbound.postmarkapp.com", "you@gmail.com")
	stream := seedEmailStream(t, st, "s-support", "sam")

	fakeSrv, fp := newFakePostmark(t)
	tp.SetSendURL(fakeSrv.URL)

	msg := message.Message{
		From:      "w-sam",
		To:        []string{"alice@example.com"},
		Subject:   "Re: Webhook question",
		Body:      "Most webhook flow issues are config or subscription mismatches.",
		InReplyTo: "<original@example.com>",
		ThreadID:  "<root@example.com>",
	}
	event, err := domain.NewMessageEvent(
		event.ID("e-1"),
		stream.ID,
		"w-sam",
		msg,
		time.Now().UTC(),
		"org-test",
	)
	if err != nil {
		t.Fatalf("new event: %v", err)
	}

	if err := tp.Emit(context.Background(), event); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	got := fp.snapshot()
	if len(got) != 1 {
		t.Fatalf("postmark requests = %d, want 1", len(got))
	}
	req := got[0]
	if h := req.headers.Get("X-Postmark-Server-Token"); h != "secret-token" {
		t.Fatalf("token header = %q", h)
	}
	if req.payload["From"] != "you@gmail.com" {
		t.Fatalf("From = %v, want you@gmail.com (server-config from)", req.payload["From"])
	}
	if req.payload["To"] != "alice@example.com" {
		t.Fatalf("To = %v", req.payload["To"])
	}
	if req.payload["ReplyTo"] != "abc123+sam@inbound.postmarkapp.com" {
		t.Fatalf("ReplyTo = %v", req.payload["ReplyTo"])
	}
	if req.payload["Subject"] != "Re: Webhook question" {
		t.Fatalf("Subject = %v", req.payload["Subject"])
	}
	if !strings.Contains(req.payload["TextBody"].(string), "Most webhook flow issues") {
		t.Fatalf("TextBody = %v", req.payload["TextBody"])
	}
	headers, ok := req.payload["Headers"].([]any)
	if !ok || len(headers) != 2 {
		t.Fatalf("Headers = %v, want 2 entries (In-Reply-To, References)", req.payload["Headers"])
	}
}

// TestEmitOverridesFromIfRealAddress: when the role's Message.From is
// a real email address (not a WorkerID), use it as the From header
// instead of the server-config default. Lets a future "billing" agent
// send From a different verified Sender Signature.
func TestEmitOverridesFromIfRealAddress(t *testing.T) {
	t.Parallel()
	tp, st, _, _, reg := newTestTransport(t)
	setPostmarkConfig(t, reg, "tok", "abc123@inbound.postmarkapp.com", "default@x.com")
	stream := seedEmailStream(t, st, "s-billing", "billing")

	fakeSrv, fp := newFakePostmark(t)
	tp.SetSendURL(fakeSrv.URL)

	msg := message.Message{
		From: "billing@x.com",
		To:   []string{"alice@example.com"},
		Body: "...",
	}
	event, _ := domain.NewMessageEvent("e-1", stream.ID, "w-billing", msg, time.Now().UTC(), "org-test")
	if err := tp.Emit(context.Background(), event); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	got := fp.snapshot()
	if got[0].payload["From"] != "billing@x.com" {
		t.Fatalf("From = %v, want override", got[0].payload["From"])
	}
}

func TestEmitNoRecipient(t *testing.T) {
	t.Parallel()
	tp, st, _, _, reg := newTestTransport(t)
	setPostmarkConfig(t, reg, "tok", "abc123@inbound.postmarkapp.com", "you@gmail.com")
	stream := seedEmailStream(t, st, "s-support", "sam")

	msg := message.Message{
		Body: "I forgot the recipient",
	}
	event, _ := domain.NewMessageEvent("e-1", stream.ID, "", msg, time.Now().UTC(), "org-test")
	err := tp.Emit(context.Background(), event)
	if err == nil || !strings.Contains(err.Error(), "no recipient") {
		t.Fatalf("err = %v", err)
	}
}

func TestEmitPostmarkError(t *testing.T) {
	t.Parallel()
	tp, st, _, _, reg := newTestTransport(t)
	setPostmarkConfig(t, reg, "tok", "abc123@inbound.postmarkapp.com", "you@gmail.com")
	stream := seedEmailStream(t, st, "s-support", "sam")

	fakeSrv, fp := newFakePostmark(t)
	fp.status = http.StatusUnprocessableEntity
	tp.SetSendURL(fakeSrv.URL)

	msg := message.Message{
		To:   []string{"alice@example.com"},
		Body: "...",
	}
	event, _ := domain.NewMessageEvent("e-1", stream.ID, "w-sam", msg, time.Now().UTC(), "org-test")
	err := tp.Emit(context.Background(), event)
	if err == nil || !strings.Contains(err.Error(), "postmark 422") {
		t.Fatalf("err = %v, want postmark 422", err)
	}
}

func TestAliasAddressHashForm(t *testing.T) {
	t.Parallel()
	c := postmark.Config{Inbound: "abc123@inbound.postmarkapp.com"}
	if got := c.AliasAddress("sam"); got != "abc123+sam@inbound.postmarkapp.com" {
		t.Fatalf("got %q", got)
	}
}
