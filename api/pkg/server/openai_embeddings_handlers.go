package server

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/helixml/helix/api/pkg/openai/manager"
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
	user := getRequestUser(r)

	if !hasUser(user) {
		http.Error(rw, "unauthorized", http.StatusUnauthorized)
		log.Error().Msg("unauthorized")
		return
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
		Provider: s.Cfg.Embeddings.Provider,
	})
	if err != nil {
		log.Error().Err(err).Msg("error getting client")
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	resp, err := client.CreateEmbeddings(r.Context(), embeddingRequest)
	if err != nil {
		log.Error().Err(err).Msg("error creating embeddings")
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(rw).Encode(resp)
}
