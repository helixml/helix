//go:build oauth_integration
// +build oauth_integration

package skills_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// GitHubProviderHandler handles GitHub-specific OAuth testing
type GitHubProviderHandler struct {
	logger                 zerolog.Logger
	username               string
	password               string
	setupToken             string // Personal access token for repo creation/deletion
	gmailCredentialsBase64 string
}

// NewGitHubProviderHandler creates a new GitHub provider handler
func NewGitHubProviderHandler(logger zerolog.Logger) (*GitHubProviderHandler, error) {
	handler := &GitHubProviderHandler{
		logger: logger,
	}

	// Load GitHub credentials from environment
	err := handler.loadCredentials()
	if err != nil {
		return nil, fmt.Errorf("failed to load GitHub credentials: %w", err)
	}

	return handler, nil
}

// loadCredentials loads GitHub credentials from environment variables
func (h *GitHubProviderHandler) loadCredentials() error {
	h.username = os.Getenv("GITHUB_SKILL_TEST_OAUTH_USERNAME")
	if h.username == "" {
		return fmt.Errorf("GITHUB_SKILL_TEST_OAUTH_USERNAME environment variable not set")
	}

	h.password = os.Getenv("GITHUB_SKILL_TEST_OAUTH_PASSWORD")
	if h.password == "" {
		return fmt.Errorf("GITHUB_SKILL_TEST_OAUTH_PASSWORD environment variable not set")
	}

	h.setupToken = os.Getenv("GITHUB_SKILL_TEST_SETUP_PAT")
	if h.setupToken == "" {
		return fmt.Errorf("GITHUB_SKILL_TEST_SETUP_PAT environment variable not set")
	}

	h.gmailCredentialsBase64 = os.Getenv("GMAIL_CREDENTIALS_BASE64")
	if h.gmailCredentialsBase64 == "" {
		return fmt.Errorf("GMAIL_CREDENTIALS_BASE64 environment variable not set")
	}

	h.logger.Info().
		Str("username", h.username).
		Int("password_length", len(h.password)).
		Msg("GitHub credentials loaded")

	return nil
}

// GetOAuthProviderConfig returns the OAuth provider configuration for GitHub
func (h *GitHubProviderHandler) GetOAuthProviderConfig(clientID, clientSecret string) OAuthProviderConfig {
	return OAuthProviderConfig{
		Name:         "GitHub Skills Test",
		ProviderType: "github",
		ClientID:     clientID,
		ClientSecret: clientSecret,
		AuthURL:      "https://github.com/login/oauth/authorize",
		TokenURL:     "https://github.com/login/oauth/access_token",
		UserInfoURL:  "https://api.github.com/user",
		Scopes:       []string{"repo", "user:read"},
	}
}

// GetOAuthProviderConfig returns the OAuth provider configuration for browser automation
func (h *GitHubProviderHandler) GetOAuthBrowserConfig() OAuthProviderConfig {
	return OAuthProviderConfig{
		LoginConfig: LoginConfig{
			UsernameSelector:    `input[name="login"]`,
			PasswordSelector:    `input[name="password"]`,
			LoginButtonSelector: `input[type="submit"][value="Sign in"], button[type="submit"]`,
		},
		AuthorizeButtonSelector: `button[name="authorize"], input[type="submit"][value="Authorize"]`,
	}
}

// GetCredentials returns the OAuth credentials for GitHub
func (h *GitHubProviderHandler) GetCredentials() OAuthCredentials {
	return OAuthCredentials{
		Username: h.username,
		Password: h.password,
	}
}

// CreateDeviceVerificationHandler creates a device verification handler for GitHub
func (h *GitHubProviderHandler) CreateDeviceVerificationHandler(browserAutomation *OAuthBrowserAutomation) (DeviceVerificationHandler, error) {
	handler, err := NewGitHubDeviceVerificationHandler(h.logger, h.gmailCredentialsBase64, browserAutomation)
	if err != nil {
		return nil, fmt.Errorf("failed to create GitHub device verification handler: %w", err)
	}
	return handler, nil
}

// CreateTestRepositories creates test repositories on GitHub
func (h *GitHubProviderHandler) CreateTestRepositories() ([]string, error) {
	h.logger.Info().Msg("Creating test repositories on GitHub")

	var testRepos []string

	// Create a test repository with some content
	repoName := fmt.Sprintf("helix-test-repo-%d", time.Now().Unix())

	err := h.createGitHubRepo(repoName, "Test repository for Helix OAuth skills testing")
	if err != nil {
		return nil, fmt.Errorf("failed to create test repository: %w", err)
	}

	testRepos = append(testRepos, repoName)

	// Add some test content to the repository
	err = h.addContentToRepo(repoName, "README.md", "# Helix Test Repository\n\nThis is a test repository created for testing Helix OAuth skills integration.\n\n## Test Data\n\n- Repository: "+repoName+"\n- Created by: Helix OAuth E2E Test\n- Purpose: Testing GitHub skills integration\n")
	if err != nil {
		return nil, fmt.Errorf("failed to add content to test repository: %w", err)
	}

	err = h.addContentToRepo(repoName, "test-file.txt", "This is a test file for Helix to read and understand.\nIt contains some sample data that the agent should be able to access.")
	if err != nil {
		return nil, fmt.Errorf("failed to add test file to repository: %w", err)
	}

	h.logger.Info().
		Str("repo_name", repoName).
		Msg("Successfully created test repository with content")

	return testRepos, nil
}

// CleanupTestRepositories deletes test repositories from GitHub
func (h *GitHubProviderHandler) CleanupTestRepositories(testRepos []string) {
	h.logger.Info().Msg("Cleaning up test repositories")

	for _, repoName := range testRepos {
		err := h.deleteGitHubRepo(repoName)
		if err != nil {
			h.logger.Error().Err(err).Str("repo", repoName).Msg("Failed to delete GitHub repository")
		} else {
			h.logger.Info().Str("repo", repoName).Msg("GitHub repository deleted")
		}
	}
}

// createGitHubRepo creates a new repository on GitHub
func (h *GitHubProviderHandler) createGitHubRepo(name, description string) error {
	reqBody := map[string]interface{}{
		"name":        name,
		"description": description,
		"private":     true,
		"auto_init":   true,
	}

	return h.makeGitHubAPICall("POST", "https://api.github.com/user/repos", reqBody, nil)
}

// addContentToRepo adds content to a GitHub repository
func (h *GitHubProviderHandler) addContentToRepo(repoName, filename, content string) error {
	// First try to get existing file to see if it already exists
	getURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", h.username, repoName, filename)

	client := &http.Client{Timeout: 30 * time.Second}
	getReq, err := http.NewRequest("GET", getURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create GET request: %w", err)
	}
	getReq.Header.Set("Authorization", "token "+h.setupToken)
	getReq.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(getReq)
	if err != nil {
		return fmt.Errorf("failed to check if file exists: %w", err)
	}
	defer resp.Body.Close()

	reqBody := map[string]interface{}{
		"message": "Add " + filename + " via Helix test",
		"content": base64.StdEncoding.EncodeToString([]byte(content)),
	}

	// If file exists (200), we need to include the SHA for updates
	if resp.StatusCode == 200 {
		var existingFile map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&existingFile); err == nil {
			if sha, ok := existingFile["sha"].(string); ok {
				reqBody["sha"] = sha
				h.logger.Info().
					Str("filename", filename).
					Str("sha", sha).
					Msg("File exists, including SHA for update")
			}
		}
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", h.username, repoName, filename)
	return h.makeGitHubAPICall("PUT", url, reqBody, nil)
}

// deleteGitHubRepo deletes a repository from GitHub
func (h *GitHubProviderHandler) deleteGitHubRepo(name string) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s", h.username, name)
	return h.makeGitHubAPICall("DELETE", url, nil, nil)
}

// makeGitHubAPICall makes an authenticated API call to GitHub
func (h *GitHubProviderHandler) makeGitHubAPICall(method, url string, body interface{}, result interface{}) error {
	client := &http.Client{Timeout: 30 * time.Second}

	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = strings.NewReader(string(bodyBytes))
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "token "+h.setupToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make API call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API error: %d %s - %s", resp.StatusCode, resp.Status, string(respBody))
	}

	if result != nil && resp.StatusCode != 204 {
		return json.NewDecoder(resp.Body).Decode(result)
	}

	return nil
}

// GetTestQueries returns test queries for GitHub skills
func (h *GitHubProviderHandler) GetTestQueries(testRepos []string) []TestQuery {
	queries := []TestQuery{
		{
			Query:            "What is my GitHub username?",
			Name:             "GitHub Username Query",
			ExpectedKeywords: []string{"username", "github"},
		},
		{
			Query:            "List my GitHub repositories",
			Name:             "GitHub Repository Listing",
			ExpectedKeywords: []string{"repositor", "repo"},
		},
	}

	// Add repository-specific queries if test repos exist
	if len(testRepos) > 0 {
		queries = append(queries, TestQuery{
			Query:            fmt.Sprintf("What issues are open in my repository %s?", testRepos[0]),
			Name:             "GitHub Issues Query",
			ExpectedKeywords: []string{"issue", "repository"},
		})

		queries = append(queries, TestQuery{
			Query:            fmt.Sprintf("What files are in the repository %s?", testRepos[0]),
			Name:             "GitHub Files Query",
			ExpectedKeywords: []string{"file", "repository", "README"},
		})
	}

	return queries
}

// ValidateOAuthCredentials validates that the OAuth credentials work
func (h *GitHubProviderHandler) ValidateOAuthCredentials(clientID, clientSecret string) error {
	h.logger.Info().Msg("Validating GitHub OAuth credentials")

	// Make a test API call to validate credentials
	// We can't easily validate OAuth app credentials without going through the full flow,
	// but we can validate that our setup token works
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", "https://api.github.com/user", nil)
	if err != nil {
		return fmt.Errorf("failed to create validation request: %w", err)
	}

	req.Header.Set("Authorization", "token "+h.setupToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to validate credentials: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API validation failed: %d %s - %s", resp.StatusCode, resp.Status, string(respBody))
	}

	var userInfo map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&userInfo)
	if err != nil {
		return fmt.Errorf("failed to decode user info: %w", err)
	}

	login, ok := userInfo["login"].(string)
	if !ok || login != h.username {
		return fmt.Errorf("username mismatch: expected %s, got %v", h.username, login)
	}

	h.logger.Info().
		Str("username", login).
		Msg("GitHub credentials validated successfully")

	return nil
}

// TestQuery represents a test query for skills
type TestQuery struct {
	Query            string
	Name             string
	ExpectedKeywords []string
}
