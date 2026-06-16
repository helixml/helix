// Outbound tests (FR-10/11/12/13). Emit renders an appended Event on a
// KindSlack stream into a chat.postMessage carrying the bound channel,
// the Worker's persona (username + icon_url), and thread_ts for
// threaded replies. Driven against a fake Slack API (httptest.Server)
// so no network is touched.
package slack_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	slacktransport "github.com/helixml/helix/api/pkg/org/infrastructure/transports/slack"
)

// fakeSlack records the last chat.postMessage form and replies ok.
type fakeSlack struct {
	mu     sync.Mutex
	form   url.Values
	calls  int
	server *httptest.Server
}

func newFakeSlack(t *testing.T) *fakeSlack {
	t.Helper()
	f := &fakeSlack{}
	mux := http.NewServeMux()
	mux.HandleFunc("/chat.postMessage", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.mu.Lock()
		f.form = r.PostForm
		f.calls++
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "channel": r.PostForm.Get("channel"), "ts": "1700000002.000300"})
	})
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

// apiURL returns the base URL slack-go expects (trailing slash).
func (f *fakeSlack) apiURL() string { return f.server.URL + "/" }

func (f *fakeSlack) get(key string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.form == nil {
		return ""
	}
	return f.form.Get(key)
}

func (f *fakeSlack) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func newTestOutbound(t *testing.T, persona slacktransport.PersonaResolver) (*slacktransport.Outbound, *store.Store, *configregistry.Registry, *fakeSlack) {
	t.Helper()
	st := orggorm.GetOrgTestDB(t)
	reg := configregistry.New(st.Configs)
	reg.Register(configregistry.Spec{Key: "transport.slack", Type: configregistry.TypeObject, Secrets: []string{"bot_token"}})
	fake := newFakeSlack(t)
	out := slacktransport.NewOutbound(reg, st, persona, slog.New(slog.NewTextHandler(io.Discard, nil)))
	out.SetAPIURL(fake.apiURL())
	return out, st, reg, fake
}

// emitEvent builds and emits a worker-published event on a slack stream.
func emitEvent(t *testing.T, out *slacktransport.Outbound, stream streaming.Stream, source string, msg streaming.Message) error {
	t.Helper()
	ev, err := streaming.NewMessageEvent(streaming.EventID("e-1"), stream.ID, source, msg, time.Now().UTC(), stream.OrganizationID)
	if err != nil {
		t.Fatalf("new event: %v", err)
	}
	return out.Emit(context.Background(), stream, ev)
}

func TestOutbound_PostsWithPersonaAndThread(t *testing.T) {
	persona := func(_ context.Context, _, workerID string) (slacktransport.Persona, error) {
		return slacktransport.Persona{Username: "Sam the Support Bot", IconURL: "https://example.com/sam.png"}, nil
	}
	out, st, reg, fake := newTestOutbound(t, persona)
	setSlackInstall(t, reg, "org-a", "xoxb-token-a", "TAAA")
	stream := seedSlackStream(t, st, "org-a", "str-a", "C123")

	err := emitEvent(t, out, stream, "w-sam", streaming.Message{Body: "hello channel", ThreadID: "1699999999.000001"})
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if fake.callCount() != 1 {
		t.Fatalf("postMessage calls = %d, want 1", fake.callCount())
	}
	if got := fake.get("channel"); got != "C123" {
		t.Errorf("channel = %q, want C123", got)
	}
	if got := fake.get("text"); got != "hello channel" {
		t.Errorf("text = %q, want 'hello channel'", got)
	}
	if got := fake.get("username"); got != "Sam the Support Bot" {
		t.Errorf("username = %q, want persona username", got)
	}
	if got := fake.get("icon_url"); got != "https://example.com/sam.png" {
		t.Errorf("icon_url = %q, want persona icon", got)
	}
	if got := fake.get("thread_ts"); got != "1699999999.000001" {
		t.Errorf("thread_ts = %q, want Message.ThreadID", got)
	}
}

func TestOutbound_NoThreadOmitsThreadTS(t *testing.T) {
	out, st, reg, fake := newTestOutbound(t, slacktransport.DefaultPersonaResolver)
	setSlackInstall(t, reg, "org-a", "xoxb-token-a", "TAAA")
	stream := seedSlackStream(t, st, "org-a", "str-a", "C123")

	if err := emitEvent(t, out, stream, "w-sam", streaming.Message{Body: "top level"}); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if got := fake.get("thread_ts"); got != "" {
		t.Errorf("thread_ts = %q, want empty for non-threaded message", got)
	}
	// Default persona uses the bare worker id as username.
	if got := fake.get("username"); got != "w-sam" {
		t.Errorf("username = %q, want w-sam (default persona)", got)
	}
}

func TestOutbound_MissingTokenTypedError(t *testing.T) {
	out, st, _, fake := newTestOutbound(t, slacktransport.DefaultPersonaResolver)
	// No install config set for org-a → no bot token.
	stream := seedSlackStream(t, st, "org-a", "str-a", "C123")

	err := emitEvent(t, out, stream, "w-sam", streaming.Message{Body: "hi"})
	if err == nil {
		t.Fatalf("Emit() = nil, want error for missing token")
	}
	if fake.callCount() != 0 {
		t.Fatalf("postMessage called %d times without a token, want 0", fake.callCount())
	}
}

// TestOutbound_SatisfiesPort is a compile-time-ish guard that Outbound
// is a streaming.Outbound (the dispatcher registers it by that port).
func TestOutbound_SatisfiesPort(t *testing.T) {
	var _ streaming.Outbound = (*slacktransport.Outbound)(nil)
}
