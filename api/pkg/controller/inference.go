package controller

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/tools"
	"github.com/helixml/helix/api/pkg/types"

	openai "github.com/lukemarsden/go-openai2"
	"github.com/rs/zerolog/log"
)

type ChatCompletionOptions struct {
	AppID       string
	AssistantID string
	RAGSourceID string

	// TODO: API tool query param overrides
}

// ChatCompletion is used by the OpenAI compatible API. Doesn't handle any historical sessions, etc.
// Runs the OpenAI with tools/app configuration and returns the response.
func (c *Controller) ChatCompletion(ctx context.Context, user *types.User, req openai.ChatCompletionRequest, opts *ChatCompletionOptions) (openai.ChatCompletionResponse, error) {

	toolResp, ok, err := c.evaluateToolUsage(ctx, user, req, opts)
	if err != nil {
		return openai.ChatCompletionResponse{}, fmt.Errorf("failed to load tools: %w", err)
	}

	if ok {
		return openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{
				{
					Message: openai.ChatCompletionMessage{
						Content: toolResp.Message,
					},
				},
			},
		}, nil
	}

	// TODO: setup RAG prompt if source set

	resp, err := c.openAIClient.CreateChatCompletion(ctx, req)
	if err != nil {
		log.Err(err).Msg("error creating chat completion")
		return openai.ChatCompletionResponse{}, err
	}

	return resp, nil
}

func (c *Controller) authorizeUserToApp(user *types.User, app *types.App) error {
	if (!app.Global && !app.Shared) && app.Owner != user.ID {
		return system.NewHTTPError403(fmt.Sprintf("you do not have access to the app with the id: %s", app.ID))
	}

	return nil
}

func (c *Controller) evaluateToolUsage(ctx context.Context, user *types.User, req openai.ChatCompletionRequest, opts *ChatCompletionOptions) (*tools.RunActionResponse, bool, error) {
	assistant, err := c.loadAssistant(ctx, user, opts)
	if err != nil {
		return nil, false, err
	}

	if len(assistant.Tools) == 0 {
		return nil, false, fmt.Errorf("no tools found in assistant")
	}

	// Get last message from the chat completion messages
	var lastMessage string

	if len(req.Messages) > 0 {
		lastMessage = req.Messages[len(req.Messages)-1].Content
	}

	var options []tools.Option

	// If assistant has configured an actionable template, use it
	if assistant != nil && assistant.IsActionableTemplate != "" {
		options = append(options, tools.WithIsActionableTemplate(assistant.IsActionableTemplate))
	}

	// TODO: replace history with other types so we don't put in whole
	// integrations as we don't have that type here

	history := []*types.Interaction{}

	isActionable, err := c.ToolsPlanner.IsActionable(ctx, "dummy", "dummy", assistant.Tools, history, lastMessage, options...)
	if err != nil {
		log.Error().Err(err).Msg("failed to evaluate of the message is actionable, skipping to general knowledge")
		return nil, false, fmt.Errorf("failed to evaluate of the message is actionable: %w", err)
	}

	if !isActionable.Actionable() {
		return nil, false, nil
	}

	selectedTool, ok := getToolFromAction(assistant.Tools, isActionable.Api)
	if !ok {
		return nil, false, fmt.Errorf("tool not found for action: %s", isActionable.Api)
	}

	resp, err := c.ToolsPlanner.RunAction(ctx, "dummy", "dummy", selectedTool, history, lastMessage, isActionable.Api)
	if err != nil {
		return nil, false, fmt.Errorf("failed to perform action: %w", err)
	}

	return resp, true, nil
}

func (c *Controller) loadAssistant(ctx context.Context, user *types.User, opts *ChatCompletionOptions) (*types.AssistantConfig, error) {
	if opts.AppID == "" {
		return &types.AssistantConfig{}, nil
	}

	app, err := c.Options.Store.GetApp(ctx, opts.AppID)
	if err != nil {
		return nil, fmt.Errorf("error getting app: %w", err)
	}

	if (!app.Global && !app.Shared) && app.Owner != user.ID {
		return nil, fmt.Errorf("you do not have access to the app with the id: %s", app.ID)
	}

	assistant := data.GetAssistant(app, opts.AssistantID)

	if assistant == nil {
		return nil, fmt.Errorf("we could not find the assistant with ID %s, in app %s", opts.AssistantID, app.ID)
	}

	return assistant, nil
}

// ChatCompletion is used by the OpenAI compatible API. Doesn't handle any historical sessions, etc.
// Runs the OpenAI with tools/app configuration and returns the stream.
func (c *Controller) ChatCompletionStream(ctx context.Context, user *types.User, req openai.ChatCompletionRequest, opts *ChatCompletionOptions) (*openai.ChatCompletionStream, error) {

	// TODO: setup app settings
	// TODO: setup RAG prompt if source set

	stream, err := c.openAIClient.CreateChatCompletionStream(ctx, req)
	if err != nil {
		log.Err(err).Msg("error creating chat completion stream")
		return nil, err
	}

	return stream, nil
}

// SaveChatCompletion used to persist the chat completion response to the database as a session.
func (c *Controller) SaveChatCompletion(ctx context.Context, user *types.User, req openai.ChatCompletionRequest, resp openai.ChatCompletionResponse) error {
	return nil
}

func sessionToChatCompletion(session *types.Session) (*openai.ChatCompletionRequest, error) {
	var messages []openai.ChatCompletionMessage

	// Adjust length
	var interactions []*types.Interaction
	if len(session.Interactions) > 10 {
		first, err := data.GetFirstUserInteraction(session.Interactions)
		if err != nil {
			log.Err(err).Msg("error getting first user interaction")
		} else {
			interactions = append(interactions, first)
			interactions = append(interactions, data.GetLastInteractions(session, 10)...)
		}
	} else {
		interactions = session.Interactions
	}

	// Adding the system prompt first
	if session.Metadata.SystemPrompt != "" {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: session.Metadata.SystemPrompt,
		})
	}

	for _, interaction := range interactions {
		switch interaction.Creator {

		case types.CreatorTypeUser:
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleUser,
				Content: interaction.Message,
			})
		case types.CreatorTypeSystem:
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleAssistant,
				Content: interaction.Message,
			})
		case types.CreatorTypeTool:
			messages = append(messages, openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleUser,
				Content:    interaction.Message,
				ToolCalls:  interaction.ToolCalls,
				ToolCallID: interaction.ToolCallID,
			})
		}
	}

	var (
		responseFormat *openai.ChatCompletionResponseFormat
		tools          []openai.Tool
		toolChoice     any
	)

	// If the last interaction has response format, use it
	last, _ := data.GetLastSystemInteraction(interactions)
	if last != nil && last.ResponseFormat.Type == types.ResponseFormatTypeJSONObject {
		responseFormat = &openai.ChatCompletionResponseFormat{
			Type:   openai.ChatCompletionResponseFormatTypeJSONObject,
			Schema: last.ResponseFormat.Schema,
		}
	}

	if last != nil && len(last.Tools) > 0 {
		tools = last.Tools
		toolChoice = last.ToolChoice
	}

	// TODO: temperature, etc.

	return &openai.ChatCompletionRequest{
		Model:          string(session.ModelName),
		Stream:         session.Metadata.Stream,
		Messages:       messages,
		ResponseFormat: responseFormat,
		Tools:          tools,
		ToolChoice:     toolChoice,
	}, nil
}
