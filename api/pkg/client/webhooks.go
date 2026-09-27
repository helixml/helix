package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/helixml/helix/api/pkg/types"
)

// Organization webhook endpoints (Standard Webhooks). Every call needs the
// organization owner.

// WebhookEndpointRequest is the body of POST
// /organizations/{id}/webhook-endpoints. Empty Events subscribes to all events.
type WebhookEndpointRequest struct {
	URL         string   `json:"url"`
	Description string   `json:"description,omitempty"`
	ProjectID   string   `json:"project_id,omitempty"`
	Events      []string `json:"events,omitempty"`
	Enabled     *bool    `json:"enabled,omitempty"`
}

// WebhookEndpointWithSecret is the create response: the endpoint plus its
// signing secret, which is returned only once.
type WebhookEndpointWithSecret struct {
	Endpoint *types.WebhookEndpoint `json:"endpoint"`
	Secret   string                 `json:"secret"`
}

// WebhookDeliveryView is one delivery with the type of the event it carries.
type WebhookDeliveryView struct {
	*types.WebhookDelivery
	EventType string `json:"event_type"`
}

func webhookEndpointsPath(orgID string) string {
	return fmt.Sprintf("/organizations/%s/webhook-endpoints", url.PathEscape(orgID))
}

// ListWebhookEndpoints returns the organization's webhook endpoints.
func (c *HelixClient) ListWebhookEndpoints(ctx context.Context, orgID string) ([]*types.WebhookEndpoint, error) {
	var endpoints []*types.WebhookEndpoint
	if err := c.makeRequest(ctx, http.MethodGet, webhookEndpointsPath(orgID), nil, &endpoints); err != nil {
		return nil, err
	}
	return endpoints, nil
}

// CreateWebhookEndpoint creates an endpoint and returns its signing secret.
func (c *HelixClient) CreateWebhookEndpoint(ctx context.Context, orgID string, req *WebhookEndpointRequest) (*WebhookEndpointWithSecret, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	var resp WebhookEndpointWithSecret
	if err := c.makeRequest(ctx, http.MethodPost, webhookEndpointsPath(orgID), bytes.NewReader(body), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// DeleteWebhookEndpoint disables and removes an endpoint.
func (c *HelixClient) DeleteWebhookEndpoint(ctx context.Context, orgID, endpointID string) error {
	return c.makeRequest(ctx, http.MethodDelete, webhookEndpointsPath(orgID)+"/"+url.PathEscape(endpointID), nil, nil)
}

// ListWebhookDeliveries returns an endpoint's most recent deliveries (up to 50).
func (c *HelixClient) ListWebhookDeliveries(ctx context.Context, orgID, endpointID string) ([]*WebhookDeliveryView, error) {
	var deliveries []*WebhookDeliveryView
	if err := c.makeRequest(ctx, http.MethodGet, webhookEndpointsPath(orgID)+"/"+url.PathEscape(endpointID)+"/deliveries", nil, &deliveries); err != nil {
		return nil, err
	}
	return deliveries, nil
}

// ReplayWebhookDelivery re-queues one delivery.
func (c *HelixClient) ReplayWebhookDelivery(ctx context.Context, orgID, endpointID, deliveryID string) (*types.WebhookDelivery, error) {
	var delivery types.WebhookDelivery
	path := webhookEndpointsPath(orgID) + "/" + url.PathEscape(endpointID) + "/deliveries/" + url.PathEscape(deliveryID) + "/replay"
	if err := c.makeRequest(ctx, http.MethodPost, path, nil, &delivery); err != nil {
		return nil, err
	}
	return &delivery, nil
}
