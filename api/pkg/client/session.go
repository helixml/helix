package client

import (
	"context"
	"net/http"
	"strconv"

	"github.com/helixml/helix/api/pkg/types"
)

func (c *HelixClient) ListSessions(ctx context.Context, f *SessionFilter) (*types.SessionsList, error) {
	var sessions types.SessionsList

	path := "/sessions"

	// Add query parameters if provided
	if f != nil {
		queryParams := []string{}

		if f.OrganizationID != "" {
			queryParams = append(queryParams, "organization_id="+f.OrganizationID)
		}

		if f.Offset > 0 {
			queryParams = append(queryParams, "offset="+strconv.Itoa(f.Offset))
		}

		if f.Limit > 0 {
			queryParams = append(queryParams, "limit="+strconv.Itoa(f.Limit))
		}

		if len(queryParams) > 0 {
			path += "?"
			for i, param := range queryParams {
				if i > 0 {
					path += "&"
				}
				path += param
			}
		}
	}

	err := c.makeRequest(ctx, http.MethodGet, path, nil, &sessions)
	if err != nil {
		return nil, err
	}

	return &sessions, nil
}
