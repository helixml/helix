package server

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	helixcrypto "github.com/helixml/helix/api/pkg/crypto"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/webhooks"
	"github.com/rs/zerolog/log"
)

type webhookEndpointRequest struct {
	URL         string   `json:"url"`
	Description string   `json:"description,omitempty"`
	ProjectID   string   `json:"project_id,omitempty"`
	Events      []string `json:"events,omitempty"`
	Enabled     *bool    `json:"enabled,omitempty"`
}

type webhookEndpointSecretResponse struct {
	Endpoint *types.WebhookEndpoint `json:"endpoint"`
	Secret   string                 `json:"secret"`
}

type webhookDeliveryView struct {
	*types.WebhookDelivery
	EventType string `json:"event_type"`
}

func (s *HelixAPIServer) authorizeWebhookOwner(r *http.Request) (string, bool) {
	orgID, err := s.resolveOrgID(r.Context(), mux.Vars(r)["id"])
	if err != nil {
		return "", false
	}
	if _, err := s.authorizeOrgOwner(r.Context(), getRequestUser(r), orgID); err != nil {
		return "", false
	}
	return orgID, true
}

// createWebhookEndpoint godoc
// @Summary Create an organization webhook endpoint
// @Description Creates a Standard Webhooks endpoint. The signing secret is returned once.
// @Tags organizations
// @Param id path string true "Organization ID"
// @Param request body webhookEndpointRequest true "Webhook endpoint"
// @Success 201 {object} webhookEndpointSecretResponse
// @Router /api/v1/organizations/{id}/webhook-endpoints [post]
// @Security BearerAuth
func (s *HelixAPIServer) createWebhookEndpoint(w http.ResponseWriter, r *http.Request) {
	orgID, ok := s.authorizeWebhookOwner(r)
	if !ok {
		http.Error(w, "organization owner access required", http.StatusForbidden)
		return
	}
	var input webhookEndpointRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := s.validateWebhookEndpointInput(r, orgID, &input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	existing, err := s.Store.ListWebhookEndpoints(r.Context(), orgID)
	if err != nil {
		http.Error(w, "failed to inspect webhook endpoints", http.StatusInternalServerError)
		return
	}
	activeCount := 0
	for _, endpoint := range existing {
		if endpoint.Status == types.WebhookEndpointStatusActive {
			activeCount++
		}
	}
	if activeCount >= 25 {
		http.Error(w, "organization webhook endpoint limit reached", http.StatusConflict)
		return
	}
	secret, err := generateWebhookSecret()
	if err != nil {
		http.Error(w, "failed to generate signing secret", http.StatusInternalServerError)
		return
	}
	key, err := s.getEncryptionKey()
	if err != nil {
		http.Error(w, "failed to load encryption key", http.StatusInternalServerError)
		return
	}
	encrypted, err := helixcrypto.EncryptAES256GCM([]byte(secret), key)
	if err != nil {
		http.Error(w, "failed to encrypt signing secret", http.StatusInternalServerError)
		return
	}
	now := time.Now().UTC()
	user := getRequestUser(r)
	endpoint := &types.WebhookEndpoint{
		ID: "whep_" + uuid.NewString(), OrganizationID: orgID, ProjectID: input.ProjectID,
		URL: input.URL, Description: input.Description, Events: normalizeWebhookEvents(input.Events),
		Status: types.WebhookEndpointStatusActive, SecretEncrypted: encrypted,
		SecretPreview: secret[len(secret)-4:], CreatedBy: user.ID, UpdatedBy: user.ID,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.Store.CreateWebhookEndpoint(r.Context(), endpoint); err != nil {
		log.Error().Err(err).Str("organization_id", orgID).Msg("failed to create webhook endpoint")
		http.Error(w, "failed to create webhook endpoint", http.StatusInternalServerError)
		return
	}
	writeResponse(w, webhookEndpointSecretResponse{Endpoint: endpoint, Secret: secret}, http.StatusCreated)
}

// listWebhookEndpoints godoc
// @Summary List organization webhook endpoints
// @Tags organizations
// @Param id path string true "Organization ID"
// @Success 200 {array} types.WebhookEndpoint
// @Router /api/v1/organizations/{id}/webhook-endpoints [get]
// @Security BearerAuth
func (s *HelixAPIServer) listWebhookEndpoints(w http.ResponseWriter, r *http.Request) {
	orgID, ok := s.authorizeWebhookOwner(r)
	if !ok {
		http.Error(w, "organization owner access required", http.StatusForbidden)
		return
	}
	endpoints, err := s.Store.ListWebhookEndpoints(r.Context(), orgID)
	if err != nil {
		http.Error(w, "failed to list webhook endpoints", http.StatusInternalServerError)
		return
	}
	writeResponse(w, endpoints, http.StatusOK)
}

// updateWebhookEndpoint godoc
// @Summary Update an organization webhook endpoint
// @Tags organizations
// @Param id path string true "Organization ID"
// @Param endpoint_id path string true "Webhook endpoint ID"
// @Param request body webhookEndpointRequest true "Webhook endpoint"
// @Success 200 {object} types.WebhookEndpoint
// @Router /api/v1/organizations/{id}/webhook-endpoints/{endpoint_id} [put]
// @Security BearerAuth
func (s *HelixAPIServer) updateWebhookEndpoint(w http.ResponseWriter, r *http.Request) {
	orgID, ok := s.authorizeWebhookOwner(r)
	if !ok {
		http.Error(w, "organization owner access required", http.StatusForbidden)
		return
	}
	endpoint, err := s.Store.GetWebhookEndpoint(r.Context(), orgID, mux.Vars(r)["endpoint_id"])
	if err != nil {
		http.Error(w, "webhook endpoint not found", http.StatusNotFound)
		return
	}
	var input webhookEndpointRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := s.validateWebhookEndpointInput(r, orgID, &input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	endpoint.URL = input.URL
	endpoint.Description = input.Description
	endpoint.ProjectID = input.ProjectID
	endpoint.Events = normalizeWebhookEvents(input.Events)
	if input.Enabled != nil {
		if *input.Enabled {
			endpoint.Status = types.WebhookEndpointStatusActive
			endpoint.DisabledReason = ""
		} else {
			endpoint.Status = types.WebhookEndpointStatusDisabled
			endpoint.DisabledReason = "disabled by organization owner"
		}
	}
	endpoint.UpdatedBy = getRequestUser(r).ID
	if err := s.Store.UpdateWebhookEndpointConfig(r.Context(), endpoint); err != nil {
		http.Error(w, "failed to update webhook endpoint", http.StatusInternalServerError)
		return
	}
	writeResponse(w, endpoint, http.StatusOK)
}

// deleteWebhookEndpoint godoc
// @Summary Disable an organization webhook endpoint
// @Tags organizations
// @Param id path string true "Organization ID"
// @Param endpoint_id path string true "Webhook endpoint ID"
// @Success 204
// @Router /api/v1/organizations/{id}/webhook-endpoints/{endpoint_id} [delete]
// @Security BearerAuth
func (s *HelixAPIServer) deleteWebhookEndpoint(w http.ResponseWriter, r *http.Request) {
	orgID, ok := s.authorizeWebhookOwner(r)
	if !ok {
		http.Error(w, "organization owner access required", http.StatusForbidden)
		return
	}
	endpoint, err := s.Store.GetWebhookEndpoint(r.Context(), orgID, mux.Vars(r)["endpoint_id"])
	if err != nil {
		http.Error(w, "webhook endpoint not found", http.StatusNotFound)
		return
	}
	if err := s.Store.DisableWebhookEndpoint(r.Context(), orgID, endpoint.ID, "deleted by organization owner", getRequestUser(r).ID); err != nil {
		http.Error(w, "failed to disable webhook endpoint", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// rotateWebhookEndpointSecret godoc
// @Summary Rotate a webhook signing secret
// @Description Returns the new secret once. Helix signs with both keys for a 24-hour overlap.
// @Tags organizations
// @Param id path string true "Organization ID"
// @Param endpoint_id path string true "Webhook endpoint ID"
// @Success 200 {object} webhookEndpointSecretResponse
// @Router /api/v1/organizations/{id}/webhook-endpoints/{endpoint_id}/rotate-secret [post]
// @Security BearerAuth
func (s *HelixAPIServer) rotateWebhookEndpointSecret(w http.ResponseWriter, r *http.Request) {
	orgID, ok := s.authorizeWebhookOwner(r)
	if !ok {
		http.Error(w, "organization owner access required", http.StatusForbidden)
		return
	}
	endpoint, err := s.Store.GetWebhookEndpoint(r.Context(), orgID, mux.Vars(r)["endpoint_id"])
	if err != nil {
		http.Error(w, "webhook endpoint not found", http.StatusNotFound)
		return
	}
	secret, err := generateWebhookSecret()
	if err != nil {
		http.Error(w, "failed to generate signing secret", http.StatusInternalServerError)
		return
	}
	key, err := s.getEncryptionKey()
	if err != nil {
		http.Error(w, "failed to load encryption key", http.StatusInternalServerError)
		return
	}
	encrypted, err := helixcrypto.EncryptAES256GCM([]byte(secret), key)
	if err != nil {
		http.Error(w, "failed to encrypt signing secret", http.StatusInternalServerError)
		return
	}
	expires := time.Now().UTC().Add(24 * time.Hour)
	endpoint.PreviousSecretEncrypted = endpoint.SecretEncrypted
	endpoint.PreviousSecretExpiresAt = &expires
	endpoint.SecretEncrypted = encrypted
	endpoint.SecretPreview = secret[len(secret)-4:]
	endpoint.UpdatedBy = getRequestUser(r).ID
	if err := s.Store.RotateWebhookEndpointSecret(r.Context(), endpoint); err != nil {
		http.Error(w, "failed to rotate webhook signing secret", http.StatusInternalServerError)
		return
	}
	writeResponse(w, webhookEndpointSecretResponse{Endpoint: endpoint, Secret: secret}, http.StatusOK)
}

// listWebhookDeliveries godoc
// @Summary List recent webhook deliveries
// @Tags organizations
// @Param id path string true "Organization ID"
// @Param endpoint_id path string true "Webhook endpoint ID"
// @Success 200 {array} webhookDeliveryView
// @Router /api/v1/organizations/{id}/webhook-endpoints/{endpoint_id}/deliveries [get]
// @Security BearerAuth
func (s *HelixAPIServer) listWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	orgID, ok := s.authorizeWebhookOwner(r)
	if !ok {
		http.Error(w, "organization owner access required", http.StatusForbidden)
		return
	}
	endpointID := mux.Vars(r)["endpoint_id"]
	if _, err := s.Store.GetWebhookEndpoint(r.Context(), orgID, endpointID); err != nil {
		http.Error(w, "webhook endpoint not found", http.StatusNotFound)
		return
	}
	deliveries, err := s.Store.ListWebhookDeliveries(r.Context(), endpointID, 50)
	if err != nil {
		http.Error(w, "failed to list webhook deliveries", http.StatusInternalServerError)
		return
	}
	views := make([]webhookDeliveryView, 0, len(deliveries))
	for _, delivery := range deliveries {
		view := webhookDeliveryView{WebhookDelivery: delivery}
		if event, eventErr := s.Store.GetWebhookEvent(r.Context(), delivery.EventID); eventErr == nil {
			view.EventType = event.Type
		}
		views = append(views, view)
	}
	writeResponse(w, views, http.StatusOK)
}

// replayWebhookDelivery godoc
// @Summary Replay a webhook delivery
// @Tags organizations
// @Param id path string true "Organization ID"
// @Param endpoint_id path string true "Webhook endpoint ID"
// @Param delivery_id path string true "Webhook delivery ID"
// @Success 202 {object} types.WebhookDelivery
// @Router /api/v1/organizations/{id}/webhook-endpoints/{endpoint_id}/deliveries/{delivery_id}/replay [post]
// @Security BearerAuth
func (s *HelixAPIServer) replayWebhookDelivery(w http.ResponseWriter, r *http.Request) {
	orgID, ok := s.authorizeWebhookOwner(r)
	if !ok {
		http.Error(w, "organization owner access required", http.StatusForbidden)
		return
	}
	endpointID := mux.Vars(r)["endpoint_id"]
	if _, err := s.Store.GetWebhookEndpoint(r.Context(), orgID, endpointID); err != nil {
		http.Error(w, "webhook endpoint not found", http.StatusNotFound)
		return
	}
	deliveryID := mux.Vars(r)["delivery_id"]
	if err := s.Store.ReplayWebhookDelivery(r.Context(), endpointID, deliveryID, time.Now().UTC()); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "delivery not found or currently processing", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to replay webhook delivery", http.StatusInternalServerError)
		return
	}
	delivery, _ := s.Store.GetWebhookDelivery(r.Context(), endpointID, deliveryID)
	writeResponse(w, delivery, http.StatusAccepted)
}

func (s *HelixAPIServer) validateWebhookEndpointInput(r *http.Request, orgID string, input *webhookEndpointRequest) error {
	input.URL = strings.TrimSpace(input.URL)
	input.Description = strings.TrimSpace(input.Description)
	if len(input.URL) > 2048 {
		return errors.New("webhook URL is too long")
	}
	if len(input.Description) > 1024 {
		return errors.New("webhook description is too long")
	}
	if err := webhooks.ValidateEndpointURL(input.URL, s.Cfg.Webhooks.AllowPrivateEndpoints); err != nil {
		return err
	}
	if input.ProjectID != "" {
		project, err := s.Store.GetProject(r.Context(), input.ProjectID)
		if err != nil || project.OrganizationID != orgID {
			return errors.New("project does not belong to the organization")
		}
	}
	return validateWebhookEvents(input.Events)
}

func validateWebhookEvents(events []string) error {
	supported := make(map[string]struct{}, len(types.SupportedWebhookEvents)+1)
	supported["*"] = struct{}{}
	for _, event := range types.SupportedWebhookEvents {
		supported[event] = struct{}{}
	}
	for _, event := range events {
		if _, ok := supported[event]; !ok {
			return errors.New("unsupported webhook event: " + event)
		}
	}
	return nil
}

func normalizeWebhookEvents(events []string) []string {
	if len(events) == 0 {
		return append([]string(nil), types.SupportedWebhookEvents...)
	}
	seen := make(map[string]struct{}, len(events))
	result := make([]string, 0, len(events))
	for _, event := range events {
		if _, ok := seen[event]; ok {
			continue
		}
		seen[event] = struct{}{}
		result = append(result, event)
	}
	sort.Strings(result)
	return result
}

func generateWebhookSecret() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "whsec_" + base64.StdEncoding.EncodeToString(raw), nil
}
