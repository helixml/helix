package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/agent/prompts"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/types"

	"io"

	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

func MessageWhenToolError(errorString string, toolCallID string) *openai.ChatCompletionMessage {
	// return openai.ToolMessage("Error occurred while running. Do not retry", toolCallID)
	return &openai.ChatCompletionMessage{
		Role:       openai.ChatMessageRoleTool,
		ToolCallID: toolCallID,
		Content:    fmt.Sprintf("Error '%s' occurred while running. Do not retry", errorString),
	}
}

func MessageWhenToolErrorWithRetry(errorString string, toolCallID string) *openai.ChatCompletionMessage {
	return &openai.ChatCompletionMessage{
		Role:       openai.ChatMessageRoleTool,
		ToolCallID: toolCallID,
		Content:    fmt.Sprintf("Error: %s.\nRetry", errorString),
	}
}

func (a *Agent) SkillContextRunner(ctx context.Context, meta Meta, messageHistory *MessageList, llm *LLM, outChan chan Response, memoryBlock *MemoryBlock, skill *Skill, skillToolCallID string) (*openai.ChatCompletionMessage, error) {
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

		ctx = oai.SetStep(ctx, &oai.Step{
			Step: types.LLMCallStep(fmt.Sprintf("skill_context_runner (%s | iteration %d)", skill.Name, iterationNumber)),
		})

		// Check if we should expect tool calls based on available tools
		expectsToolCalls := len(skill.GetTools()) > 0
		var toolsToCall []openai.ToolCall

		// Always use streaming to show the reasoning process, but handle tool calls appropriately
		// DEBUG: Log exactly what we're sending to LLM to catch empty content
		log.Debug().
			Int("message_count", len(params.Messages)).
			Str("model", modelToUse.Model).
			Str("skill_name", skill.Name).
			Int("iteration", iterationNumber).
			Msg("🔍 DEBUG: About to call LLM streaming in SkillContextRunner")

		for i, msg := range params.Messages {
			log.Debug().
				Int("msg_index", i).
				Str("role", msg.Role).
				Str("content", msg.Content).
				Bool("content_empty", msg.Content == "").
				Int("multi_content_parts", len(msg.MultiContent)).
				Int("tool_calls", len(msg.ToolCalls)).
				Str("skill_name", skill.Name).
				Msg("🔍 DEBUG: Message being sent to LLM in SkillContextRunner")

			if msg.Content == "" && len(msg.MultiContent) == 0 && len(msg.ToolCalls) == 0 {
				log.Error().
					Int("msg_index", i).
					Str("role", msg.Role).
					Str("skill_name", skill.Name).
					Msg("🚨 FOUND EMPTY MESSAGE in SkillContextRunner - This will cause 'inputs cannot be empty' error!")
			}
		}

		stream, err := llm.NewStreaming(ctx, modelToUse, params)
		if err != nil {
			log.Error().Err(err).Msg("Error calling LLM while running skill")
			return MessageWhenToolErrorWithRetry("Network error", skillToolCallID), err
		}
		defer stream.Close()

		var fullContent strings.Builder
		var hasContent bool
		var completion openai.ChatCompletionMessage

		for {
			chunk, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				log.Error().Err(err).Msg("Error streaming")
				return MessageWhenToolErrorWithRetry("Network error", skillToolCallID), err
			}

			if len(chunk.Choices) == 0 {
				log.Debug().Any("chunk", chunk).Msg("No choices in chunk")
				continue
			}

			choice := chunk.Choices[0]

			// Stream content to user if available
			if choice.Delta.Content != "" {
				content := choice.Delta.Content
				fullContent.WriteString(content)
				hasContent = true

				// Send partial text to user immediately
				outChan <- Response{
					Content: content,
					Type:    ResponseTypePartialText,
				}
			}

			// Collect tool calls as they come in (if tools are expected)
			if expectsToolCalls && choice.Delta.ToolCalls != nil {
				for _, toolCall := range choice.Delta.ToolCalls {
					// Handle nil index case
					if toolCall.Index == nil {
						continue
					}
					index := *toolCall.Index

					// Extend toolsToCall array if needed
					for len(toolsToCall) <= index {
						toolsToCall = append(toolsToCall, openai.ToolCall{})
					}

					// Append to existing tool call at this index
					if toolCall.ID != "" {
						toolsToCall[index].ID = toolCall.ID
					}
					if toolCall.Type != "" {
						toolsToCall[index].Type = toolCall.Type
					}
					if toolCall.Function.Name != "" {
						toolsToCall[index].Function.Name = toolCall.Function.Name
					}
					if toolCall.Function.Arguments != "" {
						toolsToCall[index].Function.Arguments += toolCall.Function.Arguments
					}
				}
			}

			// Build the completion message from the final chunk
			if choice.FinishReason != "" {
				completion = openai.ChatCompletionMessage{
					Role:      openai.ChatMessageRoleAssistant,
					Content:   fullContent.String(),
					ToolCalls: toolsToCall,
				}
			}
		}

		// Add the complete message to history for context
		messageHistory.Add(&completion)

		// Log the completion for debugging
		log.Debug().
			Interface("completion", completion).
			Str("model", modelToUse.Model).
			Msg("Received completion from LLM")

		// Check what to do next based on tool calls and content
		if expectsToolCalls && len(toolsToCall) == 0 {
			// If we expected tool calls but got none, check if we have valid content
			if hasContent && fullContent.String() != "" {
				log.Debug().
					Str("content", fullContent.String()).
					Msg("Received direct response from LLM")
				break
			}
			// If no content and no tool calls, this is an error
			log.Error().Msg("Received empty response with no tool calls")
			return &openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleTool,
				Content:    "Error: The skill execution did not produce a valid response",
				ToolCallID: skillToolCallID,
			}, nil
		} else if !expectsToolCalls {
			// No tools expected, so we're done if we have content
			if !hasContent {
				log.Error().Msg("Received empty response with no content")
				return &openai.ChatCompletionMessage{
					Role:       openai.ChatMessageRoleTool,
					Content:    "Error: The skill execution did not produce a valid response",
					ToolCallID: skillToolCallID,
				}, nil
			}
			// We've completed with streaming, so break the loop
			break
		}

		// If we have tool calls to execute, continue with the tool execution logic
		if len(toolsToCall) > 0 {
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
					arguments := map[string]any{}
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
						messageHistory.Add(MessageWhenToolError(result.err.Error(), result.toolCall.ID))
					case errors.As(result.err, &retErr):
						messageHistory.Add(MessageWhenToolErrorWithRetry(result.err.Error(), skillToolCallID))
					default:
						messageHistory.Add(MessageWhenToolError(result.err.Error(), result.toolCall.ID))
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
		Role: openai.ChatMessageRoleTool,
		// Content:    "Error: The skill execution did not produce a valid response",
		MultiContent: []openai.ChatMessagePart{
			{
				Type: openai.ChatMessagePartTypeText,
				Text: "Error: The skill execution did not produce a valid response",
			},
		},
		ToolCallID: skillToolCallID,
	}, nil
}

func (a *Agent) SkillDirectRunner(ctx context.Context, meta Meta, skill *Skill, toolCall openai.ToolCall) (*openai.ChatCompletionMessage, error) {
	if len(skill.Tools) == 0 {
		return nil, fmt.Errorf("skill %s has no tools", skill.Name)
	}

	// If we have more than one tool in a direct skill - not supported
	if len(skill.Tools) > 1 {
		return nil, fmt.Errorf("skill %s has more than one tool, direct skills are not supported", skill.Name)
	}

	log.Info().Str("skill", skill.Name).Str("arguments", toolCall.Function.Arguments).Msg("Running skill")

	tool := skill.Tools[0]

	arguments := map[string]any{}
	err := json.Unmarshal([]byte(toolCall.Function.Arguments), &arguments)
	if err != nil {
		log.Error().Err(err).Msg("Error unmarshaling tool arguments")
		return &openai.ChatCompletionMessage{
			Role:       openai.ChatMessageRoleTool,
			Content:    fmt.Sprintf("Error: failed to unmarshal tool arguments: %s", err.Error()),
			ToolCallID: toolCall.ID,
		}, nil
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

	if err != nil {
		return MessageWhenToolError(err.Error(), toolCall.ID), nil
	}

	return &openai.ChatCompletionMessage{
		Role:       openai.ChatMessageRoleTool,
		Content:    output,
		ToolCallID: toolCall.ID,
	}, nil
}
