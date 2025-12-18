package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	kodit "github.com/helixml/kodit/clients/go"
	"github.com/oapi-codegen/oapi-codegen/v2/pkg/securityprovider"
	"github.com/rs/zerolog/log"
)

// KoditError represents an error from the Kodit service with HTTP status code
type KoditError struct {
	StatusCode int
	Message    string
}

func (e *KoditError) Error() string {
	return fmt.Sprintf("kodit returned status %d: %s", e.StatusCode, e.Message)
}

// IsKoditNotFound returns true if the error is a Kodit 404 error
func IsKoditNotFound(err error) bool {
	var koditErr *KoditError
	if errors.As(err, &koditErr) {
		return koditErr.StatusCode == http.StatusNotFound
	}
	return false
}

// IsKoditError returns true if the error is a Kodit error and returns the status code
func IsKoditError(err error) (int, bool) {
	var koditErr *KoditError
	if errors.As(err, &koditErr) {
		return koditErr.StatusCode, true
	}
	return 0, false
}

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
	KoditRepositoryResponse     = kodit.RepositoryResponse
	KoditEnrichmentListResponse struct {
		Data []KoditEnrichmentData `json:"data"`
	}
	KoditEnrichmentData struct {
		Type       string                    `json:"type"`
		ID         string                    `json:"id"`
		Attributes KoditEnrichmentAttributes `json:"attributes"`
		CommitSHA  string                    `json:"commit_sha,omitempty"` // Added for frontend
	}
	KoditEnrichmentAttributes struct {
		Type      string    `json:"type"`
		Subtype   *string   `json:"subtype,omitempty"`
		Content   string    `json:"content"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}
)

// KoditIndexingStatus is an alias to the Kodit client type for repository status
type KoditIndexingStatus = kodit.RepositoryStatusSummaryResponse

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
		return nil, &KoditError{StatusCode: resp.StatusCode, Message: string(body)}
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
		return nil, &KoditError{StatusCode: resp.StatusCode, Message: string(body)}
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
		return nil, &KoditError{StatusCode: resp.StatusCode, Message: string(body)}
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
		return nil, &KoditError{StatusCode: resp.StatusCode, Message: string(body)}
	}

	var response struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return response.Data, nil
}

// SearchFilters represents the filters for kodit search API
type SearchFilters struct {
	Repositories []string `json:"repositories"`
	CommitSHA    []string `json:"commit_sha,omitempty"`
}

// RelationshipData represents a single relationship data item
type RelationshipData struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// RelationshipsAssociations represents the associations structure in relationships
type RelationshipsAssociations struct {
	Data []RelationshipData `json:"data"`
}

// RelationshipsWrapper represents the top-level relationships structure
type RelationshipsWrapper struct {
	Associations RelationshipsAssociations `json:"associations"`
}

// KoditSearchResult represents a search result with properly typed fields for frontend consumption
type KoditSearchResult struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Language string `json:"language"`
	Content  string `json:"content"`
	FilePath string `json:"file_path"` // File path from DerivesFrom
}

// SearchSnippets searches for code snippets in a repository from Kodit
func (s *KoditService) SearchSnippets(ctx context.Context, koditRepoID, query string, limit int, commitSHA string) ([]KoditSearchResult, error) {
	if !s.enabled {
		return nil, fmt.Errorf("kodit service not enabled")
	}

	if query == "" {
		return []KoditSearchResult{}, nil
	}

	// Build search request body
	searchType := "search"
	textAttr := &kodit.SearchAttributes_Text{}
	limitAttr := &kodit.SearchAttributes_Limit{}

	// Set text query using the proper method
	if err := textAttr.FromSearchAttributesText0(query); err != nil {
		return nil, fmt.Errorf("failed to set search text: %w", err)
	}

	// Set limit
	if limit <= 0 {
		limit = 20
	}
	if err := limitAttr.FromSearchAttributesLimit0(limit); err != nil {
		return nil, fmt.Errorf("failed to set search limit: %w", err)
	}

	requestBody := kodit.SearchRequest{
		Data: kodit.SearchData{
			Type: &searchType,
			Attributes: kodit.SearchAttributes{
				Text:  textAttr,
				Limit: limitAttr,
			},
		},
	}

	// Set repository filter using kodit.SearchFilters
	koditFilters := kodit.SearchFilters{}

	// Set sources (repositories) filter
	sourcesAttr := &kodit.SearchFilters_Sources{}
	if err := sourcesAttr.FromSearchFiltersSources0([]string{koditRepoID}); err != nil {
		return nil, fmt.Errorf("failed to set sources filter: %w", err)
	}
	koditFilters.Sources = sourcesAttr

	// Add commit SHA filter if provided
	if commitSHA != "" {
		commitShaAttr := &kodit.SearchFilters_CommitSha{}
		if err := commitShaAttr.FromSearchFiltersCommitSha0([]string{commitSHA}); err != nil {
			return nil, fmt.Errorf("failed to set commit_sha filter: %w", err)
		}
		koditFilters.CommitSha = commitShaAttr
	}

	// Set the filters on the request using the proper method
	filtersAttr := &kodit.SearchAttributes_Filters{}
	if err := filtersAttr.FromSearchFilters(koditFilters); err != nil {
		return nil, fmt.Errorf("failed to set filters: %w", err)
	}
	requestBody.Data.Attributes.Filters = filtersAttr

	// Log request body for debugging
	reqBodyDebug, _ := json.Marshal(requestBody)
	log.Debug().Str("request_body", string(reqBodyDebug)).Str("query", query).Msg("Sending search request to Kodit")

	resp, err := s.client.SearchSnippetsApiV1SearchPost(ctx, requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to search snippets: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, &KoditError{StatusCode: resp.StatusCode, Message: string(body)}
	}

	var response kodit.SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Convert to simplified result format
	results := make([]KoditSearchResult, 0, len(response.Data))
	for _, snippet := range response.Data {
		// Extract file path from DerivesFrom if available
		filePath := ""
		if len(snippet.Attributes.DerivesFrom) > 0 {
			filePath = snippet.Attributes.DerivesFrom[0].Path
		}

		results = append(results, KoditSearchResult{
			ID:       snippet.Id,
			Type:     snippet.Type,
			Language: snippet.Attributes.Content.Language,
			Content:  snippet.Attributes.Content.Value,
			FilePath: filePath,
		})
	}

	return results, nil
}

// GetRepositoryStatus fetches indexing status for a repository from Kodit
func (s *KoditService) GetRepositoryStatus(ctx context.Context, koditRepoID string) (*KoditIndexingStatus, error) {
	if !s.enabled {
		return nil, fmt.Errorf("kodit service not enabled")
	}

	var repoIDInt int
	if _, err := fmt.Sscanf(koditRepoID, "%d", &repoIDInt); err != nil {
		return nil, fmt.Errorf("invalid repository ID format (expected numeric): %w", err)
	}

	resp, err := s.client.GetStatusSummaryApiV1RepositoriesRepoIdStatusSummaryGet(ctx, repoIDInt)
	if err != nil {
		return nil, fmt.Errorf("failed to get repository status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, &KoditError{StatusCode: resp.StatusCode, Message: string(body)}
	}

	parsed, err := kodit.ParseGetStatusSummaryApiV1RepositoriesRepoIdStatusSummaryGetResponse(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse status response: %w", err)
	}

	if parsed.JSON200 == nil {
		return nil, fmt.Errorf("unexpected nil response from kodit status endpoint")
	}

	return parsed.JSON200, nil
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

	var relData RelationshipsWrapper
	if err := json.Unmarshal(relationshipsJSON, &relData); err != nil {
		log.Warn().Err(err).Msg("extractCommitSHA: failed to unmarshal relationships")
		return ""
	}

	// Look for associations -> data array -> find commit type
	for _, item := range relData.Associations.Data {
		if item.Type == "commit" {
			log.Debug().Str("commit_sha", item.ID).Msg("extractCommitSHA: found commit SHA")
			return item.ID // This is the commit SHA
		}
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
	if union == nil {
		return time.Time{}
	}
	switch v := union.(type) {
	case *kodit.EnrichmentAttributes_CreatedAt:
		if v == nil {
			return time.Time{}
		}
		if t, err := v.AsEnrichmentAttributesCreatedAt0(); err == nil {
			return t
		}
	case *kodit.EnrichmentAttributes_UpdatedAt:
		if v == nil {
			return time.Time{}
		}
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
