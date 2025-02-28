package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

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
	if !isSocket {
		user = getRequestUser(r)
		if !hasUser(user) {
			http.Error(rw, "unauthorized", http.StatusUnauthorized)
			log.Error().Msg("unauthorized")
			return
		}
	} else {
		// For socket connections, create a dummy user with socket owner type
		user = &types.User{
			Type: types.OwnerTypeSocket,
		}
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 10*MEGABYTE))
	if err != nil {
		log.Error().Err(err).Msg("error reading body")
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	// Log the raw request body
	log.Debug().RawJSON("raw_request", body).Msg("raw embedding request")

	var embeddingRequest openai.EmbeddingRequest
	if err := json.Unmarshal(body, &embeddingRequest); err != nil {
		log.Error().Err(err).Msg("error unmarshalling body")
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	// Log the request details for debugging
	inputStr := fmt.Sprint(embeddingRequest.Input)
	if len(inputStr) > 100 {
		inputStr = inputStr[:97] + "..."
	}
	log.Debug().
		Str("model", string(embeddingRequest.Model)).
		Str("input_preview", inputStr).
		Int("input_length", len(fmt.Sprint(embeddingRequest.Input))).
		Msg("creating embeddings request")

	client, err := s.providerManager.GetClient(r.Context(), &manager.GetClientRequest{
		Provider: s.Cfg.RAG.PGVector.Provider,
	})
	if err != nil {
		log.Error().Err(err).Msg("error getting client")
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	// Log just the essential fields
	log.Debug().
		Str("model", string(embeddingRequest.Model)).
		Interface("input_type", fmt.Sprintf("%T", embeddingRequest.Input)).
		Func(func(e *zerolog.Event) {
			if arr, ok := embeddingRequest.Input.([]interface{}); ok {
				e.Int("input_array_length", len(arr))
				if len(arr) > 0 {
					e.Str("first_item_type", fmt.Sprintf("%T", arr[0]))
					// Log first few characters of first item if it's a string
					if str, ok := arr[0].(string); ok {
						preview := str
						if len(preview) > 50 {
							preview = preview[:50] + "..."
						}
						e.Str("first_item_preview", preview)
					}
				}
			}
		}).
		Msg("embedding request fields")

	// Convert input to proper format if needed
	if arr, ok := embeddingRequest.Input.([]interface{}); ok {
		// Convert []interface{} to []string
		strArr := make([]string, len(arr))
		for i, v := range arr {
			if str, ok := v.(string); ok {
				strArr[i] = str
			} else {
				log.Error().
					Interface("item_type", fmt.Sprintf("%T", v)).
					Int("index", i).
					Msg("non-string item in input array")
				http.Error(rw, "input array must contain only strings", http.StatusBadRequest)
				return
			}
		}
		embeddingRequest.Input = strArr
	}

	// Create a clean request without unsupported parameters
	cleanRequest := openai.EmbeddingRequest{
		Model: embeddingRequest.Model,
		Input: embeddingRequest.Input,
		// Explicitly omit dimensions and encoding_format
	}

	resp, err := client.CreateEmbeddings(r.Context(), cleanRequest)
	if err != nil {
		log.Error().
			Err(err).
			Str("model", string(embeddingRequest.Model)).
			Interface("input_type", fmt.Sprintf("%T", embeddingRequest.Input)).
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
