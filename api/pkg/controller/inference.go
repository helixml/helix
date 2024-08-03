package controller

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/data"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/prompts"
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

	QueryParams map[string]string
}

// ChatCompletion is used by the OpenAI compatible API. Doesn't handle any historical sessions, etc.
// Runs the OpenAI with tools/app configuration and returns the response.
func (c *Controller) ChatCompletion(ctx context.Context, user *types.User, req openai.ChatCompletionRequest, opts *ChatCompletionOptions) (*openai.ChatCompletionResponse, error) {

	// Check whether the app is configured for the call,
	// if yes, execute the tools and return the response
	toolResp, ok, err := c.evaluateToolUsage(ctx, user, req, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to load tools: %w", err)
	}

	if ok {
		return toolResp, nil
	}

	// Check for an extra RAG context
	ragResults, err := c.evaluateRAG(ctx, user, req, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to load RAG: %w", err)
	}

	if len(ragResults) > 0 {
		// Extend last message with the RAG results
		extended, err := prompts.RAGInferencePrompt(getLastMessage(req), ragResults)
		if err != nil {
			return nil, fmt.Errorf("failed to extend message with RAG results: %w", err)
		}

		// Update the last message with the extended message
		req.Messages[len(req.Messages)-1].Content = extended
	}

	resp, err := c.openAIClient.CreateChatCompletion(ctx, req)
	if err != nil {
		log.Err(err).Msg("error creating chat completion")
		return nil, err
	}

	return &resp, nil
}

// ChatCompletion is used by the OpenAI compatible API. Doesn't handle any historical sessions, etc.
// Runs the OpenAI with tools/app configuration and returns the stream.
func (c *Controller) ChatCompletionStream(ctx context.Context, user *types.User, req openai.ChatCompletionRequest, opts *ChatCompletionOptions) (*openai.ChatCompletionStream, error) {

	// Check whether the app is configured for the call,
	// if yes, execute the tools and return the response
	// toolResp, ok, err := c.evaluateToolUsage(ctx, user, req, opts)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to load tools: %w", err)
	// }

	// if ok {
	// return openai.ChatCompletionStream{
	// 	Choices: []openai.ChatCompletionChoice{
	// 		{
	// 			Message: openai.ChatCompletionMessage{
	// 				Content: toolResp.Message,
	// 			},
	// 		},
	// 	},
	// }, nil
	// }
	toolRespStream, ok, err := c.evaluateToolUsageStream(ctx, user, req, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to load tools: %w", err)
	}

	if ok {
		return toolRespStream, nil
	}

	// Check for an extra RAG context
	ragResults, err := c.evaluateRAG(ctx, user, req, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to load RAG: %w", err)
	}

	if len(ragResults) > 0 {
		// Extend last message with the RAG results
		extended, err := prompts.RAGInferencePrompt(getLastMessage(req), ragResults)
		if err != nil {
			return nil, fmt.Errorf("failed to extend message with RAG results: %w", err)
		}

		// Update the last message with the extended message
		req.Messages[len(req.Messages)-1].Content = extended
	}

	stream, err := c.openAIClient.CreateChatCompletionStream(ctx, req)
	if err != nil {
		log.Err(err).Msg("error creating chat completion stream")
		return nil, err
	}

	return stream, nil
}

func (c *Controller) authorizeUserToApp(user *types.User, app *types.App) error {
	if (!app.Global && !app.Shared) && app.Owner != user.ID {
		return system.NewHTTPError403(fmt.Sprintf("you do not have access to the app with the id: %s", app.ID))
	}

	return nil
}

func (c *Controller) evaluateToolUsage(ctx context.Context, user *types.User, req openai.ChatCompletionRequest, opts *ChatCompletionOptions) (*openai.ChatCompletionResponse, bool, error) {
	_, sessionID, interactionID := oai.GetContextValues(ctx)

	selectedTool, isActionable, ok, err := c.selectAndConfigureTool(ctx, user, req, opts)
	if err != nil {
		return nil, false, fmt.Errorf("failed to select and configure tool: %w", err)
	}

	if !ok {
		return nil, false, nil
	}

	lastMessage := getLastMessage(req)
	history := types.HistoryFromChatCompletionRequest(req)

	resp, err := c.ToolsPlanner.RunAction(ctx, sessionID, interactionID, selectedTool, history, lastMessage, isActionable.Api)
	if err != nil {
		return nil, false, fmt.Errorf("failed to perform action: %w", err)
	}

	return &openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{
			{
				Message: openai.ChatCompletionMessage{
					Content: resp.Message,
				},
			},
		},
	}, true, nil
}

func (c *Controller) evaluateToolUsageStream(ctx context.Context, user *types.User, req openai.ChatCompletionRequest, opts *ChatCompletionOptions) (*openai.ChatCompletionStream, bool, error) {
	_, sessionID, interactionID := oai.GetContextValues(ctx)

	selectedTool, isActionable, ok, err := c.selectAndConfigureTool(ctx, user, req, opts)
	if err != nil {
		return nil, false, fmt.Errorf("failed to select and configure tool: %w", err)
	}

	if !ok {
		return nil, false, nil
	}

	lastMessage := getLastMessage(req)
	history := types.HistoryFromChatCompletionRequest(req)

	stream, err := c.ToolsPlanner.RunActionStream(ctx, sessionID, interactionID, selectedTool, history, lastMessage, isActionable.Api)
	if err != nil {
		return nil, false, fmt.Errorf("failed to perform action: %w", err)
	}

	return stream, true, nil
}

func (c *Controller) selectAndConfigureTool(ctx context.Context, user *types.User, req openai.ChatCompletionRequest, opts *ChatCompletionOptions) (*types.Tool, *tools.IsActionableResponse, bool, error) {
	assistant, err := c.loadAssistant(ctx, user, opts)
	if err != nil {
		return nil, nil, false, err
	}

	if len(assistant.Tools) == 0 {
		return nil, nil, false, nil
	}

	// Get last message from the chat completion messages
	lastMessage := getLastMessage(req)

	var options []tools.Option

	// If assistant has configured an actionable template, use it
	if assistant != nil && assistant.IsActionableTemplate != "" {
		options = append(options, tools.WithIsActionableTemplate(assistant.IsActionableTemplate))
	}

	history := types.HistoryFromChatCompletionRequest(req)

	_, sessionID, interactionID := oai.GetContextValues(ctx)

	isActionable, err := c.ToolsPlanner.IsActionable(ctx, sessionID, interactionID, assistant.Tools, history, lastMessage, options...)
	if err != nil {
		log.Error().Err(err).Msg("failed to evaluate of the message is actionable, skipping to general knowledge")
		return nil, nil, false, fmt.Errorf("failed to evaluate of the message is actionable: %w", err)
	}

	if !isActionable.Actionable() {
		return nil, nil, false, nil
	}

	selectedTool, ok := getToolFromAction(assistant.Tools, isActionable.Api)
	if !ok {
		return nil, nil, false, fmt.Errorf("tool not found for action: %s", isActionable.Api)
	}

	if len(opts.QueryParams) > 0 && selectedTool.Config.API != nil {
		selectedTool.Config.API.Query = make(map[string]string)

		for k, v := range opts.QueryParams {
			selectedTool.Config.API.Query[k] = v
		}
	}

	return selectedTool, isActionable, true, nil

	// resp, err := c.ToolsPlanner.RunAction(ctx, sessionID, interactionID, selectedTool, history, lastMessage, isActionable.Api)
	// if err != nil {
	// 	return nil, false, fmt.Errorf("failed to perform action: %w", err)
	// }

	// return resp, true, nil
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

func (c *Controller) evaluateRAG(ctx context.Context, user *types.User, req openai.ChatCompletionRequest, opts *ChatCompletionOptions) ([]*types.SessionRAGResult, error) {
	if opts.RAGSourceID == "" {
		return []*types.SessionRAGResult{}, nil
	}

	entity, err := c.Options.Store.GetDataEntity(ctx, opts.RAGSourceID)
	if err != nil {
		return nil, fmt.Errorf("error getting data entity: %w", err)
	}

	if entity.Owner != user.ID {
		return nil, fmt.Errorf("you do not have access to the data entity with the id: %s", entity.ID)
	}

	return c.Options.RAG.Query(ctx, &types.SessionRAGQuery{
		Prompt:            getLastMessage(req),
		DataEntityID:      entity.ID,
		DistanceThreshold: entity.Config.RAGSettings.Threshold,
		DistanceFunction:  entity.Config.RAGSettings.DistanceFunction,
		MaxResults:        entity.Config.RAGSettings.ResultsCount,
	})
}

// SaveChatCompletion used to persist the chat completion response to the database as a session.
func (c *Controller) SaveChatCompletion(ctx context.Context, user *types.User, req openai.ChatCompletionRequest, resp openai.ChatCompletionResponse) error {
	return nil
}

func getLastMessage(req openai.ChatCompletionRequest) string {
	if len(req.Messages) > 0 {
		return req.Messages[len(req.Messages)-1].Content
	}

	return ""
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
