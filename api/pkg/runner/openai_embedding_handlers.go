package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

// Add this right after the package declaration
type loggerRoundTripper struct {
	next        http.RoundTripper
	logResponse func(*http.Response)
}

func (l *loggerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := l.next.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	if l.logResponse != nil {
		l.logResponse(resp)
	}

	return resp, err
}

// Custom RoundTripper for logging request details
type requestLoggerTransport struct {
	next   http.RoundTripper
	slotID string
}

// RoundTrip implements the http.RoundTripper interface
func (t *requestLoggerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Only log if it's the embeddings endpoint
	if strings.Contains(req.URL.Path, "embeddings") {
		// Log request URL and method
		log.Info().
			Str("component", "runner").
			Str("operation", "embedding").
			Str("slot_id", t.slotID).
			Str("request_method", req.Method).
			Str("request_url", req.URL.String()).
			Msg("🔍 OUTGOING HTTP REQUEST TO VLLM")

		// Log request headers
		headerJSON, _ := json.MarshalIndent(req.Header, "", "  ")
		log.Info().
			Str("component", "runner").
			Str("operation", "embedding").
			Str("slot_id", t.slotID).
			Str("request_headers", string(headerJSON)).
			Msg("🔍 OUTGOING HTTP HEADERS TO VLLM")

		// Log request body if possible
		if req.Body != nil {
			bodyBytes, err := io.ReadAll(req.Body)
			if err != nil {
				log.Error().
					Str("component", "runner").
					Str("operation", "embedding").
					Str("slot_id", t.slotID).
					Str("error", err.Error()).
					Msg("Failed to read request body for logging")
			} else {
				// Log the raw body
				rawBody := string(bodyBytes)
				log.Info().
					Str("component", "runner").
					Str("operation", "embedding").
					Str("slot_id", t.slotID).
					Str("raw_request_body", rawBody).
					Msg("🔍 RAW HTTP REQUEST BODY TO VLLM")

				// Restore the body for the actual request
				req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			}
		}
	}

	// Continue with the original round trip
	return t.next.RoundTrip(req)
}

// Add this function above the createEmbedding function
// CreateOpenaiClientWithHTTPClient creates an OpenAI client with a custom HTTP client
func CreateOpenaiClientWithHTTPClient(_ context.Context, endpoint string, httpClient *http.Client) (*openai.Client, error) {
	config := openai.DefaultConfig("unused")
	config.BaseURL = endpoint
	config.HTTPClient = httpClient
	return openai.NewClientWithConfig(config), nil
}

func (s *HelixRunnerAPIServer) createEmbedding(rw http.ResponseWriter, r *http.Request) {
	if s.runnerOptions == nil {
		http.Error(rw, "runner server not initialized", http.StatusInternalServerError)
		return
	}
	slotID := mux.Vars(r)["slot_id"]
	slotUUID, err := uuid.Parse(slotID)
	if err != nil {
		http.Error(rw, fmt.Sprintf("invalid slot id: %s", slotID), http.StatusBadRequest)
		return
	}
	log.Debug().Str("slot_id", slotUUID.String()).Msg("create embedding")

	body, err := io.ReadAll(io.LimitReader(r.Body, 10*MEGABYTE))
	if err != nil {
		log.Error().
			Str("component", "runner").
			Str("operation", "embedding").
			Str("slot_id", slotUUID.String()).
			Str("error_type", "request_read_error").
			Str("error_message", err.Error()).
			Msg("❌ Error reading embedding request body")
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	// Log the raw incoming request
	var prettyRequest bytes.Buffer
	if err := json.Indent(&prettyRequest, body, "", "  "); err == nil {
		log.Debug().
			Str("component", "runner").
			Str("operation", "embedding").
			Str("slot_id", slotUUID.String()).
			Str("raw_request", prettyRequest.String()).
			Msg("📥 Raw embedding request received")
	}

	// Try to parse request as flexible embedding request first
	var embeddingRequest openai.EmbeddingRequest
	var flexibleRequest types.FlexibleEmbeddingRequest
	var isFlexible bool

	// First try to unmarshal as FlexibleEmbeddingRequest
	err = json.Unmarshal(body, &flexibleRequest)
	if err != nil {
		log.Error().
			Str("component", "runner").
			Str("operation", "embedding").
			Str("slot_id", slotUUID.String()).
			Str("error_type", "json_unmarshal_error").
			Str("error_message", err.Error()).
			Str("raw_body", string(body)).
			Msg("❌ Error unmarshalling flexible embedding request")
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	// Check if this is a Chat Embeddings API request
	if len(flexibleRequest.Messages) > 0 {
		isFlexible = true
		log.Info().
			Str("component", "runner").
			Str("operation", "embedding").
			Str("slot_id", slotUUID.String()).
			Str("model", flexibleRequest.Model).
			Int("message_count", len(flexibleRequest.Messages)).
			Msg("📥 Chat Embeddings API request detected")
	} else {
		// Fall back to standard embedding request
		err = json.Unmarshal(body, &embeddingRequest)
		if err != nil {
			log.Error().
				Str("component", "runner").
				Str("operation", "embedding").
				Str("slot_id", slotUUID.String()).
				Str("error_type", "json_unmarshal_error").
				Str("error_message", err.Error()).
				Str("raw_body", string(body)).
				Msg("❌ Error unmarshalling standard embedding request")
			http.Error(rw, err.Error(), http.StatusBadRequest)
			return
		}
	}

	// Log the embedding request details with comprehensive information
	reqInfo := map[string]interface{}{
		"is_flexible": isFlexible,
		"endpoint":    fmt.Sprintf("/v1/slots/%s/v1/embedding", slotUUID.String()),
		"method":      "POST",
	}

	if isFlexible {
		reqInfo["model"] = flexibleRequest.Model
		reqInfo["encoding_format"] = flexibleRequest.EncodingFormat
		reqInfo["dimensions"] = flexibleRequest.Dimensions
		if len(flexibleRequest.Messages) > 0 {
			reqInfo["message_count"] = len(flexibleRequest.Messages)
		}
	} else {
		reqInfo["model"] = string(embeddingRequest.Model)
		reqInfo["encoding_format"] = string(embeddingRequest.EncodingFormat)
		reqInfo["dimensions"] = embeddingRequest.Dimensions
	}

	reqInfoJSON, _ := json.Marshal(reqInfo)
	log.Info().
		Str("component", "runner").
		Str("operation", "embedding").
		Str("slot_id", slotUUID.String()).
		RawJSON("request_details", reqInfoJSON).
		Msg("🧮 Processing embedding request")

	slot, ok := s.slots.Load(slotUUID)
	if !ok {
		log.Warn().
			Str("component", "runner").
			Str("operation", "embedding").
			Str("slot_id", slotUUID.String()).
			Str("error_type", "slot_not_found").
			Msg("❌ Slot not found for embedding request")
		http.Error(rw, fmt.Sprintf("slot %s not found", slotUUID.String()), http.StatusNotFound)
		return
	}

	addCorsHeaders(rw)
	if r.Method == http.MethodOptions {
		return
	}

	// Model validation based on whether we have a standard or flexible request
	if isFlexible {
		if flexibleRequest.Model == "" {
			flexibleRequest.Model = slot.Model
		}
		if flexibleRequest.Model != slot.Model {
			log.Warn().
				Str("component", "runner").
				Str("operation", "embedding").
				Str("slot_id", slotUUID.String()).
				Str("expected_model", slot.Model).
				Str("requested_model", flexibleRequest.Model).
				Str("error_type", "model_mismatch").
				Msg("❌ Model mismatch for flexible embedding request")
			http.Error(rw, fmt.Sprintf("model mismatch, expecting %s", slot.Model), http.StatusBadRequest)
			return
		}
	} else {
		if embeddingRequest.Model == "" {
			embeddingRequest.Model = openai.EmbeddingModel(slot.Model)
		}
		if embeddingRequest.Model != openai.EmbeddingModel(slot.Model) {
			log.Warn().
				Str("component", "runner").
				Str("operation", "embedding").
				Str("slot_id", slotUUID.String()).
				Str("expected_model", slot.Model).
				Str("requested_model", string(embeddingRequest.Model)).
				Str("error_type", "model_mismatch").
				Msg("❌ Model mismatch for embedding request")
			http.Error(rw, fmt.Sprintf("model mismatch, expecting %s", slot.Model), http.StatusBadRequest)
			return
		}
	}

	ownerID := s.runnerOptions.ID
	user := getRequestUser(r)
	if user != nil {
		ownerID = user.ID
		if user.TokenType == types.TokenTypeRunner {
			ownerID = oai.RunnerID
		}
	}

	ctx := oai.SetContextValues(r.Context(), &oai.ContextValues{
		OwnerID:         ownerID,
		SessionID:       "n/a",
		InteractionID:   "n/a",
		OriginalRequest: body,
	})

	// Create the openai client
	vllmEndpoint := fmt.Sprintf("%s/v1", slot.URL())
	log.Debug().
		Str("component", "runner").
		Str("operation", "embedding").
		Str("slot_id", slotUUID.String()).
		Str("vllm_endpoint", vllmEndpoint).
		Msg("Creating OpenAI client for VLLM endpoint")

	// Create a transport that logs raw responses
	type responseLoggerTransport struct {
		http.RoundTripper
	}

	// Override RoundTrip to capture and log raw responses
	rawResponseBody := ""
	logTransport := responseLoggerTransport{
		RoundTripper: http.DefaultTransport,
	}

	// Implement the RoundTripper interface
	logTransport.RoundTripper = &loggerRoundTripper{
		next: logTransport.RoundTripper,
		logResponse: func(resp *http.Response) {
			if resp != nil && resp.Body != nil {
				// Read the body
				bodyBytes, err := io.ReadAll(resp.Body)
				if err != nil {
					log.Error().
						Str("component", "runner").
						Str("operation", "embedding").
						Str("slot_id", slotUUID.String()).
						Str("error", err.Error()).
						Msg("Failed to read response body for logging")
				} else {
					// Store the raw body for logging
					rawResponseBody = string(bodyBytes)
					// Log the raw response body for debugging
					log.Debug().
						Str("component", "runner").
						Str("operation", "embedding").
						Str("slot_id", slotUUID.String()).
						Str("status_code", fmt.Sprintf("%d", resp.StatusCode)).
						Str("content_type", resp.Header.Get("Content-Type")).
						Str("raw_response_body", rawResponseBody).
						Msg("📥 Raw response captured from embedding request")
					// Create a new body for the response
					resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
				}
			} else {
				log.Warn().
					Str("component", "runner").
					Str("operation", "embedding").
					Str("slot_id", slotUUID.String()).
					Msg("Unable to log response - response or response body is nil")
			}
		},
	}

	// Add custom transport to log raw HTTP request data
	originalTransport := logTransport.RoundTripper

	// Chain the transports together
	requestLogger := &requestLoggerTransport{
		next:   originalTransport,
		slotID: slotUUID.String(),
	}

	// Use the request logger as the transport
	logTransport.RoundTripper = requestLogger

	// Create a custom client with the logging transport
	httpClient := &http.Client{
		Transport: logTransport,
		// Timeout:   300 * time.Second,
	}

	// Create the OpenAI client with the custom HTTP client
	openAIClient, err := CreateOpenaiClientWithHTTPClient(ctx, vllmEndpoint, httpClient)
	if err != nil {
		log.Error().
			Str("component", "runner").
			Str("operation", "embedding").
			Str("slot_id", slotUUID.String()).
			Str("model", slot.Model).
			Str("vllm_endpoint", vllmEndpoint).
			Str("error_type", "client_creation_error").
			Str("error_message", err.Error()).
			Msg("❌ Error creating OpenAI client for embedding")
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Debug().
		Str("component", "runner").
		Str("operation", "embedding").
		Str("slot_id", slotUUID.String()).
		Str("model", slot.Model).
		Str("vllm_endpoint", vllmEndpoint).
		Msg("Sending embedding request to VLLM")

	// Start processing based on request type
	var resp openai.EmbeddingResponse
	var startTime time.Time
	var durationMs int64
	var requestJSON []byte

	if isFlexible {
		// Log the flexible request
		requestJSON, err = json.MarshalIndent(flexibleRequest, "", "  ")
		if err == nil {
			log.Info().
				Str("component", "runner").
				Str("operation", "embedding").
				Str("slot_id", slotUUID.String()).
				Str("model", slot.Model).
				Str("full_request", string(requestJSON)).
				Msg("🔍 FULL FLEXIBLE EMBEDDING REQUEST BEING SENT TO VLLM")
		}

		// For now, we need to modify the request if it uses the Chat Embeddings API format
		// We'll either convert it to the standard format if possible, or forward it directly
		if len(flexibleRequest.Messages) > 0 {
			log.Info().
				Str("component", "runner").
				Str("operation", "embedding").
				Str("slot_id", slotUUID.String()).
				Str("model", slot.Model).
				Int("message_count", len(flexibleRequest.Messages)).
				Msg("⚠️ Chat Embeddings API request format detected")

			// Send the raw request to VLLM as is
			startTime = time.Now()
			requestBodyBytes, _ := json.Marshal(flexibleRequest)

			// Create a new HTTP request
			req, httpErr := http.NewRequestWithContext(ctx, "POST", vllmEndpoint+"/embeddings", bytes.NewBuffer(requestBodyBytes))
			if httpErr != nil {
				log.Error().
					Str("component", "runner").
					Str("operation", "embedding").
					Str("slot_id", slotUUID.String()).
					Str("error", httpErr.Error()).
					Msg("❌ Failed to create HTTP request for chat embeddings")
				http.Error(rw, httpErr.Error(), http.StatusInternalServerError)
				return
			}

			// Set headers
			req.Header.Set("Content-Type", "application/json")

			// Send the request
			httpResp, httpErr := httpClient.Do(req)
			durationMs = time.Since(startTime).Milliseconds()

			if httpErr != nil {
				log.Error().
					Str("component", "runner").
					Str("operation", "embedding").
					Str("slot_id", slotUUID.String()).
					Str("error", httpErr.Error()).
					Int64("duration_ms", durationMs).
					Msg("❌ Failed to send HTTP request for chat embeddings")
				http.Error(rw, httpErr.Error(), http.StatusInternalServerError)
				return
			}
			defer httpResp.Body.Close()

			// Read the response
			respBody, readErr := io.ReadAll(httpResp.Body)
			if readErr != nil {
				log.Error().
					Str("component", "runner").
					Str("operation", "embedding").
					Str("slot_id", slotUUID.String()).
					Str("error", readErr.Error()).
					Int64("duration_ms", durationMs).
					Msg("❌ Failed to read HTTP response for chat embeddings")
				http.Error(rw, readErr.Error(), http.StatusInternalServerError)
				return
			}

			// Check if the response is an error
			if httpResp.StatusCode != http.StatusOK {
				log.Error().
					Str("component", "runner").
					Str("operation", "embedding").
					Str("slot_id", slotUUID.String()).
					Int("status_code", httpResp.StatusCode).
					Str("response", string(respBody)).
					Int64("duration_ms", durationMs).
					Msg("❌ VLLM returned error for chat embeddings")
				http.Error(rw, string(respBody), httpResp.StatusCode)
				return
			}

			// Pass through the raw response
			rw.Header().Set("Content-Type", "application/json")
			rw.WriteHeader(http.StatusOK)
			_, writeErr := rw.Write(respBody)
			if writeErr != nil {
				log.Error().
					Str("component", "runner").
					Str("operation", "embedding").
					Str("slot_id", slotUUID.String()).
					Str("error", writeErr.Error()).
					Msg("❌ Failed to write response for chat embeddings")
			}
			return
		}

		// Standard embedding with Input field
		log.Info().
			Str("component", "runner").
			Str("operation", "embedding").
			Str("slot_id", slotUUID.String()).
			Str("model", slot.Model).
			Msg("Converting flexible embedding request to standard format")

		// Convert to standard OpenAI embedding request
		embeddingRequest = openai.EmbeddingRequest{
			Model:          openai.EmbeddingModel(flexibleRequest.Model),
			Input:          flexibleRequest.Input,
			EncodingFormat: openai.EmbeddingEncodingFormat(flexibleRequest.EncodingFormat),
			Dimensions:     flexibleRequest.Dimensions,
		}
	}

	// At this point, we're using the standard OpenAI embedding format
	requestJSON, err = json.MarshalIndent(embeddingRequest, "", "  ")
	if err == nil {
		log.Info().
			Str("component", "runner").
			Str("operation", "embedding").
			Str("slot_id", slotUUID.String()).
			Str("model", slot.Model).
			Str("full_request", string(requestJSON)).
			Msg("🔍 FULL EMBEDDING REQUEST BEING SENT TO VLLM")

		// If input is a string, check and log token count estimate
		if inputStr, ok := embeddingRequest.Input.(string); ok {
			log.Info().
				Str("component", "runner").
				Str("operation", "embedding").
				Str("slot_id", slotUUID.String()).
				Int("estimated_token_count", len(inputStr)/4). // Rough estimate
				Msg("🔢 EMBEDDING INPUT TOKEN COUNT ESTIMATE")

			// Check for vision tokens
			if strings.Contains(inputStr, "<|vision_start|>") ||
				strings.Contains(inputStr, "<|image_pad|>") ||
				strings.Contains(inputStr, "<|vision_end|>") {
				log.Info().
					Str("component", "runner").
					Str("operation", "embedding").
					Str("slot_id", slotUUID.String()).
					Msg("⚠️ VISION TOKENS DETECTED IN EMBEDDING REQUEST")
			}
		}

		// If input is an array, log the length
		if inputArr, ok := embeddingRequest.Input.([]any); ok {
			log.Info().
				Str("component", "runner").
				Str("operation", "embedding").
				Str("slot_id", slotUUID.String()).
				Int("input_array_length", len(inputArr)).
				Msg("📊 EMBEDDING INPUT ARRAY LENGTH")
		}
	} else {
		log.Error().
			Str("component", "runner").
			Str("operation", "embedding").
			Str("slot_id", slotUUID.String()).
			Str("error", err.Error()).
			Msg("❌ Failed to marshal embedding request for logging")
	}

	startTime = time.Now()
	resp, err = openAIClient.CreateEmbeddings(ctx, embeddingRequest)
	durationMs = time.Since(startTime).Milliseconds()

	// Log the raw response body if we captured it
	if rawResponseBody != "" {
		log.Debug().
			Str("component", "runner").
			Str("operation", "embedding").
			Str("slot_id", slotUUID.String()).
			Str("raw_response", rawResponseBody).
			Msg("📄 Raw embedding response received")

		// Check for plain text error response (non-JSON) where the model doesn't support embeddings
		if strings.Contains(rawResponseBody, "does not support Embeddings API") {
			errMsg := strings.TrimSpace(rawResponseBody)
			log.Error().
				Str("component", "runner").
				Str("operation", "embedding").
				Str("slot_id", slotUUID.String()).
				Str("model", string(embeddingRequest.Model)).
				Str("vllm_endpoint", vllmEndpoint).
				Str("error_message", errMsg).
				Msg("❌ Model does not support embeddings API")

			http.Error(rw, errMsg, http.StatusInternalServerError)
			return
		}
	}

	if err != nil {
		errorDetails := map[string]interface{}{
			"error_type":    fmt.Sprintf("%T", err),
			"error_message": err.Error(),
			"slot_id":       slotUUID.String(),
			"model":         string(embeddingRequest.Model),
			"duration_ms":   durationMs,
			"vllm_endpoint": vllmEndpoint,
			"raw_response":  rawResponseBody,
		}

		// Check for specific error types for more details
		if apiErr, ok := err.(*openai.APIError); ok {
			errorDetails["api_error_type"] = apiErr.Type
			errorDetails["api_error_code"] = apiErr.Code
			errorDetails["api_error_param"] = apiErr.Param
			errorDetails["api_error_status"] = apiErr.HTTPStatusCode

			// Log token count error specifically
			if strings.Contains(apiErr.Message, "maximum context length") && strings.Contains(apiErr.Message, "tokens") {
				log.Error().
					Str("component", "runner").
					Str("operation", "embedding").
					Str("slot_id", slotUUID.String()).
					Str("error", "TOKEN LIMIT EXCEEDED").
					Str("full_error", apiErr.Message).
					Msg("⚠️⚠️⚠️ TOKEN COUNT EXCEEDS MODEL LIMIT ⚠️⚠️⚠️")
			}
		}

		errorJSON, _ := json.Marshal(errorDetails)
		log.Error().
			Str("component", "runner").
			Str("operation", "embedding").
			Str("slot_id", slotUUID.String()).
			Str("model", string(embeddingRequest.Model)).
			Int64("duration_ms", durationMs).
			RawJSON("error_details", errorJSON).
			Str("raw_response", rawResponseBody).
			Msg("❌ Error generating embeddings")

		// Try to parse token count from error message for better diagnostics
		if strings.Contains(err.Error(), "maximum context length") && strings.Contains(err.Error(), "tokens") {
			tokenInfo := extractTokenInfoFromError(err.Error())
			if tokenInfo.MaxTokens > 0 && tokenInfo.RequestedTokens > 0 {
				log.Error().
					Str("component", "runner").
					Str("operation", "embedding").
					Str("slot_id", slotUUID.String()).
					Int("max_tokens", tokenInfo.MaxTokens).
					Int("requested_tokens", tokenInfo.RequestedTokens).
					Msg("🔢 TOKEN COUNT DETAILS FROM ERROR")
			}
		}

		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	// Check if the raw response contains an error message despite 200 status code
	if rawResponseBody != "" {
		// Check if the response contains an error object
		var errorResponse struct {
			Object  string `json:"object"`
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    int    `json:"code"`
		}

		if err := json.Unmarshal([]byte(rawResponseBody), &errorResponse); err == nil {
			if errorResponse.Object == "error" || errorResponse.Type == "BadRequestError" {
				errMsg := fmt.Sprintf("VLLM endpoint returned error with status 200: %s", errorResponse.Message)
				log.Error().
					Str("component", "runner").
					Str("operation", "embedding").
					Str("slot_id", slotUUID.String()).
					Str("model", string(embeddingRequest.Model)).
					Str("vllm_endpoint", vllmEndpoint).
					Str("error_type", errorResponse.Type).
					Int("error_code", errorResponse.Code).
					Str("error_message", errorResponse.Message).
					Str("raw_response", rawResponseBody).
					Msg("❌ Error response from VLLM with 200 status code")

				http.Error(rw, errMsg, http.StatusInternalServerError)
				return
			}
		}
	}

	// Check if the response has empty data array but no error
	if len(resp.Data) == 0 {
		errMsg := "VLLM endpoint returned status 200 but empty embedding array"
		log.Error().
			Str("component", "runner").
			Str("operation", "embedding").
			Str("slot_id", slotUUID.String()).
			Str("model", string(embeddingRequest.Model)).
			Str("vllm_endpoint", vllmEndpoint).
			Str("error", errMsg).
			Str("raw_response", rawResponseBody).
			Msg("❌ Empty embedding response from VLLM")

		http.Error(rw, errMsg, http.StatusInternalServerError)
		return
	}

	// Log the embedding response details
	respInfo := map[string]interface{}{
		"embedding_count": len(resp.Data),
		"model":           resp.Model,
		"duration_ms":     durationMs,
		"slot_id":         slotUUID.String(),
		"vllm_endpoint":   vllmEndpoint,
	}

	// Add usage information if available
	if resp.Usage.PromptTokens > 0 {
		respInfo["prompt_tokens"] = resp.Usage.PromptTokens
		respInfo["total_tokens"] = resp.Usage.TotalTokens
	}

	// Add embedding dimensions if available
	if len(resp.Data) > 0 {
		respInfo["embedding_dimensions"] = len(resp.Data[0].Embedding)

		// Include a small sample of the first embedding for debugging
		if len(resp.Data[0].Embedding) > 5 {
			sampleValues := resp.Data[0].Embedding[:5]
			respInfo["embedding_sample"] = sampleValues
		}
	}

	respInfoJSON, _ := json.Marshal(respInfo)
	log.Info().
		Str("component", "runner").
		Str("operation", "embedding").
		Str("slot_id", slotUUID.String()).
		Str("model", string(embeddingRequest.Model)).
		Int64("duration_ms", durationMs).
		RawJSON("response_details", respInfoJSON).
		Msg("✅ Successfully generated embeddings")

	rw.Header().Set("Content-Type", "application/json")

	if r.URL.Query().Get("pretty") == "true" {
		// Pretty print the response with indentation
		bts, err := json.MarshalIndent(resp, "", "  ")
		if err != nil {
			log.Error().
				Str("component", "runner").
				Str("operation", "embedding").
				Str("slot_id", slotUUID.String()).
				Str("error_type", "response_marshal_error").
				Str("error_message", err.Error()).
				Msg("❌ Error marshalling embedding response")
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		_, _ = rw.Write(bts)
		return
	}

	err = json.NewEncoder(rw).Encode(resp)
	if err != nil {
		log.Error().
			Str("component", "runner").
			Str("operation", "embedding").
			Str("slot_id", slotUUID.String()).
			Str("error_type", "response_encode_error").
			Str("error_message", err.Error()).
			Msg("❌ Error writing embedding response")
	}
}

// Add this helper function at the end of the file
type tokenErrorInfo struct {
	MaxTokens       int
	RequestedTokens int
}

// extractTokenInfoFromError extracts token count information from VLLM error messages
func extractTokenInfoFromError(errMsg string) tokenErrorInfo {
	info := tokenErrorInfo{}

	// Look for patterns like: "maximum context length is 8192 tokens. However, you requested 98583 tokens"
	maxPattern := "maximum context length is (\\d+) tokens"
	requestedPattern := "you requested (\\d+) tokens"

	maxMatches := regexp.MustCompile(maxPattern).FindStringSubmatch(errMsg)
	if len(maxMatches) > 1 {
		info.MaxTokens, _ = strconv.Atoi(maxMatches[1])
	}

	requestedMatches := regexp.MustCompile(requestedPattern).FindStringSubmatch(errMsg)
	if len(requestedMatches) > 1 {
		info.RequestedTokens, _ = strconv.Atoi(requestedMatches[1])
	}

	return info
}
