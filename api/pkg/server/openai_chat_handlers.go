package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/model"
	oai "github.com/helixml/helix/api/pkg/openai"
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
	ownerID := user.ID
	if user.TokenType == types.TokenTypeRunner {
		ownerID = oai.RunnerID
	}

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
