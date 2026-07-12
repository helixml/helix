package server

import (
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
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
