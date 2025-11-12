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
	Type      string    `json:"type"`
	Subtype   *string   `json:"subtype"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

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
