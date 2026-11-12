package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/crypto"
)

// The OAuth state must expire: a state without an issue time (or an old
// one) would otherwise stay usable to complete an install forever.
func TestSlackOAuthCallbackRejectsStateWithoutIssueTime(t *testing.T) {
	t.Setenv("HELIX_ENCRYPTION_KEY", "slack-oauth-test")
	key, err := crypto.GetEncryptionKey()
	if err != nil {
		t.Fatal(err)
	}
	state, err := crypto.EncryptAES256GCM([]byte("org-1|app-1"), key)
	if err != nil {
		t.Fatal(err)
	}
	server := &HelixAPIServer{Cfg: &config.ServerConfig{}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/slack/oauth/callback?state="+url.QueryEscape(state), nil)
	rec := httptest.NewRecorder()
	server.slackOAuthCallback(rec, req)
	if rec.Code != http.StatusBadRequest || rec.Body.String() != "invalid state\n" {
		t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
	}
}
