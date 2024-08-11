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

func (c *Client) ListApps(f *AppFilter) ([]*types.App, error) {
	resp, err := c.httpClient.Get(c.url + "/apps")
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

func (c *Client) CreateApp(app *types.App) (*types.App, error) {
	bts, err := json.Marshal(app)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Post(c.url+"/apps", "application/json", bytes.NewBuffer(bts))
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
