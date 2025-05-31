package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/agent/prompts"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

func MessageWhenToolError(toolCallID string) *openai.ChatCompletionMessage {
	// return openai.ToolMessage("Error occurred while running. Do not retry", toolCallID)
	return &openai.ChatCompletionMessage{
		ToolCallID: toolCallID,
		Content:    "Error occurred while running. Do not retry",
	}
}

func MessageWhenToolErrorWithRetry(errorString string, toolCallID string) *openai.ChatCompletionMessage {
	return &openai.ChatCompletionMessage{
		ToolCallID: toolCallID,
		Content:    fmt.Sprintf("Error: %s.\nRetry", errorString),
	}
}

func (a *Agent) SkillContextRunner(ctx context.Context, meta Meta, messageHistory *MessageList, llm *LLM, outChan chan Response, memoryBlock *MemoryBlock, skill *Skill, skillToolCallID string, isConversational bool) (*openai.ChatCompletionMessage, error) {
	log.Info().Str("skill", skill.Name).Msg("Running skill")

	promptData := prompts.SkillContextRunnerPromptData{
		MainAgentSystemPrompt: a.prompt,
		SkillSystemPrompt:     skill.SystemPrompt,
		MemoryBlocks:          memoryBlock.Parse(),
	}
	systemPrompt, err := prompts.SkillContextRunnerPrompt(promptData)
	if err != nil {
		log.Error().Err(err).Msg("Error getting system prompt")
		return nil, err
	}
	messageHistory.AddFirst(systemPrompt)

	isFirstIteration := true
	iterationNumber := 0

	for {
		if iterationNumber >= a.maxIterations {
			return &openai.ChatCompletionMessage{
				Role: openai.ChatMessageRoleTool,
				Content: fmt.Sprintf("Error: max iterations (%d) reached for skill '%s'. Try adjusting model, reasoning effort or max iterations.",
					a.maxIterations, skill.Name,
				),
				ToolCallID: skillToolCallID,
			}, nil
		}

		iterationNumber++

		modelToUse := llm.SmallReasoningModel
		reasoningEffort := llm.SmallReasoningModel.ReasoningEffort

		if isFirstIteration {
			// First iteration is when the main planning happens - use the bigger model.
			modelToUse = llm.ReasoningModel
			isFirstIteration = false
			reasoningEffort = llm.ReasoningModel.ReasoningEffort
		}

		params := openai.ChatCompletionRequest{
			Messages:        messageHistory.All(),
			Model:           modelToUse.Model,
			ReasoningEffort: reasoningEffort,
		}

		log.Info().
			Str("skill", skill.Name).
			Interface("tools", skill.Tools).
			Int("iteration", iterationNumber).
			Str("reasoning_effort", reasoningEffort).
			Str("model", modelToUse.Model).
			Msg("Running skill")

		if len(skill.GetTools()) > 0 {
			params.Tools = skill.GetTools()
		}

		// we need this because we need to send thoughts to the user. The thoughts sending go routine
		// doesn't get the tool calls from here tool calls but instead as an assistant message
		messageHistoryBeforeLLMCall := messageHistory.Clone()

		ctx = oai.SetStep(ctx, &oai.Step{
			Step: types.LLMCallStep(fmt.Sprintf("skill_context_runner (%s | iteration %d)", skill.Name, iterationNumber)),
		})

		completion, err := llm.New(ctx, modelToUse, params)
		if err != nil {
			log.Error().Err(err).Msg("Error calling LLM while running skill")
			return MessageWhenToolErrorWithRetry("Network error", skillToolCallID), err
		}
		messageHistory.Add(&completion.Choices[0].Message)

		// Check if both tool call and content are non-empty
		bothToolCallAndContent := completion.Choices[0].Message.ToolCalls != nil && completion.Choices[0].Message.Content != ""
		if bothToolCallAndContent {
			log.Error().Interface("message", completion.Choices[0].Message).Msg("Expectation is that tool call and content shouldn't both be non-empty")
		}

		// if there is no tool call, break
		if completion.Choices[0].Message.ToolCalls == nil {
			break
		}
		toolsToCall := completion.Choices[0].Message.ToolCalls

		if isConversational {
			// sending fake thoughts to the user to keep the user engaged
			go a.sendThoughtsAboutTools(ctx, llm, messageHistoryBeforeLLMCall, toolsToCall, outChan)
		}

		// Create a wait group to wait for all tool executions to complete
		var wg sync.WaitGroup
		// Create a channel to collect results from goroutines
		resultChan := make(chan struct {
			toolCall *openai.ToolCall
			output   string
			err      error
		}, len(toolsToCall))

		for _, toolCall := range toolsToCall {
			wg.Add(1)

			go func(toolCall *openai.ToolCall) {
				defer wg.Done()

				tool, err := skill.GetTool(toolCall.Function.Name)
				if err != nil {
					log.Error().
						Str("tool", toolCall.Function.Name).
						Str("skill", skill.Name).
						Err(err).
						Msg("Error getting tool")

					resultChan <- struct {
						toolCall *openai.ToolCall
						output   string
						err      error
					}{toolCall, "", err}
					return
				}

				if tool.StatusMessage() != "" {
					outChan <- Response{
						Content: tool.StatusMessage(),
						Type:    ResponseTypeStatus,
					}
				}

				log.Info().Str("tool", tool.Name()).Str("arguments", toolCall.Function.Arguments).Msg("Tool")
				arguments := map[string]interface{}{}
				err = json.Unmarshal([]byte(toolCall.Function.Arguments), &arguments)
				if err != nil {
					log.Error().Err(err).Msg("Error unmarshaling tool arguments")
					resultChan <- struct {
						toolCall *openai.ToolCall
						output   string
						err      error
					}{toolCall, "", err}
					return
				}

				startTime := time.Now()

				output, err := tool.Execute(ctx, meta, arguments)

				stepInfo := &types.StepInfo{
					Created:    startTime,
					Name:       tool.Name(),
					Icon:       tool.Icon(),
					Type:       types.StepInfoTypeToolUse,
					Message:    output,
					Details:    types.StepInfoDetails{Arguments: arguments},
					DurationMs: time.Since(startTime).Milliseconds(),
				}

				// Add error to the step info if it exists
				if err != nil {
					stepInfo.Error = err.Error()
				}

				// Instrument the output
				_ = a.emitter.EmitStepInfo(ctx, stepInfo)

				resultChan <- struct {
					toolCall *openai.ToolCall
					output   string
					err      error
				}{toolCall, output, err}
			}(&toolCall)
		}

		// Start a goroutine to close the result channel when all tools are done
		go func() {
			wg.Wait()
			close(resultChan)
		}()

		// Process results as they come in
		for result := range resultChan {
			if result.err != nil {
				log.Error().Err(result.err).Msg("Error executing tool")
				switch {
				case errors.As(result.err, &ignErr):
					messageHistory.Add(MessageWhenToolError(result.toolCall.ID))
				case errors.As(result.err, &retErr):
					messageHistory.Add(MessageWhenToolErrorWithRetry(result.err.Error(), skillToolCallID))
				default:
					messageHistory.Add(MessageWhenToolError(result.toolCall.ID))
				}
				continue
			}

			messageHistory.Add(&openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleTool,
				Content:    result.output,
				ToolCallID: result.toolCall.ID,
			})
		}
	}
	allMessages := messageHistory.All()
	lastMessage := allMessages[len(allMessages)-1]

	// If it's a ChatCompletionMessage, convert it to a tool message
	if lastMessage.Role == openai.ChatMessageRoleAssistant {
		if lastMessage.Content == "" {
			return &openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleTool,
				Content:    "Error: The skill execution did not produce a valid response",
				ToolCallID: skillToolCallID,
			}, nil
		}
		return &openai.ChatCompletionMessage{
			Role:       openai.ChatMessageRoleTool,
			Content:    lastMessage.Content,
			ToolCallID: skillToolCallID,
		}, nil
	}

	log.Error().Str("type", fmt.Sprintf("%T", lastMessage)).Msg("Unexpected message type in SkillContextRunner result")

	return &openai.ChatCompletionMessage{
		Role:       openai.ChatMessageRoleTool,
		Content:    "Error: The skill execution did not produce a valid response",
		ToolCallID: skillToolCallID,
	}, nil
}
