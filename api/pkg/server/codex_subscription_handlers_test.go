package server

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func validCodexCredentials() types.CodexAuthCredentials {
	return types.CodexAuthCredentials{
		AuthMode: "chatgpt",
		Tokens: types.CodexAuthTokens{
			IDToken: "id", AccessToken: "access", RefreshToken: "refresh", AccountID: "account",
		},
		LastRefresh: time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC),
	}
}

func TestValidateCodexCredentials(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		require.NoError(t, validateCodexCredentials(validCodexCredentials()))
	})
	t.Run("rejects API auth", func(t *testing.T) {
		credentials := validCodexCredentials()
		credentials.AuthMode = "apikey"
		require.EqualError(t, validateCodexCredentials(credentials), "auth_mode must be chatgpt")
	})
	t.Run("rejects incomplete tokens", func(t *testing.T) {
		credentials := validCodexCredentials()
		credentials.Tokens.RefreshToken = ""
		require.EqualError(t, validateCodexCredentials(credentials), "id_token, access_token, refresh_token, and account_id are required")
	})
}

func TestDecodeCodexCredentialsRejectsUnknownFields(t *testing.T) {
	_, err := decodeCodexCredentials(strings.NewReader(`{"auth_mode":"chatgpt","unexpected":true}`))
	require.ErrorContains(t, err, "unknown field")
}

func TestNormalizeCodexSubscriptionCredentialsRemovesAPIKey(t *testing.T) {
	credentials := validCodexCredentials()
	apiKey := "must-not-override-chatgpt-tokens"
	credentials.OpenAIAPIKey = &apiKey

	normalizeCodexSubscriptionCredentials(&credentials)

	require.Nil(t, credentials.OpenAIAPIKey)
}

func TestCodexDeviceAuthOutputPatterns(t *testing.T) {
	output := "\x1b[94mhttps://auth.openai.com/codex/device\x1b[0m code \x1b[94m2E5J-JKA6Q\x1b[0m"
	clean := ansiEscapePattern.ReplaceAllString(output, "")
	require.Contains(t, clean, codexDeviceURL)
	require.Equal(t, "2E5J-JKA6Q", codexDeviceCodePattern.FindString(clean))
}

func TestNewCodexLoginAgentUsesHeadlessSandbox(t *testing.T) {
	agent := newCodexLoginAgent("org_1", "ses_1", "usr_1")

	require.Equal(t, "headless", agent.DesktopType)
	require.Equal(t, 1, agent.VCPUs)
	require.Equal(t, 2048, agent.MemoryMB)
	require.Zero(t, agent.DisplayWidth)
	require.Zero(t, agent.DisplayHeight)
	require.Equal(t, []string{"HELIX_SERVER_SETUP=1"}, agent.Env)
}

func TestCreateCodexSubscriptionFromCredentialsEnablesOrgHarness(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	mockStore.EXPECT().CreateCodexSubscription(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, subscription *types.CodexSubscription) (*types.CodexSubscription, error) {
			subscription.ID = "codex_sub_1"
			return subscription, nil
		},
	)
	mockStore.EXPECT().UpsertOrgCodeAgentHarnesses(
		gomock.Any(), "org_1", "user_1", gomock.Any(),
	).DoAndReturn(func(_ context.Context, _, _ string, updates []types.OrgCodeAgentHarnessUpdate) ([]*types.OrgCodeAgentHarness, error) {
		require.Len(t, updates, 1)
		assert.Equal(t, types.CodeAgentRuntimeCodexCLI, updates[0].Runtime)
		assert.True(t, updates[0].Enabled)
		require.NotNil(t, updates[0].SubscriptionEnabled)
		assert.True(t, *updates[0].SubscriptionEnabled)
		return nil, nil
	})

	subscription, err := server.createCodexSubscriptionFromCredentials(
		context.Background(), "user_1", "org_1", validCodexCredentials(),
	)
	require.NoError(t, err)
	assert.Equal(t, "codex_sub_1", subscription.ID)
}

func TestCreateCodexSubscriptionFromCredentialsRollsBackOnHarnessFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	mockStore.EXPECT().CreateCodexSubscription(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, subscription *types.CodexSubscription) (*types.CodexSubscription, error) {
			subscription.ID = "codex_sub_1"
			return subscription, nil
		},
	)
	mockStore.EXPECT().UpsertOrgCodeAgentHarnesses(
		gomock.Any(), "org_1", "user_1", gomock.Any(),
	).Return(nil, errors.New("database unavailable"))
	mockStore.EXPECT().DeleteCodexSubscription(gomock.Any(), "codex_sub_1").Return(nil)

	_, err := server.createCodexSubscriptionFromCredentials(
		context.Background(), "user_1", "org_1", validCodexCredentials(),
	)
	require.ErrorContains(t, err, "enable subscription runtime")
}
