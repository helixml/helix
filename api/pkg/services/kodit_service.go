package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

// KoditService handles communication with Kodit code intelligence service
type KoditService struct {
	baseURL   string
	apiKey    string
	client    *http.Client
	enabled   bool
}

// NewKoditService creates a new Kodit service client
func NewKoditService(baseURL string, apiKey string) *KoditService {
	if baseURL == "" {
		log.Info().Msg("Kodit service not configured (no base URL)")
		return &KoditService{enabled: false}
	}

	return &KoditService{
		baseURL: baseURL,
		apiKey:  apiKey,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		enabled: true,
	}
}

// KoditRepositoryCreateRequest represents a request to register a repository with Kodit
type KoditRepositoryCreateRequest struct {
	Data KoditRepositoryCreateData `json:"data"`
}

type KoditRepositoryCreateData struct {
	Type       string                            `json:"type"`
	Attributes KoditRepositoryCreateAttributes `json:"attributes"`
}

type KoditRepositoryCreateAttributes struct {
	RemoteURI string `json:"remote_uri"`
}

// KoditRepositoryResponse represents Kodit's repository response
type KoditRepositoryResponse struct {
	Data KoditRepositoryData `json:"data"`
}

type KoditRepositoryData struct {
	Type       string                      `json:"type"`
	ID         string                      `json:"id"`
	Attributes KoditRepositoryAttributes `json:"attributes"`
}

type KoditRepositoryAttributes struct {
	RemoteURI string    `json:"remote_uri"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// KoditEnrichmentListResponse represents Kodit's enrichments list
type KoditEnrichmentListResponse struct {
	Data []KoditEnrichmentData `json:"data"`
}

type KoditEnrichmentData struct {
	Type       string                    `json:"type"`
	ID         string                    `json:"id"`
	Attributes KoditEnrichmentAttributes `json:"attributes"`
}

type KoditEnrichmentAttributes struct {
	Type      string    `json:"type"`      // High-level type: usage, developer, living_documentation
	Subtype   *string   `json:"subtype"`   // Specific type: snippet, example, api_docs, architecture, cookbook, commit_description, etc.
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Kodit enrichment type constants (high-level)
const (
	KoditEnrichmentTypeUsage               = "usage"                // How to use the code (snippets, examples, cookbooks)
	KoditEnrichmentTypeDeveloper           = "developer"            // Development docs (architecture, API docs, schemas)
	KoditEnrichmentTypeLivingDocumentation = "living_documentation" // Dynamic docs (commit descriptions, changes)
)

// Kodit enrichment subtype constants (specific types)
const (
	// Usage subtypes
	KoditEnrichmentSubtypeSnippet  = "snippet"  // Code snippets extracted from codebase
	KoditEnrichmentSubtypeExample  = "example"  // Full example files and documentation code blocks
	KoditEnrichmentSubtypeCookbook = "cookbook" // How-to guides and usage patterns

	// Developer subtypes
	KoditEnrichmentSubtypeArchitecture   = "architecture"    // High-level architecture documentation
	KoditEnrichmentSubtypeAPIDocs        = "api_docs"        // API documentation and interfaces
	KoditEnrichmentSubtypeDatabaseSchema = "database_schema" // Database schemas and ORM models

	// Living documentation subtypes
	KoditEnrichmentSubtypeCommitDescription = "commit_description" // Human-readable commit descriptions
)

// RegisterRepository registers a repository with Kodit for indexing
func (s *KoditService) RegisterRepository(ctx context.Context, cloneURL string) (*KoditRepositoryResponse, error) {
	if !s.enabled {
		log.Debug().Msg("Kodit service not enabled, skipping repository registration")
		return nil, nil
	}

	request := KoditRepositoryCreateRequest{
		Data: KoditRepositoryCreateData{
			Type: "repository",
			Attributes: KoditRepositoryCreateAttributes{
				RemoteURI: cloneURL,
			},
		},
	}

	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", s.baseURL+"/api/v1/repositories", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if s.apiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.apiKey))
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("kodit returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var response KoditRepositoryResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	log.Info().
		Str("clone_url", cloneURL).
		Str("kodit_repo_id", response.Data.ID).
		Msg("Registered repository with Kodit")

	return &response, nil
}

// GetRepositoryEnrichments fetches enrichments for a repository from Kodit
func (s *KoditService) GetRepositoryEnrichments(ctx context.Context, koditRepoID string) (*KoditEnrichmentListResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("kodit service not enabled")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/api/v1/repositories/%s/enrichments", s.baseURL, koditRepoID), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if s.apiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.apiKey))
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("kodit returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var response KoditEnrichmentListResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Filter out internal summary enrichments (these are only used by MCP)
	filteredData := make([]KoditEnrichmentData, 0, len(response.Data))
	for _, enrichment := range response.Data {
		subtype := ""
		if enrichment.Attributes.Subtype != nil {
			subtype = *enrichment.Attributes.Subtype
		}
		// Exclude internal summary types
		if subtype == "snippet_summary" || subtype == "example_summary" {
			continue
		}
		filteredData = append(filteredData, enrichment)
	}
	response.Data = filteredData

	// Truncate content to reduce data transfer and improve UI display
	// Keep first 500 characters with ellipsis if truncated
	const maxContentLength = 500
	for i := range response.Data {
		if len(response.Data[i].Attributes.Content) > maxContentLength {
			response.Data[i].Attributes.Content = response.Data[i].Attributes.Content[:maxContentLength] + "..."
		}
	}

	return &response, nil
}

// GetRepositoryStatus fetches indexing status for a repository from Kodit
func (s *KoditService) GetRepositoryStatus(ctx context.Context, koditRepoID string) (map[string]interface{}, error) {
	if !s.enabled {
		return nil, fmt.Errorf("kodit service not enabled")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/api/v1/repositories/%s/status", s.baseURL, koditRepoID), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if s.apiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.apiKey))
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("kodit returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var response map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return response, nil
}
