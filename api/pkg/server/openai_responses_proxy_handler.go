package server

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/pricing"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/toolcall"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

const openAIResponsesLogTimeout = 15 * time.Second

type openAIResponsesRequest struct {
	Model  string `json:"model"`
	Stream bool   `json:"stream"`
}

type openAIResponsesUsage struct {
	InputTokens       int64 `json:"input_tokens"`
	OutputTokens      int64 `json:"output_tokens"`
	TotalTokens       int64 `json:"total_tokens"`
	InputTokenDetails struct {
		CachedTokens int64 `json:"cached_tokens"`
	} `json:"input_tokens_details"`
}

type openAIResponsesEnvelope struct {
	ID    string                   `json:"id"`
	Model string                   `json:"model"`
	Usage *openAIResponsesUsage    `json:"usage"`
	Error *openAIResponsesAPIError `json:"error"`
}

type openAIResponsesAPIError struct {
	Message string `json:"message"`
}

type openAIResponsesEvent struct {
	Type     string                   `json:"type"`
	Response *openAIResponsesEnvelope `json:"response"`
	Error    *openAIResponsesAPIError `json:"error"`
}

type openAIResponsesProxyResult struct {
	Response         []byte
	Usage            *openAIResponsesUsage
	Model            string
	Error            string
	Terminal         bool
	TimeToFirstToken time.Duration
}

// openAIResponsesProxyHandler is the native Codex API-key path. The sandbox
// carries only a session-scoped Helix key; this handler validates the task's
// selected OpenAI model, replaces the key at the trust boundary, and accounts
// for the completed Responses API call.
func (s *HelixAPIServer) openAIResponsesProxyHandler(w http.ResponseWriter, r *http.Request) {
	addCorsHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "user is required", http.StatusUnauthorized)
		return
	}
	selection, err := s.codeAgentProviderSelection(r.Context(), user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if selection.Runtime != types.CodeAgentRuntimeCodexCLI {
		http.Error(w, fmt.Sprintf("/v1/responses is reserved for the Codex harness, got %q", selection.Runtime), http.StatusBadRequest)
		return
	}
	endpoint, err := s.resolveCodeAgentProviderEndpoint(r.Context(), user, selection.ProviderRef)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !external_agent.CodeAgentRuntimeAllowsProvider(selection.Runtime, endpoint.Name) {
		http.Error(w, fmt.Sprintf("Codex API-key mode requires the openai provider; provider %q is not compatible", endpoint.Name), http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read Responses request: "+err.Error(), http.StatusBadRequest)
		return
	}
	var request openAIResponsesRequest
	if err := json.Unmarshal(body, &request); err != nil {
		http.Error(w, "invalid Responses request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if request.Model == "" || selection.Model == "" || request.Model != selection.Model {
		http.Error(w, fmt.Sprintf("Responses request model %q does not match task model %q", request.Model, selection.Model), http.StatusBadRequest)
		return
	}

	if endpoint.BillingEnabled {
		if s.modelInfoProvider == nil {
			http.Error(w, "model pricing is not configured", http.StatusPreconditionFailed)
			return
		}
		if _, err := s.modelInfoProvider.GetModelInfo(r.Context(), &model.ModelInfoRequest{
			BaseURL: endpoint.BaseURL, Provider: endpoint.Name, Model: request.Model,
		}); err != nil {
			http.Error(w, fmt.Sprintf("could not find model information for model %q: %s", request.Model, err), http.StatusPreconditionFailed)
			return
		}
	}

	if s.Controller != nil {
		hasEnough, err := s.Controller.HasEnoughBalance(r.Context(), user, user.OrganizationID, endpoint.BillingEnabled)
		if err != nil {
			http.Error(w, "failed to check balance: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if !hasEnough {
			http.Error(w, "Insufficient balance", http.StatusPaymentRequired)
			return
		}
	}

	apiKey, err := providerEndpointAPIKey(endpoint)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	baseURL, err := url.Parse(strings.TrimRight(endpoint.BaseURL, "/"))
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
		http.Error(w, fmt.Sprintf("openai provider has invalid base URL %q", endpoint.BaseURL), http.StatusBadGateway)
		return
	}

	upstreamRequest := r.Clone(r.Context())
	upstreamRequest.Body = io.NopCloser(bytes.NewReader(body))
	upstreamRequest.ContentLength = int64(len(body))
	upstreamRequest.URL.Scheme = baseURL.Scheme
	upstreamRequest.URL.Host = baseURL.Host
	upstreamRequest.URL.Path = joinProxyPath(baseURL.Path, strings.TrimPrefix(r.URL.Path, "/v1"))
	upstreamRequest.Host = baseURL.Host
	removeHopByHopHeaders(upstreamRequest.Header)
	for key, value := range endpoint.Headers {
		upstreamRequest.Header.Set(key, value)
	}
	upstreamRequest.Header.Set("Authorization", "Bearer "+apiKey)

	transport := s.openAIResponsesTransport
	if transport == nil {
		transport = http.DefaultTransport
	}
	started := time.Now()
	response, err := transport.RoundTrip(upstreamRequest)
	if err != nil {
		log.Error().Err(err).Str("provider", endpoint.Name).Str("base_url", endpoint.BaseURL).Msg("OpenAI Responses proxy failed")
		s.recordOpenAIResponsesCall(context.WithoutCancel(r.Context()), user, selection, endpoint, body, request.Stream, started, openAIResponsesProxyResult{Error: err.Error()})
		http.Error(w, "OpenAI Responses proxy failed", http.StatusBadGateway)
		return
	}
	defer response.Body.Close()

	copyResponseHeaders(w.Header(), response.Header)
	w.WriteHeader(response.StatusCode)
	result, copyErr := proxyOpenAIResponsesBody(w, response, started)
	if copyErr != nil {
		log.Error().Err(copyErr).Str("provider", endpoint.Name).Msg("failed to stream OpenAI Responses body")
		if result.Error == "" {
			result.Error = copyErr.Error()
		}
	}
	if response.StatusCode >= http.StatusBadRequest && result.Error == "" {
		result.Error = http.StatusText(response.StatusCode)
	}
	s.recordOpenAIResponsesCall(context.WithoutCancel(r.Context()), user, selection, endpoint, body, request.Stream, started, result)
}

func newOpenAIResponsesTransport(tlsSkipVerify bool) http.RoundTripper {
	if !tlsSkipVerify {
		return http.DefaultTransport
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // explicit enterprise setting
	return transport
}

func proxyOpenAIResponsesBody(w http.ResponseWriter, response *http.Response, started time.Time) (openAIResponsesProxyResult, error) {
	if !strings.HasPrefix(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
		body, err := io.ReadAll(response.Body)
		if err != nil {
			return openAIResponsesProxyResult{}, err
		}
		if _, err := w.Write(body); err != nil {
			return openAIResponsesProxyResult{}, err
		}
		result := parseOpenAIResponsesEnvelope(body)
		result.Response = body
		result.TimeToFirstToken = time.Since(started)
		return result, nil
	}

	flusher, _ := w.(http.Flusher)
	reader := bufio.NewReader(response.Body)
	var result openAIResponsesProxyResult
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			if result.TimeToFirstToken == 0 {
				result.TimeToFirstToken = time.Since(started)
			}
			if _, writeErr := w.Write(line); writeErr != nil {
				return result, writeErr
			}
			if flusher != nil {
				flusher.Flush()
			}
			if event, ok := parseOpenAIResponsesEvent(line); ok {
				result = mergeOpenAIResponsesResult(result, event)
				if event.Terminal {
					separator := []byte("\n")
					if bytes.HasSuffix(line, []byte("\r\n")) {
						separator = []byte("\r\n")
					}
					if _, writeErr := w.Write(separator); writeErr != nil {
						return result, writeErr
					}
					if flusher != nil {
						flusher.Flush()
					}
					return result, nil
				}
			}
		}
		if err == io.EOF {
			return result, nil
		}
		if err != nil {
			return result, err
		}
	}
}

func parseOpenAIResponsesEnvelope(body []byte) openAIResponsesProxyResult {
	var envelope openAIResponsesEnvelope
	if json.Unmarshal(body, &envelope) != nil {
		return openAIResponsesProxyResult{}
	}
	result := openAIResponsesProxyResult{Usage: envelope.Usage, Model: envelope.Model}
	if envelope.Error != nil {
		result.Error = envelope.Error.Message
	}
	return result
}

func parseOpenAIResponsesEvent(line []byte) (openAIResponsesProxyResult, bool) {
	data := bytes.TrimSpace(line)
	if !bytes.HasPrefix(data, []byte("data:")) {
		return openAIResponsesProxyResult{}, false
	}
	data = bytes.TrimSpace(bytes.TrimPrefix(data, []byte("data:")))
	if bytes.Equal(data, []byte("[DONE]")) {
		return openAIResponsesProxyResult{}, false
	}
	var event openAIResponsesEvent
	if json.Unmarshal(data, &event) != nil {
		return openAIResponsesProxyResult{}, false
	}
	result := openAIResponsesProxyResult{Response: append([]byte(nil), data...)}
	result.Terminal = event.Type == "response.completed" ||
		event.Type == "response.failed" ||
		event.Type == "response.incomplete"
	if event.Response != nil {
		result.Usage = event.Response.Usage
		result.Model = event.Response.Model
		if event.Response.Error != nil {
			result.Error = event.Response.Error.Message
		}
	}
	if event.Error != nil {
		result.Error = event.Error.Message
	}
	return result, true
}

func mergeOpenAIResponsesResult(current, event openAIResponsesProxyResult) openAIResponsesProxyResult {
	if event.Usage != nil {
		current.Usage = event.Usage
	}
	if event.Model != "" {
		current.Model = event.Model
	}
	if event.Error != "" {
		current.Error = event.Error
	}
	if len(event.Response) > 0 {
		current.Response = event.Response
	}
	if event.Terminal {
		current.Terminal = true
	}
	return current
}

func (s *HelixAPIServer) recordOpenAIResponsesCall(
	ctx context.Context,
	user *types.User,
	selection *codeAgentProviderSelection,
	endpoint *types.ProviderEndpoint,
	requestBody []byte,
	stream bool,
	started time.Time,
	result openAIResponsesProxyResult,
) {
	if len(s.openAIResponsesLogStores) == 0 && (!endpoint.BillingEnabled || s.openAIResponsesBillingLogger == nil) {
		return
	}
	usage := result.Usage
	if usage == nil {
		usage = &openAIResponsesUsage{}
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}
	modelName := result.Model
	if modelName == "" {
		modelName = selection.Model
	}
	var cost pricing.TokenCost
	if s.modelInfoProvider != nil {
		info, err := s.modelInfoProvider.GetModelInfo(ctx, &model.ModelInfoRequest{
			BaseURL: endpoint.BaseURL, Provider: endpoint.Name, Model: modelName,
		})
		if err != nil {
			log.Warn().Err(err).Str("provider", endpoint.Name).Str("model", modelName).Msg("failed to get Responses model pricing")
		} else if cost, err = pricing.CalculateTokenPrice(info, pricing.TokenUsage{
			PromptTokens: usage.InputTokens, CompletionTokens: usage.OutputTokens, CacheReadTokens: usage.InputTokenDetails.CachedTokens,
		}); err != nil {
			log.Error().Err(err).Str("provider", endpoint.Name).Str("model", modelName).Msg("failed to calculate Responses token price")
		}
	}
	providerID := endpoint.ID
	if providerID == "" {
		providerID = endpoint.Name
	}
	tools := validateResponsesToolCalls(requestBody, result.Response)

	call := &types.LLMCall{
		ID:                 system.GenerateLLMCallID(),
		Created:            started,
		AppID:              selection.Attribution.AppID,
		SessionID:          selection.Attribution.SessionID,
		CodeAgentRuntime:   selection.Runtime,
		InteractionID:      "n/a",
		OrganizationID:     user.OrganizationID,
		ProjectID:          user.ProjectID,
		SpecTaskID:         user.SpecTaskID,
		Model:              modelName,
		Provider:           providerID,
		Step:               types.LLMCallStepDefault,
		OriginalRequest:    requestBody,
		Request:            requestBody,
		Response:           result.Response,
		DurationMs:         time.Since(started).Milliseconds(),
		TimeToFirstTokenMs: result.TimeToFirstToken.Milliseconds(),
		PromptTokens:       usage.InputTokens,
		CompletionTokens:   usage.OutputTokens,
		TotalTokens:        usage.TotalTokens,
		CacheReadTokens:    usage.InputTokenDetails.CachedTokens,
		PromptCost:         cost.PromptCost,
		CompletionCost:     cost.CompletionCost,
		CacheReadCost:      cost.CacheReadCost,
		TotalCost:          cost.Total(),
		UserID:             user.ID,
		Stream:             stream,
		Error:              result.Error,
		FinishReason:       tools.FinishReason,
		ToolsOffered:       tools.Result.ToolsOffered,
		ToolCallsReturned:  tools.Result.Calls,
		ToolCallErrors:     tools.Result.Errors,
		ToolCallErrorKinds: tools.Result.KindsString(),
	}
	logCtx, cancel := context.WithTimeout(ctx, openAIResponsesLogTimeout)
	defer cancel()
	for _, logStore := range s.openAIResponsesLogStores {
		if _, err := logStore.CreateLLMCall(logCtx, call); err != nil {
			log.Error().Err(err).Msg("failed to log OpenAI Responses call")
		}
	}
	if endpoint.BillingEnabled && s.openAIResponsesBillingLogger != nil {
		if _, err := s.openAIResponsesBillingLogger.CreateLLMCall(logCtx, call); err != nil {
			log.Error().Err(err).Msg("failed to bill OpenAI Responses call")
		}
	}
}

func copyResponseHeaders(dst, src http.Header) {
	for key, values := range src {
		dst[key] = append([]string(nil), values...)
	}
	removeHopByHopHeaders(dst)
}

func removeHopByHopHeaders(header http.Header) {
	for _, name := range []string{"Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade"} {
		header.Del(name)
	}
}

func joinProxyPath(basePath, requestPath string) string {
	return strings.TrimRight(basePath, "/") + "/" + strings.TrimLeft(requestPath, "/")
}

// responsesToolCallVerdict pairs the tool call validation with the finish
// reason it implies. The Responses API has no finish_reason field — it reports
// a status — so a turn that emitted function calls is recorded as "tool_calls"
// to match what the Chat Completions and Anthropic proxies write.
type responsesToolCallVerdict struct {
	Result       toolcall.Result
	FinishReason string
}

// validateResponsesToolCalls checks the function calls in a Responses API
// result against the schemas the request offered. The response is either a
// full envelope (non-streaming) or the terminal response.completed event
// (streaming), so the output items are read from either shape. Built-in tools
// (web_search, file_search) carry no name or parameters and do not surface as
// function_call items, so they are left out of both sides.
func validateResponsesToolCalls(requestBody, response []byte) responsesToolCallVerdict {
	if len(requestBody) == 0 || len(response) == 0 {
		return responsesToolCallVerdict{}
	}

	var request struct {
		Tools []struct {
			Type       string          `json:"type"`
			Name       string          `json:"name"`
			Parameters json.RawMessage `json:"parameters"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(requestBody, &request); err != nil {
		log.Debug().Err(err).Msg("failed to parse Responses request for tool schemas")
		return responsesToolCallVerdict{}
	}

	type outputItem struct {
		Type      string `json:"type"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}
	var envelope struct {
		Status   string       `json:"status"`
		Output   []outputItem `json:"output"`
		Response *struct {
			Status string       `json:"status"`
			Output []outputItem `json:"output"`
		} `json:"response"`
	}
	if err := json.Unmarshal(response, &envelope); err != nil {
		log.Debug().Err(err).Msg("failed to parse Responses response for tool calls")
		return responsesToolCallVerdict{}
	}
	output, status := envelope.Output, envelope.Status
	if envelope.Response != nil {
		output, status = envelope.Response.Output, envelope.Response.Status
	}

	tools := make([]toolcall.Tool, 0, len(request.Tools))
	for _, tool := range request.Tools {
		if tool.Name == "" {
			continue
		}
		tools = append(tools, toolcall.Tool{Name: tool.Name, Schema: tool.Parameters})
	}

	var calls []toolcall.Call
	for _, item := range output {
		if item.Type != "function_call" {
			continue
		}
		calls = append(calls, toolcall.Call{Name: item.Name, Arguments: item.Arguments})
	}

	verdict := responsesToolCallVerdict{Result: toolcall.Validate(tools, calls), FinishReason: status}
	if len(calls) > 0 {
		verdict.FinishReason = "tool_calls"
	}
	return verdict
}
