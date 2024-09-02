package client

import (
	"context"
	"net/url"

	"github.com/helixml/helix/api/pkg/filestore"
)

func (c *HelixClient) FilestoreList(ctx context.Context, path string) ([]filestore.FileStoreItem, error) {
	var resp []filestore.FileStoreItem

	url := url.URL{
		Path: "/filestore/list",
	}

	query := url.Query()
	query.Add("path", path)
	url.RawQuery = query.Encode()

	err := c.makeRequest("GET", url.String(), nil, &resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
}
