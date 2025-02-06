package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

// POST https://app.tryhelix.ai/v1/chat/completions

// createTool godoc
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
		string(s.Cfg.Inference.Provider),
		chatCompletionRequest.Model,
		types.SessionModeInference,
		types.SessionTypeText,
		false,
		false,
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

	id := system.GenerateOpenAIResponseID()

	ctx := oai.SetContextValues(r.Context(), &oai.ContextValues{
		OwnerID:         ownerID,
		SessionID:       id,
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
		OpenAIResponseID: id,
	}

	if user.AppID != "" {
		options.AppID = user.AppID

		ctx = oai.SetContextAppID(ctx, user.AppID)

		log.Debug().Str("app_id", options.AppID).Msg("using app_id from request")

		// Check if the appID contains a LORA
		assistant, err := s.getAppLoraAssistant(ctx, options.AppID)
		if err != nil {
			log.Error().Err(err).Msg("error getting app assistant")
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Debug().Interface("assistant", assistant).Msg("got app assistant")

		// If it has a Lora, we must use the old sessions handler.
		if assistant != nil {
			// Override the request's query parameters to set the app details
			query := r.URL.Query()
			query.Set("app_id", options.AppID)
			query.Set("assistant_id", assistant.ID)
			query.Set("lora_id", assistant.LoraID)
			r.URL.RawQuery = query.Encode()

			// Create a new body in the format the sessions API is expecting
			messages := []*types.Message{}
			for _, message := range chatCompletionRequest.Messages {
				messages = append(messages, &types.Message{
					Role: types.CreatorType(message.Role),
					Content: types.MessageContent{
						ContentType: types.MessageContentTypeText,
						Parts:       []any{message.Content},
					},
				})
			}
			sessionBody := types.SessionChatRequest{
				Model:    chatCompletionRequest.Model,
				Stream:   chatCompletionRequest.Stream,
				Messages: messages,
				// Do not set lora_id or lora_dir here. It will break the logic in the handler.
			}
			body, err := json.Marshal(sessionBody)
			if err != nil {
				log.Error().Err(err).Msg("error marshalling session body")
				http.Error(rw, err.Error(), http.StatusInternalServerError)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			log.Debug().Str("app_id", options.AppID).Str("lora_id", assistant.LoraID).Msg("overriding app_id in request and passing to old Session handler")
			s.startChatSessionLegacyHandler(ctx, user, &sessionBody, r, rw)
			return
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

		err = json.NewEncoder(rw).Encode(resp)
		if err != nil {
			log.Error().Err(err).Msg("error writing response")
		}
		return
	}

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

func (s *HelixAPIServer) getAppLoraAssistant(ctx context.Context, appID string) (*types.AssistantConfig, error) {
	app, err := s.Store.GetAppWithTools(ctx, appID)
	if err != nil {
		return nil, err
	}

	// The old code had this in:
	// TODO: support > 1 assistant
	// because the sessions API can only handle one assistant at a time...
	var assistant *types.AssistantConfig
	if len(app.Config.Helix.Assistants) > 0 {
		if app.Config.Helix.Assistants[0].LoraID != "" {
			assistant = &app.Config.Helix.Assistants[0]
		}
	}

	return assistant, nil
}
