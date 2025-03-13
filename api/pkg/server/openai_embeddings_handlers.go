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
		log.Error().
			Err(err).
			Str("model", string(embeddingRequest.Model)).
			Str("provider", s.Cfg.RAG.PGVector.Provider).
			Msg("error creating embeddings")
		if apiErr, ok := err.(*openai.APIError); ok {
			errLog := log.Error().
				Int("status_code", apiErr.HTTPStatusCode).
				Str("type", apiErr.Type)
			if apiErr.Code != nil {
				errLog.Interface("code", apiErr.Code)
			}
			if apiErr.Param != nil {
				errLog.Str("param", *apiErr.Param)
			}
			errLog.Msg("vllm api error details")
		}
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(rw).Encode(resp)
}
