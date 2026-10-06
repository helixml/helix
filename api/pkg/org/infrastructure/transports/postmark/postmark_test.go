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

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/org/application/configregistry"

	"github.com/helixml/helix/api/pkg/org/application/publishing"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/domain/trigger"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	"github.com/helixml/helix/api/pkg/org/infrastructure/transports/postmark"
	"github.com/helixml/helix/api/pkg/org/infrastructure/wakebus"
	"github.com/helixml/helix/api/pkg/pubsub"
)

// recordingPublisher wraps the real publish use case so tests can
// assert what a delivery produced.
type recordingPublisher struct {
	inner  *publishing.Publishing
	mu     sync.Mutex
	events []streaming.Event
}

func (d *recordingPublisher) PublishDelivery(ctx context.Context, orgID, triggerID string, eventID streaming.EventID, msg streaming.Message) (streaming.Event, error) {
	e, err := d.inner.PublishDelivery(ctx, orgID, triggerID, eventID, msg)
	if err != nil {
		return e, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.events = append(d.events, e)
	return e, nil
}

func (d *recordingPublisher) snapshot() []streaming.Event {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]streaming.Event, len(d.events))
	copy(out, d.events)
	return out
}

func newTestTransport(t *testing.T) (*postmark.Transport, *store.Store, *recordingPublisher, *wakebus.Bus, *configregistry.Registry) {
	t.Helper()
	st := orggorm.GetOrgTestDB(t)
	ps, err := pubsub.NewInMemoryNats()
	if err != nil {
		t.Fatalf("NewInMemoryNats: %v", err)
	}
	bc := wakebus.New(ps)
	rd := &recordingPublisher{inner: publishing.New(publishing.Deps{Triggers: st.Triggers, Events: st.Events, Hub: bc, Now: func() time.Time { return time.Now().UTC() }, NewID: uuid.NewString})}
	reg := configregistry.New(st.Configs)
	reg.Register(configregistry.Spec{
		Key:     "transport.postmark",
		Type:    configregistry.TypeObject,
		Secrets: []string{"token"},
	})
	tp := postmark.New("org-test", reg, st, rd, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return tp, st, rd, bc, reg
}

func setPostmarkConfig(t *testing.T, reg *configregistry.Registry, token, inbound, from string) {
	t.Helper()
	val, _ := json.Marshal(map[string]string{"token": token, "inbound": inbound, "from": from})
	if err := reg.Set(context.Background(), "org-test", "transport.postmark", string(val)); err != nil {
		t.Fatalf("set config: %v", err)
	}
}

func seedEmailTrigger(t *testing.T, st *store.Store, id, alias string) trigger.Trigger {
	t.Helper()
	cfg, _ := json.Marshal(transport.EmailConfig{Alias: alias})
	row, err := trigger.New(id, "org-test", id, "", transport.KindEmail, cfg, "w-owner", time.Now().UTC())
	if err != nil {
		t.Fatalf("new trigger: %v", err)
	}
	if err := st.Triggers.Create(context.Background(), row); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	return row
}

// TestInboundHappyPath: a Postmark inbound POST with `+sam` alias
// lands as an event on the s-support Trigger, with all envelope
// fields populated and the publish path exercised.
func TestInboundHappyPath(t *testing.T) {
	t.Parallel()
	tp, st, rd, _, reg := newTestTransport(t)
	setPostmarkConfig(t, reg, "tok", "abc123@inbound.postmarkapp.com", "you@gmail.com")
	seedEmailTrigger(t, st, "s-support", "sam")

	srv := httptest.NewServer(tp.HandleInbound())
	t.Cleanup(srv.Close)

	payload := map[string]any{
		"From":              "alice@example.com",
		"OriginalRecipient": "abc123+sam@inbound.postmarkapp.com",
		"To":                "abc123+sam@inbound.postmarkapp.com",
		"Subject":           "Webhook topic isn't firing",
		"MessageID":         "<msg-1@example.com>",
		"TextBody":          "I've got a topic set up but POSTs don't wake the worker.",
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
	if msg.Subject != "Webhook topic isn't firing" {
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
	seedEmailTrigger(t, st, "s-support", "sam")
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
	seedEmailTrigger(t, st, "s-support", "sam") // alias=sam exists
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
	seedEmailTrigger(t, st, "s-support", "sam")
	srv := httptest.NewServer(tp.HandleInbound())
	t.Cleanup(srv.Close)

	body, _ := json.Marshal(map[string]any{
		"From":              "alice@example.com",
		"OriginalRecipient": "abc123+sam@inbound.postmarkapp.com",
		"Subject":           "Re: Webhook topic isn't firing",
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

func TestAliasAddressHashForm(t *testing.T) {
	t.Parallel()
	c := postmark.Config{Inbound: "abc123@inbound.postmarkapp.com"}
	if got := c.AliasAddress("sam"); got != "abc123+sam@inbound.postmarkapp.com" {
		t.Fatalf("got %q", got)
	}
}
