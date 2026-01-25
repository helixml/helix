package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"golang.org/x/oauth2/google"
)

// VertexAIConfig holds configuration for Vertex AI Claude access
type VertexAIConfig struct {
	ProjectID          string // Google Cloud project ID
	Region             string // Region (e.g., "us-east1", "global")
	ServiceAccountJSON string // Service account JSON credentials
}

// VertexAITransformer handles request transformation for Vertex AI
type VertexAITransformer struct {
	tokenCache     map[string]*cachedToken
	tokenCacheLock sync.RWMutex
}

type cachedToken struct {
	token     string
	expiresAt time.Time
}

// NewVertexAITransformer creates a new Vertex AI transformer
func NewVertexAITransformer() *VertexAITransformer {
	return &VertexAITransformer{
		tokenCache: make(map[string]*cachedToken),
	}
}

// IsVertexAIEndpoint checks if the endpoint is configured for Vertex AI
func IsVertexAIEndpoint(endpoint *types.ProviderEndpoint) bool {
	return endpoint.Provider == types.ProviderVertexAI ||
		strings.Contains(endpoint.BaseURL, "aiplatform.googleapis.com")
}

// TransformRequest transforms an Anthropic API request for Vertex AI
// It modifies the request body to:
// 1. Remove the "model" field (it goes in the URL)
// 2. Add "anthropic_version": "vertex-2023-10-16"
func TransformRequestForVertexAI(body []byte) ([]byte, string, error) {
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, "", fmt.Errorf("failed to parse request body: %w", err)
	}

	// Extract and remove model from request body
	modelID, _ := req["model"].(string)
	delete(req, "model")

	// Add Vertex AI specific version
	req["anthropic_version"] = "vertex-2023-10-16"

	transformed, err := json.Marshal(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal transformed request: %w", err)
	}

	return transformed, modelID, nil
}

// GetVertexAIURL constructs the Vertex AI endpoint URL
// Format: https://{LOCATION}-aiplatform.googleapis.com/v1/projects/{PROJECT_ID}/locations/{LOCATION}/publishers/anthropic/models/{MODEL_ID}:streamRawPredict
func GetVertexAIURL(projectID, region, modelID string, streaming bool) string {
	action := "rawPredict"
	if streaming {
		action = "streamRawPredict"
	}

	// Ensure model ID has proper versioning (required by Vertex)
	// If no version suffix, log a warning
	if !strings.Contains(modelID, "@") {
		log.Warn().
			Str("model", modelID).
			Msg("Vertex AI model ID should include version suffix (e.g., claude-sonnet-4@20250514)")
	}

	return fmt.Sprintf(
		"https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/anthropic/models/%s:%s",
		region,
		projectID,
		region,
		modelID,
		action,
	)
}

// GetAccessToken returns a Google Cloud access token for the service account
func (v *VertexAITransformer) GetAccessToken(ctx context.Context, serviceAccountJSON string) (string, error) {
	// Check cache first
	v.tokenCacheLock.RLock()
	cached, ok := v.tokenCache[serviceAccountJSON]
	v.tokenCacheLock.RUnlock()

	if ok && time.Now().Before(cached.expiresAt.Add(-5*time.Minute)) {
		return cached.token, nil
	}

	// Get new token
	creds, err := google.CredentialsFromJSON(ctx, []byte(serviceAccountJSON),
		"https://www.googleapis.com/auth/cloud-platform",
	)
	if err != nil {
		return "", fmt.Errorf("failed to parse service account credentials: %w", err)
	}

	token, err := creds.TokenSource.Token()
	if err != nil {
		return "", fmt.Errorf("failed to get access token: %w", err)
	}

	// Cache the token
	v.tokenCacheLock.Lock()
	v.tokenCache[serviceAccountJSON] = &cachedToken{
		token:     token.AccessToken,
		expiresAt: token.Expiry,
	}
	v.tokenCacheLock.Unlock()

	return token.AccessToken, nil
}

// ParseVertexAIConfig extracts Vertex AI configuration from a ProviderEndpoint
// The endpoint should have:
// - VertexProjectID: Google Cloud project ID
// - VertexRegion: Region (defaults to "global")
// - APIKey: Service account JSON credentials
func ParseVertexAIConfig(endpoint *types.ProviderEndpoint) (*VertexAIConfig, error) {
	config := &VertexAIConfig{
		ProjectID:          endpoint.VertexProjectID,
		Region:             endpoint.VertexRegion,
		ServiceAccountJSON: endpoint.APIKey,
	}

	// Default region to "global" if not specified
	if config.Region == "" {
		config.Region = "global"
	}

	if config.ProjectID == "" {
		return nil, fmt.Errorf("vertex_project_id is required for Vertex AI provider")
	}

	if config.ServiceAccountJSON == "" {
		return nil, fmt.Errorf("api_key (service account JSON) is required for Vertex AI provider")
	}

	return config, nil
}

// IsStreamingRequest checks if the request body indicates a streaming request
func IsStreamingRequest(body []byte) bool {
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		return false
	}
	stream, ok := req["stream"].(bool)
	return ok && stream
}

// ModifyRequestForVertexAI transforms an HTTP request for Vertex AI
// This should be called from the proxy's Director function when the endpoint is Vertex AI
func (v *VertexAITransformer) ModifyRequestForVertexAI(r *http.Request, endpoint *types.ProviderEndpoint) error {
	ctx := r.Context()

	// Parse Vertex AI config from endpoint
	config, err := ParseVertexAIConfig(endpoint)
	if err != nil {
		return fmt.Errorf("failed to parse Vertex AI config: %w", err)
	}

	// Read and transform request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("failed to read request body: %w", err)
	}
	r.Body.Close()

	transformedBody, modelID, err := TransformRequestForVertexAI(body)
	if err != nil {
		return fmt.Errorf("failed to transform request: %w", err)
	}

	// If no model ID in body, use a default or error
	if modelID == "" {
		return fmt.Errorf("model ID is required in request body")
	}

	// Determine if streaming
	streaming := IsStreamingRequest(body)

	// Build Vertex AI URL
	vertexURL := GetVertexAIURL(config.ProjectID, config.Region, modelID, streaming)

	// Get access token
	accessToken, err := v.GetAccessToken(ctx, config.ServiceAccountJSON)
	if err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}

	// Update request URL
	r.URL.Scheme = "https"
	r.URL.Host = fmt.Sprintf("%s-aiplatform.googleapis.com", config.Region)
	r.URL.Path = fmt.Sprintf("/v1/projects/%s/locations/%s/publishers/anthropic/models/%s:%s",
		config.ProjectID, config.Region, modelID, map[bool]string{true: "streamRawPredict", false: "rawPredict"}[streaming])
	r.Host = r.URL.Host

	// Set authorization header
	r.Header.Set("Authorization", "Bearer "+accessToken)
	r.Header.Set("Content-Type", "application/json")

	// Remove Anthropic-specific headers
	r.Header.Del("x-api-key")
	r.Header.Del("anthropic-version")

	// Set new body
	r.Body = io.NopCloser(bytes.NewReader(transformedBody))
	r.ContentLength = int64(len(transformedBody))

	log.Debug().
		Str("project_id", config.ProjectID).
		Str("region", config.Region).
		Str("model", modelID).
		Bool("streaming", streaming).
		Str("url", vertexURL).
		Msg("Transformed request for Vertex AI")

	return nil
}
