package server

import (
	"context"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// A bot instance key sits in a sandbox that reads untrusted pages and chats
// with untrusted users, so these tests take the attacker's side: given the key
// for instance A, what can it reach?

func botInstanceUser() *types.User {
	return &types.User{
		ID:         "usr_owner",
		APIKeyType: types.APIkeytypeBotInstance,
		SessionID:  "ses_A",
		ProjectID:  "prj_bot",
	}
}

func botInstanceAuth(t *testing.T, profile *types.BotInstanceProfile) *authMiddleware {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	st.EXPECT().GetSession(gomock.Any(), "ses_A").Return(&types.Session{
		ID:       "ses_A",
		Metadata: types.SessionMetadata{SessionRole: types.SessionRoleOrgBotInstance, BotInstance: profile},
	}, nil).AnyTimes()
	return &authMiddleware{store: st}
}

func TestBotInstanceKeyAllowsItsOwnSandboxNeeds(t *testing.T) {
	profile := &types.BotInstanceProfile{MCPServers: []string{"chrome-devtools", "helix-session", "My CRM"}}
	auth := botInstanceAuth(t, profile)
	u := botInstanceUser()
	allowed := []struct{ method, path string }{
		{"GET", "/api/v1/sessions/ses_A/zed-config"},
		{"POST", "/api/v1/sessions/ses_A/zed-config/user"},
		{"POST", "/api/v1/sessions/ses_A/agent-startup-error"},
		{"POST", "/api/v1/sessions/ses_A/agent-config-applied"},
		{"GET", "/api/v1/ws/user?session_id=ses_A"},
		{"GET", "/api/v1/external-agents/sync?session_id=ses_A"},
		{"GET", "/api/v1/revdial?runnerid=desktop-ses_A"},
		{"GET", "/api/v1/revdial?revdial.dialer=dialer-token"},
		{"POST", "/v1/chat/completions"},
		{"POST", "/v1/responses"},
		{"POST", "/v1/messages"},
		{"GET", "/v1/models"},
		{"POST", "/api/v1/mcp/helix-org"},
		{"POST", "/api/v1/mcp/session?session_id=ses_A"},
		{"POST", "/api/v1/mcp/external/my-crm"},
	}
	for _, c := range allowed {
		if !auth.botInstanceKeyAllows(context.Background(), u, req(c.method, c.path)) {
			t.Errorf("should allow %s %s", c.method, c.path)
		}
	}
}

func TestBotInstanceKeyDeniesEverythingElse(t *testing.T) {
	auth := botInstanceAuth(t, &types.BotInstanceProfile{MCPServers: []string{"chrome-devtools"}})
	u := botInstanceUser()
	denied := []struct{ method, path string }{
		// Another session's plumbing.
		{"GET", "/api/v1/sessions/ses_B/zed-config"},
		{"GET", "/api/v1/ws/user?session_id=ses_B"},
		{"GET", "/api/v1/ws/user"},
		{"GET", "/api/v1/external-agents/sync?session_id=ses_B"},
		{"GET", "/api/v1/revdial?runnerid=desktop-ses_B"},
		{"GET", "/api/v1/revdial"},
		// Its own session, but not a sandbox route.
		{"GET", "/api/v1/sessions/ses_A"},
		{"GET", "/api/v1/sessions/ses_A/interactions"},
		{"DELETE", "/api/v1/sessions/ses_A"},
		{"POST", "/api/v1/sessions/ses_A/zed-config"},
		// Chatting, into itself or anything else.
		{"POST", "/api/v1/sessions/chat"},
		{"POST", "/api/v1/sessions/ses_A/messages"},
		{"POST", "/api/v1/prompt-history/sync"},
		// Tenant inventory and everything the helix CLI would reach for.
		{"GET", "/api/v1/projects"},
		{"GET", "/api/v1/projects/prj_bot"},
		{"GET", "/api/v1/sessions"},
		{"GET", "/api/v1/apps"},
		{"GET", "/api/v1/secrets"},
		{"GET", "/api/v1/spec-tasks"},
		{"GET", "/api/v1/orgs/unmanned-org/bots"},
		{"GET", "/api/v1/git/repositories"},
		{"GET", "/api/v1/config"},
		// MCP servers the profile doesn't keep, and a spoofed session.
		{"POST", "/api/v1/mcp/session?session_id=ses_A"},
		{"POST", "/api/v1/mcp/desktop?session_id=ses_A"},
		{"POST", "/api/v1/mcp/kodit?session_id=ses_A"},
		{"POST", "/api/v1/mcp/helix?app_id=app_1&session_id=ses_A"},
		{"POST", "/api/v1/mcp/helix-tasks"},
		{"POST", "/api/v1/mcp/external/crm"},
		{"POST", "/api/v1/mcp/helix-org?session_id=ses_B"},
		// Wrong method on an allowed path.
		{"DELETE", "/v1/chat/completions"},
		{"GET", "/api/v1/sessions/ses_A/agent-startup-error"},
	}
	for _, c := range denied {
		if auth.botInstanceKeyAllows(context.Background(), u, req(c.method, c.path)) {
			t.Errorf("should deny %s %s", c.method, c.path)
		}
	}
}

func TestBotInstanceKeyWithoutSessionAllowsNothing(t *testing.T) {
	auth := botInstanceAuth(t, nil)
	u := botInstanceUser()
	u.SessionID = ""
	if auth.botInstanceKeyAllows(context.Background(), u, req("POST", "/v1/chat/completions")) {
		t.Fatal("a key with no session must reach nothing")
	}
}
