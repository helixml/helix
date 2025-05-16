package server

import (
	"encoding/json"
	"io"
	"net"
	"net/http"

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

	var flexEmbeddingRequest types.FlexibleEmbeddingRequest
	var embeddingRequest openai.EmbeddingRequest
	var isFlexible bool

	// First try to parse as flexible embedding request
	err = json.Unmarshal(body, &flexEmbeddingRequest)
	if err != nil {
		log.Error().Err(err).Msg("error unmarshalling flexible embedding request")
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	// If Messages field is set, this is a Chat Embeddings API request
	if len(flexEmbeddingRequest.Messages) > 0 {
		isFlexible = true
	} else {
		// Otherwise, try to parse as standard OpenAI embedding request
		err = json.Unmarshal(body, &embeddingRequest)
		if err != nil {
			log.Error().Err(err).Msg("error unmarshalling standard embedding request")
			http.Error(rw, err.Error(), http.StatusBadRequest)
			return
		}
	}

	client, err := s.providerManager.GetClient(r.Context(), &manager.GetClientRequest{
		Provider: s.Cfg.RAG.PGVector.Provider,
	})
	if err != nil {
		log.Error().Err(err).Msg("error getting client")
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	if isFlexible {
		log.Debug().Msg("Processing flexible embedding request with Chat Embeddings API format")

		// Create flexible embeddings with the OpenAI-compatible client
		// For now, we'll convert the request to the standard format
		// In the future, we'll need to extend the client interface to support flexible embeddings
		log.Warn().Msg("Chat Embeddings API format detected but not fully implemented, falling back to standard embedding")

		// Convert flexible request to standard OpenAI format if possible
		if flexEmbeddingRequest.Input != nil {
			embeddingRequest = openai.EmbeddingRequest{
				Model:          openai.EmbeddingModel(flexEmbeddingRequest.Model),
				Input:          flexEmbeddingRequest.Input,
				EncodingFormat: openai.EmbeddingEncodingFormat(flexEmbeddingRequest.EncodingFormat),
				Dimensions:     flexEmbeddingRequest.Dimensions,
			}

			resp, err := client.CreateEmbeddings(r.Context(), embeddingRequest)
			if err != nil {
				log.Error().Err(err).Msg("error creating embeddings")
				http.Error(rw, err.Error(), http.StatusInternalServerError)
				return
			}

			writeResponse(rw, resp, http.StatusOK)
		} else {
			// Cannot handle Chat Embeddings API format yet
			log.Error().Msg("Chat Embeddings API with messages detected but not supported by this endpoint")
			http.Error(rw, "Chat Embeddings API with messages not supported", http.StatusBadRequest)
		}
		return
	} else {
		// Create a clean request without unsupported parameters
		cleanRequest := openai.EmbeddingRequest{
			Model:          embeddingRequest.Model,
			Input:          embeddingRequest.Input,
			EncodingFormat: "float",
			// Explicitly omit dimensions
		}

		resp, err := client.CreateEmbeddings(r.Context(), cleanRequest)
		if err != nil {
			log.Error().Err(err).Msg("error creating embeddings")
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		writeResponse(rw, resp, http.StatusOK)
		return
	}
}
