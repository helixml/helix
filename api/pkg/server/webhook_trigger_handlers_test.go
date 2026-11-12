package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/trigger"
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
	// New trigger: a client-supplied secret is ignored and one is generated,
	// without touching the caller's shared struct.
	shared := &types.AzureDevOpsTrigger{Enabled: true, WebhookSecret: "chosen-by-client"}
	created := types.Trigger{AzureDevOps: shared}
	changed, err := ensureWebhookTriggerSecret(&created, nil)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Len(t, created.AzureDevOps.WebhookSecret, 64)
	assert.NotEqual(t, "chosen-by-client", created.AzureDevOps.WebhookSecret)
	assert.Equal(t, "chosen-by-client", shared.WebhookSecret, "shared app config struct untouched")
	secret := created.AzureDevOps.WebhookSecret

	// Edit: the previous secret wins over anything the client sends.
	edited := types.Trigger{AzureDevOps: &types.AzureDevOpsTrigger{Enabled: true, WebhookSecret: "attacker-chosen"}}
	changed, err = ensureWebhookTriggerSecret(&edited, &created)
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Equal(t, secret, edited.AzureDevOps.WebhookSecret)

	// Disable keeps the secret, and re-enabling keeps it too.
	disabled := types.Trigger{AzureDevOps: &types.AzureDevOpsTrigger{Enabled: false}}
	_, err = ensureWebhookTriggerSecret(&disabled, &edited)
	require.NoError(t, err)
	assert.Equal(t, secret, disabled.AzureDevOps.WebhookSecret)
	reenabled := types.Trigger{AzureDevOps: &types.AzureDevOpsTrigger{Enabled: true}}
	_, err = ensureWebhookTriggerSecret(&reenabled, &disabled)
	require.NoError(t, err)
	assert.Equal(t, secret, reenabled.AzureDevOps.WebhookSecret)

	// A disabled trigger with no history gets no secret; non-webhook triggers
	// are left alone.
	fresh := types.Trigger{AzureDevOps: &types.AzureDevOpsTrigger{WebhookSecret: "x"}}
	changed, err = ensureWebhookTriggerSecret(&fresh, nil)
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Empty(t, fresh.AzureDevOps.WebhookSecret)
	cron := types.Trigger{Cron: &types.CronTrigger{}}
	changed, err = ensureWebhookTriggerSecret(&cron, nil)
	require.NoError(t, err)
	assert.False(t, changed)
}

// A delivery with the right secret gets past the check and is handed to the
// trigger manager (which loads the app — the first thing ADO processing does).
func TestWebhookTriggerHandler_AuthorizedDeliveryReachesProcessWebhook(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetTriggerConfiguration(gomock.Any(), &store.GetTriggerConfigurationQuery{ID: "trg-1"}).
		Return(adoTriggerConfig("s3cret"), nil)
	mockStore.EXPECT().GetApp(gomock.Any(), "app-1").Return(nil, errors.New("stop here"))

	server := &HelixAPIServer{
		Store:   mockStore,
		trigger: trigger.NewTriggerManager(&config.ServerConfig{}, mockStore, nil, nil),
	}
	r := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/trg-1?token=s3cret", strings.NewReader(`{"eventType":"build.complete"}`))
	r = mux.SetURLVars(r, map[string]string{"id": "trg-1"})
	rec := httptest.NewRecorder()

	server.webhookTriggerHandler(rec, r)

	assert.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "stop here")
}

func TestWebhookTriggerRequestAuthorized_DisabledTriggerAcceptsNothing(t *testing.T) {
	triggerConfig := adoTriggerConfig("s3cret")
	triggerConfig.Trigger.AzureDevOps.Enabled = false
	r := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/trg-1?token=s3cret", nil)
	assert.False(t, webhookTriggerRequestAuthorized(triggerConfig, r))
}

func TestBackfillWebhookTriggerSecrets(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	legacy := &types.TriggerConfiguration{ID: "trg-legacy", Trigger: types.Trigger{AzureDevOps: &types.AzureDevOpsTrigger{Enabled: true}}}
	mockStore.EXPECT().ListTriggerConfigurations(gomock.Any(), &store.ListTriggerConfigurationsQuery{}).
		Return([]*types.TriggerConfiguration{
			legacy,
			adoTriggerConfig("already"),
			{ID: "trg-cron", Trigger: types.Trigger{Cron: &types.CronTrigger{}}},
		}, nil)
	mockStore.EXPECT().UpdateTriggerConfiguration(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, tc *types.TriggerConfiguration) (*types.TriggerConfiguration, error) {
			assert.Equal(t, "trg-legacy", tc.ID)
			assert.Len(t, tc.Trigger.AzureDevOps.WebhookSecret, 64)
			return tc, nil
		})

	(&HelixAPIServer{Store: mockStore}).backfillWebhookTriggerSecrets(context.Background())
}

func TestPopulateWebhookTriggerURL(t *testing.T) {
	server := &HelixAPIServer{Cfg: &config.ServerConfig{}}
	server.Cfg.WebServer.URL = "https://helix.example"
	triggerConfig := adoTriggerConfig("abc123")
	server.populateWebhookTriggerURL(triggerConfig)
	assert.Equal(t, "https://helix.example/api/v1/webhooks/trg-1?token=abc123", triggerConfig.WebhookURL)
}
