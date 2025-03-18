package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/filestore"
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
	Provider    string

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

	if assistant.Provider != "" {
		opts.Provider = assistant.Provider
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

		modelName, err := model.ProcessModelName(c.Options.Config.Inference.Provider, req.Model, types.SessionModeInference, types.SessionTypeText, false, false)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid model name '%s': %w", req.Model, err)
		}

		req.Model = modelName
	}

	if assistant.RAGSourceID != "" {
		opts.RAGSourceID = assistant.RAGSourceID
	}

	err = c.enrichPromptWithKnowledge(ctx, user, &req, assistant, opts)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to enrich prompt with knowledge: %w", err)
	}

	client, err := c.getClient(ctx, user.ID, opts.Provider)
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

	if assistant.Provider != "" {
		opts.Provider = assistant.Provider
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

		modelName, err := model.ProcessModelName(c.Options.Config.Inference.Provider, req.Model, types.SessionModeInference, types.SessionTypeText, false, false)
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

	client, err := c.getClient(ctx, user.ID, opts.Provider)
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

func (c *Controller) getClient(ctx context.Context, owner, provider string) (oai.Client, error) {
	if provider == "" {
		// If not set, use the default provider
		provider = c.Options.Config.Inference.Provider
	}

	log.Trace().
		Str("provider", provider).
		Str("owner", owner).
		Msg("getting OpenAI API client")

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

	if err := c.emitStepInfo(ctx, &types.StepInfo{
		Name:    selectedTool.Name,
		Type:    types.StepInfoTypeToolUse,
		Message: "Running action",
	}); err != nil {
		log.Debug().Err(err).Msg("failed to emit run action step info")
	}

	var options []tools.Option

	apieClient, err := c.getClient(ctx, user.ID, opts.Provider)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get client: %w", err)
	}

	options = append(options, tools.WithClient(apieClient))

	resp, err := c.ToolsPlanner.RunAction(ctx, vals.SessionID, vals.InteractionID, selectedTool, history, isActionable.API, options...)
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

	var options []tools.Option

	apieClient, err := c.getClient(ctx, user.ID, opts.Provider)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get client: %w", err)
	}

	options = append(options, tools.WithClient(apieClient))

	stream, err := c.ToolsPlanner.RunActionStream(ctx, vals.SessionID, vals.InteractionID, selectedTool, history, isActionable.API, options...)
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

	apieClient, err := c.getClient(ctx, user.ID, opts.Provider)
	if err != nil {
		return nil, nil, false, fmt.Errorf("failed to get client: %w", err)
	}

	options = append(options, tools.WithClient(apieClient))

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

	app, err := c.Options.Store.GetAppWithTools(ctx, opts.AppID)
	if err != nil {
		return nil, fmt.Errorf("error getting app: %w", err)
	}

	if err := c.AuthorizeUserToApp(ctx, user, app); err != nil {
		return nil, err
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

	// Parse document IDs from the completion request
	documentIDs := rag.ParseDocumentIDs(prompt)

	log.Trace().Interface("documentIDs", documentIDs).Msg("document IDs")

	ragResults, err := c.Options.RAG.Query(ctx, &types.SessionRAGQuery{
		Prompt:            prompt,
		DataEntityID:      entity.ID,
		DistanceThreshold: entity.Config.RAGSettings.Threshold,
		DistanceFunction:  entity.Config.RAGSettings.DistanceFunction,
		MaxResults:        entity.Config.RAGSettings.ResultsCount,
		DocumentIDList:    documentIDs,
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

			// Parse document IDs from the completion request
			documentIDs := rag.ParseDocumentIDs(prompt)

			log.Trace().Interface("documentIDs", documentIDs).Msg("document IDs")

			ragResults, err := ragClient.Query(ctx, &types.SessionRAGQuery{
				Prompt:            prompt,
				DataEntityID:      knowledge.GetDataEntityID(),
				DistanceThreshold: knowledge.RAGSettings.Threshold,
				DistanceFunction:  knowledge.RAGSettings.DistanceFunction,
				MaxResults:        knowledge.RAGSettings.ResultsCount,
				DocumentIDList:    documentIDs,
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

	log.Trace().
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

	// Log the updated document IDs
	log.Debug().
		Str("session_id", session.ID).
		Interface("document_ids", session.Metadata.DocumentIDs).
		Msg("updating session with document IDs")

	// Update the session metadata
	updatedMeta, err := c.UpdateSessionMetadata(ctx, session, &session.Metadata)
	if err != nil {
		log.Error().Err(err).Str("session_id", session.ID).Msg("failed to update session metadata with document IDs")
		return err
	}

	// Verify the update by fetching the session again and logging its document IDs
	verifySession, verifyErr := c.Options.Store.GetSession(ctx, session.ID)
	if verifyErr != nil {
		log.Error().Err(verifyErr).Str("session_id", session.ID).Msg("failed to verify session update")
	} else {
		log.Debug().
			Str("session_id", session.ID).
			Interface("document_ids", verifySession.Metadata.DocumentIDs).
			Interface("updated_meta", updatedMeta).
			Msg("verified session document IDs after update")
	}

	return nil
}
