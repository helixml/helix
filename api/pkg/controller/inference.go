package controller

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/model"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/prompts"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/rag"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/tools"
	"github.com/helixml/helix/api/pkg/types"
	"gopkg.in/yaml.v2"

	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

type ChatCompletionOptions struct {
	AppID       string
	AssistantID string
	RAGSourceID string
	Provider    types.Provider

	QueryParams map[string]string
}

// ChatCompletion is used by the OpenAI compatible API. Doesn't handle any historical sessions, etc.
// Runs the OpenAI with tools/app configuration and returns the response.
// Returns the updated request because the controller mutates it when doing e.g. tools calls and RAG
func (c *Controller) ChatCompletion(ctx context.Context, user *types.User, req openai.ChatCompletionRequest, opts *ChatCompletionOptions) (*openai.ChatCompletionResponse, *openai.ChatCompletionRequest, error) {
	assistant, err := c.loadAssistant(ctx, user, opts)
	if err != nil {
		log.Info().Msg("no assistant found")
		return nil, nil, err
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

	if assistant.Model != "" {
		req.Model = assistant.Model

		modelName, err := model.ProcessModelName(string(c.Options.Config.Inference.Provider), req.Model, types.SessionModeInference, types.SessionTypeText, false, false)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid model name '%s': %w", req.Model, err)
		}

		req.Model = modelName
	}

	if assistant.RAGSourceID != "" {
		opts.RAGSourceID = assistant.RAGSourceID
	}

	if assistant.Provider != "" {
		opts.Provider = assistant.Provider
	}

	err = c.enrichPromptWithKnowledge(ctx, user, &req, assistant, opts)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to enrich prompt with knowledge: %w", err)
	}

	client, err := c.getClient(ctx, opts.Provider)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get client: %v", err)
	}

	resp, err := client.CreateChatCompletion(ctx, req)
	if err != nil {
		log.Err(err).Msg("error creating chat completion")
		return nil, nil, err
	}

	return &resp, &req, nil
}

// ChatCompletionStream is used by the OpenAI compatible API. Doesn't handle any historical sessions, etc.
// Runs the OpenAI with tools/app configuration and returns the stream.
func (c *Controller) ChatCompletionStream(ctx context.Context, user *types.User, req openai.ChatCompletionRequest, opts *ChatCompletionOptions) (*openai.ChatCompletionStream, *openai.ChatCompletionRequest, error) {
	req.Stream = true

	assistant, err := c.loadAssistant(ctx, user, opts)
	if err != nil {
		log.Info().Msg("no assistant found")
		return nil, nil, err
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

	if assistant.Model != "" {
		req.Model = assistant.Model

		modelName, err := model.ProcessModelName(string(c.Options.Config.Inference.Provider), req.Model, types.SessionModeInference, types.SessionTypeText, false, false)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid model name '%s': %w", req.Model, err)
		}

		req.Model = modelName
	}

	if assistant.RAGSourceID != "" {
		opts.RAGSourceID = assistant.RAGSourceID
	}

	if assistant.Provider != "" {
		opts.Provider = assistant.Provider
	}

	// Check for knowledge
	err = c.enrichPromptWithKnowledge(ctx, user, &req, assistant, opts)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to enrich prompt with knowledge: %w", err)
	}

	client, err := c.getClient(ctx, opts.Provider)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get client: %v", err)
	}

	stream, err := client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		log.Err(err).Msg("error creating chat completion stream")
		return nil, nil, err
	}

	return stream, &req, nil
}

func (c *Controller) getClient(ctx context.Context, provider types.Provider) (oai.Client, error) {
	if provider == "" {
		provider = c.Options.Config.Inference.Provider
	}

	client, err := c.providerManager.GetClient(ctx, &manager.GetClientRequest{
		Provider: provider,
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

	if err := c.emitStepInfo(ctx, &types.StepInfo{
		Name:    selectedTool.Name,
		Type:    types.StepInfoTypeToolUse,
		Message: "Running action",
	}); err != nil {
		log.Debug().Err(err).Msg("failed to emit run action step info")
	}

	resp, err := c.ToolsPlanner.RunAction(ctx, vals.SessionID, vals.InteractionID, selectedTool, history, isActionable.API)
	if err != nil {
		if emitErr := c.emitStepInfo(ctx, &types.StepInfo{
			Name:    selectedTool.Name,
			Type:    types.StepInfoTypeToolUse,
			Message: fmt.Sprintf("Action failed: %s", err),
		}); emitErr != nil {
			log.Debug().Err(err).Msg("failed to emit run action step info")
		}

		return nil, false, fmt.Errorf("failed to perform action: %w", err)
	}

	if err := c.emitStepInfo(ctx, &types.StepInfo{
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

	if err := c.emitStepInfo(ctx, &types.StepInfo{
		Name:    selectedTool.Name,
		Type:    types.StepInfoTypeToolUse,
		Message: "Running action",
	}); err != nil {
		log.Debug().Err(err).Msg("failed to emit step info")
	}

	stream, err := c.ToolsPlanner.RunActionStream(ctx, vals.SessionID, vals.InteractionID, selectedTool, history, isActionable.API)
	if err != nil {
		log.Warn().
			Err(err).
			Str("tool", selectedTool.Name).
			Str("action", isActionable.API).
			Msg("failed to perform action")

		if emitErr := c.emitStepInfo(ctx, &types.StepInfo{
			Name:    selectedTool.Name,
			Type:    types.StepInfoTypeToolUse,
			Message: fmt.Sprintf("Action failed: %s", err),
		}); emitErr != nil {
			log.Debug().Err(err).Msg("failed to emit step info")
		}

		return nil, false, fmt.Errorf("failed to perform action: %w", err)
	}

	if err := c.emitStepInfo(ctx, &types.StepInfo{
		Name:    selectedTool.Name,
		Type:    types.StepInfoTypeToolUse,
		Message: "Action completed",
	}); err != nil {
		log.Debug().Err(err).Msg("failed to emit step info")
	}

	return stream, true, nil
}

func (c *Controller) selectAndConfigureTool(ctx context.Context, user *types.User, req openai.ChatCompletionRequest, opts *ChatCompletionOptions) (*types.Tool, *tools.IsActionableResponse, bool, error) {
	assistant, err := c.loadAssistant(ctx, user, opts)
	if err != nil {
		log.Info().Msg("no assistant found")
		return nil, nil, false, err
	}

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
	// If assistant has configured a model, use it
	if assistant != nil && assistant.Model != "" {
		options = append(options, tools.WithModel(assistant.Model))
	}

	history := types.HistoryFromChatCompletionRequest(req)

	vals, ok := oai.GetContextValues(ctx)
	if !ok {
		vals = &oai.ContextValues{}
	}

	if err := c.emitStepInfo(ctx, &types.StepInfo{
		Name:    "is_actionable",
		Type:    types.StepInfoTypeToolUse,
		Message: "Checking if we should use tools",
	}); err != nil {
		log.Debug().Err(err).Msg("failed to emit step info")
	}

	isActionable, err := c.ToolsPlanner.IsActionable(ctx, vals.SessionID, vals.InteractionID, assistant.Tools, history, options...)
	if err != nil {
		log.Error().Err(err).Msg("failed to evaluate if the message is actionable, skipping to general knowledge")
		return nil, nil, false, fmt.Errorf("failed to evaluate if the message is actionable: %w", err)
	}

	if !isActionable.Actionable() {
		if err := c.emitStepInfo(ctx, &types.StepInfo{
			Name:    "is_actionable",
			Type:    types.StepInfoTypeToolUse,
			Message: "Message is not actionable",
		}); err != nil {
			log.Debug().Err(err).Msg("failed to emit step info")
		}

		return nil, nil, false, nil
	}

	selectedTool, ok := tools.GetToolFromAction(assistant.Tools, isActionable.API)
	if !ok {
		return nil, nil, false, fmt.Errorf("tool not found for action: %s", isActionable.API)
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

	return selectedTool, isActionable, true, nil
}

func (c *Controller) loadAssistant(ctx context.Context, user *types.User, opts *ChatCompletionOptions) (*types.AssistantConfig, error) {
	if opts.AppID == "" {
		return &types.AssistantConfig{}, nil
	}

	// TODO: change GetAppWithTools to GetApp when we've updated all inference
	// code to use apis, gptscripts, and zapier fields directly. Meanwhile, the
	// flattened tools list is the internal only representation, and should not
	// be exposed to the user.
	app, err := c.Options.Store.GetAppWithTools(ctx, opts.AppID)
	if err != nil {
		return nil, fmt.Errorf("error getting app: %w", err)
	}

	if (!app.Global && !app.Shared) && app.Owner != user.ID {
		return nil, fmt.Errorf("you do not have access to the app with the id: %s", app.ID)
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

	ragResults, err := c.Options.RAG.Query(ctx, &types.SessionRAGQuery{
		Prompt:            getLastMessage(req),
		DataEntityID:      entity.ID,
		DistanceThreshold: entity.Config.RAGSettings.Threshold,
		DistanceFunction:  entity.Config.RAGSettings.DistanceFunction,
		MaxResults:        entity.Config.RAGSettings.ResultsCount,
	})
	if err != nil {
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
		case knowledge.Source.Content != nil:
			backgroundKnowledge = append(backgroundKnowledge, &prompts.BackgroundKnowledge{
				Description: knowledge.Description,
				Content:     *knowledge.Source.Content,
			})

			usedKnowledge = knowledge
		default:
			ragClient, err := c.GetRagClient(ctx, knowledge)
			if err != nil {
				return nil, nil, fmt.Errorf("error getting RAG client: %w", err)
			}

			if err := c.emitStepInfo(ctx, &types.StepInfo{
				Name:    knowledge.Name,
				Type:    types.StepInfoTypeRAG,
				Message: "Searching for knowledge",
			}); err != nil {
				log.Debug().Err(err).Msg("failed to emit step info")
			}

			ragResults, err := ragClient.Query(ctx, &types.SessionRAGQuery{
				Prompt:            prompt,
				DataEntityID:      knowledge.GetDataEntityID(),
				DistanceThreshold: knowledge.RAGSettings.Threshold,
				DistanceFunction:  knowledge.RAGSettings.DistanceFunction,
				MaxResults:        knowledge.RAGSettings.ResultsCount,
			})
			if err != nil {
				return nil, nil, fmt.Errorf("error querying RAG: %w", err)
			}

			if err := c.emitStepInfo(ctx, &types.StepInfo{
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
			}
		}
	}

	return backgroundKnowledge, usedKnowledge, nil
}

func (c *Controller) emitStepInfo(ctx context.Context, stepInfo *types.StepInfo) error {
	vals, ok := oai.GetContextValues(ctx)
	if !ok {
		log.Warn().Msg("context values with session info not found")
		return fmt.Errorf("context values with session info not found")
	}

	queue := pubsub.GetSessionQueue(vals.OwnerID, vals.SessionID)
	event := &types.WebsocketEvent{
		Type:          types.WebsocketEventProcessingStepInfo,
		SessionID:     vals.SessionID,
		InteractionID: vals.InteractionID,
		Owner:         vals.OwnerID,
		StepInfo:      stepInfo,
	}
	bts, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal step info: %w", err)
	}

	log.Info().
		Str("queue", queue).
		Str("step_name", stepInfo.Name).
		Str("step_message", stepInfo.Message).
		Msg("emitting step info")

	// TODO: save in the database too

	return c.Options.PubSub.Publish(ctx, queue, bts)
}

// TODO: use different struct with just document ID and content
func extendMessageWithKnowledge(req *openai.ChatCompletionRequest, ragResults []*prompts.RagContent, k *types.Knowledge, knowledgeResults []*prompts.BackgroundKnowledge) error {
	lastMessage := getLastMessage(*req)

	promptRequest := &prompts.KnowledgePromptRequest{
		UserPrompt:       lastMessage,
		RAGResults:       ragResults,
		KnowledgeResults: knowledgeResults,
	}

	if k != nil && k.RAGSettings.PromptTemplate != "" {
		promptRequest.PromptTemplate = k.RAGSettings.PromptTemplate
	}

	extended, err := prompts.KnowledgePrompt(promptRequest)
	if err != nil {
		return fmt.Errorf("failed to extend message with knowledge: %w", err)
	}

	req.Messages[len(req.Messages)-1].Content = extended

	return nil
}

// setSystemPrompt if the assistant has a system prompt, set it in the request. If there is already
// provided system prompt, overwrite it and if there is no system prompt, set it as the first message
func setSystemPrompt(req *openai.ChatCompletionRequest, systemPrompt string) openai.ChatCompletionRequest {
	if systemPrompt == "" {
		// Nothing to do
		return *req
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
