package server

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// envValue returns the value of "KEY" from a slice of "KEY=VALUE" entries, and
// whether it was present.
func envValue(env []string, key string) (string, bool) {
	prefix := key + "="
	for _, e := range env {
		if len(e) >= len(prefix) && e[:len(prefix)] == prefix {
			return e[len(prefix):], true
		}
	}
	return "", false
}

func newSubscriptionModelTestServer(t *testing.T) (*HelixAPIServer, *store.MockStore) {
	t.Helper()
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}
	return server, mockStore
}

func claudeSubscriptionApp(model string) *types.App {
	return &types.App{
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{{
					CodeAgentRuntime:        types.CodeAgentRuntimeClaudeCode,
					CodeAgentCredentialType: types.CodeAgentCredentialTypeSubscription,
					ClaudeSubscriptionModel: model,
				}},
			},
		},
	}
}

func TestSubscriptionEnvForSession_DefaultsToOpus(t *testing.T) {
	server, mockStore := newSubscriptionModelTestServer(t)
	session := &types.Session{ID: "ses_1", ParentApp: "app_1", Owner: "user_1"}

	// Empty ClaudeSubscriptionModel => default to Opus.
	mockStore.EXPECT().GetApp(gomock.Any(), "app_1").Return(claudeSubscriptionApp(""), nil)
	mockStore.EXPECT().
		GetEffectiveClaudeSubscription(gomock.Any(), "user_1", "").
		Return(&types.ClaudeSubscription{Status: "active", CredentialType: "oauth"}, nil)

	env := server.subscriptionEnvForSession(context.Background(), session)

	model, ok := envValue(env, "ANTHROPIC_MODEL")
	require.True(t, ok, "ANTHROPIC_MODEL should be set: %v", env)
	require.Equal(t, "claude-opus-4-6", model)
}

func TestSubscriptionEnvForSession_HonoursOverride(t *testing.T) {
	server, mockStore := newSubscriptionModelTestServer(t)
	session := &types.Session{ID: "ses_2", ParentApp: "app_2", Owner: "user_2"}

	mockStore.EXPECT().GetApp(gomock.Any(), "app_2").Return(claudeSubscriptionApp("claude-haiku-4-5-latest"), nil)
	mockStore.EXPECT().
		GetEffectiveClaudeSubscription(gomock.Any(), "user_2", "").
		Return(&types.ClaudeSubscription{Status: "active", CredentialType: "oauth"}, nil)

	env := server.subscriptionEnvForSession(context.Background(), session)

	model, ok := envValue(env, "ANTHROPIC_MODEL")
	require.True(t, ok, "ANTHROPIC_MODEL should be set: %v", env)
	require.Equal(t, "claude-haiku-4-5-latest", model)
}

func TestSubscriptionEnvForSession_ApiKeyModeNoModel(t *testing.T) {
	server, mockStore := newSubscriptionModelTestServer(t)
	session := &types.Session{ID: "ses_3", ParentApp: "app_3", Owner: "user_3"}

	// api_key mode: the function returns nil before any subscription lookup,
	// so no ANTHROPIC_MODEL is injected.
	apiKeyApp := claudeSubscriptionApp("")
	apiKeyApp.Config.Helix.Assistants[0].CodeAgentCredentialType = types.CodeAgentCredentialTypeAPIKey
	mockStore.EXPECT().GetApp(gomock.Any(), "app_3").Return(apiKeyApp, nil)

	env := server.subscriptionEnvForSession(context.Background(), session)

	_, ok := envValue(env, "ANTHROPIC_MODEL")
	require.False(t, ok, "ANTHROPIC_MODEL must not be set in api_key mode: %v", env)
}
