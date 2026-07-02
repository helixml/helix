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

// agentToolNudge is appended to the system prompt of any tool-enabled chat
// completion. Some models (e.g. GLM) narrate a plan and return finish_reason
// "stop" with no tool call; the Zed agent reads "no tool call" as end-of-turn
// and hands control back to the user, who then has to prod it. Claude chains
// planning into the tool call in one turn, so it never strands the user. This
// directive pushes narrate-then-stop models to act in the same turn.
const agentToolNudge = "You have tools available. When you state an action you are about to take, call the corresponding tool in the same turn. Never end your turn with only a plan, a description of what you will do next, or a question about whether to proceed. If any work remains, call a tool. Stop only when the task is complete or you genuinely need input from the user."

// injectAgentToolNudge appends agentToolNudge to the request's system prompt,
// but only for tool-enabled requests (a request with no tools cannot act via a
// tool, so the directive would be noise). It merges into an existing leading
// system message when that message uses plain string content, otherwise it
// prepends a fresh system message (go-openai rejects a message that sets both
// Content and MultiContent).
func injectAgentToolNudge(req *openai.ChatCompletionRequest) {
	if len(req.Tools) == 0 {
		return
	}
	if len(req.Messages) > 0 &&
		req.Messages[0].Role == openai.ChatMessageRoleSystem &&
		len(req.Messages[0].MultiContent) == 0 {
		if req.Messages[0].Content != "" {
			req.Messages[0].Content += "\n\n"
		}
		req.Messages[0].Content += agentToolNudge
		return
	}
	req.Messages = append([]openai.ChatCompletionMessage{{
		Role:    openai.ChatMessageRoleSystem,
		Content: agentToolNudge,
	}}, req.Messages...)
}

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

	if !s.Cfg.Inference.DisableAgentToolNudge {
		injectAgentToolNudge(&chatCompletionRequest)
	}

	ownerID := user.ID
	if user.TokenType == types.TokenTypeRunner {
		ownerID = oai.RunnerID
	}

	// Special handling for kodit-model proxy
	// When Kodit sends requests with model "kodit-model", we substitute with the configured model from SystemSettings
	if chatCompletionRequest.Model == "kodit-model" {
		settings, err := s.Store.GetEffectiveSystemSettings(r.Context())
		if err != nil {
			log.Error().Err(err).Msg("failed to get system settings for kodit-model substitution")
			http.Error(rw, "Failed to get system settings", http.StatusInternalServerError)
			return
		}
		if settings.KoditEnrichmentProvider == "" || settings.KoditEnrichmentModel == "" {
			log.Warn().Msg("kodit-model requested but no enrichment model configured in system settings")
			http.Error(rw, "Code Intelligence model not configured. Please configure the enrichment model in Admin > System Settings.", http.StatusBadRequest)
			return
		}

		// Combine provider and model into the format expected by Helix routing
		// e.g., "together_ai" + "Qwen/Qwen3-8B" -> "together_ai/Qwen/Qwen3-8B"
		resolvedModel := settings.KoditEnrichmentProvider + "/" + settings.KoditEnrichmentModel
		log.Debug().
			Str("original_model", "kodit-model").
			Str("provider", settings.KoditEnrichmentProvider).
			Str("model", settings.KoditEnrichmentModel).
			Str("resolved_model", resolvedModel).
			Msg("substituted kodit-model with configured enrichment model")
		chatCompletionRequest.Model = resolvedModel
	}

	// Find which provider owns this model by checking all providers' cached model lists.
	// This handles both unprefixed models (e.g., "claude-haiku-4-5-20251001" → anthropic)
	// and HuggingFace-style IDs (e.g., "Qwen/Qwen3-Coder") that might be incorrectly
	// parsed as provider prefixes.
	var validatedProvider string
	foundProvider, bareModel := s.findProviderWithModel(r.Context(), chatCompletionRequest.Model, ownerID, user.OrganizationID)
	if foundProvider != "" {
		validatedProvider = foundProvider
		// Strip the provider/ prefix before forwarding — upstream APIs don't
		// know our naming scheme. bareModel is the upstream id.
		chatCompletionRequest.Model = bareModel
		log.Debug().
			Str("model", chatCompletionRequest.Model).
			Str("provider", foundProvider).
			Msg("found provider via model list lookup")
	}

	// If no provider found by model name, fall back to prefix parsing
	if validatedProvider == "" {
		providerFromModel, modelWithoutPrefix := model.ParseProviderFromModel(chatCompletionRequest.Model)
		if providerFromModel != "" {
			// Check if this prefix is a known provider (global or user-defined)
			if s.isKnownProvider(r.Context(), providerFromModel, ownerID, user.OrganizationID) {
				validatedProvider = providerFromModel
				chatCompletionRequest.Model = modelWithoutPrefix
			}
			// If not a known provider, treat the whole string as the model name (e.g., "meta-llama/Model")
		}
	}

	// Last-resort: resolve the owning provider with a live model-list fetch.
	// findProviderWithModel above only reads the model cache, which is warmed
	// lazily by /v1/models and the picker. On a cold cache an unprefixed model
	// id (e.g. one a downstream Helix forwarded after stripping its provider
	// prefix) has nothing to match and would fall through to the default
	// provider and 500. This only runs when both cache lookup and prefix
	// parsing failed, so prefixed / cache-warm requests are unaffected.
	if validatedProvider == "" {
		if foundProvider, bareModel := s.resolveModelProviderLive(r.Context(), chatCompletionRequest.Model, ownerID, user.OrganizationID); foundProvider != "" {
			validatedProvider = foundProvider
			chatCompletionRequest.Model = bareModel
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
		ProjectID:       user.ProjectID,
		SpecTaskID:      user.SpecTaskID,
		SessionID:       responseID,
		InteractionID:   "n/a",
		OriginalRequest: body,
	})

	options := &controller.ChatCompletionOptions{
		OrganizationID: user.OrganizationID,
		AppID:          r.URL.Query().Get("app_id"),
		AssistantID:    r.URL.Query().Get("assistant_id"),
		RAGSourceID:    r.URL.Query().Get("rag_source_id"),
		Provider:       validatedProvider,
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

// isKnownProvider checks if a provider name exists as a global, user-defined, or org-scoped provider
func (s *HelixAPIServer) isKnownProvider(ctx context.Context, providerName, ownerID, orgID string) bool {
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
	// Check for user-defined providers (user's own)
	_, err = s.Store.GetProviderEndpoint(ctx, &store.GetProviderEndpointsQuery{
		Name:  providerName,
		Owner: ownerID,
	})
	if err == nil {
		return true
	}
	// Check for org-scoped providers (endpoint owned by the org, not the user)
	if orgID != "" && orgID != ownerID {
		_, err = s.Store.GetProviderEndpoint(ctx, &store.GetProviderEndpointsQuery{
			Name:  providerName,
			Owner: orgID,
		})
		if err == nil {
			return true
		}
	}
	// Check for admin-created global endpoints (owned by other users but endpoint_type = 'global')
	// These are visible to all users but owned by the admin who created them
	providers, err := s.Store.ListProviderEndpoints(ctx, &store.ListProviderEndpointsQuery{
		Owner:      ownerID,
		WithGlobal: true,
	})
	if err == nil {
		for _, p := range providers {
			if p.Name == providerName {
				return true
			}
		}
	}
	return false
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
// findProviderWithModel returns the provider that owns `modelName` plus the
// bare upstream model id (with any provider/ prefix stripped). Returns
// ("", "") when no match. Caller must use the returned bare id when
// forwarding the request — upstream APIs (api.openai.com etc.) don't know
// our prefix scheme.
func (s *HelixAPIServer) findProviderWithModel(ctx context.Context, modelName, ownerID, orgID string) (string, string) {
	// First check global providers from env vars (these are not in the database)
	// Their cache key uses "system" as owner
	globalProviders, err := s.providerManager.ListProviders(ctx, "")
	if err == nil {
		for _, globalProvider := range globalProviders {
			residue := modelName
			if strings.HasPrefix(modelName, string(globalProvider)+"/") {
				residue = modelName[len(globalProvider)+1:]
			}
			cacheKey := fmt.Sprintf("%s:%s", globalProvider, types.OwnerTypeSystem)
			// Read via loadCachedModels: the cache stores the wrapped
			// cachedModels{Models,FetchedAt} payload (the only writer is
			// refreshProviderModels). Unmarshalling raw into []OpenAIModel
			// here silently failed for every dynamically-fetched provider,
			// so this lookup only ever matched static model lists.
			if cm, found := s.loadCachedModels(cacheKey); found {
				for _, m := range cm.Models {
					if m.ID == modelName || m.ID == residue {
						log.Debug().
							Str("model", modelName).
							Str("provider", string(globalProvider)).
							Str("bare_model", residue).
							Msg("found full model name in global provider's cached model list")
						return string(globalProvider), residue
					}
				}
			}
		}
	}

	// Check database-stored provider endpoints for the user (user + global from DB)
	providers, err := s.Store.ListProviderEndpoints(ctx, &store.ListProviderEndpointsQuery{
		Owner:      ownerID,
		WithGlobal: true,
	})
	if err != nil {
		log.Warn().Err(err).Msg("failed to list provider endpoints for model lookup")
		return "", ""
	}

	// Also check org-scoped endpoints if the user belongs to an org
	if orgID != "" && orgID != ownerID {
		orgProviders, err := s.Store.ListProviderEndpoints(ctx, &store.ListProviderEndpointsQuery{
			Owner:      orgID,
			WithGlobal: false, // Global already covered above
		})
		if err != nil {
			log.Warn().Err(err).Str("org_id", orgID).Msg("failed to list org provider endpoints for model lookup")
		} else {
			providers = append(providers, orgProviders...)
		}
	}

	// Check each provider's model list for the full model name.
	//
	// We accept two shapes:
	//
	//   1. Bare upstream id (`gpt-5.2`) — direct match against cached/static.
	//   2. Helix-prefixed id (`<provider>/gpt-5.2`) — Zed's settings.json
	//      stores the prefixed form (mapHelixToZedProvider emits it) and
	//      sends it back on /v1/chat/completions. Provider names with a
	//      slash (e.g. `user/openai`) make first-slash splitting in
	//      ParseProviderFromModel useless, so we bypass that here by
	//      stripping the provider's literal `Name + "/"` prefix and
	//      matching the residue against cached/static ids.
	for _, provider := range providers {
		residue := modelName
		if strings.HasPrefix(modelName, provider.Name+"/") {
			residue = modelName[len(provider.Name)+1:]
		}

		// First check the cached model list (dynamically fetched from provider's /v1/models)
		// This is where the UI gets its model list from. Read via loadCachedModels
		// to match the wrapped cachedModels{} payload format the writer uses.
		cacheKey := fmt.Sprintf("%s:%s", provider.Name, provider.Owner)
		if cm, found := s.loadCachedModels(cacheKey); found {
			for _, m := range cm.Models {
				if m.ID == modelName || m.ID == residue {
					log.Debug().
						Str("model", modelName).
						Str("provider", provider.Name).
						Str("bare_model", residue).
						Msg("found full model name in provider's cached model list")
					return provider.Name, residue
				}
			}
		}

		// Fall back to checking the static Models field stored in database
		for _, m := range provider.Models {
			if m == modelName || m == residue {
				log.Debug().
					Str("model", modelName).
					Str("provider", provider.Name).
					Str("bare_model", residue).
					Msg("found full model name in provider's static model list")
				return provider.Name, residue
			}
		}
	}

	return "", ""
}
