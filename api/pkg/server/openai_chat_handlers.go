package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/model"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

const (
	BYTE = 1 << (10 * iota)
	KILOBYTE
	MEGABYTE
)

// POST https://app.helix.ml/v1/chat/completions

// createChatCompletion godoc
// @Summary Stream responses for chat
// @Description Creates a model response for the given chat conversation.
// @Tags    chat
// @Success 200 {object} openai.ChatCompletionResponse
// @Param request    body openai.ChatCompletionRequest true "Request body with options for conversational AI.")
// @Router /v1/chat/completions [post]
// @Security BearerAuth
// @externalDocs.url https://platform.openai.com/docs/api-reference/chat/create
func (s *HelixAPIServer) createChatCompletion(rw http.ResponseWriter, r *http.Request) {
	addCorsHeaders(rw)
	if r.Method == http.MethodOptions {
		return
	}

	user := getRequestUser(r)

	if !hasUserOrRunner(user) {
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

	var chatCompletionRequest openai.ChatCompletionRequest
	err = json.Unmarshal(body, &chatCompletionRequest)
	if err != nil {
		log.Error().Err(err).Msg("error unmarshalling body")
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	ownerID := user.ID
	if user.TokenType == types.TokenTypeRunner {
		ownerID = oai.RunnerID
	}

	// Parse provider prefix from model name (e.g., "openrouter/gpt-4" -> provider="openrouter", model="gpt-4")
	// But first, check if the full model name (with slash) exists in any provider's model list.
	// This handles HuggingFace-style model IDs like "Qwen/Qwen3-Coder" that might be incorrectly
	// parsed as provider prefixes when there's also a provider named "Qwen".
	var validatedProvider string
	if strings.Contains(chatCompletionRequest.Model, "/") {
		// Model name contains a slash - could be a HuggingFace model ID
		// Check if any provider has this exact full model name in their model list
		foundProvider := s.findProviderWithModel(r.Context(), chatCompletionRequest.Model, ownerID)
		if foundProvider != "" {
			// Found a provider with this exact model - use it and keep the full model name
			validatedProvider = foundProvider
			log.Debug().
				Str("model", chatCompletionRequest.Model).
				Str("provider", foundProvider).
				Msg("using full model name match (avoiding HF prefix collision)")
		}
	}

	// If we didn't find a full model match, fall back to prefix parsing
	if validatedProvider == "" {
		providerFromModel, modelWithoutPrefix := model.ParseProviderFromModel(chatCompletionRequest.Model)
		if providerFromModel != "" {
			// Check if this prefix is a known provider (global or user-defined)
			if s.isKnownProvider(r.Context(), providerFromModel, ownerID) {
				validatedProvider = providerFromModel
				chatCompletionRequest.Model = modelWithoutPrefix
			}
			// If not a known provider, treat the whole string as the model name (e.g., "meta-llama/Model")
		}
	}

	modelName, err := model.ProcessModelName(
		s.Cfg.Inference.Provider,
		chatCompletionRequest.Model,
		types.SessionTypeText,
	)
	if err != nil {
		log.Error().Err(err).Msg("error processing model name")
		http.Error(rw, "invalid model name: "+err.Error(), http.StatusBadRequest)
		return
	}

	chatCompletionRequest.Model = modelName

	responseID := system.GenerateOpenAIResponseID()

	ctx := oai.SetContextValues(r.Context(), &oai.ContextValues{
		OwnerID:         ownerID,
		SessionID:       responseID,
		InteractionID:   "n/a",
		OriginalRequest: body,
	})

	options := &controller.ChatCompletionOptions{
		AppID:       r.URL.Query().Get("app_id"),
		AssistantID: r.URL.Query().Get("assistant_id"),
		RAGSourceID: r.URL.Query().Get("rag_source_id"),
		Provider:    validatedProvider,
		QueryParams: func() map[string]string {
			params := make(map[string]string)
			for key, values := range r.URL.Query() {
				if len(values) > 0 {
					params[key] = values[0]
				}
			}
			return params
		}(),
	}

	var app *types.App

	switch {
	// If app ID is set from authentication token
	case user.AppID != "":
		// Basic sanity validation to see whether app ID from URL query matches
		// the app ID from the authentication token
		if options.AppID != "" && user.AppID != options.AppID {
			log.Error().Str("app_id", user.AppID).Str("requested_app_id", options.AppID).Msg("app IDs do not match")
			http.Error(rw, "URL query app_id does not match token app_id", http.StatusBadRequest)
			return
		}

		app, err = s.Store.GetApp(ctx, user.AppID)
		if err != nil {
			log.Error().Err(err).Str("app_id", user.AppID).Msg("error getting app")
			http.Error(rw, fmt.Sprintf("Error getting app: %s", err), http.StatusInternalServerError)
			return
		}

		options.AppID = user.AppID
	// If app is set through URL query options
	case options.AppID != "":
		app, err = s.Store.GetApp(ctx, options.AppID)
		if err != nil {
			log.Error().Err(err).Str("app_id", options.AppID).Msg("error getting app")
			http.Error(rw, fmt.Sprintf("Error getting app: %s", err), http.StatusInternalServerError)
			return
		}
	}

	// If app is set
	if app != nil {
		// If app has org - set it
		if app.OrganizationID != "" {
			options.OrganizationID = app.OrganizationID
		}

		ctx = oai.SetContextAppID(ctx, app.ID)
		ctx = oai.SetContextOrganizationID(ctx, options.OrganizationID)

		log.Debug().Str("app_id", options.AppID).Msg("using app_id from request")

		// If an app_id is being used, verify that the user has access to it

		if err := s.authorizeUserToApp(ctx, user, app, types.ActionGet); err != nil {
			log.Error().Err(err).Str("app_id", options.AppID).Str("user_id", user.ID).Msg("user is not authorized to access this app")
			http.Error(rw, fmt.Sprintf("Not authorized to access app: %s", err), http.StatusForbidden)
			return
		}

		// Get any existing session ID from the query parameters to tie the responses to a specific session
		if sessionID := r.URL.Query().Get("session_id"); sessionID != "" {
			ctx = oai.SetContextSessionID(ctx, sessionID)
			log.Debug().Str("session_id", sessionID).Msg("setting session_id in context for document tracking")
		}
	}

	ctx = oai.SetContextAppID(ctx, options.AppID)

	// Non-streaming request returns the response immediately
	if !chatCompletionRequest.Stream {
		resp, _, err := s.Controller.ChatCompletion(ctx, user, chatCompletionRequest, options)
		if err != nil {
			log.Error().Err(err).Msg("error creating chat completion")
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		rw.Header().Set("Content-Type", "application/json")

		if r.URL.Query().Get("pretty") == "true" {
			// Pretty print the response with indentation
			bts, err := json.MarshalIndent(resp, "", "  ")
			if err != nil {
				log.Error().Err(err).Msg("error marshalling response")
				http.Error(rw, err.Error(), http.StatusInternalServerError)
				return
			}

			_, _ = rw.Write(bts)
			return
		}

		resp.ID = responseID

		err = json.NewEncoder(rw).Encode(resp)
		if err != nil {
			log.Error().Err(err).Msg("error writing response")
		}
		return
	}

	//  Will instruct the agent to send thoughts about tools and decisions
	options.Conversational = true

	// Streaming request, receive and write the stream in chunks
	stream, _, err := s.Controller.ChatCompletionStream(ctx, user, chatCompletionRequest, options)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	defer stream.Close()

	rw.Header().Set("Content-Type", "text/event-stream")
	rw.Header().Set("Cache-Control", "no-cache")
	rw.Header().Set("Connection", "keep-alive")

	// Write the stream into the response
	for {
		response, err := stream.Recv()
		response.ID = responseID
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		// Write the response to the client
		bts, err := json.Marshal(response)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := writeChunk(rw, bts); err != nil {
			log.Error().Msgf("failed to write completion chunk: %v", err)
		}
	}
}

// isKnownProvider checks if a provider name exists as a global or user-defined provider
func (s *HelixAPIServer) isKnownProvider(ctx context.Context, providerName, ownerID string) bool {
	// Check global providers first (fast path)
	if types.IsGlobalProvider(providerName) {
		return true
	}
	// Check for system-owned global providers (e.g., dynamic providers from env vars)
	_, err := s.Store.GetProviderEndpoint(ctx, &store.GetProviderEndpointsQuery{
		Name:      providerName,
		Owner:     string(types.OwnerTypeSystem),
		OwnerType: types.OwnerTypeSystem,
	})
	if err == nil {
		return true
	}
	// Check for user-defined providers
	_, err = s.Store.GetProviderEndpoint(ctx, &store.GetProviderEndpointsQuery{
		Name:  providerName,
		Owner: ownerID,
	})
	return err == nil
}

// findProviderWithModel searches all accessible providers for one that has the given model
// in its model list. This checks:
// 1. Global providers from env vars (helix, openai, togetherai, anthropic, vllm) - via cached model lists
// 2. Database-stored provider endpoints - via cached model lists AND static Models field
//
// This is used to handle HuggingFace-style model IDs (e.g., "Qwen/Qwen3-Coder") that might be
// incorrectly parsed as provider prefixes when there's also a provider named "Qwen".
//
// Returns the provider name if found, empty string otherwise.
func (s *HelixAPIServer) findProviderWithModel(ctx context.Context, modelName, ownerID string) string {
	// First check global providers from env vars (these are not in the database)
	// Their cache key uses "system" as owner
	globalProviders, err := s.providerManager.ListProviders(ctx, "")
	if err == nil {
		for _, globalProvider := range globalProviders {
			cacheKey := fmt.Sprintf("%s:%s", globalProvider, types.OwnerTypeSystem)
			if cached, found := s.cache.Get(cacheKey); found {
				var cachedModels []types.OpenAIModel
				if err := json.Unmarshal([]byte(cached), &cachedModels); err == nil {
					for _, m := range cachedModels {
						if m.ID == modelName {
							log.Debug().
								Str("model", modelName).
								Str("provider", string(globalProvider)).
								Msg("found full model name in global provider's cached model list")
							return string(globalProvider)
						}
					}
				}
			}
		}
	}

	// Now check database-stored provider endpoints (user + global from DB)
	providers, err := s.Store.ListProviderEndpoints(ctx, &store.ListProviderEndpointsQuery{
		Owner:      ownerID,
		WithGlobal: true,
	})
	if err != nil {
		log.Warn().Err(err).Msg("failed to list provider endpoints for model lookup")
		return ""
	}

	// Check each provider's model list for the full model name
	for _, provider := range providers {
		// First check the cached model list (dynamically fetched from provider's /v1/models)
		// This is where the UI gets its model list from
		cacheKey := fmt.Sprintf("%s:%s", provider.Name, provider.Owner)
		if cached, found := s.cache.Get(cacheKey); found {
			var cachedModels []types.OpenAIModel
			if err := json.Unmarshal([]byte(cached), &cachedModels); err == nil {
				for _, m := range cachedModels {
					if m.ID == modelName {
						log.Debug().
							Str("model", modelName).
							Str("provider", provider.Name).
							Msg("found full model name in provider's cached model list")
						return provider.Name
					}
				}
			}
		}

		// Fall back to checking the static Models field stored in database
		for _, m := range provider.Models {
			if m == modelName {
				log.Debug().
					Str("model", modelName).
					Str("provider", provider.Name).
					Msg("found full model name in provider's static model list")
				return provider.Name
			}
		}
	}

	return ""
}
