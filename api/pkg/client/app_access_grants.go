package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/helixml/helix/api/pkg/types"
)

type AppAccessGrantsFilter struct {
	AppID string
}

func (c *HelixClient) ListAppAccessGrants(ctx context.Context, f *AppAccessGrantsFilter) ([]*types.AccessGrant, error) {
	var apps []*types.AccessGrant

	err := c.makeRequest(ctx, http.MethodGet, fmt.Sprintf("/apps/%s/access-grants", f.AppID), nil, &apps)
	if err != nil {
		return nil, err
	}
	return apps, nil
}

func (c *HelixClient) CreateAppAccessGrant(ctx context.Context, appID string, grant *types.CreateAccessGrantRequest) (*types.AccessGrant, error) {
	bts, err := json.Marshal(grant)
	if err != nil {
		return nil, err
	}

	var createdGrant types.AccessGrant
	err = c.makeRequest(ctx, http.MethodPost, fmt.Sprintf("/apps/%s/access-grants", appID), bytes.NewBuffer(bts), &createdGrant)
	if err != nil {
		return nil, err
	}
	return &createdGrant, nil
}

func (c *HelixClient) DeleteAppAccessGrant(ctx context.Context, appID string, grantID string) error {
	err := c.makeRequest(ctx, http.MethodDelete, fmt.Sprintf("/apps/%s/access-grants/%s", appID, grantID), nil, nil)
	if err != nil {
		return err
	}
	return nil
}

func (c *HelixClient) GetAppUserAccess(ctx context.Context, appID string) (*types.UserAppAccessResponse, error) {
	var response types.UserAppAccessResponse
	err := c.makeRequest(ctx, http.MethodGet, fmt.Sprintf("/apps/%s/user-access", appID), nil, &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}
