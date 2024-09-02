package client

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	"github.com/helixml/helix/api/pkg/types"
)

type AppFilter struct {
}

func (c *HelixClient) ListApps(f *AppFilter) ([]*types.App, error) {
	var apps []*types.App
	err := c.makeRequest(http.MethodGet, "/apps", nil, &apps)
	if err != nil {
		return nil, err
	}
	return apps, nil
}

func (c *HelixClient) GetApp(appID string) (*types.App, error) {
	var app types.App
	err := c.makeRequest(http.MethodGet, "/apps/"+appID, nil, &app)
	if err != nil {
		return nil, err
	}
	return &app, nil
}

func (c *HelixClient) CreateApp(app *types.App) (*types.App, error) {
	bts, err := json.Marshal(app)
	if err != nil {
		return nil, err
	}

	var createdApp types.App
	err = c.makeRequest(http.MethodPost, "/apps", bytes.NewBuffer(bts), &createdApp)
	if err != nil {
		return nil, err
	}
	return &createdApp, nil
}

func (c *HelixClient) UpdateApp(app *types.App) (*types.App, error) {
	bts, err := json.Marshal(app)
	if err != nil {
		return nil, err
	}

	var updatedApp types.App
	err = c.makeRequest(http.MethodPut, "/apps/"+app.ID, bytes.NewBuffer(bts), &updatedApp)
	if err != nil {
		return nil, err
	}

	return &updatedApp, nil
}

func (c *HelixClient) DeleteApp(appID string, deleteKnowledge bool) error {
	query := url.Values{}
	query.Add("knowledge", strconv.FormatBool(deleteKnowledge))

	url := "/apps/" + appID + "?" + query.Encode()

	err := c.makeRequest(http.MethodDelete, url, nil, nil)
	if err != nil {
		return err
	}

	return nil
}
