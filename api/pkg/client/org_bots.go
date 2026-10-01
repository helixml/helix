package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
)

// Org Bots API client: /orgs/{org}/bots and a bot's instances. Requests and
// responses are the org REST DTOs. activate/restart/chat and synchronous
// instance calls can take longer than the default request timeout; pass a
// ctx with a deadline for those.

func orgBotPath(orgID, botID string) string {
	return fmt.Sprintf("/orgs/%s/bots/%s", url.PathEscape(orgID), url.PathEscape(botID))
}

// ListOrgBots returns every bot in the organization.
func (c *HelixClient) ListOrgBots(ctx context.Context, orgID string) ([]orgapi.BotDTO, error) {
	var bots []orgapi.BotDTO
	if err := c.makeRequest(ctx, http.MethodGet, fmt.Sprintf("/orgs/%s/bots", url.PathEscape(orgID)), nil, &bots); err != nil {
		return nil, err
	}
	return bots, nil
}

// GetOrgBot returns one bot plus its runtime context. A missing bot returns
// an error wrapping ErrNotFound.
func (c *HelixClient) GetOrgBot(ctx context.Context, orgID, botID string) (*orgapi.BotDetailDTO, error) {
	var detail orgapi.BotDetailDTO
	if err := c.makeRequest(ctx, http.MethodGet, orgBotPath(orgID, botID), nil, &detail); err != nil {
		if strings.Contains(err.Error(), "status code 404") {
			return nil, fmt.Errorf("bot %s: %w", botID, ErrNotFound)
		}
		return nil, err
	}
	return &detail, nil
}

// CreateOrgBot creates a bot. Tools are merged with the default worker set.
func (c *HelixClient) CreateOrgBot(ctx context.Context, orgID string, req *orgapi.CreateBotRequest) (*orgapi.CreateBotResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	var resp orgapi.CreateBotResponse
	if err := c.makeRequest(ctx, http.MethodPost, fmt.Sprintf("/orgs/%s/bots", url.PathEscape(orgID)), bytes.NewReader(body), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// UpdateOrgBot patches a bot: nil fields are left unchanged, Tools replaces
// the whole list.
func (c *HelixClient) UpdateOrgBot(ctx context.Context, orgID, botID string, req *orgapi.UpdateBotRequest) (*orgapi.BotDTO, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	var bot orgapi.BotDTO
	if err := c.makeRequest(ctx, http.MethodPatch, orgBotPath(orgID, botID), bytes.NewReader(body), &bot); err != nil {
		return nil, err
	}
	return &bot, nil
}

// DeleteOrgBot deletes a bot, its instances, app and reporting lines. The
// bot's project is archived; its repositories are kept.
func (c *HelixClient) DeleteOrgBot(ctx context.Context, orgID, botID string) error {
	return c.makeRequest(ctx, http.MethodDelete, orgBotPath(orgID, botID), nil, nil)
}

// ActivateOrgBot starts the bot's sandbox (202). The project is ensured
// synchronously; the session starts asynchronously.
func (c *HelixClient) ActivateOrgBot(ctx context.Context, orgID, botID string) (*orgapi.BotActivateDTO, error) {
	var resp orgapi.BotActivateDTO
	if err := c.makeRequest(ctx, http.MethodPost, orgBotPath(orgID, botID)+"/activate", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// StopOrgBot stops the bot's sandbox.
func (c *HelixClient) StopOrgBot(ctx context.Context, orgID, botID string) error {
	return c.makeRequest(ctx, http.MethodPost, orgBotPath(orgID, botID)+"/stop", nil, nil)
}

// RestartOrgBot restarts the bot with a fresh session.
func (c *HelixClient) RestartOrgBot(ctx context.Context, orgID, botID string) (*orgapi.BotActivateDTO, error) {
	var resp orgapi.BotActivateDTO
	if err := c.makeRequest(ctx, http.MethodPost, orgBotPath(orgID, botID)+"/restart", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ApplyOrgBotConfig recreates the bot's container with its current config,
// keeping the session.
func (c *HelixClient) ApplyOrgBotConfig(ctx context.Context, orgID, botID string) error {
	return c.makeRequest(ctx, http.MethodPost, orgBotPath(orgID, botID)+"/apply-config", nil, nil)
}

// EnsureOrgBotChat provisions the bot's project and chat app without
// starting a sandbox, and returns their ids.
func (c *HelixClient) EnsureOrgBotChat(ctx context.Context, orgID, botID string) (*orgapi.BotChatDTO, error) {
	var resp orgapi.BotChatDTO
	if err := c.makeRequest(ctx, http.MethodPost, orgBotPath(orgID, botID)+"/chat", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ListOrgBotInstances lists a bot's instances, newest first.
func (c *HelixClient) ListOrgBotInstances(ctx context.Context, orgID, botID string) ([]orgapi.BotInstanceDTO, error) {
	var instances []orgapi.BotInstanceDTO
	if err := c.makeRequest(ctx, http.MethodGet, orgBotPath(orgID, botID)+"/instances", nil, &instances); err != nil {
		return nil, err
	}
	return instances, nil
}

// CreateOrgBotInstance starts a new instance of a bot. The sandbox starts
// asynchronously; the response carries the new session id.
func (c *HelixClient) CreateOrgBotInstance(ctx context.Context, orgID, botID string, req *orgapi.CreateBotInstanceRequest) (*orgapi.BotInstanceDTO, error) {
	if req == nil {
		req = &orgapi.CreateBotInstanceRequest{} // the handler requires a JSON body
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	var instance orgapi.BotInstanceDTO
	if err := c.makeRequest(ctx, http.MethodPost, orgBotPath(orgID, botID)+"/instances", bytes.NewReader(body), &instance); err != nil {
		return nil, err
	}
	return &instance, nil
}

// DeleteOrgBotInstance deletes an instance: its sandbox, workspace and session.
func (c *HelixClient) DeleteOrgBotInstance(ctx context.Context, orgID, botID, sessionID string) error {
	return c.makeRequest(ctx, http.MethodDelete, orgBotPath(orgID, botID)+"/instances/"+url.PathEscape(sessionID), nil, nil)
}
