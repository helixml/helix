package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	kodit "github.com/helixml/kodit/clients/go"
	"github.com/oapi-codegen/oapi-codegen/v2/pkg/securityprovider"
	"github.com/rs/zerolog/log"
)

// KoditService handles communication with Kodit code intelligence service
type KoditService struct {
	enabled bool
	client  *kodit.Client
}

// NewKoditService creates a new Kodit service client
func NewKoditService(baseURL, apiKey string) *KoditService {
	if baseURL == "" {
		log.Info().Msg("Kodit service not configured (no base URL)")
		return &KoditService{enabled: false}
	}

	options := []kodit.ClientOption{
		kodit.WithHTTPClient(&http.Client{Timeout: 30 * time.Second}),
	}

	if apiKey != "" {
		basicAuth, err := securityprovider.NewSecurityProviderBasicAuth("my_user", "my_pass")
		if err != nil {
			log.Error().Err(err).Msg("Failed to create basic auth security provider")
			return &KoditService{enabled: false}
		}
		options = append(options, kodit.WithRequestEditorFn(basicAuth.Intercept))
	}

	client, err := kodit.NewClient(baseURL, options...)
	if err != nil {
		log.Error().Err(err).Str("base_url", baseURL).Msg("Failed to create Kodit client")
		return &KoditService{enabled: false}
	}

	return &KoditService{enabled: true, client: client}
}

// Enrichment type constants
const (
	EnrichmentTypeUsage               = "usage"
	EnrichmentTypeDeveloper           = "developer"
	EnrichmentTypeLivingDocumentation = "living_documentation"
)

// Enrichment subtype constants
const (
	EnrichmentSubtypeSnippet           = "snippet"
	EnrichmentSubtypeExample           = "example"
	EnrichmentSubtypeCookbook          = "cookbook"
	EnrichmentSubtypeArchitecture      = "architecture"
	EnrichmentSubtypeAPIDocs           = "api_docs"
	EnrichmentSubtypeDatabaseSchema    = "database_schema"
	EnrichmentSubtypeCommitDescription = "commit_description"
)

// Response types for API compatibility (simple type aliases)
type (
	KoditRepositoryResponse      = kodit.RepositoryResponse
	KoditEnrichmentListResponse  struct {
		Data []KoditEnrichmentData `json:"data"`
	}
	KoditEnrichmentData struct {
		Type       string                    `json:"type"`
		ID         string                    `json:"id"`
		Attributes KoditEnrichmentAttributes `json:"attributes"`
		CommitSHA  string                    `json:"commit_sha,omitempty"` // Added for frontend
	}
	KoditEnrichmentAttributes struct {
		Type      string     `json:"type"`
		Subtype   *string    `json:"subtype,omitempty"`
		Content   string     `json:"content"`
		CreatedAt time.Time  `json:"created_at"`
		UpdatedAt time.Time  `json:"updated_at"`
	}
)

// RegisterRepository registers a repository with Kodit for indexing
func (s *KoditService) RegisterRepository(ctx context.Context, cloneURL string) (*KoditRepositoryResponse, error) {
	if !s.enabled {
		log.Debug().Msg("Kodit service not enabled, skipping repository registration")
		return nil, nil
	}

	repoType := "repository"
	resp, err := s.client.CreateRepositoryApiV1RepositoriesPost(ctx, kodit.RepositoryCreateRequest{
		Data: kodit.RepositoryCreateData{
			Type:       &repoType,
			Attributes: kodit.RepositoryCreateAttributes{RemoteUri: cloneURL},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create repository: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("kodit returned status %d: %s", resp.StatusCode, body)
	}

	var response kodit.RepositoryResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	log.Info().Str("clone_url", cloneURL).Str("kodit_repo_id", response.Data.Id).Msg("Registered repository with Kodit")
	return &response, nil
}

// GetRepositoryEnrichments fetches enrichments for a repository from Kodit
// enrichmentType can be: "usage", "developer", "living_documentation" (or empty for all)
// commitSHA can be specified to filter enrichments for a specific commit
func (s *KoditService) GetRepositoryEnrichments(ctx context.Context, koditRepoID, enrichmentType, commitSHA string) (*KoditEnrichmentListResponse, error) {
	if !s.enabled {
		return nil, fmt.Errorf("kodit service not enabled")
	}

	var resp *http.Response
	var err error

	// Use different endpoint based on whether commit SHA is provided
	if commitSHA != "" {
		// Use commit-specific endpoint: /api/v1/repositories/{repo_id}/commits/{commit_sha}/enrichments
		var params *kodit.ListCommitEnrichmentsApiV1RepositoriesRepoIdCommitsCommitShaEnrichmentsGetParams
		if enrichmentType != "" {
			// Use JSON marshaling to work around unexported union field in generated code
			paramsJSON, _ := json.Marshal(map[string]string{"enrichment_type": enrichmentType})
			params = &kodit.ListCommitEnrichmentsApiV1RepositoriesRepoIdCommitsCommitShaEnrichmentsGetParams{}
			json.Unmarshal(paramsJSON, params)
		}
		resp, err = s.client.ListCommitEnrichmentsApiV1RepositoriesRepoIdCommitsCommitShaEnrichmentsGet(ctx, koditRepoID, commitSHA, params)
	} else {
		// Use repository-wide endpoint: /api/v1/repositories/{repo_id}/enrichments
		var params *kodit.ListRepositoryEnrichmentsApiV1RepositoriesRepoIdEnrichmentsGetParams
		if enrichmentType != "" {
			// Use JSON marshaling to work around unexported union field in generated code
			paramsJSON, _ := json.Marshal(map[string]string{"enrichment_type": enrichmentType})
			params = &kodit.ListRepositoryEnrichmentsApiV1RepositoriesRepoIdEnrichmentsGetParams{}
			json.Unmarshal(paramsJSON, params)
		}
		resp, err = s.client.ListRepositoryEnrichmentsApiV1RepositoriesRepoIdEnrichmentsGet(ctx, koditRepoID, params)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list enrichments: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("kodit returned status %d: %s", resp.StatusCode, body)
	}

	var apiResponse kodit.EnrichmentListResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return filterAndConvertEnrichments(apiResponse.Data), nil
}

// GetEnrichment fetches a single enrichment by ID directly from Kodit
func (s *KoditService) GetEnrichment(ctx context.Context, enrichmentID string) (*KoditEnrichmentData, error) {
	if !s.enabled {
		return nil, fmt.Errorf("kodit service not enabled")
	}

	resp, err := s.client.GetEnrichmentApiV1EnrichmentsEnrichmentIdGet(ctx, enrichmentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get enrichment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("kodit returned status %d: %s", resp.StatusCode, body)
	}

	var apiResponse kodit.EnrichmentResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Convert to our response type
	e := apiResponse.Data
	subtype := extractString(e.Attributes.Subtype)
	var subtypePtr *string
	if subtype != "" {
		subtypePtr = &subtype
	}

	return &KoditEnrichmentData{
		Type: deref(e.Type),
		ID:   e.Id,
		Attributes: KoditEnrichmentAttributes{
			Type:      e.Attributes.Type,
			Subtype:   subtypePtr,
			Content:   e.Attributes.Content, // Full content, no truncation
			CreatedAt: extractTime(e.Attributes.CreatedAt),
			UpdatedAt: extractTime(e.Attributes.UpdatedAt),
		},
	}, nil
}


// GetRepositoryCommits fetches commits for a repository from Kodit
func (s *KoditService) GetRepositoryCommits(ctx context.Context, koditRepoID string, limit int) ([]map[string]any, error) {
	if !s.enabled {
		return nil, fmt.Errorf("kodit service not enabled")
	}

	// Build params with limit
	var params *kodit.ListRepositoryCommitsApiV1RepositoriesRepoIdCommitsGetParams
	if limit > 0 {
		paramsJSON, _ := json.Marshal(map[string]int{"limit": limit})
		params = &kodit.ListRepositoryCommitsApiV1RepositoriesRepoIdCommitsGetParams{}
		json.Unmarshal(paramsJSON, params)
	}

	resp, err := s.client.ListRepositoryCommitsApiV1RepositoriesRepoIdCommitsGet(ctx, koditRepoID, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list commits: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("kodit returned status %d: %s", resp.StatusCode, body)
	}

	var response struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return response.Data, nil
}

// GetRepositoryStatus fetches indexing status for a repository from Kodit
func (s *KoditService) GetRepositoryStatus(ctx context.Context, koditRepoID string) (map[string]any, error) {
	if !s.enabled {
		return nil, fmt.Errorf("kodit service not enabled")
	}

	var repoIDInt int
	if _, err := fmt.Sscanf(koditRepoID, "%d", &repoIDInt); err != nil {
		return nil, fmt.Errorf("invalid repository ID format (expected numeric): %w", err)
	}

	resp, err := s.client.GetIndexStatusApiV1RepositoriesRepoIdStatusGet(ctx, repoIDInt)
	if err != nil {
		return nil, fmt.Errorf("failed to get repository status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("kodit returned status %d: %s", resp.StatusCode, body)
	}

	var response map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return response, nil
}

// filterAndConvertEnrichments filters out internal summaries and converts to simplified types
func filterAndConvertEnrichments(enrichments []kodit.EnrichmentData) *KoditEnrichmentListResponse {
	const maxContentLength = 500
	result := &KoditEnrichmentListResponse{Data: make([]KoditEnrichmentData, 0, len(enrichments))}

	for _, e := range enrichments {
		subtype := extractString(e.Attributes.Subtype)

		// Skip internal summary types
		if subtype == "snippet_summary" || subtype == "example_summary" {
			continue
		}

		content := e.Attributes.Content
		if len(content) > maxContentLength {
			content = content[:maxContentLength] + "..."
		}

		var subtypePtr *string
		if subtype != "" {
			subtypePtr = &subtype
		}

		// Extract commit SHA from relationships
		commitSHA := extractCommitSHA(e.Relationships)

		result.Data = append(result.Data, KoditEnrichmentData{
			Type:      deref(e.Type),
			ID:        e.Id,
			CommitSHA: commitSHA,
			Attributes: KoditEnrichmentAttributes{
				Type:      e.Attributes.Type,
				Subtype:   subtypePtr,
				Content:   content,
				CreatedAt: extractTime(e.Attributes.CreatedAt),
				UpdatedAt: extractTime(e.Attributes.UpdatedAt),
			},
		})
	}

	return result
}

// extractCommitSHA extracts commit SHA from enrichment relationships
func extractCommitSHA(relationships *kodit.EnrichmentData_Relationships) string {
	if relationships == nil {
		log.Debug().Msg("extractCommitSHA: relationships is nil")
		return ""
	}

	// Marshal and unmarshal to access unexported union field
	relationshipsJSON, err := json.Marshal(relationships)
	if err != nil {
		log.Warn().Err(err).Msg("extractCommitSHA: failed to marshal relationships")
		return ""
	}

	log.Debug().RawJSON("relationships", relationshipsJSON).Msg("extractCommitSHA: relationships JSON")

	var relData map[string]interface{}
	if err := json.Unmarshal(relationshipsJSON, &relData); err != nil {
		log.Warn().Err(err).Msg("extractCommitSHA: failed to unmarshal relationships")
		return ""
	}

	// Look for associations -> data array -> find commit type
	if associations, ok := relData["associations"].(map[string]interface{}); ok {
		if data, ok := associations["data"].([]interface{}); ok {
			for _, item := range data {
				if assoc, ok := item.(map[string]interface{}); ok {
					if assocType, ok := assoc["type"].(string); ok && assocType == "commit" {
						if id, ok := assoc["id"].(string); ok {
							log.Debug().Str("commit_sha", id).Msg("extractCommitSHA: found commit SHA")
							return id // This is the commit SHA
						}
					}
				}
			}
		} else {
			log.Debug().Msg("extractCommitSHA: associations.data is not an array")
		}
	} else {
		log.Debug().Msg("extractCommitSHA: no associations found in relationships")
	}

	log.Debug().Msg("extractCommitSHA: no commit SHA found")
	return ""
}

func extractString(union *kodit.EnrichmentAttributes_Subtype) string {
	if union == nil {
		return ""
	}
	if s, err := union.AsEnrichmentAttributesSubtype0(); err == nil {
		return s
	}
	return ""
}

func extractTime(union any) time.Time {
	switch v := union.(type) {
	case *kodit.EnrichmentAttributes_CreatedAt:
		if t, err := v.AsEnrichmentAttributesCreatedAt0(); err == nil {
			return t
		}
	case *kodit.EnrichmentAttributes_UpdatedAt:
		if t, err := v.AsEnrichmentAttributesUpdatedAt0(); err == nil {
			return t
		}
	}
	return time.Time{}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
