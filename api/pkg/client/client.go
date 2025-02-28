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
	CreateApp(ctx context.Context, app *types.App) (*types.App, error)
	GetApp(ctx context.Context, appID string) (*types.App, error)
	UpdateApp(ctx context.Context, app *types.App) (*types.App, error)
	DeleteApp(ctx context.Context, appID string, deleteKnowledge bool) error
	ListApps(ctx context.Context, f *AppFilter) ([]*types.App, error)

	RunAPIAction(ctx context.Context, appID string, action string, parameters map[string]string) (*types.RunAPIActionResponse, error)

	ListKnowledge(ctx context.Context, f *KnowledgeFilter) ([]*types.Knowledge, error)
	GetKnowledge(ctx context.Context, id string) (*types.Knowledge, error)
	DeleteKnowledge(ctx context.Context, id string) error
	RefreshKnowledge(ctx context.Context, id string) error
	SearchKnowledge(ctx context.Context, f *KnowledgeSearchQuery) ([]*types.KnowledgeSearchResult, error)

	ListSecrets(ctx context.Context) ([]*types.Secret, error)
	CreateSecret(ctx context.Context, secret *types.CreateSecretRequest) (*types.Secret, error)
	UpdateSecret(ctx context.Context, id string, secret *types.Secret) (*types.Secret, error)
	DeleteSecret(ctx context.Context, id string) error

	ListKnowledgeVersions(ctx context.Context, f *KnowledgeVersionsFilter) ([]*types.KnowledgeVersion, error)

	FilestoreList(ctx context.Context, path string) ([]filestore.Item, error)
	FilestoreUpload(ctx context.Context, path string, file io.Reader) error
	FilestoreDelete(ctx context.Context, path string) error

	ListProviderEndpoints(ctx context.Context) ([]*types.ProviderEndpoint, error)
	GetProviderEndpoint(ctx context.Context, id string) (*types.ProviderEndpoint, error)
	CreateProviderEndpoint(ctx context.Context, endpoint *types.ProviderEndpoint) (*types.ProviderEndpoint, error)
	UpdateProviderEndpoint(ctx context.Context, endpoint *types.ProviderEndpoint) (*types.ProviderEndpoint, error)
	DeleteProviderEndpoint(ctx context.Context, id string) error

	// Organizations
	ListOrganizations(ctx context.Context) ([]*types.Organization, error)
	GetOrganization(ctx context.Context, reference string) (*types.Organization, error)
	CreateOrganization(ctx context.Context, organization *types.Organization) (*types.Organization, error)
	UpdateOrganization(ctx context.Context, id string, organization *types.Organization) (*types.Organization, error)

	// Organization Members
	ListOrganizationMembers(ctx context.Context, organizationID string) ([]*types.OrganizationMembership, error)
	AddOrganizationMember(ctx context.Context, organizationID string, req *types.AddOrganizationMemberRequest) (*types.OrganizationMembership, error)
	UpdateOrganizationMember(ctx context.Context, organizationID, userID string, req *types.UpdateOrganizationMemberRequest) (*types.OrganizationMembership, error)
	RemoveOrganizationMember(ctx context.Context, organizationID, userID string) error

	// App Access Grants
	ListAppAccessGrants(ctx context.Context, appID string) ([]*types.AccessGrant, error)
	CreateAppAccessGrant(ctx context.Context, appID string, grant *types.CreateAccessGrantRequest) (*types.AccessGrant, error)
	DeleteAppAccessGrant(ctx context.Context, appID string, grantID string) error
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

func (c *HelixClient) makeRequest(ctx context.Context, method, path string, body io.Reader, v interface{}) error {
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	fullURL := c.url + path

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

	req, err := http.NewRequestWithContext(reqCtx, method, fullURL, body)
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

	if resp.StatusCode >= 300 {
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
