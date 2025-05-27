package server

import (
	"encoding/json"
	"io"
	"net"
	"net/http"

	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/rs/zerolog/log"
)

// createEmbeddings godoc
// @Summary Creates an embedding vector representing the input text
// @Description Creates an embedding vector representing the input text. Supports both standard OpenAI embedding format and Chat Embeddings API format with messages.
// @Tags    embeddings
// @Success 200 {object} types.FlexibleEmbeddingResponse
// @Param request    body types.FlexibleEmbeddingRequest true "Request body with options for embeddings. Can use either 'input' field (standard) or 'messages' field (Chat Embeddings API).")
// @Router /v1/embeddings [post]
// @Security BearerAuth
// @externalDocs.url https://platform.openai.com/docs/api-reference/embeddings/create
func (s *HelixAPIServer) createEmbeddings(rw http.ResponseWriter, r *http.Request) {
	// Check if this is a socket connection by looking at the underlying connection type
	isSocket := false
	if h, ok := r.Context().Value(http.LocalAddrContextKey).(*net.UnixAddr); ok && h != nil {
		isSocket = true
	}

	// Socket connections are pre-authorized, only check authorization for non-socket requests
	if !isSocket {
		user := getRequestUser(r)
		if !hasUser(user) {
			http.Error(rw, "unauthorized", http.StatusUnauthorized)
			log.Error().Msg("unauthorized")
			return
		}
	}

	// Read and parse the request body as FlexibleEmbeddingRequest
	body, err := io.ReadAll(io.LimitReader(r.Body, 10*MEGABYTE))
	if err != nil {
		log.Error().Err(err).Msg("error reading request body")
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	var embeddingRequest types.FlexibleEmbeddingRequest
	if err := json.Unmarshal(body, &embeddingRequest); err != nil {
		log.Error().Err(err).Msg("error parsing embedding request")
		http.Error(rw, "invalid JSON request", http.StatusBadRequest)
		return
	}

	if embeddingRequest.Model == "" {
		log.Error().Msg("model field is required")
		http.Error(rw, "model field is required", http.StatusBadRequest)
		return
	}

	// Get the appropriate client for the provider
	client, err := s.providerManager.GetClient(r.Context(), &manager.GetClientRequest{
		Provider: s.Cfg.RAG.PGVector.Provider,
	})
	if err != nil {
		log.Error().Err(err).Msg("error getting client")
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	// Forward the request to the provider using the flexible embeddings method
	resp, err := client.CreateFlexibleEmbeddings(r.Context(), embeddingRequest)
	if err != nil {
		log.Error().Err(err).Msg("error creating embeddings")
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, resp, http.StatusOK)
}
