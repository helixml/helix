package server

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/rs/zerolog/log"
)

// webhookTriggerSecretHeader carries a trigger's webhook secret. The secret is
// also accepted as the HTTP basic-auth password (Azure DevOps service hooks
// support basic auth natively) or as the "token" query parameter, which is
// what the advertised WebhookURL embeds.
const webhookTriggerSecretHeader = "X-Helix-Webhook-Secret"

var errWebhookTriggerUnauthorized = errors.New("invalid or missing webhook secret")

// webhookTriggerHandler - webhook trigger handler for Azure DevOps, GitHub, etc.
// Body can be dynamic and configuration will be looked up through the trigger configuration ID.
// The request must carry the trigger's webhook secret; the payload runs as the
// app owner, so an unauthenticated delivery would let anyone drive the agent.
func (s *HelixAPIServer) webhookTriggerHandler(w http.ResponseWriter, r *http.Request) {
	id := getID(r)

	triggerConfig, err := s.Store.GetTriggerConfiguration(r.Context(), &store.GetTriggerConfigurationQuery{
		ID: id,
	})
	if err != nil {
		writeErrResponse(w, err, http.StatusNotFound)
		return
	}

	if !webhookTriggerRequestAuthorized(triggerConfig, r) {
		log.Warn().Str("trigger_config_id", triggerConfig.ID).Msg("rejected webhook trigger delivery without a valid secret")
		writeErrResponse(w, errWebhookTriggerUnauthorized, http.StatusUnauthorized)
		return
	}

	defer r.Body.Close()

	bts, err := io.ReadAll(r.Body)
	if err != nil {
		writeErrResponse(w, err, http.StatusInternalServerError)
		return
	}

	log.Debug().
		Str("trigger_config_id", triggerConfig.ID).
		Str("trigger_config_app_id", triggerConfig.AppID).
		Msgf("Received webhook trigger for trigger configuration %s", id)

	err = s.trigger.ProcessWebhook(r.Context(), triggerConfig, bts)
	if err != nil {
		writeErrResponse(w, err, http.StatusInternalServerError)
		return
	}

	writeResponse(w, nil, http.StatusOK)
}

// webhookTriggerSecret returns the configured webhook secret, or "" when the
// trigger type does not accept webhook deliveries or has no secret yet.
func webhookTriggerSecret(triggerConfig *types.TriggerConfiguration) string {
	if triggerConfig.Trigger.AzureDevOps != nil {
		return triggerConfig.Trigger.AzureDevOps.WebhookSecret
	}
	return ""
}

// webhookTriggerRequestAuthorized reports whether r presents the trigger's
// webhook secret. A trigger without a secret accepts nothing.
func webhookTriggerRequestAuthorized(triggerConfig *types.TriggerConfiguration, r *http.Request) bool {
	secret := webhookTriggerSecret(triggerConfig)
	if secret == "" {
		return false
	}
	candidates := []string{r.Header.Get(webhookTriggerSecretHeader), r.URL.Query().Get("token")}
	if _, password, ok := r.BasicAuth(); ok {
		candidates = append(candidates, password)
	}
	if bearer, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		candidates = append(candidates, bearer)
	}
	for _, candidate := range candidates {
		if candidate != "" && hmac.Equal([]byte(candidate), []byte(secret)) {
			return true
		}
	}
	return false
}

func generateWebhookTriggerSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate webhook secret: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// ensureWebhookTriggerSecret gives an enabled Azure DevOps trigger a webhook
// secret, keeping previous's secret when the incoming trigger omits it so an
// edit does not invalidate the URL already configured upstream. It reports
// whether trigger was changed.
func ensureWebhookTriggerSecret(trigger *types.Trigger, previous *types.Trigger) (bool, error) {
	if trigger.AzureDevOps == nil || !trigger.AzureDevOps.Enabled || trigger.AzureDevOps.WebhookSecret != "" {
		return false, nil
	}
	// Copy before writing: the caller's AzureDevOps may be shared with an app
	// config, which must not carry the secret.
	azureDevOps := *trigger.AzureDevOps
	trigger.AzureDevOps = &azureDevOps
	if previous != nil && previous.AzureDevOps != nil && previous.AzureDevOps.WebhookSecret != "" {
		trigger.AzureDevOps.WebhookSecret = previous.AzureDevOps.WebhookSecret
		return true, nil
	}
	secret, err := generateWebhookTriggerSecret()
	if err != nil {
		return false, err
	}
	trigger.AzureDevOps.WebhookSecret = secret
	return true, nil
}

// populateWebhookTriggerURL fills WebhookURL for webhook-driven triggers. The
// URL embeds the secret so it can be pasted into the upstream service as is.
func (s *HelixAPIServer) populateWebhookTriggerURL(triggerConfig *types.TriggerConfiguration) {
	if triggerConfig.Trigger.AzureDevOps == nil || !triggerConfig.Trigger.AzureDevOps.Enabled {
		return
	}
	webhookURL := fmt.Sprintf("%s/api/v1/webhooks/%s", s.Cfg.WebServer.URL, triggerConfig.ID)
	if secret := webhookTriggerSecret(triggerConfig); secret != "" {
		webhookURL += "?token=" + url.QueryEscape(secret)
	}
	triggerConfig.WebhookURL = webhookURL
}

// backfillWebhookTriggerSecret persists a secret on an enabled webhook trigger
// saved before secrets were required, so its owner can see a working URL.
func (s *HelixAPIServer) backfillWebhookTriggerSecret(ctx context.Context, triggerConfig *types.TriggerConfiguration) *types.TriggerConfiguration {
	changed, err := ensureWebhookTriggerSecret(&triggerConfig.Trigger, nil)
	if err != nil || !changed {
		return triggerConfig
	}
	updated, err := s.Store.UpdateTriggerConfiguration(ctx, triggerConfig)
	if err != nil {
		log.Warn().Err(err).Str("trigger_config_id", triggerConfig.ID).Msg("failed to store webhook trigger secret")
		triggerConfig.Trigger.AzureDevOps.WebhookSecret = ""
		return triggerConfig
	}
	return updated
}
