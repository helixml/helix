package client

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/avast/retry-go/v4"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

type Client interface {
	CreateApp(ctx context.Context, app *types.App) (*types.App, error)
	GetApp(ctx context.Context, appID string) (*types.App, error)
	UpdateApp(ctx context.Context, app *types.App) (*types.App, error)
	DeleteApp(ctx context.Context, appID string, deleteKnowledge bool) error
	ListApps(ctx context.Context, f *AppFilter) ([]*types.App, error)

	GetAppAPIKeys(ctx context.Context, appID string) ([]*types.ApiKey, error)

	// Sessions
	ListSessions(ctx context.Context, f *SessionFilter) (*types.SessionsList, error)

	RunAPIAction(ctx context.Context, appID string, action string, parameters map[string]interface{}) (*types.RunAPIActionResponse, error)

	ListKnowledge(ctx context.Context, f *KnowledgeFilter) ([]*types.Knowledge, error)
	GetKnowledge(ctx context.Context, id string) (*types.Knowledge, error)
	DeleteKnowledge(ctx context.Context, id string) error
	RefreshKnowledge(ctx context.Context, id string) error
	CompleteKnowledgePreparation(ctx context.Context, id string) error
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
	ListAppAccessGrants(ctx context.Context, f *AppAccessGrantsFilter) ([]*types.AccessGrant, error)
	CreateAppAccessGrant(ctx context.Context, appID string, grant *types.CreateAccessGrantRequest) (*types.AccessGrant, error)
	DeleteAppAccessGrant(ctx context.Context, appID, grantID string) error

	// Get current user's access level for an app
	GetAppUserAccess(ctx context.Context, appID string) (*types.UserAppAccessResponse, error)

	// Helix Models
	ListHelixModels(ctx context.Context, f *store.ListModelsQuery) ([]*types.Model, error)
	CreateHelixModel(ctx context.Context, model *types.Model) (*types.Model, error)
	UpdateHelixModel(ctx context.Context, id string, model *types.Model) (*types.Model, error)
	DeleteHelixModel(ctx context.Context, id string) error

	// System Settings
	GetSystemSettings(ctx context.Context) (*types.SystemSettingsResponse, error)
	UpdateSystemSettings(ctx context.Context, settings *types.SystemSettingsRequest) (*types.SystemSettingsResponse, error)

	// Wolf Pairing
	GetWolfPendingPairRequests(ctx context.Context) ([]byte, error)
	CompleteWolfPairing(ctx context.Context, data []byte) ([]byte, error)

	// Users
	ListUsers(ctx context.Context, f *UserFilter) (*types.PaginatedUsersList, error)
	GetCurrentUser(ctx context.Context) (*types.User, error)
	UpdateOwnPassword(ctx context.Context, newPassword string) error
	AdminResetPassword(ctx context.Context, userID string, newPassword string) (*types.User, error)
	AdminDeleteUser(ctx context.Context, userID string) error
}

type SessionFilter struct {
	OrganizationID string
	Offset         int
	Limit          int
}

type UserFilter struct {
	Email    string
	Username string
	Page     int
	PerPage  int
}

// HelixClient is the client for the helix api
type HelixClient struct {
	httpClient *http.Client
	apiKey     string
	url        string
}

const (
	DefaultURL = "https://app.helix.ml"
)

func NewClientFromEnv() (*HelixClient, error) {
	cfg, err := config.LoadCliConfig()
	if err != nil {
		return nil, err
	}

	return NewClient(cfg.URL, cfg.APIKey, cfg.TLSSkipVerify)
}

func NewClient(url, apiKey string, tlsSkipVerify bool) (*HelixClient, error) {
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

	// Create HTTP client with optional TLS skip verify for enterprise environments
	httpClient := &http.Client{}
	if tlsSkipVerify {
		// Clone the default transport to preserve all default settings
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		httpClient.Transport = transport
	}

	return &HelixClient{
		httpClient: httpClient,
		apiKey:     apiKey,
		url:        url,
	}, nil
}

func (c *HelixClient) makeRequest(ctx context.Context, method, path string, body io.Reader, v interface{}) error {
	return retry.Do(func() error {
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
	},
		retry.LastErrorOnly(true),
		retry.Attempts(3),
		retry.Delay(time.Second),
		retry.RetryIf(func(err error) bool {
			// If the error is a connection refused, it means the server is not running
			return strings.Contains(err.Error(), "connection refused")
		}),
	)
}

// GetSystemSettings retrieves the current system settings
func (c *HelixClient) GetSystemSettings(ctx context.Context) (*types.SystemSettingsResponse, error) {
	var settings types.SystemSettingsResponse
	err := c.makeRequest(ctx, http.MethodGet, "/system/settings", nil, &settings)
	if err != nil {
		return nil, err
	}
	return &settings, nil
}

// UpdateSystemSettings updates the system settings
func (c *HelixClient) UpdateSystemSettings(ctx context.Context, settings *types.SystemSettingsRequest) (*types.SystemSettingsResponse, error) {
	reqBody, err := json.Marshal(settings)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	var response types.SystemSettingsResponse
	err = c.makeRequest(ctx, http.MethodPut, "/system/settings", strings.NewReader(string(reqBody)), &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}

// Wolf Pairing methods
func (c *HelixClient) GetWolfPendingPairRequests(ctx context.Context) ([]byte, error) {
	var response json.RawMessage
	err := c.makeRequest(ctx, http.MethodGet, "/wolf/pairing/pending", nil, &response)
	if err != nil {
		return nil, err
	}
	return []byte(response), nil
}

func (c *HelixClient) CompleteWolfPairing(ctx context.Context, data []byte) ([]byte, error) {
	var response json.RawMessage
	err := c.makeRequest(ctx, http.MethodPost, "/wolf/pairing/complete", strings.NewReader(string(data)), &response)
	if err != nil {
		return nil, err
	}
	return []byte(response), nil
}

// User methods

// ListUsers returns a paginated list of users (admin only)
func (c *HelixClient) ListUsers(ctx context.Context, f *UserFilter) (*types.PaginatedUsersList, error) {
	path := "/users?"
	if f != nil {
		if f.Email != "" {
			path += "email=" + f.Email + "&"
		}
		if f.Username != "" {
			path += "username=" + f.Username + "&"
		}
		if f.Page > 0 {
			path += fmt.Sprintf("page=%d&", f.Page)
		}
		if f.PerPage > 0 {
			path += fmt.Sprintf("per_page=%d&", f.PerPage)
		}
	}
	var response types.PaginatedUsersList
	err := c.makeRequest(ctx, http.MethodGet, path, nil, &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}

// GetCurrentUserStatus returns the status of the currently authenticated user
// Note: This returns limited info (user ID, admin flag, slug). Use GetCurrentUser for full details.
func (c *HelixClient) GetCurrentUserStatus(ctx context.Context) (*types.UserStatus, error) {
	var response types.UserStatus
	err := c.makeRequest(ctx, http.MethodGet, "/status", nil, &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}

// GetUser returns full user details by ID
func (c *HelixClient) GetUser(ctx context.Context, userID string) (*types.User, error) {
	var response types.User
	err := c.makeRequest(ctx, http.MethodGet, "/users/"+userID, nil, &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}

// GetCurrentUser returns the full details of the currently authenticated user
func (c *HelixClient) GetCurrentUser(ctx context.Context) (*types.User, error) {
	// First get the user ID from status
	status, err := c.GetCurrentUserStatus(ctx)
	if err != nil {
		return nil, err
	}
	// Then fetch full user details
	return c.GetUser(ctx, status.User)
}

// UpdateOwnPassword updates the password for the currently authenticated user
func (c *HelixClient) UpdateOwnPassword(ctx context.Context, newPassword string) error {
	reqBody, err := json.Marshal(types.PasswordUpdateRequest{
		NewPassword: newPassword,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}
	return c.makeRequest(ctx, http.MethodPost, "/auth/password-update", strings.NewReader(string(reqBody)), nil)
}

// AdminResetPassword resets the password for any user (admin only)
func (c *HelixClient) AdminResetPassword(ctx context.Context, userID string, newPassword string) (*types.User, error) {
	reqBody, err := json.Marshal(map[string]string{
		"new_password": newPassword,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	var response types.User
	err = c.makeRequest(ctx, http.MethodPut, "/admin/users/"+userID+"/password", strings.NewReader(string(reqBody)), &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}

// AdminDeleteUser deletes a user by ID (admin only)
func (c *HelixClient) AdminDeleteUser(ctx context.Context, userID string) error {
	return c.makeRequest(ctx, http.MethodDelete, "/admin/users/"+userID, nil, nil)
}
