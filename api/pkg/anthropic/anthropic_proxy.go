package anthropic

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
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

type providerEndpointContextKeyType string
type startTimeContextKeyType string

var providerEndpointContextKey providerEndpointContextKeyType = "provider_endpoint"
var startTimeContextKey startTimeContextKeyType = "start_time"

func SetRequestProviderEndpoint(req *http.Request, endpoint *types.ProviderEndpoint) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), providerEndpointContextKey, endpoint))
}

func GetRequestProviderEndpoint(ctx context.Context) *types.ProviderEndpoint {
	endpointIntf := ctx.Value(providerEndpointContextKey)
	if endpointIntf == nil {
		return nil
	}
	return endpointIntf.(*types.ProviderEndpoint)
}

func setStartTime(req *http.Request, time time.Time) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), startTimeContextKey, time))
}

func getStartTime(req *http.Request) (time.Time, bool) {
	startTimeIntf := req.Context().Value(startTimeContextKey)
	if startTimeIntf == nil {
		return time.Now(), false
	}
	return startTimeIntf.(time.Time), true
}

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

	p.anthropicReverseProxy.ModifyResponse = p.anthropicAPIProxyModifyResponse
	p.anthropicReverseProxy.Director = p.anthropicAPIProxyDirector

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

	for key, value := range endpoint.Headers {
		r.Header.Set(key, value)
	}

	// Remove authorization header
	r.Header.Del("Authorization")
}

// anthropicAPIProxyModifyResponse - parses the response
func (s *Proxy) anthropicAPIProxyModifyResponse(response *http.Response) error {

	// If content type is "text/event-stream", then process the stream
	if strings.Contains(response.Header.Get("Content-Type"), "text/event-stream") {
		pr, pw := io.Pipe()
		body := response.Body
		response.Body = pr

		// TODO: assemble the response body into a buffer
		go func() {
			defer pw.Close()
			reader := bufio.NewReader(body)
			for {
				line, err := reader.ReadBytes('\n')
				if err != nil {
					return
				}

				// TODO: read chunks and look for the usage
				fmt.Println(string(line))
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
		s.logLLMCall(ctx, time.Now(), buf, buf, nil, false, time.Since(start).Milliseconds())
	}()

	return nil
}

func (s *Proxy) logLLMCall(ctx context.Context, createdAt time.Time, req []byte, resp []byte, apiError error, stream bool, durationMs int64) {

	provider := GetRequestProviderEndpoint(ctx)

	var respMessage anthropic.Message
	if err := respMessage.UnmarshalJSON(resp); err != nil {
		log.Error().Err(err).Msg("failed to parse anthropic response for usage")
		return
	}
	// TODO: Extract and log usage information from resp
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
		OriginalRequest:  req,
		Request:          req,
		Response:         resp,
		Provider:         string(provider.Name),
		DurationMs:       durationMs,
		PromptTokens:     int64(usage.InputTokens),
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
