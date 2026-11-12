package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func adoTriggerConfig(secret string) *types.TriggerConfiguration {
	return &types.TriggerConfiguration{
		ID:    "trg-1",
		AppID: "app-1",
		Trigger: types.Trigger{AzureDevOps: &types.AzureDevOpsTrigger{
			Enabled:       true,
			WebhookSecret: secret,
		}},
	}
}

func TestWebhookTriggerRequestAuthorized(t *testing.T) {
	const secret = "s3cret"
	newReq := func(target string) *http.Request {
		return httptest.NewRequest(http.MethodPost, target, strings.NewReader("{}"))
	}

	assert.False(t, webhookTriggerRequestAuthorized(adoTriggerConfig(secret), newReq("/api/v1/webhooks/trg-1")), "no secret presented")
	assert.False(t, webhookTriggerRequestAuthorized(adoTriggerConfig(secret), newReq("/api/v1/webhooks/trg-1?token=wrong")), "wrong secret")
	assert.False(t, webhookTriggerRequestAuthorized(adoTriggerConfig(""), newReq("/api/v1/webhooks/trg-1?token=")), "trigger without a secret accepts nothing")
	assert.False(t, webhookTriggerRequestAuthorized(&types.TriggerConfiguration{ID: "cron"}, newReq("/api/v1/webhooks/cron?token=x")), "non-webhook trigger")

	assert.True(t, webhookTriggerRequestAuthorized(adoTriggerConfig(secret), newReq("/api/v1/webhooks/trg-1?token="+secret)))

	r := newReq("/api/v1/webhooks/trg-1")
	r.Header.Set(webhookTriggerSecretHeader, secret)
	assert.True(t, webhookTriggerRequestAuthorized(adoTriggerConfig(secret), r))

	r = newReq("/api/v1/webhooks/trg-1")
	r.SetBasicAuth("azure", secret)
	assert.True(t, webhookTriggerRequestAuthorized(adoTriggerConfig(secret), r))

	r = newReq("/api/v1/webhooks/trg-1")
	r.Header.Set("Authorization", "Bearer "+secret)
	assert.True(t, webhookTriggerRequestAuthorized(adoTriggerConfig(secret), r))
}

func TestWebhookTriggerHandler_RejectsUnauthenticatedDelivery(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetTriggerConfiguration(gomock.Any(), &store.GetTriggerConfigurationQuery{ID: "trg-1"}).
		Return(adoTriggerConfig("s3cret"), nil)

	// s.trigger is nil: reaching ProcessWebhook would panic the test.
	server := &HelixAPIServer{Store: mockStore}
	r := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/trg-1", strings.NewReader(`{"eventType":"git.pullrequest.created"}`))
	r = mux.SetURLVars(r, map[string]string{"id": "trg-1"})
	rec := httptest.NewRecorder()

	server.webhookTriggerHandler(rec, r)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestEnsureWebhookTriggerSecret(t *testing.T) {
	// Generates a secret without touching the caller's shared struct.
	shared := &types.AzureDevOpsTrigger{Enabled: true}
	trigger := types.Trigger{AzureDevOps: shared}
	changed, err := ensureWebhookTriggerSecret(&trigger, nil)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Len(t, trigger.AzureDevOps.WebhookSecret, 64)
	assert.Empty(t, shared.WebhookSecret)

	// Keeps the previous secret on edit.
	edited := types.Trigger{AzureDevOps: &types.AzureDevOpsTrigger{Enabled: true}}
	changed, err = ensureWebhookTriggerSecret(&edited, &trigger)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, trigger.AzureDevOps.WebhookSecret, edited.AzureDevOps.WebhookSecret)

	// Leaves non-webhook and disabled triggers alone.
	cron := types.Trigger{Cron: &types.CronTrigger{}}
	changed, err = ensureWebhookTriggerSecret(&cron, nil)
	require.NoError(t, err)
	assert.False(t, changed)
	disabled := types.Trigger{AzureDevOps: &types.AzureDevOpsTrigger{}}
	changed, err = ensureWebhookTriggerSecret(&disabled, nil)
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Empty(t, disabled.AzureDevOps.WebhookSecret)
}

func TestPopulateWebhookTriggerURL(t *testing.T) {
	server := &HelixAPIServer{Cfg: &config.ServerConfig{}}
	server.Cfg.WebServer.URL = "https://helix.example"
	triggerConfig := adoTriggerConfig("abc123")
	server.populateWebhookTriggerURL(triggerConfig)
	assert.Equal(t, "https://helix.example/api/v1/webhooks/trg-1?token=abc123", triggerConfig.WebhookURL)
}
