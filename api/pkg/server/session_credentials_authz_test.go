package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/crypto"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

func encryptedJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	key, err := crypto.GetEncryptionKey()
	require.NoError(t, err)
	enc, err := crypto.EncryptAES256GCM(raw, key)
	require.NoError(t, err)
	return enc
}

func sessionCredentialRequest(sessionID string, user types.User) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+sessionID+"/claude-credentials", nil)
	req = req.WithContext(setRequestUser(context.Background(), user))
	return mux.SetURLVars(req, map[string]string{"id": sessionID})
}

// A session owner who is not the subscription's owner must not receive an
// org-owned or delegated subscription's tokens; only the session's own
// sandbox key (or a per-runner token for its sandbox host) may.
func TestGetSessionClaudeCredentials_OrgSubscriptionOnlyForSessionRuntime(t *testing.T) {
	t.Setenv("HELIX_ENCRYPTION_KEY", "test-key")
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	session := &types.Session{ID: "ses_1", Owner: "usr_member", OrganizationID: "org_1", SandboxID: "host1"}
	mockStore.EXPECT().GetSession(gomock.Any(), "ses_1").Return(session, nil).AnyTimes()
	mockStore.EXPECT().GetSessionClaudeSubscription(gomock.Any(), gomock.Any()).Return(&types.ClaudeSubscription{
		ID: "sub_org", OwnerID: "org_1", OwnerType: types.OwnerTypeOrg, CredentialType: "setup_token",
		EncryptedCredentials: encryptedJSON(t, types.ClaudeSetupTokenCredentials{SetupToken: "org-setup-token"}),
	}, nil).AnyTimes()
	server := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: mockStore}

	for name, user := range map[string]types.User{
		"owner browser session":   {ID: "usr_member", TokenType: types.TokenTypeSession},
		"owner personal api key":  {ID: "usr_member", TokenType: types.TokenTypeAPIKey},
		"other session's api key": {ID: "usr_member", TokenType: types.TokenTypeAPIKey, SessionID: "ses_other"},
		"shared runner token":     {TokenType: types.TokenTypeRunner},
	} {
		t.Run(name, func(t *testing.T) {
			_, httpErr := server.getSessionClaudeCredentials(nil, sessionCredentialRequest("ses_1", user))
			require.NotNil(t, httpErr)
			require.Equal(t, http.StatusForbidden, httpErr.StatusCode)
		})
	}

	for name, user := range map[string]types.User{
		"session api key":               {ID: "usr_member", TokenType: types.TokenTypeAPIKey, SessionID: "ses_1"},
		"per-runner token for its host": {TokenType: types.TokenTypeRunner, RunnerID: "host1"},
	} {
		t.Run(name, func(t *testing.T) {
			resp, httpErr := server.getSessionClaudeCredentials(nil, sessionCredentialRequest("ses_1", user))
			require.Nil(t, httpErr)
			require.Equal(t, "org-setup-token", resp.SetupToken)
		})
	}
}

// The owner of a personal subscription may still use it from their own session.
func TestGetSessionClaudeCredentials_PersonalSubscriptionForItsOwner(t *testing.T) {
	t.Setenv("HELIX_ENCRYPTION_KEY", "test-key")
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetSession(gomock.Any(), "ses_1").Return(&types.Session{ID: "ses_1", Owner: "usr_member"}, nil)
	mockStore.EXPECT().GetSessionClaudeSubscription(gomock.Any(), gomock.Any()).Return(&types.ClaudeSubscription{
		OwnerID: "usr_member", OwnerType: types.OwnerTypeUser, CredentialType: "setup_token",
		EncryptedCredentials: encryptedJSON(t, types.ClaudeSetupTokenCredentials{SetupToken: "mine"}),
	}, nil)
	server := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: mockStore}

	resp, httpErr := server.getSessionClaudeCredentials(nil, sessionCredentialRequest("ses_1", types.User{ID: "usr_member", TokenType: types.TokenTypeSession}))
	require.Nil(t, httpErr)
	require.Equal(t, "mine", resp.SetupToken)
}

func TestUpdateSessionCodexCredentials_SessionOwnerCannotOverwriteOrgSubscription(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetSession(gomock.Any(), "ses_1").Return(&types.Session{ID: "ses_1", Owner: "usr_member", OrganizationID: "org_1"}, nil)
	mockStore.EXPECT().GetEffectiveCodexSubscription(gomock.Any(), "usr_member", "org_1").Return(
		&types.CodexSubscription{ID: "sub_org", OwnerID: "org_1", OwnerType: types.OwnerTypeOrg}, nil)
	mockStore.EXPECT().UpdateCodexSubscriptionCredentialsIfNewer(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	server := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: mockStore}

	body := `{"auth_mode":"chatgpt","tokens":{"id_token":"a.b.c","access_token":"x","refresh_token":"y","account_id":"acc"},"last_refresh":"2030-01-01T00:00:00Z"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/sessions/ses_1/codex-credentials", strings.NewReader(body))
	req = req.WithContext(setRequestUser(context.Background(), types.User{ID: "usr_member", TokenType: types.TokenTypeSession}))
	req = mux.SetURLVars(req, map[string]string{"id": "ses_1"})
	_, httpErr := server.updateSessionCodexCredentials(nil, req)
	require.NotNil(t, httpErr)
	require.Equal(t, http.StatusForbidden, httpErr.StatusCode, httpErr.Message)
}
