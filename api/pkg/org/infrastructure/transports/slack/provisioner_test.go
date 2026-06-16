// Provisioner tests (FR-6). Install ensures the bot is a member of the
// bound channel (conversations.join); Status reports membership,
// degrading to "unknown" on an API failure rather than erroring (the
// streaming.Inbound contract). Driven against a fake Slack API.
package slack_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	slacktransport "github.com/helixml/helix/api/pkg/org/infrastructure/transports/slack"
)

// fakeConversations serves conversations.join / conversations.info.
type fakeConversations struct {
	server   *httptest.Server
	isMember atomic.Bool
	joinErr  atomic.Bool
	infoErr  atomic.Bool
	joined   atomic.Int32
}

func newFakeConversations(t *testing.T) *fakeConversations {
	t.Helper()
	f := &fakeConversations{}
	mux := http.NewServeMux()
	mux.HandleFunc("/conversations.join", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.joined.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if f.joinErr.Load() {
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "channel_not_found"})
			return
		}
		f.isMember.Store(true)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "channel": map[string]any{"id": r.PostForm.Get("channel"), "is_member": true}})
	})
	mux.HandleFunc("/conversations.info", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		if f.infoErr.Load() {
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "channel_not_found"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "channel": map[string]any{"id": r.PostForm.Get("channel"), "is_member": f.isMember.Load()}})
	})
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeConversations) apiURL() string { return f.server.URL + "/" }

func newTestProvisioner(t *testing.T, fake *fakeConversations) (*slacktransport.Provisioner, *store.Store, *configregistry.Registry) {
	t.Helper()
	st := orggorm.GetOrgTestDB(t)
	reg := configregistry.New(st.Configs)
	reg.Register(configregistry.Spec{Key: "transport.slack", Type: configregistry.TypeObject, Secrets: []string{"bot_token"}})
	p := slacktransport.NewProvisioner(reg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	p.SetAPIURL(fake.apiURL())
	return p, st, reg
}

func TestProvisioner_InstallJoinsChannel(t *testing.T) {
	fake := newFakeConversations(t)
	p, st, reg := newTestProvisioner(t, fake)
	setSlackInstall(t, reg, "org-a", "xoxb-a", "TAAA")
	stream := seedSlackStream(t, st, "org-a", "str-a", "C123")

	_, err := p.Install(context.Background(), "org-a", stream)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if fake.joined.Load() != 1 {
		t.Fatalf("conversations.join calls = %d, want 1", fake.joined.Load())
	}

	state, err := p.Status(context.Background(), "org-a", stream)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if state.State != "installed" {
		t.Fatalf("post-install status = %q, want installed", state.State)
	}
}

func TestProvisioner_StatusMissingWhenNotMember(t *testing.T) {
	fake := newFakeConversations(t)
	p, st, reg := newTestProvisioner(t, fake)
	setSlackInstall(t, reg, "org-a", "xoxb-a", "TAAA")
	stream := seedSlackStream(t, st, "org-a", "str-a", "C123")
	// isMember defaults false → not joined.

	state, err := p.Status(context.Background(), "org-a", stream)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if state.State != "missing" {
		t.Fatalf("status = %q, want missing", state.State)
	}
}

func TestProvisioner_StatusUnknownOnAPIError(t *testing.T) {
	fake := newFakeConversations(t)
	fake.infoErr.Store(true)
	p, st, reg := newTestProvisioner(t, fake)
	setSlackInstall(t, reg, "org-a", "xoxb-a", "TAAA")
	stream := seedSlackStream(t, st, "org-a", "str-a", "C123")

	state, err := p.Status(context.Background(), "org-a", stream)
	if err != nil {
		t.Fatalf("Status should degrade, not error: %v", err)
	}
	if state.State != "unknown" {
		t.Fatalf("status = %q, want unknown on API failure", state.State)
	}
}

func TestProvisioner_StatusUnknownWhenNotInstalled(t *testing.T) {
	fake := newFakeConversations(t)
	p, st, _ := newTestProvisioner(t, fake)
	// No transport.slack config for org-a → org not connected.
	stream := seedSlackStream(t, st, "org-a", "str-a", "C123")

	state, err := p.Status(context.Background(), "org-a", stream)
	if err != nil {
		t.Fatalf("Status should degrade, not error: %v", err)
	}
	if state.State != "unknown" {
		t.Fatalf("status = %q, want unknown when org has no install", state.State)
	}
}

func TestProvisioner_InstallUpstreamError(t *testing.T) {
	fake := newFakeConversations(t)
	fake.joinErr.Store(true)
	p, st, reg := newTestProvisioner(t, fake)
	setSlackInstall(t, reg, "org-a", "xoxb-a", "TAAA")
	stream := seedSlackStream(t, st, "org-a", "str-a", "C123")

	_, err := p.Install(context.Background(), "org-a", stream)
	if err == nil {
		t.Fatalf("Install() = nil, want error when join fails")
	}
}
