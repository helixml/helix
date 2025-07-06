package skills

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// GitHubTestUtils provides utility functions for GitHub API operations during testing
type GitHubTestUtils struct {
	token    string
	username string
	logger   zerolog.Logger
}

// NewGitHubTestUtils creates a new GitHub test utilities instance
func NewGitHubTestUtils(token, username string, logger zerolog.Logger) *GitHubTestUtils {
	return &GitHubTestUtils{
		token:    token,
		username: username,
		logger:   logger,
	}
}

// CreateRepo creates a new repository on GitHub
func (g *GitHubTestUtils) CreateRepo(name, description string) error {
	reqBody := map[string]interface{}{
		"name":        name,
		"description": description,
		"private":     true,
		"auto_init":   true,
	}

	return g.makeAPICall("POST", "https://api.github.com/user/repos", reqBody, nil)
}

// DeleteRepo deletes a repository from GitHub
func (g *GitHubTestUtils) DeleteRepo(name string) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s", g.username, name)
	return g.makeAPICall("DELETE", url, nil, nil)
}

// AddContentToRepo adds content to a GitHub repository
func (g *GitHubTestUtils) AddContentToRepo(repoName, filename, content string) error {
	// First try to get existing file to see if it already exists
	getURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", g.username, repoName, filename)

	client := &http.Client{Timeout: 30 * time.Second}
	getReq, err := http.NewRequest("GET", getURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create GET request: %w", err)
	}
	getReq.Header.Set("Authorization", "token "+g.token)
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
				g.logger.Info().
					Str("filename", filename).
					Str("sha", sha).
					Msg("File exists, including SHA for update")
			}
		}
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", g.username, repoName, filename)
	return g.makeAPICall("PUT", url, reqBody, nil)
}

// makeAPICall makes an authenticated API call to GitHub
func (g *GitHubTestUtils) makeAPICall(method, url string, body interface{}, result interface{}) error {
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

	req.Header.Set("Authorization", "token "+g.token)
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
