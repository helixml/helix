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

	// Resolve the provider to use for embeddings
	embeddingsProvider := s.Cfg.RAG.PGVector.Provider

	// Special handling for rag-embedding placeholder model
	// When Haystack sends requests with model "rag-embedding", we substitute with the configured model from SystemSettings
	if embeddingRequest.Model == "rag-embedding" {
		settings, err := s.Store.GetEffectiveSystemSettings(r.Context())
		if err != nil {
			log.Error().Err(err).Msg("failed to get system settings for rag-embedding substitution")
			http.Error(rw, "Failed to get system settings", http.StatusInternalServerError)
			return
		}
		if settings.RAGEmbeddingsProvider == "" || settings.RAGEmbeddingsModel == "" {
			log.Warn().Msg("rag-embedding requested but no embedding model configured in system settings")
			http.Error(rw, "RAG embedding model not configured. Please configure the embedding model in Admin > System Settings.", http.StatusBadRequest)
			return
		}

		embeddingsProvider = settings.RAGEmbeddingsProvider
		resolvedModel := settings.RAGEmbeddingsProvider + "/" + settings.RAGEmbeddingsModel
		log.Debug().
			Str("original_model", "rag-embedding").
			Str("provider", settings.RAGEmbeddingsProvider).
			Str("model", settings.RAGEmbeddingsModel).
			Str("resolved_model", resolvedModel).
			Msg("substituted rag-embedding with configured embedding model")
		embeddingRequest.Model = resolvedModel
	}

	// Get the appropriate client for the provider
	client, err := s.providerManager.GetClient(r.Context(), &manager.GetClientRequest{
		Provider: embeddingsProvider,
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
