package server

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

// createEmbeddings godoc
// @Summary Creates an embedding vector representing the input text
// @Description Creates an embedding vector representing the input text
// @Tags    embeddings
// @Success 200 {object} openai.EmbeddingResponse
// @Param request    body openai.EmbeddingRequest true "Request body with options for embeddings.")
// @Router /v1/embeddings [post]
// @Security BearerAuth
// @externalDocs.url https://platform.openai.com/docs/api-reference/embeddings/create
func (s *HelixAPIServer) createEmbeddings(rw http.ResponseWriter, r *http.Request) {
	// Check if this is a socket connection by looking at the underlying connection type
	isSocket := false
	if h, ok := r.Context().Value(http.LocalAddrContextKey).(*net.UnixAddr); ok && h != nil {
		isSocket = true
	}

	var user *types.User
	// Socket connections are pre-authorized, only check authorization for non-socket requests
	if !isSocket {
		user = getRequestUser(r)
		if !hasUser(user) {
			http.Error(rw, "unauthorized", http.StatusUnauthorized)
			log.Error().Msg("unauthorized")
			return
		}
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 10*MEGABYTE))
	if err != nil {
		log.Error().Err(err).Msg("error reading body")
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	var embeddingRequest openai.EmbeddingRequest
	if err := json.Unmarshal(body, &embeddingRequest); err != nil {
		log.Error().Err(err).Msg("error unmarshalling body")
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	client, err := s.providerManager.GetClient(r.Context(), &manager.GetClientRequest{
		Provider: s.Cfg.RAG.PGVector.Provider,
	})
	if err != nil {
		log.Error().Err(err).Msg("error getting client")
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	// Create a clean request without unsupported parameters
	cleanRequest := openai.EmbeddingRequest{
		Model:          embeddingRequest.Model,
		Input:          embeddingRequest.Input,
		EncodingFormat: "float",
		// Explicitly omit dimensions
	}

	resp, err := client.CreateEmbeddings(r.Context(), cleanRequest)
	if err != nil {
		if strings.Contains(err.Error(), "context cancelled") {
			// Client already gone
			return
		}

		// Better logging with more specific error information
		errLogger := log.Error().
			Err(err).
			Str("model", string(embeddingRequest.Model)).
			Str("provider", s.Cfg.RAG.PGVector.Provider)

		var statusCode int
		var errorMessage string

		if apiErr, ok := err.(*openai.APIError); ok {
			statusCode = apiErr.HTTPStatusCode
			errorMessage = apiErr.Message

			// Add detailed API error information
			errLogger = errLogger.
				Int("status_code", apiErr.HTTPStatusCode).
				Str("type", apiErr.Type)

			if apiErr.Code != nil {
				errLogger = errLogger.Interface("code", apiErr.Code)
			}
			if apiErr.Param != nil {
				errLogger = errLogger.Str("param", *apiErr.Param)
			}

			// Special handling for common error cases
			if apiErr.HTTPStatusCode == 400 && strings.Contains(strings.ToLower(apiErr.Message), "model not found") {
				errorMessage = "The requested embedding model is not found. It may not be loaded or available in this installation."
				errLogger.Msg("Embedding model not found")
			} else if apiErr.HTTPStatusCode == 404 {
				errorMessage = "The requested embedding model is not available. Check if the model is properly configured."
				errLogger.Msg("Embedding model not available")
			} else if apiErr.HTTPStatusCode == 500 {
				errorMessage = "The embedding service encountered a server error. The model may have failed to load."
				errLogger.Msg("Embedding server error")
			} else {
				errLogger.Msg("vllm api error details")
			}
		} else {
			// Generic error handling
			statusCode = http.StatusInternalServerError
			errorMessage = err.Error()

			if strings.Contains(strings.ToLower(err.Error()), "no such file") ||
				strings.Contains(strings.ToLower(err.Error()), "not found") {
				errorMessage = "The requested embedding model is not found or not properly configured."
				errLogger.Msg("Embedding model file not found")
			} else {
				errLogger.Msg("error creating embeddings")
			}
		}

		// Provide a more specific error response to the client
		http.Error(rw, errorMessage, statusCode)
		return
	}

	// Check if we got an empty response
	if len(resp.Data) == 0 {
		errMsg := "empty embedding response from model"
		log.Error().
			Str("model", string(embeddingRequest.Model)).
			Str("provider", s.Cfg.RAG.PGVector.Provider).
			Str("error", errMsg).
			Msg("embedding service returned empty data array")
		http.Error(rw, errMsg, http.StatusInternalServerError)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(rw).Encode(resp)
}
