package client

import (
	"encoding/json"
	"io"

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
