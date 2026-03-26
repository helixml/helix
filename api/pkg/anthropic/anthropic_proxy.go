package anthropic

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/model"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/logger"
	"github.com/helixml/helix/api/pkg/pricing"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"

	anthropic "github.com/anthropics/anthropic-sdk-go" // imported as anthropic
	"github.com/rs/zerolog/log"
	"golang.org/x/oauth2"
)

type Proxy struct {
	cfg                   *config.ServerConfig
	store                 store.Store
	anthropicReverseProxy *httputil.ReverseProxy
	logStores             []logger.LogStore
	billingLogger         logger.LogStore
	modelInfoProvider     model.ModelInfoProvider
	wg                    sync.WaitGroup

	// Vertex AI support
	vertexTokenSources *tokenSourceCache  // per-credentials-file token source cache
	defaultTokenSource oauth2.TokenSource // for env-var-configured built-in provider (nil if not configured)
}

func New(cfg *config.ServerConfig, store store.Store, modelInfoProvider model.ModelInfoProvider, logStores ...logger.LogStore) *Proxy {

	reverseProxy := httputil.NewSingleHostReverseProxy(nil)
	// FlushInterval -1 enables immediate flushing of each write to the client.
	// Without this, httputil.ReverseProxy buffers the response, which breaks
	// SSE streaming — especially in double-proxy setups (sandbox API → outer API
	// → Anthropic) where tool_use content blocks get truncated because the
	// buffered data never reaches the downstream reader in time.
	reverseProxy.FlushInterval = -1

	p := &Proxy{
		cfg:                   cfg,
		store:                 store,
		anthropicReverseProxy: reverseProxy,
		logStores:             logStores,
		modelInfoProvider:     modelInfoProvider,
		wg:                    sync.WaitGroup{},
		vertexTokenSources:    newTokenSourceCache(),
	}

	// Initialize default Vertex token source if configured via env vars
	if cfg.Providers.Anthropic.VertexProjectID != "" {
		ts, err := newTokenSource(context.Background(), cfg.Providers.Anthropic.VertexCredentialsJSON, cfg.Providers.Anthropic.VertexCredentialsFile)
		if err != nil {
			log.Error().Err(err).
				Str("project_id", cfg.Providers.Anthropic.VertexProjectID).
				Str("region", cfg.Providers.Anthropic.VertexRegion).
				Msg("failed to initialize Vertex AI token source — Vertex will not be available")
		} else {
			p.defaultTokenSource = ts
			log.Info().
				Str("project_id", cfg.Providers.Anthropic.VertexProjectID).
				Str("region", cfg.Providers.Anthropic.VertexRegion).
				Msg("Vertex AI configured for Anthropic proxy — all inference traffic will route through Vertex")
		}
	}

	billingLogger, err := logger.NewBillingLogger(store, cfg.Stripe.BillingEnabled)
	if err != nil {
		log.Error().Err(err).Msg("failed to initialize billing logger")
	} else {
		p.billingLogger = billingLogger
	}

	// Configure TLS skip verify if enabled
	if cfg.Tools.TLSSkipVerify {
		// Clone the default transport to preserve all default settings (timeouts, connection pooling, etc.)
		// then add InsecureSkipVerify
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
		p.anthropicReverseProxy.Transport = transport
		log.Info().
			Bool("tls_skip_verify", true).
			Msg("Anthropic proxy configured with TLS skip verify (TOOLS_TLS_SKIP_VERIFY=true) - will accept any TLS certificate")
	} else {
		log.Debug().
			Bool("tls_skip_verify", false).
			Msg("Anthropic proxy using default TLS verification (set TOOLS_TLS_SKIP_VERIFY=true in .env or extraEnv for enterprise deployments)")
	}

	p.anthropicReverseProxy.ModifyResponse = p.anthropicAPIProxyModifyResponse
	p.anthropicReverseProxy.Director = p.anthropicAPIProxyDirector

	// Add error handler to log TLS errors clearly
	p.anthropicReverseProxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		errStr := err.Error()
		if strings.Contains(errStr, "x509") || strings.Contains(errStr, "certificate") || strings.Contains(errStr, "tls:") {
			log.Error().
				Err(err).
				Str("url", r.URL.String()).
				Msg("ANTHROPIC PROXY TLS CERTIFICATE ERROR - Set TOOLS_TLS_SKIP_VERIFY=true (in .env for Docker Compose, or extraEnv in Helm chart) for enterprise/internal TLS certificates")
		} else {
			log.Error().
				Err(err).
				Str("url", r.URL.String()).
				Msg("Anthropic proxy error")
		}
		w.WriteHeader(http.StatusBadGateway)
	}

	return p
}

func (s *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	endpoint := GetRequestProviderEndpoint(r.Context())
	if endpoint == nil {
		log.Error().Msg("provider endpoint not found in context")
		return
	}

	// Note: Don't check r.Body == nil here - GET requests (like /v1/models) have no body
	// but still need to be proxied

	r = setStartTime(r, time.Now())

	s.anthropicReverseProxy.ServeHTTP(w, r)
}

func (s *Proxy) anthropicAPIProxyDirector(r *http.Request) {
	endpoint := GetRequestProviderEndpoint(r.Context())
	if endpoint == nil {
		log.Error().Msg("provider endpoint not found in context")
		return
	}

	// Remove incoming auth headers (user's Helix token, not the real provider API key)
	r.Header.Del("Authorization")
	r.Header.Del("x-api-key")
	r.Header.Del("api-key")

	// Vertex AI mode: if the endpoint has a VertexProjectID, transform the request
	// for Vertex's rawPredict/streamRawPredict API format.
	// Exception: /v1/models goes to the direct Anthropic API since Vertex has no
	// model listing endpoint. This requires an API key to be configured alongside Vertex.
	if endpoint.VertexProjectID != "" && r.URL.Path != "/v1/models" {
		tokenSource, err := s.getVertexTokenSource(r.Context(), endpoint.VertexCredentialsJSON, endpoint.VertexCredentialsFile)
		if err != nil {
			log.Error().Err(err).
				Str("vertex_project_id", endpoint.VertexProjectID).
				Msg("failed to get Vertex AI token source")
			return
		}

		if err := vertexTransformRequest(r, endpoint.VertexProjectID, endpoint.VertexRegion, tokenSource); err != nil {
			log.Error().Err(err).
				Str("vertex_project_id", endpoint.VertexProjectID).
				Str("vertex_region", endpoint.VertexRegion).
				Msg("failed to transform request for Vertex AI")
			return
		}

		log.Debug().
			Str("vertex_project_id", endpoint.VertexProjectID).
			Str("vertex_region", endpoint.VertexRegion).
			Str("url", r.URL.String()).
			Msg("request transformed for Vertex AI")
		return
	}

	// Direct Anthropic API mode: rewrite URL and set x-api-key
	// Note: Don't check r.Body == nil here - GET requests (like /v1/models) have no body

	u, err := url.Parse(endpoint.BaseURL)
	if err != nil {
		log.Error().Err(err).Msg("failed to parse provider endpoint URL")
		return
	}
	r.URL.Host = u.Host
	r.URL.Scheme = u.Scheme

	r.Host = u.Host

	// Set headers from provider endpoint (may include x-api-key)
	for key, value := range endpoint.Headers {
		r.Header.Set(key, value)
	}

	// If x-api-key not explicitly set in Headers, use endpoint.APIKey
	// This allows Anthropic providers to use the standard APIKey field
	// instead of requiring manual Headers["x-api-key"] configuration
	if r.Header.Get("x-api-key") == "" && endpoint.APIKey != "" {
		r.Header.Set("x-api-key", endpoint.APIKey)
	}
}

// getVertexTokenSource returns the appropriate OAuth2 token source for a Vertex request.
// For the built-in provider (matching env var config), it returns the default token source
// initialized at startup. For DB-configured endpoints, it lazily creates and caches a
// token source per credentials material.
func (s *Proxy) getVertexTokenSource(ctx context.Context, credentialsJSON, credentialsFile string) (oauth2.TokenSource, error) {
	// If this matches the built-in provider's credentials, use the default token source
	if s.defaultTokenSource != nil &&
		credentialsJSON == s.cfg.Providers.Anthropic.VertexCredentialsJSON &&
		credentialsFile == s.cfg.Providers.Anthropic.VertexCredentialsFile {
		return s.defaultTokenSource, nil
	}

	// For DB-configured endpoints, use the cache
	return s.vertexTokenSources.getOrCreate(ctx, credentialsJSON, credentialsFile)
}

// anthropicAPIProxyModifyResponse - parses the response
func (s *Proxy) anthropicAPIProxyModifyResponse(response *http.Response) error {

	// If content type is "text/event-stream", then process the stream
	if strings.Contains(response.Header.Get("Content-Type"), "text/event-stream") {
		// Log incoming response headers for debugging double-proxy truncation
		log.Info().
			Str("content_type", response.Header.Get("Content-Type")).
			Str("content_length", response.Header.Get("Content-Length")).
			Str("transfer_encoding", response.Header.Get("Transfer-Encoding")).
			Int64("content_length_parsed", response.ContentLength).
			Int("status_code", response.StatusCode).
			Msg("[SSE_PROXY] ModifyResponse called for SSE stream")

		// In double-proxy setups (sandbox API → outer API → Anthropic), the
		// outer proxy's ModifyResponse replaces the body with an io.Pipe but
		// may preserve the original Content-Length from Anthropic. The inner
		// proxy's httputil.ReverseProxy then stops reading after Content-Length
		// bytes, truncating the stream mid-event (typically at the tool_use
		// content_block_start). Fix: delete Content-Length and force chunked
		// transfer encoding so the full stream flows through.
		response.Header.Del("Content-Length")
		response.ContentLength = -1
		response.Header.Set("Transfer-Encoding", "chunked")

		pr, pw := io.Pipe()
		body := response.Body
		response.Body = pr

		// Process streaming response and assemble the complete message
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Error().Msgf("Recovered from panic: %v", r)
				}
			}()

			start, ok := getStartTime(response.Request)
			if !ok {
				log.Error().Msg("failed to get start time")
				return
			}

			// We will use this to store the response message
			// that we assemble from the chunks
			var respMessage anthropic.Message

			defer func() {
				log.Info().Msg("[SSE_PROXY] goroutine exiting, closing pipe writer")
				pw.Close()
			}()
			defer body.Close()

			lineNum := 0
			totalBytes := 0
			reader := bufio.NewReader(body)
			for {
				line, err := reader.ReadBytes('\n')
				lineNum++
				totalBytes += len(line)
				if err != nil {
					log.Info().
						Err(err).
						Int("line_num", lineNum).
						Int("partial_len", len(line)).
						Int("total_bytes_read", totalBytes).
						Str("partial_data", string(line)).
						Msg("[SSE_PROXY] read error (EOF or failure)")
					// Write any partial line data that came with the EOF
					if len(line) > 0 {
						_, writeErr := pw.Write(line)
						if writeErr != nil {
							log.Error().Err(writeErr).Msg("[SSE_PROXY] pipe write error on partial line")
						}
					}
					// Log the final assembled message for billing
					// Always log, even if no content (for debugging)
					bts, err := json.Marshal(respMessage)
					if err != nil {
						log.Error().Err(err).Any("resp_message", respMessage).Msg("failed to marshal resp message")
					} else {
						s.logLLMCall(response.Request.Context(), time.Now(), bts, nil, true, time.Since(start).Milliseconds())
					}
					return
				}

				// Forward the line to the client FIRST, before parsing
				_, writeErr := pw.Write(line)
				if writeErr != nil {
					log.Error().
						Err(writeErr).
						Int("line_num", lineNum).
						Msg("[SSE_PROXY] pipe write error - downstream closed?")
					// Downstream closed, stop reading
					return
				}

				if lineNum <= 5 || lineNum%10 == 0 {
					log.Debug().
						Int("line_num", lineNum).
						Int("line_len", len(line)).
						Int("total_bytes", totalBytes).
						Msg("[SSE_PROXY] forwarded line")
				}

				// Parse SSE line
				lineStr := string(line)

				if strings.HasPrefix(lineStr, "data: ") {
					data := strings.TrimPrefix(lineStr, "data: ")
					data = strings.TrimSpace(data)

					// Skip empty data and [DONE] markers
					if data == "" || data == "[DONE]" {
						log.Debug().Str("data", data).Msg("skipping empty or done marker")
						continue
					}

					// Parse the JSON data as a streaming event
					var event anthropic.MessageStreamEventUnion
					if err := event.UnmarshalJSON([]byte(data)); err != nil {
						log.Debug().Err(err).Str("data", data).Msg("failed to parse streaming event")
						continue
					}

					err = respMessage.Accumulate(event)
					if err != nil {
						log.Error().Err(err).Msg("failed to accumulate event")
						continue
					}
				}
			}
		}()
		return nil
	}

	if response.Header.Get("Content-Encoding") == "gzip" {
		gr, err := gzip.NewReader(response.Body)
		if err != nil {
			log.Error().Err(err).Msg("failed to create gzip reader for response")
			return err
		}
		defer gr.Close()
		response.Body = io.NopCloser(gr)
		response.Header.Del("Content-Encoding")
		response.Header.Del("Content-Length")
		response.ContentLength = -1
	}

	buf, err := io.ReadAll(response.Body)
	if err != nil {
		log.Error().Err(err).Msg("failed to read response body")
		return err
	}

	// Put the response buffer back as the response body
	response.Body = io.NopCloser(bytes.NewBuffer(buf))

	// Remove the cancel function from the context, we will
	// be using this to store the LLM call in the database
	ctx := context.WithoutCancel(response.Request.Context())

	s.wg.Add(1)

	go func() {
		defer s.wg.Done()

		defer func() {
			if r := recover(); r != nil {
				log.Error().Msgf("Recovered from panic: %v", r)
			}
		}()

		start, ok := getStartTime(response.Request)
		if !ok {
			log.Error().Msg("failed to get start time")
			return
		}

		// Log LLM call
		s.logLLMCall(ctx, time.Now(), buf, nil, false, time.Since(start).Milliseconds())
	}()

	return nil
}

func (s *Proxy) logLLMCall(ctx context.Context, createdAt time.Time, resp []byte, apiError error, stream bool, durationMs int64) {

	provider := GetRequestProviderEndpoint(ctx)

	var respMessage anthropic.Message
	if err := respMessage.UnmarshalJSON(resp); err != nil {
		log.Error().Err(err).Msg("failed to parse anthropic response for usage")
		return
	}

	log.Debug().Interface("usage", respMessage.Usage).Msg("anthropic usage information")

	usage := respMessage.Usage
	modelName := string(respMessage.Model)

	vals, ok := oai.GetContextValues(ctx)
	if !ok {
		// Session data will be missing for Discord, Slack, etc.
		log.Debug().Msg("failed to get context values")
		vals = &oai.ContextValues{}
	}

	orgID, _ := oai.GetContextOrganizationID(ctx)

	var (
		promptCost     float64
		completionCost float64
		totalCost      float64
	)

	// Get pricing info for the model
	modelInfo, err := s.getModelInfo(ctx, provider.BaseURL, provider.Name, modelName)
	if err == nil {
		// Calculate the cost for the call and persist it
		promptCost, completionCost, err = pricing.CalculateTokenPrice(modelInfo, usage.InputTokens, usage.OutputTokens)
		if err != nil {
			log.Error().
				Err(err).
				Str("user_id", vals.OwnerID).
				Str("model", modelName).
				Str("provider", provider.Name).
				Err(err).Msg("failed to calculate token price")
		}
		totalCost = promptCost + completionCost
	}

	log.Info().
		Str("owner_id", vals.OwnerID).
		Str("organization_id", orgID).
		Str("project_id", vals.ProjectID).
		Str("spec_task_id", vals.SpecTaskID).
		Str("model", modelName).
		Str("provider", provider.Name).
		Int("prompt_tokens", int(usage.InputTokens)).
		Int("completion_tokens", int(usage.OutputTokens)).
		Int("total_tokens", int(usage.InputTokens+usage.OutputTokens)).
		Float64("prompt_cost", promptCost).
		Float64("completion_cost", completionCost).
		Float64("total_cost", totalCost).
		Msg("logging LLM call")

	llmCall := &types.LLMCall{
		Created:          createdAt,
		SessionID:        vals.SessionID,
		InteractionID:    vals.InteractionID,
		OrganizationID:   orgID,
		ProjectID:        vals.ProjectID,
		SpecTaskID:       vals.SpecTaskID,
		Model:            modelName,
		OriginalRequest:  vals.OriginalRequest,
		Request:          vals.OriginalRequest,
		Response:         resp,
		Provider:         provider.ID,
		DurationMs:       durationMs,
		PromptTokens:     usage.InputTokens,
		CompletionTokens: usage.OutputTokens,
		TotalTokens:      usage.InputTokens + usage.OutputTokens,
		PromptCost:       promptCost,
		CompletionCost:   completionCost,
		TotalCost:        totalCost,
		UserID:           vals.OwnerID,
		Stream:           stream,
	}

	if apiError != nil {
		llmCall.Error = apiError.Error()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	for _, logStore := range s.logStores {
		_, err = logStore.CreateLLMCall(ctx, llmCall)
		if err != nil {
			log.Error().Err(err).Msg("failed to log LLM call")
		}
	}

	if s.billingLogger != nil {
		_, err = s.billingLogger.CreateLLMCall(ctx, llmCall)
		if err != nil {
			log.Error().Err(err).Msg("failed to log LLM call to billing logger")
		}
	}
}

func (s *Proxy) getModelInfo(ctx context.Context, baseURL, provider, modelName string) (*types.ModelInfo, error) {

	modelInfo, err := s.modelInfoProvider.GetModelInfo(ctx, &model.ModelInfoRequest{
		BaseURL:  baseURL,
		Provider: provider,
		Model:    modelName,
	})
	if err != nil {
		log.Warn().
			Err(err).
			Str("model", modelName).
			Str("original_model", modelName).
			Str("provider", provider).
			Err(err).Msg("failed to get model info")

		// Try stripping
		// Models can come as claude-sonnet-4-20250514, we need to strip the date
		strippedModelName := stripDateFromModelName(modelName)
		modelInfo, err = s.modelInfoProvider.GetModelInfo(ctx, &model.ModelInfoRequest{
			BaseURL:  baseURL,
			Provider: provider,
			Model:    strippedModelName,
		})
		if err != nil {
			log.Warn().
				Err(err).
				Str("model", strippedModelName).
				Str("original_model", modelName).
				Str("provider", provider).
				Err(err).Msg("failed to get model info")
			return nil, err
		}
		// OK, we got model info
	}
	return modelInfo, nil
}

func stripDateFromModelName(modelName string) string {
	// Models can come as claude-sonnet-4-20250514, we need to strip the date
	// Handle both dash and @ separators for dates
	// Examples: claude-sonnet-4-20250514 -> claude-sonnet-4
	//          claude-sonnet-4@20250514 -> claude-sonnet-4

	// Check for @ separator first (e.g., claude-sonnet-4@20250514)
	if strings.Contains(modelName, "@") {
		parts := strings.Split(modelName, "@")
		if len(parts) >= 2 {
			// Check if the last part looks like a date (8 digits)
			lastPart := parts[len(parts)-1]
			if len(lastPart) == 8 && isNumeric(lastPart) {
				return parts[0]
			}
		}
	}

	// Check for dash separator (e.g., claude-sonnet-4-20250514)
	if strings.Contains(modelName, "-") {
		parts := strings.Split(modelName, "-")
		if len(parts) >= 2 {
			// Check if the last part looks like a date (8 digits)
			lastPart := parts[len(parts)-1]
			if len(lastPart) == 8 && isNumeric(lastPart) {
				// Rejoin all parts except the last one
				return strings.Join(parts[:len(parts)-1], "-")
			}
		}
	}

	return modelName
}

// isNumeric checks if a string contains only digits
func isNumeric(s string) bool {
	for _, char := range s {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}
