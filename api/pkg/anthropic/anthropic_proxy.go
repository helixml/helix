package anthropic

import (
	"bufio"
	"bytes"
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
)

type Proxy struct {
	store                 store.Store
	anthropicReverseProxy *httputil.ReverseProxy
	logStores             []logger.LogStore
	billingLogger         logger.LogStore
	modelInfoProvider     model.ModelInfoProvider
	wg                    sync.WaitGroup
}

func New(cfg *config.ServerConfig, store store.Store, modelInfoProvider model.ModelInfoProvider, logStores ...logger.LogStore) *Proxy {

	p := &Proxy{
		store:                 store,
		anthropicReverseProxy: httputil.NewSingleHostReverseProxy(nil),
		logStores:             logStores,
		modelInfoProvider:     modelInfoProvider,
		wg:                    sync.WaitGroup{},
	}

	if cfg.Stripe.BillingEnabled {
		billingLogger, err := logger.NewBillingLogger(store, cfg.Stripe.BillingEnabled)
		if err != nil {
			log.Error().Err(err).Msg("failed to initialize billing logger")
		} else {
			p.billingLogger = billingLogger
		}
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
	r = setStartTime(r, time.Now())

	s.anthropicReverseProxy.ServeHTTP(w, r)
}

func (s *Proxy) anthropicAPIProxyDirector(r *http.Request) {
	endpoint := GetRequestProviderEndpoint(r.Context())
	if endpoint == nil {
		log.Error().Msg("provider endpoint not found in context")
		return
	}

	u, err := url.Parse(endpoint.BaseURL)
	if err != nil {
		log.Error().Err(err).Msg("failed to parse provider endpoint URL")
		return
	}
	r.URL.Host = u.Host
	r.URL.Scheme = u.Scheme

	r.Host = u.Host

	// Remove incoming auth headers (user's Helix token, not the real provider API key)
	r.Header.Del("Authorization")
	r.Header.Del("x-api-key")
	r.Header.Del("api-key")

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

// anthropicAPIProxyModifyResponse - parses the response
func (s *Proxy) anthropicAPIProxyModifyResponse(response *http.Response) error {

	// If content type is "text/event-stream", then process the stream
	if strings.Contains(response.Header.Get("Content-Type"), "text/event-stream") {
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

			defer pw.Close()
			reader := bufio.NewReader(body)
			for {
				line, err := reader.ReadBytes('\n')
				if err != nil {
					// Log the final assembled message for billing
					// Always log, even if no content (for debugging)
					bts, err := json.Marshal(respMessage)
					if err != nil {
						log.Error().Err(err).Any("resp_message", respMessage).Msg("failed to marshal resp message")
					} else {
						s.logLLMCall(response.Request.Context(), time.Now(), bts, nil, true, time.Since(start).Milliseconds())
						log.Debug().Msg("XX logging full")
					}
					return
				}

				// Parse SSE line
				lineStr := string(line)
				if strings.HasPrefix(lineStr, "data: ") {
					data := strings.TrimPrefix(lineStr, "data: ")
					data = strings.TrimSpace(data)

					// Skip empty data and [DONE] markers
					if data == "" || data == "[DONE]" {
						log.Debug().Str("data", data).Msg("skipping empty or done marker")
						_, _ = pw.Write(line)
						continue
					}

					log.Debug().Str("data", data).Msg("processing streaming data")

					// Parse the JSON data as a streaming event
					var event anthropic.MessageStreamEventUnion
					if err := event.UnmarshalJSON([]byte(data)); err != nil {
						log.Debug().Err(err).Str("data", data).Msg("failed to parse streaming event")
						_, _ = pw.Write(line)
						continue
					}

					err = respMessage.Accumulate(event)
					if err != nil {
						log.Error().Err(err).Msg("failed to accumulate event")
						_, _ = pw.Write(line)
						continue
					}
				}

				// Forward the line to the client
				_, _ = pw.Write(line)
			}
		}()
		return nil
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
	modelInfo, err := s.getModelInfo(ctx, provider.BaseURL, provider.Name, string(respMessage.Model))
	if err == nil {
		// Calculate the cost for the call and persist it
		promptCost, completionCost, err = pricing.CalculateTokenPrice(modelInfo, usage.InputTokens, usage.OutputTokens)
		if err != nil {
			log.Error().
				Err(err).
				Str("user_id", vals.OwnerID).
				Str("model", string(respMessage.Model)).
				Str("provider", provider.Name).
				Err(err).Msg("failed to calculate token price")
		}
		totalCost = promptCost + completionCost
	}

	log.Debug().
		Str("owner_id", vals.OwnerID).
		Str("organization_id", orgID).
		Str("model", string(respMessage.Model)).
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
		Model:            string(respMessage.Model),
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
	// Models can come as claude-sonnet-4-20250514, we need to strip the date
	strippedModelName := stripDateFromModelName(modelName)

	modelInfo, err := s.modelInfoProvider.GetModelInfo(ctx, &model.ModelInfoRequest{
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
