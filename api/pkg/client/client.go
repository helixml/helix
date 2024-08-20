package client

import (
	"errors"
	"net/http"
	"strings"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
)

type Client interface {
	CreateApp(app *types.App) (*types.App, error)
	UpdateApp(app *types.App) (*types.App, error)
	DeleteApp(appID string) error
	ListApps(f *AppFilter) ([]*types.App, error)

	ListKnowledge(f *KnowledgeFilter) ([]*types.Knowledge, error)
	GetKnowledge(id string) (*types.Knowledge, error)
	DeleteKnowledge(id string) error
	RefreshKnowledge(id string) error
}

// HelixClient is the client for the helix api
type HelixClient struct {
	httpClient *http.Client
	apiKey     string
	url        string
}

const (
	DefaultURL = "https://app.tryhelix.ai"
)

func NewClientFromEnv() (*HelixClient, error) {
	cfg, err := config.LoadCliConfig()
	if err != nil {
		return nil, err
	}

	return NewClient(cfg.URL, cfg.APIKey)
}

func NewClient(url, apiKey string) (*HelixClient, error) {
	if url == "" {
		url = DefaultURL
	}

	if apiKey == "" {
		return nil, errors.New("apiKey is required, find yours in https://app.tryhelix.ai/account and set HELIX_API_KEY (and optionally HELIX_URL)")
	}

	if !strings.HasSuffix(url, "/api/v1") {
		// append /api/v1 to the url
		url = url + "/api/v1"
	}

	return &HelixClient{
		httpClient: http.DefaultClient,
		apiKey:     apiKey,
		url:        url,
	}, nil
}
