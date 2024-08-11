package client

import (
	"net/http"

	"github.com/helixml/helix/api/pkg/types"
)

type Client interface {
	CreateApp(app *types.App) (*types.App, error)
	UpdateApp(app *types.App) (*types.App, error)
	DeleteApp(appID string) error
	ListApps(f *AppFilter) ([]*types.App, error)
}

// HelixClient is the client for the helix api
type HelixClient struct {
	httpClient *http.Client
	apiKey     string
	url        string
}

func NewClient(url, apiKey string) *HelixClient {
	return &HelixClient{
		httpClient: http.DefaultClient,
		apiKey:     apiKey,
		url:        url,
	}
}
