package controller

import (
	"context"
	"fmt"
	"time"

	agent "github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/agent/skill"
	azuredevops "github.com/helixml/helix/api/pkg/agent/skill/azure_devops"
	"github.com/helixml/helix/api/pkg/agent/skill/mcp"
	"github.com/helixml/helix/api/pkg/agent/skill/memory"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/openai/transport"
	"github.com/helixml/helix/api/pkg/rag"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

type runAgentRequest struct {
	OrganizationID string
	Assistant      *types.AssistantConfig
	User           *types.User
	Request        openai.ChatCompletionRequest
	Options        *ChatCompletionOptions
}

func (c *Controller) runAgent(ctx context.Context, req *runAgentRequest) (*agent.Session, error) {
	vals, ok := oai.GetContextValues(ctx)
	if !ok {
		vals = &oai.ContextValues{}
	}

	appID, ok := oai.GetContextAppID(ctx)
	if !ok {
		return nil, fmt.Errorf("appID not set in context, use 'openai.SetContextAppID()' before calling this method")
	}

	ownerID := req.User.ID
	if req.OrganizationID != "" {
		ownerID = req.OrganizationID
	}

	log.Info().
		Str("session_id", vals.SessionID).
		Str("user_id", req.User.ID).
		Str("owner_id", ownerID).
		Str("interaction_id", vals.InteractionID).
		Msg("Running agent")

	// Default memory uses Postgres to load and persist memories
	mem := agent.NewDefaultMemory(req.Assistant.Memory, c.Options.Store)

	// Assemble clients and providers

	reasoningModel, err := c.getLLMModelConfig(ctx,
		ownerID,
		withFallbackProvider(req.Assistant.ReasoningModelProvider, req.Assistant), // Defaults to top level assistant provider
		req.Assistant.ReasoningModel)
	if err != nil {
		return nil, fmt.Errorf("failed to get reasoning model config: %w", err)
	}
	reasoningModel.ReasoningEffort = req.Assistant.ReasoningModelEffort

	log.Debug().
		Str("reasoning_model_provider", withFallbackProvider(req.Assistant.ReasoningModelProvider, req.Assistant)).
		Str("reasoning_model", req.Assistant.ReasoningModel).
		Str("configured_model", reasoningModel.Model).
		Msg("Reasoning model configuration")

	generationModel, err := c.getLLMModelConfig(ctx,
		ownerID,
		withFallbackProvider(req.Assistant.GenerationModelProvider, req.Assistant), // Defaults to top level assistant provider
		req.Assistant.GenerationModel)
	if err != nil {
		return nil, fmt.Errorf("failed to get generation model config: %w", err)
	}

	log.Debug().
		Str("generation_model_provider", withFallbackProvider(req.Assistant.GenerationModelProvider, req.Assistant)).
		Str("generation_model", req.Assistant.GenerationModel).
		Str("configured_model", generationModel.Model).
		Msg("Generation model configuration")

	smallReasoningModel, err := c.getLLMModelConfig(ctx,
		ownerID,
		withFallbackProvider(req.Assistant.SmallReasoningModelProvider, req.Assistant), // Defaults to top level assistant provider
		req.Assistant.SmallReasoningModel)
	if err != nil {
		return nil, fmt.Errorf("failed to get small reasoning model config: %w", err)
	}
	smallReasoningModel.ReasoningEffort = req.Assistant.SmallReasoningModelEffort

	smallGenerationModel, err := c.getLLMModelConfig(ctx,
		ownerID,
		withFallbackProvider(req.Assistant.SmallGenerationModelProvider, req.Assistant), // Defaults to top level assistant provider
		req.Assistant.SmallGenerationModel)
	if err != nil {
		return nil, fmt.Errorf("failed to get small generation model config: %w", err)
	}

	llm := agent.NewLLM(
		reasoningModel,
		generationModel,
		smallReasoningModel,
		smallGenerationModel,
	)

	enriched, err := renderPrompt(req.Assistant.SystemPrompt, systemPromptValues{
		LocalDate: time.Now().Format("2006-01-02"),
		LocalTime: time.Now().Format("15:04:05"),
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to render system prompt")
	}

	var skills []agent.Skill

	if req.Assistant.Memory {
		skills = append(skills, memory.NewAddMemorySkill(c.Options.Store))
	}

	lastUserMessage := getLastMessage(req.Request)

	// Get API skills
	for _, assistantTool := range req.Assistant.Tools {

		if assistantTool.ToolType == types.ToolTypeAPI {
			// Use direct API skills instead of the skill context runner approach
			// This allows the main agent to orchestrate API calls with other tools (Calculator, Currency_Exchange_Rates)
			apiSkills := skill.NewDirectAPICallingSkills(c.ToolsPlanner, c.Options.OAuthManager, assistantTool)
			skills = append(skills, apiSkills...)
		}

		if assistantTool.ToolType == types.ToolTypeMCP {
			skills = append(skills, mcp.NewDirectMCPClientSkills(c.Options.MCPClientGetter, c.Options.OAuthManager, assistantTool)...)
		}

		if assistantTool.ToolType == types.ToolTypeBrowser {
			skills = append(skills, skill.NewBrowserSkill(assistantTool.Config.Browser, c.Options.Browser, llm, c.browserCache))
		}

		if assistantTool.ToolType == types.ToolTypeCalculator {
			skills = append(skills, skill.NewCalculatorSkill())
		}

		if assistantTool.ToolType == types.ToolTypeEmail {
			skills = append(skills, skill.NewSendEmailSkill(&c.Options.Config.Notifications.Email, assistantTool.Config.Email.TemplateExample))
		}

		if assistantTool.ToolType == types.ToolTypeWebSearch {
			skills = append(skills, skill.NewSearchSkill(assistantTool.Config.WebSearch, c.Options.SearchProvider, c.Options.Browser))
		}

		if assistantTool.ToolType == types.ToolTypeAzureDevOps {
			// TODO: add support for granular skill selection
			skills = append(skills, azuredevops.NewCreateThreadSkill(assistantTool.Config.AzureDevOps.OrganizationURL, assistantTool.Config.AzureDevOps.PersonalAccessToken))
			skills = append(skills, azuredevops.NewReplyToCommentSkill(assistantTool.Config.AzureDevOps.OrganizationURL, assistantTool.Config.AzureDevOps.PersonalAccessToken))
			skills = append(skills, azuredevops.NewPullRequestDiffSkill(assistantTool.Config.AzureDevOps.OrganizationURL, assistantTool.Config.AzureDevOps.PersonalAccessToken))
		}
	}

	// Get assistant knowledge
	knowledges, err := c.Options.Store.ListKnowledge(ctx, &store.ListKnowledgeQuery{
		AppID: appID,
	})
	if err != nil {
		log.Error().Err(err).Msgf("error listing knowledges for app %s", appID)
		return nil, fmt.Errorf("failed to list knowledges for app %s: %w", appID, err)
	}

	log.Info().
		Str("app_id", appID).
		Int("knowledge_count", len(knowledges)).
		Msg("Listed knowledges for app")

	knowledgeMemory := agent.NewMemoryBlock()

	// Only get from the last message the filter. If users want to filter by specific document they just
	// need to filter again
	filterActions := rag.ParseFilterActions(lastUserMessage)
	var filterDocumentIDs []string
	for _, filterAction := range filterActions {
		filterDocumentIDs = append(filterDocumentIDs, rag.ParseDocID(filterAction))
	}

	for _, knowledge := range knowledges {
		switch {
		// Filestore, Web, and SharePoint are presented to the agents as a tool that
		// can be used to search for knowledge
		case knowledge.Source.Filestore != nil, knowledge.Source.Web != nil, knowledge.Source.SharePoint != nil:
			ragClient, err := c.GetRagClient(ctx, knowledge)
			if err != nil {
				log.Error().Err(err).Msgf("error getting RAG client for knowledge %s", knowledge.ID)
				return nil, fmt.Errorf("failed to get RAG client for knowledge %s: %w", knowledge.ID, err)
			}
			skills = append(skills, skill.NewKnowledgeSkill(ragClient, knowledge, filterDocumentIDs))
		case knowledge.Source.Text != nil:
			// knowledgeBlocks = append(knowledgeBlocks, *knowledge.Source.Text)

			knowledgeBlock := agent.NewMemoryBlock()
			knowledgeBlock.AddString("name", knowledge.Name)
			knowledgeBlock.AddString("description", knowledge.Description)
			knowledgeBlock.AddString("contents", *knowledge.Source.Text)

			knowledgeMemory.AddBlock("knowledge", knowledgeBlock)
		}
	}

	helixAgent := agent.NewAgent(
		c.stepInfoEmitter,
		enriched,
		skills,
		req.Assistant.MaxIterations,
	)

	messageHistory := agent.NewMessageList()

	// Add request messages except the last user message
	for _, message := range req.Request.Messages[:len(req.Request.Messages)-1] {
		messageText, err := types.GetMessageText(&message)
		if err != nil {
			log.Error().Any("request", req.Request).Err(err).Msg("failed to get message text")
			continue
		}
		// TODO: multi-content messages
		switch message.Role {
		case openai.ChatMessageRoleUser:
			messageHistory.Add(agent.UserMessage(messageText))
		case openai.ChatMessageRoleSystem, openai.ChatMessageRoleAssistant:
			messageHistory.Add(agent.AssistantMessage(messageText))
		}
	}

	session := agent.NewSession(ctx, c.stepInfoEmitter, llm, mem, knowledgeMemory, helixAgent, messageHistory, agent.Meta{
		AppID:         appID,
		UserID:        req.User.ID,
		UserEmail:     req.User.Email,
		SessionID:     vals.SessionID,
		InteractionID: vals.InteractionID,
		Extra:         map[string]string{},
	}, req.Options.Conversational)

	// Get user message, could be in the part or content
	session.In(lastUserMessage)

	return session, nil
}

func withFallbackProvider(provider string, assistant *types.AssistantConfig) string {
	if provider == "" {
		return assistant.Provider
	}
	return provider
}

func (c *Controller) getLLMModelConfig(ctx context.Context, owner, provider, model string) (*agent.LLMModelConfig, error) {
	client, err := c.providerManager.GetClient(ctx, &manager.GetClientRequest{
		Provider: provider,
		Owner:    owner,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	return &agent.LLMModelConfig{
		Client: client,
		Model:  model,
	}, nil
}

func (c *Controller) runAgentBlocking(ctx context.Context, req *runAgentRequest) (*openai.ChatCompletionResponse, error) {
	session, err := c.runAgent(ctx, req)
	if err != nil {
		return nil, err
	}

	var response string
	for {
		out := session.Out()

		if out.Type == agent.ResponseTypePartialText {
			response += out.Content
		}
		if out.Type == agent.ResponseTypeEnd {
			break
		}
	}

	vals, ok := oai.GetContextValues(ctx)
	if !ok {
		vals = &oai.ContextValues{}
	}

	return &openai.ChatCompletionResponse{
		ID:      vals.InteractionID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Choices: []openai.ChatCompletionChoice{
			{
				Message: openai.ChatCompletionMessage{
					Role:    openai.ChatMessageRoleAssistant,
					Content: response,
				},
			},
		},
	}, nil
}

func (c *Controller) runAgentStream(ctx context.Context, req *runAgentRequest) (*openai.ChatCompletionStream, error) {
	req.Options.Conversational = true
	session, err := c.runAgent(ctx, req)
	if err != nil {
		return nil, err
	}

	stream, writer, err := transport.NewOpenAIStreamingAdapter(req.Request)
	if err != nil {
		return nil, fmt.Errorf("failed to create openai streaming adapter: %w", err)
	}

	go func() {
		defer writer.Close()

		// We might start multiple thinking processes, so we need to keep track of them.
		// Provide <think> only on the first thinking process.
		// Provide </think> only on the last thinking process.
		var (
			thinkingProcesses int
		)

		for {
			out := session.Out()

			switch out.Type {
			case agent.ResponseTypeThinkingStart:
				thinkingProcesses++
				// If we are already thinking, don't send another thinking start
				if thinkingProcesses > 1 {
					continue
				}

				_ = transport.WriteChatCompletionStream(writer, &openai.ChatCompletionStreamResponse{
					Choices: []openai.ChatCompletionStreamChoice{
						{
							Delta: openai.ChatCompletionStreamChoiceDelta{Content: "<think>"},
						},
					},
				})
			case agent.ResponseTypeThinking:
				_ = transport.WriteChatCompletionStream(writer, &openai.ChatCompletionStreamResponse{
					Choices: []openai.ChatCompletionStreamChoice{
						{
							Delta: openai.ChatCompletionStreamChoiceDelta{Content: out.Content},
						},
					},
				})
			case agent.ResponseTypeThinkingEnd:
				// Only send the end if we are the last thinking process
				if thinkingProcesses == 1 {
					thinkingProcesses--
				}
				if thinkingProcesses > 0 {
					continue
				}

				_ = transport.WriteChatCompletionStream(writer, &openai.ChatCompletionStreamResponse{
					Choices: []openai.ChatCompletionStreamChoice{
						{
							Delta: openai.ChatCompletionStreamChoiceDelta{Content: "</think>"},
						},
					},
				})
			case agent.ResponseTypePartialText:
				// If we are receiving ResponseTypePartialText it means all thinking processes are done
				// and we can close the thinking processes (if we haven't already)
				if thinkingProcesses > 0 {
					// Write the end of the thinking process
					_ = transport.WriteChatCompletionStream(writer, &openai.ChatCompletionStreamResponse{
						Choices: []openai.ChatCompletionStreamChoice{
							{
								Delta: openai.ChatCompletionStreamChoiceDelta{Content: "</think>"},
							},
						},
					})
				}
				// Write the actual content
				_ = transport.WriteChatCompletionStream(writer, &openai.ChatCompletionStreamResponse{
					Choices: []openai.ChatCompletionStreamChoice{
						{
							Delta: openai.ChatCompletionStreamChoiceDelta{Content: out.Content},
						},
					},
				})

			case agent.ResponseTypeError:
				_ = transport.WriteChatCompletionStream(writer, &openai.ChatCompletionStreamResponse{
					Choices: []openai.ChatCompletionStreamChoice{
						{
							FinishReason: openai.FinishReasonNull,
						},
					},
				})
				// Close the stream
				writer.CloseWithError(fmt.Errorf("agent error: %s", out.Content))

			case agent.ResponseTypeEnd:
				// Write the final chunk with reason stop
				_ = transport.WriteChatCompletionStream(writer, &openai.ChatCompletionStreamResponse{
					Choices: []openai.ChatCompletionStreamChoice{
						{
							FinishReason: openai.FinishReasonStop,
						},
					},
				})
				return
			}
		}
	}()

	return stream, nil
}
