package anthropic

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	// VertexAnthropicVersion is injected into request bodies when not already present.
	// This matches the Anthropic SDK's vertex package constant.
	VertexAnthropicVersion = "vertex-2023-10-16"

	// vertexAuthScope is the OAuth2 scope needed for Vertex AI API calls.
	vertexAuthScope = "https://www.googleapis.com/auth/cloud-platform"
)

// VertexBaseURL returns the Vertex AI base URL for the given region.
// "global" uses https://aiplatform.googleapis.com/, otherwise https://{region}-aiplatform.googleapis.com/.
func VertexBaseURL(region string) string {
	if region == "global" {
		return "https://aiplatform.googleapis.com/"
	}
	return fmt.Sprintf("https://%s-aiplatform.googleapis.com/", region)
}

// tokenSourceCache caches oauth2.TokenSource instances keyed by a hash of the
// credentials material. This allows DB-configured endpoints with different
// credentials to each have their own auto-refreshing token source.
type tokenSourceCache struct {
	mu    sync.Mutex
	cache map[string]oauth2.TokenSource
}

func newTokenSourceCache() *tokenSourceCache {
	return &tokenSourceCache{
		cache: make(map[string]oauth2.TokenSource),
	}
}

// getOrCreate returns a cached TokenSource for the given credentials,
// or creates a new one if not cached.
func (c *tokenSourceCache) getOrCreate(ctx context.Context, credentialsJSON, credentialsFile string) (oauth2.TokenSource, error) {
	key := tokenSourceCacheKey(credentialsJSON, credentialsFile)

	c.mu.Lock()
	defer c.mu.Unlock()

	if ts, ok := c.cache[key]; ok {
		return ts, nil
	}

	ts, err := newTokenSource(ctx, credentialsJSON, credentialsFile)
	if err != nil {
		return nil, err
	}

	c.cache[key] = ts
	return ts, nil
}

// tokenSourceCacheKey produces a stable cache key from credentials material.
// JSON content is hashed so we don't store large blobs as map keys.
// File paths are used directly (prefixed to avoid collisions).
func tokenSourceCacheKey(credentialsJSON, credentialsFile string) string {
	if credentialsJSON != "" {
		h := sha256.Sum256([]byte(credentialsJSON))
		return fmt.Sprintf("json:%x", h[:8])
	}
	if credentialsFile != "" {
		return "file:" + credentialsFile
	}
	return "adc"
}

// newTokenSource creates a Google OAuth2 TokenSource.
//
// Priority:
//  1. credentialsJSON — service account JSON provided as a string (preferred for containers)
//  2. credentialsFile — path to a service account JSON file on disk
//  3. Application Default Credentials
func newTokenSource(ctx context.Context, credentialsJSON, credentialsFile string) (oauth2.TokenSource, error) {
	if credentialsJSON != "" {
		creds, err := google.CredentialsFromJSON(ctx, []byte(credentialsJSON), vertexAuthScope)
		if err != nil {
			return nil, fmt.Errorf("failed to parse Vertex credentials from JSON string: %w", err)
		}
		log.Info().Msg("initialized Vertex AI token source from credentials JSON string")
		return creds.TokenSource, nil
	}

	if credentialsFile != "" {
		data, err := os.ReadFile(credentialsFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read Vertex credentials file %q: %w", credentialsFile, err)
		}
		creds, err := google.CredentialsFromJSON(ctx, data, vertexAuthScope)
		if err != nil {
			return nil, fmt.Errorf("failed to parse Vertex credentials from %q: %w", credentialsFile, err)
		}
		log.Info().
			Str("credentials_file", credentialsFile).
			Msg("initialized Vertex AI token source from credentials file")
		return creds.TokenSource, nil
	}

	// Fall back to Application Default Credentials
	creds, err := google.FindDefaultCredentials(ctx, vertexAuthScope)
	if err != nil {
		return nil, fmt.Errorf("failed to find Google Application Default Credentials for Vertex AI: %w", err)
	}
	log.Info().Msg("initialized Vertex AI token source from Application Default Credentials")
	return creds.TokenSource, nil
}

// vertexTransformRequest rewrites an Anthropic API request for Vertex AI.
// It modifies the request in place:
//   - Rewrites the URL path from /v1/messages to Vertex's rawPredict/streamRawPredict format
//   - Extracts "model" from the JSON body and removes it (Vertex puts model in the URL)
//   - Injects "anthropic_version" into the body if not present
//   - Sets the Authorization header with a Bearer token from the given TokenSource
//   - Removes x-api-key header (Vertex uses OAuth2, not API keys)
//
// This replicates the logic from the Anthropic SDK's vertex middleware, which is
// unexported and designed for the SDK's own client pipeline rather than a reverse proxy.
func vertexTransformRequest(r *http.Request, projectID, region string, tokenSource oauth2.TokenSource) error {
	// Get a valid OAuth2 token (auto-refreshes if expired)
	token, err := tokenSource.Token()
	if err != nil {
		return fmt.Errorf("failed to get Vertex AI OAuth2 token: %w", err)
	}

	// Set auth headers — Vertex uses Bearer tokens, not x-api-key
	r.Header.Del("x-api-key")
	r.Header.Del("api-key")
	r.Header.Set("Authorization", "Bearer "+token.AccessToken)

	// Always set the Vertex host/scheme so we never end up with an empty URL
	baseURL := VertexBaseURL(region)
	u, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("failed to parse Vertex base URL %q: %w", baseURL, err)
	}
	r.URL.Host = u.Host
	r.URL.Scheme = u.Scheme
	r.Host = u.Host

	// Read the body if present (POST requests for /v1/messages)
	if r.Body == nil {
		return nil
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("failed to read request body for Vertex transform: %w", err)
	}
	r.Body.Close()

	if len(body) == 0 {
		return nil
	}

	// Parse body as JSON to extract/modify fields
	var bodyMap map[string]json.RawMessage
	if err := json.Unmarshal(body, &bodyMap); err != nil {
		// Not valid JSON — just forward as-is with auth headers set
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
		return nil
	}

	// Inject anthropic_version if not present
	if _, exists := bodyMap["anthropic_version"]; !exists {
		versionBytes, _ := json.Marshal(VertexAnthropicVersion)
		bodyMap["anthropic_version"] = versionBytes
	}

	// Extract model name for URL construction
	var modelName string
	if modelRaw, exists := bodyMap["model"]; exists {
		if err := json.Unmarshal(modelRaw, &modelName); err == nil && modelName != "" {
			// Remove model from body — Vertex puts it in the URL
			delete(bodyMap, "model")
			// Convert Anthropic model ID format to Vertex format:
			// claude-sonnet-4-20250514 → claude-sonnet-4@20250514
			// Vertex requires @ between model name and date version
			modelName = convertModelNameToVertex(modelName)
		}
	}

	// Check if streaming
	var stream bool
	if streamRaw, exists := bodyMap["stream"]; exists {
		_ = json.Unmarshal(streamRaw, &stream)
	}

	// Rewrite URL path for /v1/messages
	if r.URL.Path == "/v1/messages" && r.Method == http.MethodPost && modelName != "" {
		specifier := "rawPredict"
		if stream {
			specifier = "streamRawPredict"
		}
		r.URL.Path = fmt.Sprintf("/v1/projects/%s/locations/%s/publishers/anthropic/models/%s:%s",
			projectID, region, modelName, specifier)
	}

	// Rewrite URL path for /v1/messages/count_tokens
	if r.URL.Path == "/v1/messages/count_tokens" && r.Method == http.MethodPost {
		r.URL.Path = fmt.Sprintf("/v1/projects/%s/locations/%s/publishers/anthropic/models/count-tokens:rawPredict",
			projectID, region)
	}

	// Re-serialize the modified body
	newBody, err := json.Marshal(bodyMap)
	if err != nil {
		return fmt.Errorf("failed to re-serialize request body after Vertex transform: %w", err)
	}

	r.Body = io.NopCloser(bytes.NewReader(newBody))
	r.ContentLength = int64(len(newBody))
	r.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(newBody)), nil
	}

	return nil
}

// convertModelNameToVertex converts Anthropic API model IDs to Vertex AI format.
// Anthropic uses dashes (claude-sonnet-4-20250514), Vertex uses @ (claude-sonnet-4@20250514).
// If the model name already contains @, it's returned as-is.
func convertModelNameToVertex(modelName string) string {
	if strings.Contains(modelName, "@") {
		return modelName
	}
	parts := strings.Split(modelName, "-")
	if len(parts) >= 2 {
		lastPart := parts[len(parts)-1]
		if len(lastPart) == 8 && isAllDigits(lastPart) {
			// Dated model: claude-sonnet-4-20250514 → claude-sonnet-4@20250514
			return strings.Join(parts[:len(parts)-1], "-") + "@" + lastPart
		}
	}
	// Versionless model (e.g. claude-sonnet-4-6, claude-opus-4-6):
	// Vertex requires a version suffix with @ separator
	if strings.HasPrefix(modelName, "claude-") {
		if strings.HasSuffix(modelName, "-latest") {
			// claude-sonnet-4-6-latest → claude-sonnet-4-6@latest
			return strings.TrimSuffix(modelName, "-latest") + "@latest"
		}
		return modelName + "@latest"
	}
	return modelName
}

func isAllDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
