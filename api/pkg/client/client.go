package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/types"
)

type Client interface {
	CreateApp(app *types.App) (*types.App, error)
	GetApp(appID string) (*types.App, error)
	UpdateApp(app *types.App) (*types.App, error)
	DeleteApp(appID string, deleteKnowledge bool) error
	ListApps(f *AppFilter) ([]*types.App, error)

	ListKnowledge(f *KnowledgeFilter) ([]*types.Knowledge, error)
	GetKnowledge(id string) (*types.Knowledge, error)
	DeleteKnowledge(id string) error
	RefreshKnowledge(id string) error

	ListSecrets() ([]*types.Secret, error)
	CreateSecret(secret *types.CreateSecretRequest) (*types.Secret, error)
	UpdateSecret(id string, secret *types.Secret) (*types.Secret, error)
	DeleteSecret(id string) error

	ListKnowledgeVersions(f *KnowledgeVersionsFilter) ([]*types.KnowledgeVersion, error)

	FilestoreList(ctx context.Context, path string) ([]filestore.FileStoreItem, error)
	FilestoreUpload(ctx context.Context, path string, file io.Reader) error
	FilestoreDelete(ctx context.Context, path string) error
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
		return nil, errors.New("apiKey is required, find yours in your helix account page and set HELIX_API_KEY and HELIX_URL")
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

func (c *HelixClient) makeRequest(method, path string, body io.Reader, v interface{}) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	fullURL := c.url + path
	fmt.Printf("Making request to Helix API: %s %s\n", method, fullURL)

	// Read and store body content for curl logging
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = io.ReadAll(body)
		if err != nil {
			return err
		}
		// Create new reader from bytes for the actual request
		body = strings.NewReader(string(bodyBytes))
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	// Build curl command
	curlCmd := fmt.Sprintf("curl -X %s '%s'", method, fullURL)
	for key, values := range req.Header {
		for _, value := range values {
			curlCmd += fmt.Sprintf(" -H '%s: %s'", key, value)
		}
	}
	if len(bodyBytes) > 0 {
		curlCmd += fmt.Sprintf(" --data-raw '%s'", string(bodyBytes))
	}
	fmt.Printf("Equivalent curl command:\n%s\n", curlCmd)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bts, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("status code %d", resp.StatusCode)
		}
		return fmt.Errorf("status code %d (%s)", resp.StatusCode, string(bts))
	}

	if v != nil {
		return json.NewDecoder(resp.Body).Decode(v)
	}

	return nil
}
