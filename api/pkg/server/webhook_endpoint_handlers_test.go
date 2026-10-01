package server

import (
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	standardwebhooks "github.com/standard-webhooks/standard-webhooks/libraries/go"
	"github.com/stretchr/testify/require"
)

func TestGenerateWebhookSecretIsStandardCompatible(t *testing.T) {
	secret, err := generateWebhookSecret()
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(secret, "whsec_"))
	_, err = standardwebhooks.NewWebhook(secret)
	require.NoError(t, err)
}

func TestNormalizeWebhookEvents(t *testing.T) {
	require.Equal(t, types.SupportedWebhookEvents, normalizeWebhookEvents(nil))
	require.Equal(t,
		[]string{types.WebhookEventArtifactPublished, types.WebhookEventSpecTaskStatusChanged},
		normalizeWebhookEvents([]string{
			types.WebhookEventSpecTaskStatusChanged,
			types.WebhookEventArtifactPublished,
			types.WebhookEventSpecTaskStatusChanged,
		}),
	)
	require.Error(t, validateWebhookEvents([]string{"unknown.event"}))
}
