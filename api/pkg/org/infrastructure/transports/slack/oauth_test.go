// OAuth install tests (FR-4, US-1). The callback exchanges the code
// Slack hands back for a bot token + team id and persists them as the
// org's transport.slack install. The org is carried in the encrypted
// state param; a state that won't decode is rejected without touching
// the store. Driven against a fake oauth.v2.access endpoint.
package slack_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	slacktransport "github.com/helixml/helix/api/pkg/org/infrastructure/transports/slack"
)

// fakeOAuth serves oauth.v2.access.
type fakeOAuth struct {
	server   *httptest.Server
	failNext bool
}

func newFakeOAuth(t *testing.T) *fakeOAuth {
	t.Helper()
	f := &fakeOAuth{}
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth.v2.access", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		if f.failNext || r.PostForm.Get("code") == "" {
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "invalid_code"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":           true,
			"access_token": "xoxb-installed-token",
			"team":         map[string]any{"id": "T0WORKSPACE", "name": "Acme"},
		})
	})
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

// trivialCodec is a reversible non-secret state codec for tests
// (production uses crypto.EncryptAES256GCM).
func encodeState(orgID string) (string, error) { return "state:" + orgID, nil }
func decodeState(state string) (string, error) {
	const p = "state:"
	if len(state) < len(p) || state[:len(p)] != p {
		return "", errBadState
	}
	return state[len(p):], nil
}

var errBadState = errorString("bad state")

type errorString string

func (e errorString) Error() string { return string(e) }

func newTestOAuth(t *testing.T, fake *fakeOAuth, app slacktransport.App) (*slacktransport.OAuth, *store.Store, *configregistry.Registry) {
	t.Helper()
	st := orggorm.GetOrgTestDB(t)
	reg := configregistry.New(st.Configs)
	reg.Register(configregistry.Spec{Key: "transport.slack", Type: configregistry.TypeObject, Secrets: []string{"bot_token"}})
	globalApp := func(context.Context) (slacktransport.App, error) { return app, nil }
	o := slacktransport.NewOAuth(globalApp, reg, encodeState, decodeState, "https://helix.example.com/api/v1/slack/oauth/callback", slog.New(slog.NewTextHandler(io.Discard, nil)))
	o.SetTokenURL(fake.server.URL + "/oauth.v2.access")
	return o, st, reg
}

func TestOAuth_CallbackPersistsInstall(t *testing.T) {
	fake := newFakeOAuth(t)
	o, st, reg := newTestOAuth(t, fake, slacktransport.App{ClientID: "cid", ClientSecret: "secret", Enabled: true})

	state, _ := encodeState("org-a")
	if err := o.HandleCallback(context.Background(), "the-code", state); err != nil {
		t.Fatalf("HandleCallback: %v", err)
	}

	var cfg slacktransport.Config
	if err := reg.GetObject(context.Background(), "org-a", "transport.slack", &cfg); err != nil {
		t.Fatalf("read persisted config: %v", err)
	}
	if cfg.BotToken != "xoxb-installed-token" {
		t.Errorf("bot_token = %q, want xoxb-installed-token", cfg.BotToken)
	}
	if cfg.TeamID != "T0WORKSPACE" {
		t.Errorf("team_id = %q, want T0WORKSPACE", cfg.TeamID)
	}
	_ = st
}

func TestOAuth_BadStateRejected(t *testing.T) {
	fake := newFakeOAuth(t)
	o, _, reg := newTestOAuth(t, fake, slacktransport.App{ClientID: "cid", ClientSecret: "secret", Enabled: true})

	err := o.HandleCallback(context.Background(), "the-code", "garbage-state")
	if err == nil {
		t.Fatalf("HandleCallback with bad state = nil, want error")
	}
	// Nothing persisted.
	var cfg slacktransport.Config
	if err := reg.GetObject(context.Background(), "org-a", "transport.slack", &cfg); err == nil {
		t.Fatalf("config persisted despite bad state")
	}
}

func TestOAuth_StartURLContainsClientAndState(t *testing.T) {
	fake := newFakeOAuth(t)
	o, _, _ := newTestOAuth(t, fake, slacktransport.App{ClientID: "my-client-id", Enabled: true})

	u, err := o.StartURL(context.Background(), "org-a", []string{"chat:write", "channels:read"})
	if err != nil {
		t.Fatalf("StartURL: %v", err)
	}
	for _, want := range []string{"client_id=my-client-id", "state=state%3Aorg-a", "chat%3Awrite", "redirect_uri="} {
		if !containsSub(u, want) {
			t.Errorf("authorize URL %q missing %q", u, want)
		}
	}
}

func containsSub(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
