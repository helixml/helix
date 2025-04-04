package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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

	"slices"

	"github.com/helixml/helix/api/pkg/oauth"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

type ChatCompletionOptions struct {
	AppID        string
	AssistantID  string
	RAGSourceID  string
	Provider     string
	QueryParams  map[string]string
	OAuthEnvVars []string // OAuth environment variables
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

	// Evaluate and add OAuth tokens
	err = c.evalAndAddOAuthTokens(ctx, client, opts, user)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to add OAuth tokens: %w", err)
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

	// Evaluate and add OAuth tokens
	err = c.evalAndAddOAuthTokens(ctx, client, opts, user)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to add OAuth tokens: %w", err)
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
	textOnlyKnowledgeResults := make([]*prompts.BackgroundKnowledge, 0, len(knowledgeResults))
	for _, result := range knowledgeResults {
		if knowledgeHasImage(result) {
			imageParts = append(imageParts, openai.ChatMessagePart{
				Type: openai.ChatMessagePartTypeImageURL,
				ImageURL: &openai.ChatMessageImageURL{
					URL: result.Content,
				},
			})
		} else {
			// Can't pass knowledge results with images to the text completion prompt
			textOnlyKnowledgeResults = append(textOnlyKnowledgeResults, result)
		}
	}

	lastMessage := getLastMessage(*req)

	extended, err := buildTextChatCompletionContent(lastMessage, ragResults, k, textOnlyKnowledgeResults)
	if err != nil {
		return openai.ChatCompletionMessage{}, fmt.Errorf("failed to build text chat completion content: %w", err)
	}

	// Now rebuild the final message with the text at the top like in the example
	res := openai.ChatCompletionMessage{
		Role: openai.ChatMessageRoleUser,
		MultiContent: append([]openai.ChatMessagePart{
			{
				Type: openai.ChatMessagePartTypeText,
				Text: extended,
			},
		}, imageParts...),
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
	lastCompletionMessage.Content = extended

	return lastCompletionMessage, nil
}

func buildTextChatCompletionContent(lastMessage string, ragResults []*prompts.RagContent, k *types.Knowledge, knowledgeResults []*prompts.BackgroundKnowledge) (string, error) {
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
		return "", fmt.Errorf("failed to extend message with knowledge: %w", err)
	}

	return extended, nil
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

	// Log the updated document IDs
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

func (c *Controller) getAppOAuthTokens(ctx context.Context, userID string, app *types.App) ([]string, error) {
	// Initialize empty slice for environment variables
	var envVars []string

	// Only proceed if we have an OAuth manager
	if c.Options.OAuthManager == nil {
		log.Debug().Msg("No OAuth manager available")
		return nil, nil
	}

	// If app is nil, return empty slice
	if app == nil {
		return nil, nil
	}

	// Keep track of providers we've seen to avoid duplicates
	seenProviders := make(map[types.OAuthProviderType]bool)

	// First, check the tools defined in assistants
	for _, assistant := range app.Config.Helix.Assistants {
		for _, tool := range assistant.Tools {
			if tool.ToolType == types.ToolTypeAPI && tool.Config.API != nil && tool.Config.API.OAuthProvider != "" {
				providerType := tool.Config.API.OAuthProvider
				requiredScopes := tool.Config.API.OAuthScopes

				// Skip if we've already processed this provider
				if seenProviders[providerType] {
					continue
				}
				seenProviders[providerType] = true

				token, err := c.Options.OAuthManager.GetTokenForTool(ctx, userID, providerType, requiredScopes)
				if err == nil && token != "" {
					envName := fmt.Sprintf("OAUTH_TOKEN_%s", strings.ToUpper(string(providerType)))
					envVars = append(envVars, fmt.Sprintf("%s=%s", envName, token))
					log.Debug().Str("provider", string(providerType)).Msg("Added OAuth token to app environment")
				} else {
					var scopeErr *oauth.ScopeError
					if errors.As(err, &scopeErr) {
						log.Warn().
							Str("app_id", app.ID).
							Str("user_id", userID).
							Str("provider", string(providerType)).
							Strs("missing_scopes", scopeErr.Missing).
							Msg("Missing required OAuth scopes for tool")
					} else {
						log.Debug().Err(err).Str("provider", string(providerType)).Msg("Failed to get OAuth token for tool")
					}
				}
			}
		}
	}

	return envVars, nil
}

func (c *Controller) evalAndAddOAuthTokens(ctx context.Context, client oai.Client, opts *ChatCompletionOptions, user *types.User) error {
	// If we already have OAuth tokens, use them
	if len(opts.OAuthEnvVars) > 0 {
		return nil
	}

	// If we have an app ID, try to get OAuth tokens
	if opts.AppID != "" && c.Options.OAuthManager != nil {
		app, err := c.Options.Store.GetApp(ctx, opts.AppID)
		if err != nil {
			log.Debug().Err(err).Str("app_id", opts.AppID).Msg("Failed to get app for OAuth tokens")
			return nil // Continue without OAuth tokens
		}

		// Get OAuth tokens as environment variables
		oauthEnvVars, err := c.getAppOAuthTokens(ctx, user.ID, app)
		if err != nil {
			log.Debug().Err(err).Str("app_id", opts.AppID).Msg("Failed to get OAuth tokens for app")
			return nil // Continue without OAuth tokens
		}

		// Add OAuth tokens to the options
		opts.OAuthEnvVars = oauthEnvVars

		// If we have tokens, add them to the client as well
		if len(oauthEnvVars) > 0 {
			for _, envVar := range oauthEnvVars {
				parts := strings.SplitN(envVar, "=", 2)
				if len(parts) == 2 {
					envKey, envValue := parts[0], parts[1]
					// Only process OAUTH_TOKEN_ variables
					if strings.HasPrefix(envKey, "OAUTH_TOKEN_") {
						// Extract provider type from env var name (e.g., OAUTH_TOKEN_GITHUB -> github)
						providerType := strings.ToLower(strings.TrimPrefix(envKey, "OAUTH_TOKEN_"))
						log.Debug().Str("provider", providerType).Msg("Added OAuth token to API client HTTP headers")
						// Add OAuth token to client (if supported)
						if retryableClient, ok := client.(*oai.RetryableClient); ok {
							retryableClient.AddOAuthToken(providerType, envValue)
						}
					}
				}
			}
		}
	}

	return nil
}
