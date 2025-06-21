package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

type AppFilter struct {
	OrganizationID string
}

func (c *HelixClient) ListApps(ctx context.Context, f *AppFilter) ([]*types.App, error) {
	var apps []*types.App

	path := "/apps"
	if f.OrganizationID != "" {
		path += "?organization_id=" + f.OrganizationID
	}

	err := c.makeRequest(ctx, http.MethodGet, path, nil, &apps)
	if err != nil {
		return nil, err
	}
	return apps, nil
}

func (c *HelixClient) GetApp(ctx context.Context, appID string) (*types.App, error) {
	var app types.App
	err := c.makeRequest(ctx, http.MethodGet, "/apps/"+appID, nil, &app)
	if err != nil {
		return nil, err
	}
	return &app, nil
}

func (c *HelixClient) GetAppAPIKeys(ctx context.Context, appID string) ([]*types.ApiKey, error) {
	var apiKeys []*types.ApiKey
	err := c.makeRequest(ctx, http.MethodGet, "/api_keys?types=app&app_id="+appID, nil, &apiKeys)
	if err != nil {
		return nil, err
	}
	return apiKeys, nil
}

func (c *HelixClient) CreateApp(ctx context.Context, app *types.App) (*types.App, error) {
	bts, err := json.Marshal(app)
	if err != nil {
		return nil, err
	}

	var createdApp types.App
	err = c.makeRequest(ctx, http.MethodPost, "/apps", bytes.NewBuffer(bts), &createdApp)
	if err != nil {
		return nil, err
	}
	return &createdApp, nil
}

func (c *HelixClient) UpdateApp(ctx context.Context, app *types.App) (*types.App, error) {
	bts, err := json.Marshal(app)
	if err != nil {
		return nil, err
	}

	var updatedApp types.App
	err = c.makeRequest(ctx, http.MethodPut, "/apps/"+app.ID, bytes.NewBuffer(bts), &updatedApp)
	if err != nil {
		return nil, err
	}

	return &updatedApp, nil
}

func (c *HelixClient) DeleteApp(ctx context.Context, appID string, deleteKnowledge bool) error {
	query := url.Values{}
	query.Add("knowledge", strconv.FormatBool(deleteKnowledge))

	url := "/apps/" + appID + "?" + query.Encode()

	err := c.makeRequest(ctx, http.MethodDelete, url, nil, nil)
	if err != nil {
		return err
	}

	return nil
}

// TODO: optimize this to not list all apps and instead use a server side filter
func (c *HelixClient) GetAppByName(ctx context.Context, name string) (*types.App, error) {
	log.Debug().Str("name", name).Msg("getting app by name")

	apps, err := c.ListApps(ctx, nil)
	if err != nil {
		log.Error().Err(err).Str("name", name).Msg("failed to list apps")
		return nil, err
	}

	log.Debug().Int("total_apps", len(apps)).Msg("searching through apps")
	for _, app := range apps {
		if app.Config.Helix.Name == name {
			log.Debug().Str("name", name).Str("id", app.ID).Msg("found matching app")
			return app, nil
		}
	}

	log.Debug().Str("name", name).Msg("app not found")
	return nil, fmt.Errorf("app with name %s not found", name)
}

func (c *HelixClient) RunAPIAction(ctx context.Context, appID string, action string, parameters map[string]interface{}) (*types.RunAPIActionResponse, error) {
	req := types.RunAPIActionRequest{
		Action:     action,
		Parameters: parameters,
	}

	bts, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	var resp types.RunAPIActionResponse
	err = c.makeRequest(ctx, http.MethodPost, fmt.Sprintf("/apps/%s/api-actions", appID), bytes.NewBuffer(bts), &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}
