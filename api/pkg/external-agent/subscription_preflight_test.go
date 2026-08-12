package external_agent

import (
	"context"
	"errors"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func subscriptionSession() *types.Session {
	return &types.Session{
		ID:             "ses_x",
		Owner:          "usr_no_sub",
		OrganizationID: "org_1",
		ParentApp:      "app_1",
	}
}

func appWithRuntime(runtime types.CodeAgentRuntime, credType types.CodeAgentCredentialType) *types.App {
	return &types.App{
		ID: "app_1",
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{
					// A non-zed_external assistant first, to prove selection
					// matches buildCodeAgentConfig (first zed_external wins).
					{Name: "other"},
					{
						Name:                    "agent",
						AgentType:               types.AgentTypeZedExternal,
						CodeAgentRuntime:        runtime,
						CodeAgentCredentialType: credType,
					},
				},
			},
		},
	}
}

// A codex_cli agent in subscription mode with no reachable ChatGPT subscription
// must be rejected before the container boots — otherwise settings-sync never
// registers codex-acp and Zed hangs forever trying to create a thread.
func TestVerifySubscriptionCredentials_CodexMissing(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	h := newTestExecutor(mockStore)

	mockStore.EXPECT().GetSession(gomock.Any(), "ses_x").Return(subscriptionSession(), nil)
	mockStore.EXPECT().GetApp(gomock.Any(), "app_1").
		Return(appWithRuntime(types.CodeAgentRuntimeCodexCLI, types.CodeAgentCredentialTypeSubscription), nil)
	mockStore.EXPECT().GetEffectiveCodexSubscription(gomock.Any(), "usr_no_sub", "org_1").
		Return(nil, store.ErrNotFound)

	err := h.verifySubscriptionCredentials(context.Background(), &types.DesktopAgent{SessionID: "ses_x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ChatGPT subscription")
	assert.Contains(t, err.Error(), "usr_no_sub")
}

func TestVerifySubscriptionCredentials_CodexActive(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	h := newTestExecutor(mockStore)

	mockStore.EXPECT().GetSession(gomock.Any(), "ses_x").Return(subscriptionSession(), nil)
	mockStore.EXPECT().GetApp(gomock.Any(), "app_1").
		Return(appWithRuntime(types.CodeAgentRuntimeCodexCLI, types.CodeAgentCredentialTypeSubscription), nil)
	mockStore.EXPECT().GetEffectiveCodexSubscription(gomock.Any(), "usr_no_sub", "org_1").
		Return(&types.CodexSubscription{Status: "active"}, nil)

	assert.NoError(t, h.verifySubscriptionCredentials(context.Background(), &types.DesktopAgent{SessionID: "ses_x"}))
}

func TestVerifySubscriptionCredentials_ClaudeMissing(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	h := newTestExecutor(mockStore)

	session := subscriptionSession()
	mockStore.EXPECT().GetSession(gomock.Any(), "ses_x").Return(session, nil)
	mockStore.EXPECT().GetApp(gomock.Any(), "app_1").
		Return(appWithRuntime(types.CodeAgentRuntimeClaudeCode, types.CodeAgentCredentialTypeSubscription), nil)
	mockStore.EXPECT().GetSessionClaudeSubscription(gomock.Any(), session).Return(nil, store.ErrNotFound)

	err := h.verifySubscriptionCredentials(context.Background(), &types.DesktopAgent{SessionID: "ses_x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Claude subscription")
}

// API-key agents never touch subscriptions, so no lookup should happen.
func TestVerifySubscriptionCredentials_APIKeyRuntimeSkipped(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	h := newTestExecutor(mockStore)

	mockStore.EXPECT().GetSession(gomock.Any(), "ses_x").Return(subscriptionSession(), nil)
	mockStore.EXPECT().GetApp(gomock.Any(), "app_1").
		Return(appWithRuntime(types.CodeAgentRuntimeCodexCLI, types.CodeAgentCredentialTypeAPIKey), nil)

	assert.NoError(t, h.verifySubscriptionCredentials(context.Background(), &types.DesktopAgent{SessionID: "ses_x"}))
}

// The Codex/Claude login desktops are started precisely to obtain a
// subscription, and have no parent app — they must never be gated.
func TestVerifySubscriptionCredentials_LoginDesktopSkipped(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	h := newTestExecutor(mockStore)

	mockStore.EXPECT().GetSession(gomock.Any(), "ses_login").
		Return(&types.Session{ID: "ses_login", Owner: "usr_no_sub"}, nil)

	assert.NoError(t, h.verifySubscriptionCredentials(context.Background(), &types.DesktopAgent{SessionID: "ses_login"}))
}

// The browser has to decide which provider login to offer. Pattern-matching the
// prose would break the moment the wording changes, so the error carries the
// provider structurally.
func TestVerifySubscriptionCredentials_ErrorIdentifiesProvider(t *testing.T) {
	for _, tc := range []struct {
		runtime      types.CodeAgentRuntime
		wantProvider string
		wantLabel    string
	}{
		{types.CodeAgentRuntimeClaudeCode, types.SubscriptionProviderClaude, "Claude"},
		{types.CodeAgentRuntimeCodexCLI, types.SubscriptionProviderCodex, "ChatGPT"},
	} {
		err := missingSubscriptionError(tc.wantProvider, tc.wantLabel, &types.Session{
			Owner: "usr_1", OrganizationID: "org_1",
		})

		var missing *types.MissingSubscriptionError
		require.True(t, errors.As(err, &missing), "runtime %s", tc.runtime)
		assert.Equal(t, tc.wantProvider, missing.Provider)
		assert.Contains(t, err.Error(), tc.wantLabel)
		assert.Contains(t, err.Error(), "usr_1")
	}
}
