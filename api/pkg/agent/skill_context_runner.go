package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/helixml/helix/api/pkg/agent/prompts"
	// "github.com/openai/openai-go"
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
	a.logger.Info("Running skill", "skill", skill.Name)

	promptData := prompts.SkillContextRunnerPromptData{
		MainAgentSystemPrompt: a.prompt,
		SkillSystemPrompt:     skill.SystemPrompt,
		MemoryBlocks:          memoryBlock.Parse(),
	}
	systemPrompt, err := prompts.SkillContextRunnerPrompt(promptData)
	if err != nil {
		a.logger.Error("Error getting system prompt", "error", err)
		return nil, err
	}
	messageHistory.AddFirst(systemPrompt)

	isFirstIteration := true
	for {
		// First iteration is when the main planning happens - use the bigger model.
		modelToUse := llm.SmallReasoningModel
		if isFirstIteration {
			modelToUse = llm.ReasoningModel
			isFirstIteration = false
		}

		params := openai.ChatCompletionRequest{
			Messages:        messageHistory.All(),
			Model:           modelToUse,
			ReasoningEffort: "high",
		}
		a.logger.Info("Running skill", "skill", skill.Name, "tools", skill.Tools)
		if len(skill.GetTools()) > 0 {
			params.Tools = skill.GetTools()
		}

		// we need this because we need to send thoughts to the user. The thoughts sending go routine
		// doesn't get the tool calls from here tool calls but instead as an assistant message
		messageHistoryBeforeLLMCall := messageHistory.Clone()

		completion, err := llm.New(ctx, params)
		if err != nil {
			a.logger.Error("Error calling LLM while running skill", "error", err)
			return MessageWhenToolErrorWithRetry("Network error", skillToolCallID), err
		}
		messageHistory.Add(&completion.Choices[0].Message)

		// Check if both tool call and content are non-empty
		bothToolCallAndContent := completion.Choices[0].Message.ToolCalls != nil && completion.Choices[0].Message.Content != ""
		if bothToolCallAndContent {
			a.logger.Error("Expectation is that tool call and content shouldn't both be non-empty", "message", completion.Choices[0].Message)
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
					a.logger.Error("Error getting tool", "error", err)
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

				a.logger.Info("Tool", "tool", tool.Name(), "arguments", toolCall.Function.Arguments)
				arguments := map[string]interface{}{}
				err = json.Unmarshal([]byte(toolCall.Function.Arguments), &arguments)
				if err != nil {
					a.logger.Error("Error unmarshalling tool arguments", "error", err)
					resultChan <- struct {
						toolCall *openai.ToolCall
						output   string
						err      error
					}{toolCall, "", err}
					return
				}

				output, err := tool.Execute(ctx, meta, arguments)
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
				a.logger.Error("Error executing tool", "error", result.err)
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
	} else {
		a.logger.Error("Unexpected message type in SkillContextRunner result", "type", fmt.Sprintf("%T", lastMessage))
		return &openai.ChatCompletionMessage{
			Role:       openai.ChatMessageRoleTool,
			Content:    "Error: The skill execution did not produce a valid response",
			ToolCallID: skillToolCallID,
		}, nil
	}
}
