package controller

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/model"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/openai/transport"
	"github.com/helixml/helix/api/pkg/prompts"
	"github.com/helixml/helix/api/pkg/rag"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/tools"
	"github.com/helixml/helix/api/pkg/types"
	"gopkg.in/yaml.v2"

	"slices"

	"github.com/helixml/helix/api/pkg/oauth"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

type ChatCompletionOptions struct {
	OrganizationID string
	AppID          string
	AssistantID    string
	RAGSourceID    string
	Provider       string
	QueryParams    map[string]string
	OAuthTokens    map[string]string // OAuth tokens mapped by provider name
	Conversational bool              // Whether to send thoughts about tools and decisions
}

// ChatCompletion is used by the OpenAI compatible API. Doesn't handle any historical sessions, etc.
// Runs the OpenAI with tools/app configuration and returns the response.
// Returns the updated request because the controller mutates it when doing e.g. tools calls and RAG
func (c *Controller) ChatCompletion(ctx context.Context, user *types.User, req openai.ChatCompletionRequest, opts *ChatCompletionOptions) (*openai.ChatCompletionResponse, *openai.ChatCompletionRequest, error) {
	if user.Deactivated {
		return nil, nil, fmt.Errorf("user is deactivated")
	}

	if user.SB {
		if c.Options.Config.SBMessage != "" {
			return &openai.ChatCompletionResponse{
				Choices: []openai.ChatCompletionChoice{
					{
						Message: openai.ChatCompletionMessage{
							Content: c.Options.Config.SBMessage,
						},
					},
				},
			}, &req, nil
		}

		return &openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{
				{
					Message: openai.ChatCompletionMessage{
						Content: "",
					},
				},
			},
		}, &req, nil
	}

	assistant, err := c.loadAssistant(ctx, user, opts)
	if err != nil {
		log.Info().Msg("no assistant found")
		return nil, nil, err
	}

	if assistant.Provider != "" {
		opts.Provider = assistant.Provider
	}

	// Check token quota before processing
	if err := c.checkInferenceTokenQuota(ctx, user.ID, opts.Provider); err != nil {
		return nil, nil, err
	}

	client, err := c.getClient(ctx, opts.OrganizationID, user.ID, opts.Provider)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get client: %v", err)
	}

	hasEnoughBalance, err := c.HasEnoughBalance(ctx, user, opts.OrganizationID, client.BillingEnabled())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to check balance: %w", err)
	}

	if !hasEnoughBalance {
		return nil, nil, fmt.Errorf("insufficient balance")
	}

	// Evaluate and add OAuth tokens
	err = c.evalAndAddOAuthTokens(ctx, client, opts, user)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to add OAuth tokens: %w", err)
	}

	if assistant.IsAgentMode() {
		log.Info().Msg("running in agent mode")

		resp, err := c.runAgentBlocking(ctx, &runAgentRequest{
			OrganizationID: opts.OrganizationID,
			Assistant:      assistant,
			User:           user,
			Request:        req,
			Options:        opts,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to run agent: %w", err)
		}
		return resp, &req, nil
	}

	if len(assistant.Tools) > 0 {
		// Check whether the app is configured for the call,
		// if yes, execute the tools and return the response
		toolResp, ok, err := c.evaluateToolUsage(ctx, user, req, opts)
		if err != nil {
			return nil, nil, fmt.Errorf("tool execution failed: %w", err)
		}

		if ok {
			return toolResp, &req, nil
		}
	}

	req = setSystemPrompt(&req, assistant.SystemPrompt)

	if assistant.Provider != "" {
		opts.Provider = assistant.Provider
	}

	if assistant.Model != "" {
		req.Model = assistant.Model

		modelName, err := model.ProcessModelName(c.Options.Config.Inference.Provider, req.Model, types.SessionTypeText)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid model name '%s': %w", req.Model, err)
		}

		req.Model = modelName
	}

	if assistant.Temperature != 0.0 {
		req.Temperature = assistant.Temperature
	}

	if assistant.FrequencyPenalty != 0.0 {
		req.FrequencyPenalty = assistant.FrequencyPenalty
	}

	if assistant.PresencePenalty != 0.0 {
		req.PresencePenalty = assistant.PresencePenalty
	}

	if assistant.TopP != 0.0 {
		req.TopP = assistant.TopP
	}

	if assistant.MaxTokens != 0 {
		req.MaxTokens = assistant.MaxTokens
	}

	if assistant.ReasoningEffort != "" && assistant.ReasoningEffort != types.ReasoningEffortNone {
		req.ReasoningEffort = assistant.ReasoningEffort
	}

	if assistant.RAGSourceID != "" {
		opts.RAGSourceID = assistant.RAGSourceID
	}

	err = c.enrichPromptWithKnowledge(ctx, user, &req, assistant, opts)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to enrich prompt with knowledge: %w", err)
	}

	resp, err := client.CreateChatCompletion(ctx, req)
	if err != nil {
		log.Err(err).
			Str("model", req.Model).
			Str("provider", opts.Provider).
			Msg("error creating chat completion")
		return nil, nil, err
	}

	return &resp, &req, nil
}

// ChatCompletionStream is used by the OpenAI compatible API. Doesn't handle any historical sessions, etc.
// Runs the OpenAI with tools/app configuration and returns the stream.
func (c *Controller) ChatCompletionStream(ctx context.Context, user *types.User, req openai.ChatCompletionRequest, opts *ChatCompletionOptions) (*openai.ChatCompletionStream, *openai.ChatCompletionRequest, error) {
	if user.Deactivated {
		return nil, nil, fmt.Errorf("user is deactivated")
	}

	if user.SB {
		msg := c.Options.Config.SBMessage

		stream, writer, err := transport.NewOpenAIStreamingAdapter(req)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create openai streaming adapter: %w", err)
		}

		go func() {
			defer writer.Close()
			_ = transport.WriteChatCompletionStream(writer, &openai.ChatCompletionStreamResponse{
				Choices: []openai.ChatCompletionStreamChoice{
					{
						Delta: openai.ChatCompletionStreamChoiceDelta{Content: msg},
					},
				},
			})
			_ = transport.WriteChatCompletionStream(writer, &openai.ChatCompletionStreamResponse{
				Choices: []openai.ChatCompletionStreamChoice{
					{
						FinishReason: openai.FinishReasonStop,
					},
				},
			})
		}()

		return stream, &req, nil

	}

	req.Stream = true

	if opts == nil {
		opts = &ChatCompletionOptions{}
	}

	log.Info().
		Str("owner_id", user.ID).
		Str("app_id", opts.AppID).
		Int("oauth_token_count", len(opts.OAuthTokens)).
		Bool("has_oauth_manager", c.Options.OAuthManager != nil).
		Msg("ChatCompletionStream called")

	assistant, err := c.loadAssistant(ctx, user, opts)
	if err != nil {
		return nil, nil, err
	}

	if assistant.Provider != "" {
		opts.Provider = assistant.Provider
	}

	// Check token quota before processing
	if err := c.checkInferenceTokenQuota(ctx, user.ID, opts.Provider); err != nil {
		return nil, nil, err
	}

	client, err := c.getClient(ctx, opts.OrganizationID, user.ID, opts.Provider)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get client: %v", err)
	}

	hasEnoughBalance, err := c.HasEnoughBalance(ctx, user, opts.OrganizationID, client.BillingEnabled())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to check balance: %w", err)
	}

	if !hasEnoughBalance {
		return nil, nil, fmt.Errorf("insufficient balance")
	}

	// Evaluate and add OAuth tokens
	err = c.evalAndAddOAuthTokens(ctx, client, opts, user)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to add OAuth tokens: %w", err)
	}

	if assistant.IsAgentMode() {
		log.Info().Msg("running in agent mode")

		resp, err := c.runAgentStream(ctx, &runAgentRequest{
			OrganizationID: opts.OrganizationID,
			Assistant:      assistant,
			User:           user,
			Request:        req,
			Options:        opts,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to run agent: %w", err)
		}
		return resp, &req, nil
	}

	if len(assistant.Tools) > 0 {
		// Check whether the app is configured for the call,
		// if yes, execute the tools and return the response
		toolRespStream, ok, err := c.evaluateToolUsageStream(ctx, user, req, opts)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load tools: %w", err)
		}

		if ok {
			return toolRespStream, &req, nil
		}
	}

	req = setSystemPrompt(&req, assistant.SystemPrompt)

	if assistant.Provider != "" {
		opts.Provider = assistant.Provider
	}

	if assistant.Model != "" {
		req.Model = assistant.Model

		modelName, err := model.ProcessModelName(c.Options.Config.Inference.Provider, req.Model, types.SessionTypeText)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid model name '%s': %w", req.Model, err)
		}

		req.Model = modelName
	}

	if assistant.Temperature != 0.0 {
		req.Temperature = assistant.Temperature
	}

	if assistant.FrequencyPenalty != 0.0 {
		req.FrequencyPenalty = assistant.FrequencyPenalty
	}

	if assistant.PresencePenalty != 0.0 {
		req.PresencePenalty = assistant.PresencePenalty
	}

	if assistant.TopP != 0.0 {
		req.TopP = assistant.TopP
	}

	if assistant.MaxTokens != 0 {
		req.MaxTokens = assistant.MaxTokens
	}

	if assistant.ReasoningEffort != "" && assistant.ReasoningEffort != types.ReasoningEffortNone {
		req.ReasoningEffort = assistant.ReasoningEffort
	}

	if assistant.RAGSourceID != "" {
		opts.RAGSourceID = assistant.RAGSourceID
	}

	// Check for knowledge
	err = c.enrichPromptWithKnowledge(ctx, user, &req, assistant, opts)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to enrich prompt with knowledge: %w", err)
	}

	stream, err := client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		log.
			Err(err).
			Str("model", req.Model).
			Str("provider", opts.Provider).
			Msg("error creating chat completion stream")
		return nil, nil, err
	}

	return stream, &req, nil
}

func (c *Controller) getClient(ctx context.Context, organizationID, userID, provider string) (oai.Client, error) {
	if provider == "" {
		// If not set, use the default provider
		provider = c.Options.Config.Inference.Provider
	}

	log.Trace().
		Str("provider", provider).
		Str("user_id", userID).
		Str("organization_id", organizationID).
		Msg("getting OpenAI API client")

	owner := userID
	if organizationID != "" {
		owner = organizationID
	}

	client, err := c.providerManager.GetClient(ctx, &manager.GetClientRequest{
		Provider: provider,
		Owner:    owner,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %v", err)
	}

	return client, nil

}

func (c *Controller) evaluateToolUsage(ctx context.Context, user *types.User, req openai.ChatCompletionRequest, opts *ChatCompletionOptions) (*openai.ChatCompletionResponse, bool, error) {
	vals, ok := oai.GetContextValues(ctx)
	if !ok {
		vals = &oai.ContextValues{}
	}

	selectedTool, isActionable, ok, err := c.selectAndConfigureTool(ctx, user, req, opts)
	if err != nil {
		return nil, false, fmt.Errorf("failed to select and configure tool: %w", err)
	}

	if !ok {
		return nil, false, nil
	}

	history := types.HistoryFromChatCompletionRequest(req)

	if err := c.stepInfoEmitter.EmitStepInfo(ctx, &types.StepInfo{
		Name:    selectedTool.Name,
		Type:    types.StepInfoTypeToolUse,
		Message: "Running action",
	}); err != nil {
		log.Debug().Err(err).Msg("failed to emit run action step info")
	}

	var options []tools.Option

	apieClient, err := c.getClient(ctx, opts.OrganizationID, user.ID, opts.Provider)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get client: %w", err)
	}

	options = append(options, tools.WithClient(apieClient))

	// Pass OAuth tokens to the tools system
	if len(opts.OAuthTokens) > 0 {
		log.Info().
			Int("token_count", len(opts.OAuthTokens)).
			Msg("Passing OAuth tokens to tools system for blocking call")
		options = append(options, tools.WithOAuthTokens(opts.OAuthTokens))
	}

	resp, err := c.ToolsPlanner.RunAction(ctx, vals.SessionID, vals.InteractionID, selectedTool, history, isActionable.API, options...)
	if err != nil {
		if emitErr := c.stepInfoEmitter.EmitStepInfo(ctx, &types.StepInfo{
			Name:    selectedTool.Name,
			Type:    types.StepInfoTypeToolUse,
			Message: fmt.Sprintf("Action failed: %s", err),
		}); emitErr != nil {
			log.Debug().Err(err).Msg("failed to emit run action step info")
		}

		return nil, false, fmt.Errorf("failed to perform action: %w", err)
	}

	if err := c.stepInfoEmitter.EmitStepInfo(ctx, &types.StepInfo{
		Name:    selectedTool.Name,
		Type:    types.StepInfoTypeToolUse,
		Message: "Action completed",
	}); err != nil {
		log.Debug().Err(err).Msg("failed to emit run action step info")
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
	vals, ok := oai.GetContextValues(ctx)
	if !ok {
		vals = &oai.ContextValues{}
	}

	selectedTool, isActionable, ok, err := c.selectAndConfigureTool(ctx, user, req, opts)
	if err != nil {
		return nil, false, fmt.Errorf("failed to select and configure tool: %w", err)
	}

	if !ok {
		return nil, false, nil
	}

	history := types.HistoryFromChatCompletionRequest(req)

	if err := c.stepInfoEmitter.EmitStepInfo(ctx, &types.StepInfo{
		Name:    selectedTool.Name,
		Type:    types.StepInfoTypeToolUse,
		Message: "Running action",
	}); err != nil {
		log.Debug().Err(err).Msg("failed to emit step info")
	}

	var options []tools.Option

	apieClient, err := c.getClient(ctx, opts.OrganizationID, user.ID, opts.Provider)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get client: %w", err)
	}

	options = append(options, tools.WithClient(apieClient))

	// Pass OAuth tokens to the tools system
	if len(opts.OAuthTokens) > 0 {
		log.Info().
			Int("token_count", len(opts.OAuthTokens)).
			Msg("Passing OAuth tokens to tools system for streaming call")
		options = append(options, tools.WithOAuthTokens(opts.OAuthTokens))
	}

	stream, err := c.ToolsPlanner.RunActionStream(ctx, vals.SessionID, vals.InteractionID, selectedTool, history, isActionable.API, options...)
	if err != nil {
		log.Warn().
			Err(err).
			Str("tool", selectedTool.Name).
			Str("action", isActionable.API).
			Msg("failed to perform action")

		if emitErr := c.stepInfoEmitter.EmitStepInfo(ctx, &types.StepInfo{
			Name:    selectedTool.Name,
			Type:    types.StepInfoTypeToolUse,
			Message: fmt.Sprintf("Action failed: %s", err),
		}); emitErr != nil {
			log.Debug().Err(err).Msg("failed to emit step info")
		}

		return nil, false, fmt.Errorf("failed to perform action: %w", err)
	}

	if err := c.stepInfoEmitter.EmitStepInfo(ctx, &types.StepInfo{
		Name:    selectedTool.Name,
		Type:    types.StepInfoTypeToolUse,
		Message: "Action completed",
	}); err != nil {
		log.Debug().Err(err).Msg("failed to emit step info")
	}

	return stream, true, nil
}

func (c *Controller) selectAndConfigureTool(ctx context.Context, user *types.User, req openai.ChatCompletionRequest, opts *ChatCompletionOptions) (*types.Tool, *tools.IsActionableResponse, bool, error) {
	log.Info().
		Str("user_id", user.ID).
		Str("app_id", opts.AppID).
		Int("message_count", len(req.Messages)).
		Bool("has_oauth_tokens", len(opts.OAuthTokens) > 0).
		Msg("Starting selectAndConfigureTool")

	assistant, err := c.loadAssistant(ctx, user, opts)
	if err != nil {
		log.Info().Msg("no assistant found")
		return nil, nil, false, err
	}

	log.Info().
		Str("assistant_id", assistant.ID).
		Str("assistant_name", assistant.Name).
		Int("tool_count", len(assistant.Tools)).
		Msg("Loaded assistant for tool selection")

	if len(assistant.Tools) == 0 {
		log.Info().
			Str("assistant_id", assistant.ID).
			Str("assistant_name", assistant.Name).
			Str("assistant_model", assistant.Model).
			Msg("assistant has no tools")
		return nil, nil, false, nil
	}

	var options []tools.Option

	// If assistant has configured an actionable template, use it
	if assistant != nil && assistant.IsActionableTemplate != "" {
		options = append(options, tools.WithIsActionableTemplate(assistant.IsActionableTemplate))
	}
	if assistant.IsActionableHistoryLength > 0 {
		options = append(options, tools.WithIsActionableHistoryLength(assistant.IsActionableHistoryLength))
	}
	// If assistant has configured a model, use it
	if assistant != nil && assistant.Model != "" {
		options = append(options, tools.WithModel(assistant.Model))
		log.Info().
			Str("assistant_id", assistant.ID).
			Str("assistant_model", assistant.Model).
			Msg("Using assistant-specific model for tools")
	}

	log.Info().
		Str("user_id", user.ID).
		Str("provider", opts.Provider).
		Msg("Getting API client for tool execution")

	apieClient, err := c.getClient(ctx, opts.OrganizationID, user.ID, opts.Provider)
	if err != nil {
		log.Warn().
			Err(err).
			Str("user_id", user.ID).
			Str("provider", opts.Provider).
			Msg("Failed to get client for tool execution")
		return nil, nil, false, fmt.Errorf("failed to get client: %w", err)
	}

	options = append(options, tools.WithClient(apieClient))

	// Check if we have OAuth tokens in options
	if len(opts.OAuthTokens) > 0 {
		tokenKeys := make([]string, 0, len(opts.OAuthTokens))
		for key := range opts.OAuthTokens {
			tokenKeys = append(tokenKeys, key)
		}
		log.Info().
			Int("token_count", len(opts.OAuthTokens)).
			Strs("token_keys", tokenKeys).
			Msg("OAuth tokens available for tool execution")
	} else {
		log.Warn().
			Str("app_id", opts.AppID).
			Msg("No OAuth tokens available in options")
	}

	history := types.HistoryFromChatCompletionRequest(req)

	vals, ok := oai.GetContextValues(ctx)
	if !ok {
		vals = &oai.ContextValues{}
	}

	if err := c.stepInfoEmitter.EmitStepInfo(ctx, &types.StepInfo{
		Name:    "is_actionable",
		Type:    types.StepInfoTypeToolUse,
		Message: "Checking if we should use tools",
	}); err != nil {
		log.Debug().Err(err).Msg("failed to emit step info")
	}

	log.Info().
		Str("session_id", vals.SessionID).
		Str("interaction_id", vals.InteractionID).
		Int("tool_count", len(assistant.Tools)).
		Int("history_message_count", len(history)).
		Msg("Checking if message is actionable")

	// Log each tool being considered
	for i, tool := range assistant.Tools {
		if tool.ToolType == types.ToolTypeAPI && tool.Config.API != nil {
			log.Info().
				Int("tool_index", i).
				Str("tool_name", tool.Name).
				Str("tool_type", string(tool.ToolType)).
				Str("oauth_provider", tool.Config.API.OAuthProvider).
				Bool("has_headers", tool.Config.API.Headers != nil).
				Int("action_count", len(tool.Config.API.Actions)).
				Msg("API tool available for action")
		} else {
			log.Info().
				Int("tool_index", i).
				Str("tool_name", tool.Name).
				Str("tool_type", string(tool.ToolType)).
				Msg("Non-API tool available for action")
		}
	}

	isActionable, err := c.ToolsPlanner.IsActionable(ctx, vals.SessionID, vals.InteractionID, assistant.Tools, history, options...)
	if err != nil {
		log.Error().Err(err).Msg("failed to evaluate if the message is actionable, skipping to general knowledge")
		return nil, nil, false, fmt.Errorf("failed to evaluate if the message is actionable: %w", err)
	}

	if !isActionable.Actionable() {
		if err := c.stepInfoEmitter.EmitStepInfo(ctx, &types.StepInfo{
			Name:    "is_actionable",
			Type:    types.StepInfoTypeToolUse,
			Message: "Message is not actionable",
		}); err != nil {
			log.Debug().Err(err).Msg("failed to emit step info")
		}

		log.Info().
			Str("session_id", vals.SessionID).
			Str("interaction_id", vals.InteractionID).
			Msg("Message is not actionable, skipping tool execution")

		return nil, nil, false, nil
	}

	// Log the IsActionable result
	log.Info().
		Str("session_id", vals.SessionID).
		Str("chosen_tool_name", isActionable.API).
		Str("api", isActionable.API).
		Bool("actionable", isActionable.Actionable()).
		Msg("Message is actionable")

	selectedTool, ok := tools.GetToolFromAction(assistant.Tools, isActionable.API)
	if !ok {
		log.Error().
			Str("chosen_tool", isActionable.API).
			Str("api", isActionable.API).
			Msg("Tool not found for action")
		return nil, nil, false, fmt.Errorf("tool not found for action: %s", isActionable.API)
	}

	// Check if the tool has the necessary OAuth provider
	if selectedTool.ToolType == types.ToolTypeAPI && selectedTool.Config.API != nil {
		log.Info().
			Str("tool_name", selectedTool.Name).
			Str("oauth_provider", selectedTool.Config.API.OAuthProvider).
			Bool("has_oauth_provider", selectedTool.Config.API.OAuthProvider != "").
			Bool("has_headers", selectedTool.Config.API.Headers != nil).
			Msg("Selected API tool OAuth configuration")

		// Add detailed debug logging
		log.Info().
			Str("tool_id", selectedTool.ID).
			Str("tool_name", selectedTool.Name).
			Str("oauth_provider", selectedTool.Config.API.OAuthProvider).
			Bool("has_oauth_provider", selectedTool.Config.API.OAuthProvider != "").
			Str("oauth_provider_type", fmt.Sprintf("%T", selectedTool.Config.API.OAuthProvider)).
			Interface("available_tokens", opts.OAuthTokens).
			Msg("DEBUG: Tool OAuth provider checking")

		// Check if there's a matching OAuth token
		if selectedTool.Config.API.OAuthProvider != "" && len(opts.OAuthTokens) > 0 {
			if token, exists := opts.OAuthTokens[selectedTool.Config.API.OAuthProvider]; exists {
				log.Info().
					Str("provider", selectedTool.Config.API.OAuthProvider).
					Bool("token_found", token != "").
					Msg("OAuth token found for selected tool")
			} else {
				// Convert map keys to a slice for logging
				tokenKeys := make([]string, 0, len(opts.OAuthTokens))
				for k := range opts.OAuthTokens {
					tokenKeys = append(tokenKeys, k)
				}

				log.Warn().
					Str("provider", selectedTool.Config.API.OAuthProvider).
					Strs("available_providers", tokenKeys).
					Msg("No matching OAuth token found for selected tool")

				// Try a case-insensitive match
				for tokenKey, tokenValue := range opts.OAuthTokens {
					if strings.EqualFold(tokenKey, selectedTool.Config.API.OAuthProvider) {
						log.Info().
							Str("provider", selectedTool.Config.API.OAuthProvider).
							Str("token_key", tokenKey).
							Bool("has_token", tokenValue != "").
							Msg("Found OAuth token with case-insensitive match")
					}
				}
			}
		}
	}

	// If assistant has configured a model, give the hint to the tool that it should use that model too
	if assistant != nil && assistant.Model != "" {
		if selectedTool.Config.API != nil && selectedTool.Config.API.Model == "" {
			log.Info().
				Str("assistant_id", assistant.ID).
				Str("assistant_name", assistant.Name).
				Str("assistant_model", assistant.Model).
				Str("tool_name", selectedTool.Name).
				Msg("assistant has configured a model, and tool has no model specified, using assistant model for tool")

			selectedTool.Config.API.Model = assistant.Model
		}
	}

	if len(opts.QueryParams) > 0 && selectedTool.Config.API != nil {
		selectedTool.Config.API.Query = make(map[string]string)

		for k, v := range opts.QueryParams {
			selectedTool.Config.API.Query[k] = v
		}
	}

	if err := c.stepInfoEmitter.EmitStepInfo(ctx, &types.StepInfo{
		Name:    selectedTool.Name,
		Type:    types.StepInfoTypeToolUse,
		Message: fmt.Sprintf("Using %s for action %s", selectedTool.Name, isActionable.API),
	}); err != nil {
		log.Debug().Err(err).Msg("failed to emit step info")
	}

	return selectedTool, isActionable, true, nil
}

func (c *Controller) loadAssistant(ctx context.Context, user *types.User, opts *ChatCompletionOptions) (*types.AssistantConfig, error) {
	if opts.AppID == "" {
		return &types.AssistantConfig{}, nil
	}

	app, err := c.Options.Store.GetAppWithTools(ctx, opts.AppID)
	if err != nil {
		return nil, fmt.Errorf("error getting app: %w", err)
	}

	// Load secrets into the app
	app, err = c.evaluateSecrets(ctx, user, app)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate secrets: %w", err)
	}

	assistant := data.GetAssistant(app, opts.AssistantID)

	if assistant == nil {
		return nil, fmt.Errorf("we could not find the assistant with ID %s, in app %s", opts.AssistantID, app.ID)
	}

	return assistant, nil
}

func (c *Controller) evaluateSecrets(ctx context.Context, user *types.User, app *types.App) (*types.App, error) {
	secrets, err := c.Options.Store.ListSecrets(ctx, &store.ListSecretsQuery{
		Owner: user.ID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets: %w", err)
	}

	var filteredSecrets []*types.Secret

	// Filter out secrets that are not for the current app
	for _, secret := range secrets {
		if secret.AppID != "" && secret.AppID != app.ID {
			continue
		}
		filteredSecrets = append(filteredSecrets, secret)
	}

	// Nothing to do
	if len(filteredSecrets) == 0 {
		return app, nil
	}

	return enrichAppWithSecrets(app, filteredSecrets)
}

func enrichAppWithSecrets(app *types.App, secrets []*types.Secret) (*types.App, error) {
	appYaml, err := yaml.Marshal(app)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal app: %w", err)
	}

	envs := make(map[string]string)
	for _, secret := range secrets {
		envs[secret.Name] = string(secret.Value)
	}

	processed, err := Eval(string(appYaml), envs)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate secrets: %w", err)
	}

	var enrichedApp types.App

	err = yaml.Unmarshal([]byte(processed), &enrichedApp)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal app: %w", err)
	}

	return &enrichedApp, nil
}

func (c *Controller) enrichPromptWithKnowledge(ctx context.Context, user *types.User, req *openai.ChatCompletionRequest, assistant *types.AssistantConfig, opts *ChatCompletionOptions) error {
	// Check for an extra RAG context
	ragResults, err := c.evaluateRAG(ctx, user, *req, opts)
	if err != nil {
		return fmt.Errorf("failed to load RAG: %w", err)
	}

	knowledgeResults, knowledge, err := c.evaluateKnowledge(ctx, *req, assistant, opts)
	if err != nil {
		return fmt.Errorf("failed to load knowledge: %w", err)
	}

	if len(ragResults) > 0 || len(knowledgeResults) > 0 {
		// Extend last message with the RAG results
		err := extendMessageWithKnowledge(req, ragResults, knowledge, knowledgeResults)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *Controller) evaluateRAG(ctx context.Context, user *types.User, req openai.ChatCompletionRequest, opts *ChatCompletionOptions) ([]*prompts.RagContent, error) {
	if opts.RAGSourceID == "" {
		return []*prompts.RagContent{}, nil
	}

	entity, err := c.Options.Store.GetDataEntity(ctx, opts.RAGSourceID)
	if err != nil {
		return nil, fmt.Errorf("error getting data entity: %w", err)
	}

	if entity.Owner != user.ID {
		return nil, fmt.Errorf("you do not have access to the data entity with the id: %s", entity.ID)
	}

	prompt := getLastMessage(req)

	// Check for empty prompt which could cause "inputs cannot be empty" error
	if strings.TrimSpace(prompt) == "" {
		log.Warn().
			Str("user_id", user.ID).
			Str("rag_source_id", opts.RAGSourceID).
			Interface("request_messages", req.Messages).
			Msg("evaluateRAG: Empty prompt detected - this may cause 'inputs cannot be empty' error")
		return []*prompts.RagContent{}, nil
	}

	// Parse document IDs from the completion request
	filterActions := rag.ParseFilterActions(prompt)
	filterDocumentIDs := make([]string, 0)
	for _, filterAction := range filterActions {
		filterDocumentIDs = append(filterDocumentIDs, rag.ParseDocID(filterAction))
	}

	log.Trace().Interface("documentIDs", filterDocumentIDs).Msg("document IDs")

	pipeline := types.TextPipeline
	if entity.Config.RAGSettings.EnableVision {
		pipeline = types.VisionPipeline
	}

	log.Debug().
		Str("user_id", user.ID).
		Str("data_entity_id", entity.ID).
		Str("prompt", prompt).
		Str("pipeline", string(pipeline)).
		Msg("evaluateRAG: About to make RAG query")

	ragResults, err := c.Options.RAG.Query(ctx, &types.SessionRAGQuery{
		Prompt:            prompt,
		DataEntityID:      entity.ID,
		DistanceThreshold: entity.Config.RAGSettings.Threshold,
		DistanceFunction:  entity.Config.RAGSettings.DistanceFunction,
		MaxResults:        entity.Config.RAGSettings.ResultsCount,
		DocumentIDList:    filterDocumentIDs,
		Pipeline:          pipeline,
	})
	if err != nil {
		log.Error().Err(err).
			Str("user_id", user.ID).
			Str("data_entity_id", entity.ID).
			Str("prompt", prompt).
			Msg("evaluateRAG: Error querying RAG")
		return nil, fmt.Errorf("error querying RAG: %w", err)
	}

	ragContent := make([]*prompts.RagContent, 0, len(ragResults))
	for _, result := range ragResults {
		ragContent = append(ragContent, &prompts.RagContent{
			DocumentID: result.DocumentID,
			Content:    result.Content,
		})
	}

	return ragContent, nil
}

func (c *Controller) evaluateKnowledge(
	ctx context.Context,
	req openai.ChatCompletionRequest,
	assistant *types.AssistantConfig,
	opts *ChatCompletionOptions) ([]*prompts.BackgroundKnowledge, *types.Knowledge, error) {
	var (
		backgroundKnowledge []*prompts.BackgroundKnowledge
		usedKnowledge       *types.Knowledge
	)

	prompt := getLastMessage(req)

	for _, k := range assistant.Knowledge {
		knowledge, err := c.Options.Store.LookupKnowledge(ctx, &store.LookupKnowledgeQuery{
			Name:  k.Name,
			AppID: opts.AppID,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("error getting knowledge: %w", err)
		}
		switch {
		// If the knowledge is a content, add it to the background knowledge
		// without anything else (no database to search in)
		case knowledge.Source.Text != nil:
			backgroundKnowledge = append(backgroundKnowledge, &prompts.BackgroundKnowledge{
				Description: knowledge.Description,
				Content:     *knowledge.Source.Text,
			})

			usedKnowledge = knowledge
		default:
			ragClient, err := c.GetRagClient(ctx, knowledge)
			if err != nil {
				return nil, nil, fmt.Errorf("error getting RAG client: %w", err)
			}

			if err := c.stepInfoEmitter.EmitStepInfo(ctx, &types.StepInfo{
				Name:    knowledge.Name,
				Type:    types.StepInfoTypeRAG,
				Message: "Searching for knowledge",
			}); err != nil {
				log.Debug().Err(err).Msg("failed to emit step info")
			}

			// Parse document IDs from the completion request
			filterActions := rag.ParseFilterActions(prompt)
			log.Trace().Interface("filterActions", filterActions).Msg("filterActions")
			filterDocumentIDs := make([]string, 0)
			for _, filterAction := range filterActions {
				filterDocumentIDs = append(filterDocumentIDs, rag.ParseDocID(filterAction))
			}
			log.Trace().Interface("inference filterDocumentIDs", filterDocumentIDs).Msg("filterDocumentIDs")

			pipeline := types.TextPipeline
			if knowledge.RAGSettings.EnableVision {
				pipeline = types.VisionPipeline
			}
			ragResults, err := ragClient.Query(ctx, &types.SessionRAGQuery{
				Prompt:            prompt,
				DataEntityID:      knowledge.GetDataEntityID(),
				DistanceThreshold: knowledge.RAGSettings.Threshold,
				DistanceFunction:  knowledge.RAGSettings.DistanceFunction,
				MaxResults:        knowledge.RAGSettings.ResultsCount,
				DocumentIDList:    filterDocumentIDs,
				Pipeline:          pipeline,
			})
			if err != nil {
				return nil, nil, fmt.Errorf("error querying RAG: %w", err)
			}

			if err := c.stepInfoEmitter.EmitStepInfo(ctx, &types.StepInfo{
				Name:    knowledge.Name,
				Type:    types.StepInfoTypeRAG,
				Message: fmt.Sprintf("Found %d results", len(ragResults)),
			}); err != nil {
				log.Debug().Err(err).Msg("failed to emit step info")
			}

			for _, result := range ragResults {
				backgroundKnowledge = append(backgroundKnowledge, &prompts.BackgroundKnowledge{
					Description: knowledge.Description,
					DocumentID:  result.DocumentID,
					Source:      result.Source,
					Content:     result.Content,
				})
			}

			if len(ragResults) > 0 {
				usedKnowledge = knowledge
				// Add debug logging before updating session with knowledge results
				sidFromCtx, sidOk := oai.GetContextSessionID(ctx)
				vals, valsOk := oai.GetContextValues(ctx)
				appID, appOk := oai.GetContextAppID(ctx)

				ctxValuesSid := ""
				if valsOk {
					ctxValuesSid = vals.SessionID
				}

				log.Debug().
					Bool("has_direct_sid", sidOk).
					Bool("has_ctx_values", valsOk).
					Bool("has_app_id", appOk).
					Str("direct_sid", sidFromCtx).
					Str("ctx_values_sid", ctxValuesSid).
					Str("app_id", appID).
					Str("knowledge_name", knowledge.Name).
					Int("rag_results_count", len(ragResults)).
					Msg("about to update session with knowledge results")

				// Enhance the RAG results with knowledge source path information
				knowledgePath := ""
				if knowledge.Source.Filestore != nil && knowledge.Source.Filestore.Path != "" {
					knowledgePath = knowledge.Source.Filestore.Path
					log.Debug().
						Str("knowledge_name", knowledge.Name).
						Str("knowledge_path", knowledgePath).
						Msg("found filestore path in knowledge source")
				}

				// If we have a knowledge path, update the source in all results
				if knowledgePath != "" {
					for i := range ragResults {
						// Only update the source if the original source is just a filename
						// and not already a full path
						if !strings.HasPrefix(ragResults[i].Source, "/") &&
							!strings.HasPrefix(ragResults[i].Source, "apps/") &&
							!strings.HasPrefix(ragResults[i].Source, c.Options.Config.Controller.FilePrefixGlobal) {

							// Preserve the original filename
							originalSource := ragResults[i].Source

							// Build the full path by joining the knowledge path with the source filename
							// Only use the filename part from the source, not any directory
							filename := filepath.Base(originalSource)
							fullPath := filepath.Join(knowledgePath, filename)

							log.Debug().
								Str("original_source", originalSource).
								Str("knowledge_path", knowledgePath).
								Str("full_path", fullPath).
								Msg("enhancing RAG result with knowledge path")

							ragResults[i].Source = fullPath
						}
					}
				}

				// Update the session metadata with the document IDs
				if err := c.UpdateSessionWithKnowledgeResults(ctx, nil, ragResults); err != nil {
					log.Error().Err(err).Msg("failed to update session with knowledge results")
				}
			}
		}
	}

	return backgroundKnowledge, usedKnowledge, nil
}

// TODO: use different struct with just document ID and content
func extendMessageWithKnowledge(req *openai.ChatCompletionRequest, ragResults []*prompts.RagContent, k *types.Knowledge, knowledgeResults []*prompts.BackgroundKnowledge) error {
	var msg openai.ChatCompletionMessage
	var err error
	if anyKnowledgeHasImages(knowledgeResults) {
		msg, err = buildVisionChatCompletionMessage(req, ragResults, k, knowledgeResults)
		if err != nil {
			return fmt.Errorf("failed to build vision chat completion message: %w", err)
		}
	} else {
		msg, err = buildTextChatCompletionMessage(req, ragResults, k, knowledgeResults)
		if err != nil {
			return fmt.Errorf("failed to build text chat completion message: %w", err)
		}
	}

	req.Messages[len(req.Messages)-1] = msg

	return nil
}

func anyKnowledgeHasImages(knowledgeResults []*prompts.BackgroundKnowledge) bool {
	return slices.ContainsFunc(knowledgeResults, knowledgeHasImage)
}

func knowledgeHasImage(result *prompts.BackgroundKnowledge) bool {
	return strings.HasPrefix(result.Content, "data:image/")
}

// buildVisionChatCompletionMessage builds a vision chat completion message
// See canon: https://platform.openai.com/docs/guides/images?api-mode=chat&lang=python#provide-multiple-image-inputs
func buildVisionChatCompletionMessage(req *openai.ChatCompletionRequest, ragResults []*prompts.RagContent, k *types.Knowledge, knowledgeResults []*prompts.BackgroundKnowledge) (openai.ChatCompletionMessage, error) {
	imageParts := make([]openai.ChatMessagePart, 0, len(req.Messages))
	for _, result := range knowledgeResults {
		if knowledgeHasImage(result) {
			imageParts = append(imageParts, openai.ChatMessagePart{
				Type: openai.ChatMessagePartTypeText,
				Text: "### START OF CONTENT FOR DOCUMENT " + rag.BuildDocumentID(result.DocumentID) + " ###",
			})
			imageParts = append(imageParts, openai.ChatMessagePart{
				Type: openai.ChatMessagePartTypeImageURL,
				ImageURL: &openai.ChatMessageImageURL{
					URL: result.Content,
				},
			})
			imageParts = append(imageParts, openai.ChatMessagePart{
				Type: openai.ChatMessagePartTypeText,
				Text: "### END OF CONTENT FOR DOCUMENT " + rag.BuildDocumentID(result.DocumentID) + " ###",
			})
		}
	}

	lastMessage := getLastMessage(*req)

	extended, err := buildTextChatCompletionContent(lastMessage, ragResults, k, knowledgeResults)
	if err != nil {
		return openai.ChatCompletionMessage{}, fmt.Errorf("failed to build text chat completion content: %w", err)
	}

	finalParts := []openai.ChatMessagePart{}

	finalParts = append(finalParts, imageParts...)
	finalParts = append(finalParts, openai.ChatMessagePart{
		Type: openai.ChatMessagePartTypeText,
		Text: extended,
	})

	// Now rebuild the final message with the text at the top like in the example
	res := openai.ChatCompletionMessage{
		Role:         openai.ChatMessageRoleUser,
		MultiContent: finalParts,
	}

	return res, nil
}

// buildTextChatCompletionMessage builds a standard text chat completion message extended with the
// text knowledge and prompt
func buildTextChatCompletionMessage(req *openai.ChatCompletionRequest, ragResults []*prompts.RagContent, k *types.Knowledge, knowledgeResults []*prompts.BackgroundKnowledge) (openai.ChatCompletionMessage, error) {
	lastMessage := getLastMessage(*req)

	extended, err := buildTextChatCompletionContent(lastMessage, ragResults, k, knowledgeResults)
	if err != nil {
		return openai.ChatCompletionMessage{}, fmt.Errorf("failed to build text chat completion content: %w", err)
	}

	lastCompletionMessage := req.Messages[len(req.Messages)-1]

	// Check if last completion message has multi-content, if yes, find the index for TEXT type message and overwrite it with the extended content.
	// Otherwise, set the content to the extended content
	if len(lastCompletionMessage.MultiContent) > 0 {
		for i, part := range lastCompletionMessage.MultiContent {
			if part.Type == openai.ChatMessagePartTypeText {
				lastCompletionMessage.MultiContent[i].Text = extended
				break
			}
		}
	} else {
		lastCompletionMessage.Content = extended
	}

	return lastCompletionMessage, nil
}

func buildTextChatCompletionContent(lastMessage string, ragResults []*prompts.RagContent, k *types.Knowledge, knowledgeResults []*prompts.BackgroundKnowledge) (string, error) {
	textOnlyKnowledgeResults := make([]*prompts.BackgroundKnowledge, 0, len(knowledgeResults))
	for _, result := range knowledgeResults {
		if !knowledgeHasImage(result) {
			textOnlyKnowledgeResults = append(textOnlyKnowledgeResults, result)
		}
	}

	promptRequest := &prompts.KnowledgePromptRequest{
		UserPrompt:       lastMessage,
		RAGResults:       ragResults,
		KnowledgeResults: textOnlyKnowledgeResults,
		IsVision:         anyKnowledgeHasImages(knowledgeResults),
	}

	if k != nil && k.RAGSettings.PromptTemplate != "" {
		promptRequest.PromptTemplate = k.RAGSettings.PromptTemplate
	}

	extended, err := prompts.KnowledgePrompt(promptRequest)
	if err != nil {
		return "", fmt.Errorf("failed to extend message with knowledge: %w", err)
	}

	return extended, nil
}

type systemPromptValues struct {
	LocalDate string // Current date such as 2024-01-01
	LocalTime string // Current time such as 12:00:00
}

func renderPrompt(prompt string, values systemPromptValues) (string, error) {
	tmpl, err := template.New("prompt").Parse(prompt)
	if err != nil {
		return "", fmt.Errorf("failed to parse prompt: %w", err)
	}

	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, values); err != nil {
		return "", fmt.Errorf("failed to execute prompt: %w", err)
	}

	return rendered.String(), nil
}

// setSystemPrompt if the assistant has a system prompt, set it in the request. If there is already
// provided system prompt, overwrite it and if there is no system prompt, set it as the first message
func setSystemPrompt(req *openai.ChatCompletionRequest, systemPrompt string) openai.ChatCompletionRequest {
	if systemPrompt == "" {
		// Nothing to do
		return *req
	}

	// Try to render template
	enriched, err := renderPrompt(systemPrompt, systemPromptValues{
		LocalDate: time.Now().Format("2006-01-02"),
		LocalTime: time.Now().Format("15:04:05"),
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to render system prompt")
	} else {
		systemPrompt = enriched
	}

	if len(req.Messages) == 0 {
		req.Messages = append(req.Messages, openai.ChatCompletionMessage{
			Role:    "system",
			Content: systemPrompt,
		})
	}

	if len(req.Messages) >= 1 && req.Messages[0].Role == openai.ChatMessageRoleSystem {
		req.Messages[0].Content = systemPrompt
	}

	// If first message is not a system message, add it as the first message
	if len(req.Messages) >= 1 && req.Messages[0].Role != openai.ChatMessageRoleSystem {
		req.Messages = append([]openai.ChatCompletionMessage{
			{
				Role:    "system",
				Content: systemPrompt,
			},
		}, req.Messages...)
	}

	return *req
}

func (c *Controller) GetRagClient(_ context.Context, knowledge *types.Knowledge) (rag.RAG, error) {
	// TODO: remove this
	if knowledge.RAGSettings.IndexURL != "" && knowledge.RAGSettings.QueryURL != "" {
		return rag.NewLlamaindex(&knowledge.RAGSettings), nil
	}

	return c.Options.RAG, nil
}

// UpdateSessionWithKnowledgeResults updates a session's metadata with document IDs from RAG results
func (c *Controller) UpdateSessionWithKnowledgeResults(ctx context.Context, session *types.Session, ragResults []*types.SessionRAGResult) error {
	// If no session is provided, try to get it from the context
	var err error
	if session == nil {
		sessionID, ok := oai.GetContextSessionID(ctx)
		if !ok {
			// this is normal in the openai api call, we don't have a session
			return nil
		}
		session, err = c.Options.Store.GetSession(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("failed to get session: %w", err)
		}
	}

	// If the session has no metadata, initialize it
	if session.Metadata.DocumentIDs == nil {
		session.Metadata.DocumentIDs = make(map[string]string)
	}

	// Create a map to store unique RAG results to avoid duplicates
	existingRagMap := make(map[string]*types.SessionRAGResult)

	log.Debug().
		Str("session_id", session.ID).
		Int("new_results_count", len(ragResults)).
		Int("existing_results_count", len(session.Metadata.SessionRAGResults)).
		Msg("starting deduplication of RAG results")

	// Add existing RAG results to the map if they exist
	if session.Metadata.SessionRAGResults != nil {
		for i, result := range session.Metadata.SessionRAGResults {
			// Create a unique key using document_id and hash of content
			key := createUniqueRagResultKey(result)
			existingRagMap[key] = result

			// Log detailed information about existing results
			contentPreview := ""
			if len(result.Content) > 50 {
				contentPreview = result.Content[:50] + "..."
			} else {
				contentPreview = result.Content
			}

			log.Debug().
				Str("session_id", session.ID).
				Int("index", i).
				Str("document_id", result.DocumentID).
				Str("key", key).
				Str("content_preview", contentPreview).
				Interface("metadata", result.Metadata).
				Msg("existing RAG result")
		}
	}

	// Add new RAG results to the map, avoiding duplicates
	for i, result := range ragResults {
		// ---- BEGIN IMAGE HANDLING ----
		if strings.HasPrefix(result.Content, "data:image/") {
			parts := strings.SplitN(result.Content, ";base64,", 2)
			if len(parts) == 2 {
				mimeType := strings.TrimPrefix(parts[0], "data:")
				imageData, err := base64.StdEncoding.DecodeString(parts[1])
				if err != nil {
					log.Warn().Err(err).Str("session_id", session.ID).Int("result_index", i).Msg("Failed to decode base64 image data in RAG result")
					// Keep original content if decoding fails
				} else {
					// Generate a unique filename
					ext := strings.TrimPrefix(mimeType, "image/")
					filename := result.DocumentID + "." + ext
					// Construct filestore path
					// Use owner ID if available, otherwise session ID for uniqueness
					ownerID := session.Owner // Assuming session owner is sufficient context
					if ownerID == "" {
						log.Error().Str("session_id", session.ID).Msg("no owner ID found")
						return fmt.Errorf("no owner ID found")
					}
					// Use filepath.Join and the global prefix from config
					sessionResultsPath, err := c.GetFilestoreResultsPath(types.OwnerContext{
						Owner:     ownerID,
						OwnerType: types.OwnerTypeUser,
					}, session.ID, "")
					if err != nil {
						log.Error().Err(err).Str("session_id", session.ID).Msg("failed to get filestore results path")
						return err
					}

					imagePath := filepath.Join(sessionResultsPath, filename)

					// Use WriteFile instead of Write
					_, err = c.Options.Filestore.WriteFile(ctx, imagePath, bytes.NewReader(imageData))
					if err != nil {
						log.Error().Err(err).Str("session_id", session.ID).Str("image_path", imagePath).Msg("Failed to write RAG image to filestore")
						// Keep original content if write fails
					} else {
						log.Info().Str("session_id", session.ID).Str("image_path", imagePath).Str("mime_type", mimeType).Int("size", len(imageData)).Msg("Saved RAG image to filestore")
						if result.Metadata == nil {
							result.Metadata = make(map[string]string)
						}
						// Add metadata about the original content type and the file path
						result.Metadata["original_content_type"] = mimeType
						result.Metadata["is_image_path"] = "true"
						appPath, err := c.GetFilestoreAppKnowledgePath(types.OwnerContext{}, session.ParentApp, result.Source)
						if err == nil {
							signedURL, err := c.Options.Filestore.SignedURL(ctx, appPath)
							if err == nil {
								result.Metadata["original_source"] = fmt.Sprintf("%s#page=%s", signedURL, result.Metadata["page_number"])
							}
						}

						// Update the source
						result.Source = imagePath
					}
				}
			}
		}
		// ---- END IMAGE HANDLING ----

		key := createUniqueRagResultKey(result)

		// Log detailed information about new results
		contentPreview := ""
		if len(result.Content) > 50 {
			contentPreview = result.Content[:50] + "..."
		} else {
			contentPreview = result.Content
		}

		// Only add if not already present
		existing, exists := existingRagMap[key]
		if !exists {
			existingRagMap[key] = result
			log.Debug().
				Str("session_id", session.ID).
				Int("index", i).
				Str("document_id", result.DocumentID).
				Str("key", key).
				Str("content_preview", contentPreview).
				Interface("metadata", result.Metadata).
				Msg("adding new RAG result")
		} else {
			// Log when we skip a result due to duplicate key
			existingPreview := ""
			if len(existing.Content) > 50 {
				existingPreview = existing.Content[:50] + "..."
			} else {
				existingPreview = existing.Content
			}

			log.Debug().
				Str("session_id", session.ID).
				Int("index", i).
				Str("document_id", result.DocumentID).
				Str("key", key).
				Str("new_content_preview", contentPreview).
				Str("existing_content_preview", existingPreview).
				Bool("content_different", result.Content != existing.Content).
				Int("new_content_length", len(result.Content)).
				Int("existing_content_length", len(existing.Content)).
				Interface("new_metadata", result.Metadata).
				Interface("existing_metadata", existing.Metadata).
				Msg("skipping duplicate RAG result")
		}
	}

	// Convert map back to array
	mergedResults := make([]*types.SessionRAGResult, 0, len(existingRagMap))
	for _, result := range existingRagMap {
		mergedResults = append(mergedResults, result)
	}

	// Store the merged RAG results in the session metadata
	session.Metadata.SessionRAGResults = mergedResults

	// Enhanced logging for RAG results
	logCtx := log.With().
		Str("session_id", session.ID).
		Int("rag_results_count", len(mergedResults)).
		Int("new_results_count", len(ragResults)).
		Int("total_unique_results", len(existingRagMap)).
		Logger()

	// Log details of createUniqueRagResultKey function
	if len(ragResults) > 0 {
		sampleResult := ragResults[0]
		key := createUniqueRagResultKey(sampleResult)

		// Create content hash for debugging
		h := sha256.New()
		h.Write([]byte(sampleResult.Content))
		contentHash := hex.EncodeToString(h.Sum(nil))

		logCtx.Debug().
			Str("sample_document_id", sampleResult.DocumentID).
			Str("sample_key", key).
			Str("content_hash", contentHash).
			Interface("sample_metadata", sampleResult.Metadata).
			Msg("sample of key generation")
	}

	if len(mergedResults) > 0 {
		// Log sample of first result for debugging
		sampleResult := mergedResults[0]
		logCtx.Debug().
			Str("first_document_id", sampleResult.DocumentID).
			Str("first_source", sampleResult.Source).
			Int("first_content_length", len(sampleResult.Content)).
			Msg("storing merged RAG results in session metadata - sample of first result")
	} else {
		logCtx.Warn().Msg("storing empty RAG results array in session metadata")
	}

	// Add or update document IDs
	for _, result := range ragResults {
		if result.DocumentID != "" {
			// First, determine the key to use in the DocumentIDs map
			key := result.Source
			if key == "" && result.Filename != "" {
				key = result.Filename
			}

			if key != "" {
				// Handle app session path prefix
				if session.ParentApp != "" {
					// For app sessions, we need the full filestore path for the document
					// Check if we already have a full path
					if !strings.HasPrefix(key, c.Options.Config.Controller.FilePrefixGlobal) &&
						!strings.HasPrefix(key, "apps/") {
						// Construct the full path using app prefix
						appPrefix := filestore.GetAppPrefix(c.Options.Config.Controller.FilePrefixGlobal, session.ParentApp)

						// First check if the source already has a path structure (could be from a knowledge path)
						// If it's a simple filename, join it with the app prefix
						// If it already has path components, preserve them
						var fullPath string
						if strings.Contains(key, "/") {
							// This already has a path structure, so keep it intact under the app prefix
							fullPath = filepath.Join(appPrefix, key)
						} else {
							// Simple filename, put it directly in the app prefix
							fullPath = filepath.Join(appPrefix, key)
						}

						log.Debug().
							Str("session_id", session.ID).
							Str("app_id", session.ParentApp).
							Str("original_key", key).
							Str("full_path", fullPath).
							Msg("constructing full filestore path for app document ID")
						key = fullPath
					}
				}

				// If the result has metadata with source_url, also store it directly
				if result.Metadata != nil && result.Metadata["source_url"] != "" {
					// Use the source_url as a key directly to allow frontend to find it
					log.Debug().
						Str("session_id", session.ID).
						Str("document_id", result.DocumentID).
						Str("source_url", result.Metadata["source_url"]).
						Msg("adding source_url mapping for document ID")
					session.Metadata.DocumentIDs[result.Metadata["source_url"]] = result.DocumentID
				} else {
					// Store the document ID mapping
					session.Metadata.DocumentIDs[key] = result.DocumentID
				}
			}
		}
	}

	logCtx.Debug().
		Str("session_id", session.ID).
		Interface("document_ids", session.Metadata.DocumentIDs).
		Msg("updating session with document IDs")

	// Update the session metadata
	_, err = c.UpdateSessionMetadata(ctx, session, &session.Metadata)
	if err != nil {
		log.Error().Err(err).Str("session_id", session.ID).Msg("failed to update session metadata with document IDs")
		return err
	}

	// Verify the update by fetching the session again and logging its document IDs
	verifySession, verifyErr := c.Options.Store.GetSession(ctx, session.ID)
	if verifyErr != nil {
		log.Error().Err(verifyErr).Str("session_id", session.ID).Msg("failed to verify session update")
	} else {
		// Enhanced verification logging
		verifyLogCtx := log.With().
			Str("session_id", session.ID).
			Interface("document_ids", verifySession.Metadata.DocumentIDs).
			Logger()

		// Log RAG results verification
		ragResultsCount := 0
		if verifySession.Metadata.SessionRAGResults != nil {
			ragResultsCount = len(verifySession.Metadata.SessionRAGResults)
		}

		verifyLogCtx.Debug().
			Int("verified_rag_results_count", ragResultsCount).
			Bool("has_rag_results", verifySession.Metadata.SessionRAGResults != nil).
			Msg("verified session metadata after update")
	}

	return nil
}

// Add detailed logging to createUniqueRagResultKey to understand how it's generating keys
func createUniqueRagResultKey(result *types.SessionRAGResult) string {
	// Create a hash of the content using SHA-256
	h := sha256.New()
	h.Write([]byte(result.Content))
	contentHash := hex.EncodeToString(h.Sum(nil))

	// Base key combines document_id and content hash
	key := result.DocumentID + "-" + contentHash

	// Log the metadata keys that might affect the result key
	metadataKeys := make([]string, 0)
	if result.Metadata != nil {
		for k := range result.Metadata {
			metadataKeys = append(metadataKeys, k)
		}
	}

	log.Trace().
		Str("document_id", result.DocumentID).
		Str("content_hash", contentHash[:8]+"...").
		Strs("metadata_keys", metadataKeys).
		Msg("creating unique RAG result key")

	// If metadata contains offset or chunk_id information, include it in the key
	// This ensures chunks from the same document are properly distinguished
	if result.Metadata != nil {
		if chunkID, ok := result.Metadata["chunk_id"]; ok {
			key += "-chunk-" + chunkID
			log.Trace().Str("document_id", result.DocumentID).Str("chunk_id", chunkID).Msg("added chunk_id to key")
		} else if offset, ok := result.Metadata["offset"]; ok {
			key += "-offset-" + offset
			log.Trace().Str("document_id", result.DocumentID).Str("offset", offset).Msg("added offset to key")
		}
	}

	log.Trace().
		Str("document_id", result.DocumentID).
		Str("final_key", key).
		Msg("final unique RAG result key")

	return key
}

func (c *Controller) getAppOAuthTokens(ctx context.Context, userID string, app *types.App) (map[string]string, error) {

	// Initialize empty map for OAuth tokens
	oauthTokens := make(map[string]string)

	log.Info().
		Str("user_id", userID).
		Str("app_id", app.ID).
		Bool("oauth_manager_available", c.Options.OAuthManager != nil).
		Msg("Retrieving OAuth tokens for app in getAppOAuthTokens")

	// Only proceed if we have an OAuth manager
	if c.Options.OAuthManager == nil {
		log.Warn().Msg("No OAuth manager available")
		return oauthTokens, nil
	}

	// If app is nil, return empty map
	if app == nil {
		log.Warn().Msg("App is nil in getAppOAuthTokens")
		return oauthTokens, nil
	}

	// Keep track of providers we've seen to avoid duplicates
	seenProviders := make(map[string]bool)

	// First, check the tools defined in assistants
	assistantCount := len(app.Config.Helix.Assistants)
	log.Info().
		Str("app_id", app.ID).
		Int("assistant_count", assistantCount).
		Msg("Checking assistants for OAuth tools")

	for aIdx, assistant := range app.Config.Helix.Assistants {
		// Check APIs array instead of Tools array
		apiCount := len(assistant.APIs)
		log.Info().
			Str("app_id", app.ID).
			Str("assistant_name", assistant.Name).
			Int("assistant_index", aIdx).
			Int("api_count", apiCount).
			Msg("Checking assistant APIs for OAuth providers")

		for tIdx, api := range assistant.APIs {
			if api.OAuthProvider != "" {
				log.Info().
					Str("app_id", app.ID).
					Str("api_name", api.Name).
					Int("api_index", tIdx).
					Str("oauth_provider", api.OAuthProvider).
					Strs("oauth_scopes", api.OAuthScopes).
					Bool("has_oauth_provider", api.OAuthProvider != "").
					Msg("Checking API for OAuth provider")

				providerName := api.OAuthProvider
				requiredScopes := api.OAuthScopes

				// Skip if we've already processed this provider
				if seenProviders[providerName] {
					log.Info().
						Str("provider_name", providerName).
						Msg("Skipping already processed provider")
					continue
				}
				seenProviders[providerName] = true

				log.Info().
					Str("provider_name", providerName).
					Strs("required_scopes", requiredScopes).
					Msg("Attempting to get OAuth token for API")

				token, err := c.Options.OAuthManager.GetTokenForTool(ctx, userID, providerName, requiredScopes)
				if err == nil && token != "" {
					// Add the token directly to the map using the original provider name
					oauthTokens[providerName] = token
					log.Info().
						Str("provider", providerName).
						Str("token_prefix", token[:10]+"...").
						Msg("Successfully retrieved OAuth token for provider")
				} else {
					var scopeErr *oauth.ScopeError
					if errors.As(err, &scopeErr) {
						log.Warn().
							Str("app_id", app.ID).
							Str("user_id", userID).
							Str("provider", providerName).
							Strs("missing_scopes", scopeErr.Missing).
							Strs("required_scopes", requiredScopes).
							Strs("available_scopes", scopeErr.Has).
							Msg("Missing required OAuth scopes for API")
					} else {
						log.Warn().
							Err(err).
							Str("provider", providerName).
							Str("error_type", fmt.Sprintf("%T", err)).
							Msg("Failed to get OAuth token for API")
					}
				}
			}
		}
	}

	// Log tokens that were found
	tokenKeys := make([]string, 0, len(oauthTokens))
	for key := range oauthTokens {
		tokenKeys = append(tokenKeys, key)
	}

	log.Info().
		Str("app_id", app.ID).
		Int("token_count", len(oauthTokens)).
		Strs("token_keys", tokenKeys).
		Msg("Completed OAuth token retrieval in getAppOAuthTokens")

	return oauthTokens, nil
}

func (c *Controller) evalAndAddOAuthTokens(ctx context.Context, client oai.Client, opts *ChatCompletionOptions, user *types.User) error {
	log.Info().
		Str("user_id", user.ID).
		Str("app_id", opts.AppID).
		Int("existing_oauth_token_count", len(opts.OAuthTokens)).
		Bool("oauth_manager_available", c.Options.OAuthManager != nil).
		Msg("Starting evalAndAddOAuthTokens")

	// If we already have OAuth tokens, use them
	if len(opts.OAuthTokens) > 0 {
		log.Info().
			Int("token_count", len(opts.OAuthTokens)).
			Msg("Using pre-existing OAuth tokens, skipping retrieval")
		return nil
	}

	// If we have an app ID, try to get OAuth tokens
	if opts.AppID != "" && c.Options.OAuthManager != nil {
		log.Debug().
			Str("app_id", opts.AppID).
			Str("user_id", user.ID).
			Msg("Retrieving app for OAuth tokens")

		app, err := c.Options.Store.GetApp(ctx, opts.AppID)
		if err != nil {
			log.Warn().
				Err(err).
				Str("app_id", opts.AppID).
				Msg("Failed to get app for OAuth tokens")
			return nil // Continue without OAuth tokens
		}

		log.Info().
			Str("app_id", app.ID).
			Str("app_name", app.Config.Helix.Name).
			Int("assistant_count", len(app.Config.Helix.Assistants)).
			Msg("Successfully retrieved app for OAuth tokens")

		// Get OAuth tokens directly as a map
		log.Debug().
			Str("app_id", app.ID).
			Str("user_id", user.ID).
			Msg("Calling getAppOAuthTokens to retrieve tokens")

		oauthTokens, err := c.getAppOAuthTokens(ctx, user.ID, app)
		if err != nil {
			log.Warn().
				Err(err).
				Str("app_id", opts.AppID).
				Msg("Failed to get OAuth tokens for app")
			return nil // Continue without OAuth tokens
		}

		log.Info().
			Str("app_id", app.ID).
			Int("oauth_token_count", len(oauthTokens)).
			Msg("Retrieved OAuth tokens from getAppOAuthTokens")

		// Add OAuth tokens to the options
		opts.OAuthTokens = oauthTokens

		// Log retrieved tokens
		if len(oauthTokens) > 0 {
			log.Info().
				Int("token_count", len(oauthTokens)).
				Msg("Successfully retrieved OAuth tokens for tools")
		}
	} else {
		if opts.AppID == "" {
			// Check if app ID is in the context
			if appID, ok := oai.GetContextAppID(ctx); ok && appID != "" {
				log.Info().
					Str("app_id_from_context", appID).
					Msg("Found app ID in context, using it for OAuth token retrieval")

				// Set the app ID in the options and recursively call this function again
				opts.AppID = appID
				return c.evalAndAddOAuthTokens(ctx, client, opts, user)
			}

			log.Debug().Msg("No app ID specified in options or context, skipping OAuth token retrieval")
		}
		if c.Options.OAuthManager == nil {
			log.Warn().Msg("OAuth manager is not available")
		}
	}

	return nil
}
