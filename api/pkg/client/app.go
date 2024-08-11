package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/helixml/helix/api/pkg/types"
)

type AppFilter struct {
}

func (c *HelixClient) ListApps(f *AppFilter) ([]*types.App, error) {
	req, err := http.NewRequest(http.MethodGet, c.url+"/apps", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bts, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var apps []*types.App
	err = json.Unmarshal(bts, &apps)
	if err != nil {
		return nil, err
	}

	return apps, nil
}

func (c *HelixClient) CreateApp(app *types.App) (*types.App, error) {
	bts, err := json.Marshal(app)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, c.url+"/apps", bytes.NewBuffer(bts))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to create app, status code: %d", resp.StatusCode)
	}

	bts, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var createdApp types.App
	err = json.Unmarshal(bts, &createdApp)
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

	req, err := http.NewRequest(http.MethodPut, c.url+"/apps/"+app.ID, bytes.NewBuffer(bts))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to update app, status code: %d", resp.StatusCode)
	}

	bts, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var updatedApp types.App
	err = json.Unmarshal(bts, &updatedApp)
	if err != nil {
		return nil, err
	}

	return &updatedApp, nil
}

func (c *HelixClient) DeleteApp(appID string) error {
	req, err := http.NewRequest(http.MethodDelete, c.url+"/apps/"+appID, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to delete app, status code: %d", resp.StatusCode)
	}

	return nil
}
