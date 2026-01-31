package azuredevops

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/git"
)

// TODO: move to separate pkg/git/azure_devops package
type AzureDevOpsClient struct { //nolint:revive
	connection          *azuredevops.Connection
	organizationURL     string
	personalAccessToken string
}

// ADOUserProfile represents the user profile response from Azure DevOps
type ADOUserProfile struct {
	ID           string `json:"id"`
	DisplayName  string `json:"displayName"`
	EmailAddress string `json:"emailAddress"`
}

func NewAzureDevOpsClientFromApp(app *types.App) (*AzureDevOpsClient, error) {
	// Check for assistants before accessing
	if len(app.Config.Helix.Assistants) == 0 {
		return nil, fmt.Errorf("app %s has no assistants configured", app.ID)
	}

	// Find credentials in the app spec
	for _, tool := range app.Config.Helix.Assistants[0].Tools {
		if tool.Config.AzureDevOps != nil &&
			tool.Config.AzureDevOps.Enabled &&
			tool.Config.AzureDevOps.OrganizationURL != "" &&
			tool.Config.AzureDevOps.PersonalAccessToken != "" {
			return NewAzureDevOpsClient(tool.Config.AzureDevOps.OrganizationURL, tool.Config.AzureDevOps.PersonalAccessToken), nil
		}
	}

	return nil, fmt.Errorf("no Azure DevOps credentials found")
}

func NewAzureDevOpsClient(organizationURL string, personalAccessToken string) *AzureDevOpsClient {
	connection := azuredevops.NewPatConnection(organizationURL, personalAccessToken)

	return &AzureDevOpsClient{
		connection:          connection,
		organizationURL:     organizationURL,
		personalAccessToken: personalAccessToken,
	}
}

// NewAzureDevOpsClientWithServicePrincipal creates a client using Azure AD Service Principal authentication.
// This uses OAuth 2.0 client credentials flow to get an access token for the Azure DevOps API.
// tenantID: Azure AD tenant ID
// clientID: App registration client ID (Application ID)
// clientSecret: App registration client secret
// organizationURL: Azure DevOps organization URL (e.g., https://dev.azure.com/org)
func NewAzureDevOpsClientWithServicePrincipal(ctx context.Context, organizationURL, tenantID, clientID, clientSecret string) (*AzureDevOpsClient, error) {
	// Get access token using client credentials flow
	accessToken, err := getAzureADAccessToken(ctx, tenantID, clientID, clientSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	// Create connection with bearer token
	// The Azure DevOps Go SDK accepts bearer tokens - we use the access token directly
	connection := azuredevops.NewPatConnection(organizationURL, accessToken)

	return &AzureDevOpsClient{
		connection:          connection,
		organizationURL:     organizationURL,
		personalAccessToken: accessToken, // Store access token for manual API calls
	}, nil
}

// getAzureADAccessToken gets an access token from Azure AD using client credentials flow
// The resource/scope for Azure DevOps is https://app.vssps.visualstudio.com/.default
func getAzureADAccessToken(ctx context.Context, tenantID, clientID, clientSecret string) (string, error) {
	tokenURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", tenantID)

	// Azure DevOps resource scope
	scope := "499b84ac-1321-427f-aa17-267ca6975798/.default" // Azure DevOps resource ID

	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)
	data.Set("scope", scope)

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to request token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errorResp struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errorResp); err == nil && errorResp.Error != "" {
			return "", fmt.Errorf("token request failed: %s - %s", errorResp.Error, errorResp.ErrorDescription)
		}
		return "", fmt.Errorf("token request failed with status: %d", resp.StatusCode)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("no access token in response")
	}

	return tokenResp.AccessToken, nil
}

// GetUserProfile fetches the authenticated user's profile from Azure DevOps
func (c *AzureDevOpsClient) GetUserProfile(ctx context.Context) (*ADOUserProfile, error) {
	// Determine the profile API URL based on organization URL
	// For Azure DevOps Services (cloud): use vssps.visualstudio.com
	// For Azure DevOps Server (self-hosted): use the organization URL with connection data API
	var profileURL string
	if strings.Contains(c.organizationURL, "dev.azure.com") || strings.Contains(c.organizationURL, "visualstudio.com") {
		// Azure DevOps Services (cloud)
		profileURL = "https://app.vssps.visualstudio.com/_apis/profile/profiles/me?api-version=7.1-preview.3"
	} else {
		// Azure DevOps Server (self-hosted) - use connection data API
		// This returns authenticated user info including display name and email
		profileURL = strings.TrimSuffix(c.organizationURL, "/") + "/_apis/connectionData?api-version=7.1-preview.1"
	}

	req, err := http.NewRequestWithContext(ctx, "GET", profileURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set Basic Auth with PAT
	req.SetBasicAuth("", c.personalAccessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// For cloud, response is ADOUserProfile directly
	// For self-hosted, response is ConnectionData with authenticatedUser field
	if strings.Contains(c.organizationURL, "dev.azure.com") || strings.Contains(c.organizationURL, "visualstudio.com") {
		var profile ADOUserProfile
		if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
		return &profile, nil
	}

	// Parse self-hosted connection data response
	var connectionData struct {
		AuthenticatedUser struct {
			ID              string `json:"id"`
			DisplayName     string `json:"customDisplayName"`
			ProviderDisplay string `json:"providerDisplayName"`
		} `json:"authenticatedUser"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&connectionData); err != nil {
		return nil, fmt.Errorf("failed to decode connection data response: %w", err)
	}

	displayName := connectionData.AuthenticatedUser.DisplayName
	if displayName == "" {
		displayName = connectionData.AuthenticatedUser.ProviderDisplay
	}

	return &ADOUserProfile{
		ID:          connectionData.AuthenticatedUser.ID,
		DisplayName: displayName,
	}, nil
}

func (c *AzureDevOpsClient) GetComments(ctx context.Context, repositoryID string, pullRequestID int, threadID int) ([]git.Comment, error) {
	gitClient, err := git.NewClient(ctx, c.connection)
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure DevOps client: %w", err)
	}

	comments, err := gitClient.GetComments(ctx, git.GetCommentsArgs{
		RepositoryId:  &repositoryID,
		PullRequestId: &pullRequestID,
		ThreadId:      &threadID,
	})
	if err != nil {
		return nil, err
	}

	return *comments, nil
}

// ListRepositories lists all repositories accessible in the Azure DevOps organization
func (c *AzureDevOpsClient) ListRepositories(ctx context.Context, project string) ([]git.GitRepository, error) {
	gitClient, err := git.NewClient(ctx, c.connection)
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure DevOps git client: %w", err)
	}

	// If project is empty, list all repos in the organization
	var projectPtr *string
	if project != "" {
		projectPtr = &project
	}

	repos, err := gitClient.GetRepositories(ctx, git.GetRepositoriesArgs{
		Project: projectPtr,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list repositories: %w", err)
	}

	if repos == nil {
		return []git.GitRepository{}, nil
	}

	return *repos, nil
}

// ListProjects lists all projects in the Azure DevOps organization by extracting from repositories
func (c *AzureDevOpsClient) ListProjects(ctx context.Context) ([]string, error) {
	gitClient, err := git.NewClient(ctx, c.connection)
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure DevOps git client: %w", err)
	}

	// Get all repos and extract unique project names
	repos, err := gitClient.GetRepositories(ctx, git.GetRepositoriesArgs{})
	if err != nil {
		return nil, fmt.Errorf("failed to list repositories: %w", err)
	}

	projectSet := make(map[string]bool)
	for _, repo := range *repos {
		if repo.Project != nil && repo.Project.Name != nil {
			projectSet[*repo.Project.Name] = true
		}
	}

	projects := make([]string, 0, len(projectSet))
	for name := range projectSet {
		projects = append(projects, name)
	}

	return projects, nil
}
