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
		log.Debug().
			Str("original_model", "rag-embedding").
			Str("provider", settings.RAGEmbeddingsProvider).
			Str("model", settings.RAGEmbeddingsModel).
			Msg("substituted rag-embedding with configured embedding model")
		// Set the model name WITHOUT provider prefix — the provider is resolved separately
		// via GetClient. Sending "openai/text-embedding-3-small" to OpenAI's API causes 404.
		embeddingRequest.Model = settings.RAGEmbeddingsModel
		// Force float encoding — the OpenAI Python SDK (used by haystack) defaults to base64,
		// but our response struct expects []float32
		embeddingRequest.EncodingFormat = "float"
	}

	// Special handling for kodit-text-embedding placeholder model
	// When Kodit sends requests with model "kodit-text-embedding", substitute with the configured text embedding model from SystemSettings
	if embeddingRequest.Model == "kodit-text-embedding" {
		settings, err := s.Store.GetEffectiveSystemSettings(r.Context())
		if err != nil {
			log.Error().Err(err).Msg("failed to get system settings for kodit-text-embedding substitution")
			http.Error(rw, "Failed to get system settings", http.StatusInternalServerError)
			return
		}
		if settings.KoditTextEmbeddingProvider == "" || settings.KoditTextEmbeddingModel == "" {
			log.Warn().Msg("kodit-text-embedding requested but no text embedding model configured in system settings")
			http.Error(rw, "Kodit text embedding model not configured. Please configure the text embedding model in Admin > System Settings.", http.StatusBadRequest)
			return
		}
		embeddingsProvider = settings.KoditTextEmbeddingProvider
		log.Debug().
			Str("original_model", "kodit-text-embedding").
			Str("provider", settings.KoditTextEmbeddingProvider).
			Str("model", settings.KoditTextEmbeddingModel).
			Msg("substituted kodit-text-embedding with configured text embedding model")
		embeddingRequest.Model = settings.KoditTextEmbeddingModel
		embeddingRequest.EncodingFormat = "float"
	}

	// Special handling for kodit-vision-embedding placeholder model
	// When Kodit sends vision embedding requests (with messages containing image_url parts)
	// using model "kodit-vision-embedding", substitute with the configured vision embedding model.
	if embeddingRequest.Model == "kodit-vision-embedding" {
		settings, err := s.Store.GetEffectiveSystemSettings(r.Context())
		if err != nil {
			log.Error().Err(err).Msg("failed to get system settings for kodit-vision-embedding substitution")
			http.Error(rw, "Failed to get system settings", http.StatusInternalServerError)
			return
		}
		if settings.KoditVisionEmbeddingProvider == "" || settings.KoditVisionEmbeddingModel == "" {
			log.Warn().Msg("kodit-vision-embedding requested but no vision embedding model configured in system settings")
			http.Error(rw, "Kodit vision embedding model not configured. Please configure the vision embedding model in Admin > System Settings.", http.StatusBadRequest)
			return
		}
		embeddingsProvider = settings.KoditVisionEmbeddingProvider
		log.Debug().
			Str("original_model", "kodit-vision-embedding").
			Str("provider", settings.KoditVisionEmbeddingProvider).
			Str("model", settings.KoditVisionEmbeddingModel).
			Msg("substituted kodit-vision-embedding with configured vision embedding model")
		embeddingRequest.Model = settings.KoditVisionEmbeddingModel
		embeddingRequest.EncodingFormat = "float"
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
