// Package agent provides the main Agent orchestrator, which uses LLM & Skills to process data.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/helixml/helix/api/pkg/agent/prompts"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/packages/param"
)

var ignErr *IgnorableError
var retErr *RetryableError

// Agent orchestrates calls to the LLM, uses Skills/Tools, and determines how to respond.
type Agent struct {
	prompt string
	skills []Skill
	logger *slog.Logger
}

// NewAgent creates an Agent by adding the prompt as a DeveloperMessage.
func NewAgent(prompt string, skills []Skill) *Agent {
	// Validate that all skills have both Description and SystemPrompt set
	for _, skill := range skills {
		if skill.Description == "" {
			panic(fmt.Sprintf("skill '%s' is missing a Description", skill.Name))
		}
		if skill.SystemPrompt == "" {
			panic(fmt.Sprintf("skill '%s' is missing a SystemPrompt", skill.Name))
		}
	}

	return &Agent{
		prompt: prompt,
		skills: skills,
		logger: slog.Default(),
	}
}

func (a *Agent) GetLogger() *slog.Logger {
	return a.logger
}

func (a *Agent) SetLogger(logger *slog.Logger) {
	a.logger = logger
}

func (a *Agent) GetSkill(name string) (*Skill, error) {
	for _, skill := range a.skills {
		if skill.Name == name {
			return &skill, nil
		}
	}
	return nil, fmt.Errorf("skill %s not found", name)
}

// summarizeMultipleToolResults summarizes results when multiple tools were called
func (a *Agent) summarizeMultipleToolResults(ctx context.Context, clonedMessages *MessageList, llm *LLM) (string, error) {
	clonedMessages.AddFirst("Craft a helpful answer to user's question based on the tool call results. Be concise and to the point.")
	params := openai.ChatCompletionNewParams{
		Messages: clonedMessages.All(),
		Model:    llm.GenerationModel,
		StreamOptions: openai.ChatCompletionStreamOptionsParam{
			IncludeUsage: param.Opt[bool]{Value: true},
		},
	}

	stream := llm.NewStreaming(ctx, params)
	defer stream.Close()

	var fullResponse strings.Builder

	for stream.Next() {
		chunk := stream.Current()

		if len(chunk.Choices) == 0 {
			a.logger.Error("No choices in chunk", "chunk", chunk)
			continue
		}

		if chunk.Choices[0].Delta.Content != "" {
			fullResponse.WriteString(chunk.Choices[0].Delta.Content)
		}
	}

	if stream.Err() != nil {
		a.logger.Error("Error streaming", "error", stream.Err())
		return "", stream.Err()
	}

	return fullResponse.String(), nil
}

func (a *Agent) StopTool() openai.ChatCompletionToolParam {
	return openai.ChatCompletionToolParam{
		Function: openai.FunctionDefinitionParam{
			Name: "stop",
			Description: param.Opt[string]{
				Value: `Request a stop after tool execution when one of the belwo is true
1. You have answer for user request
2. You have completed the task
3. You don't know what to do next with the given tools or information`,
			},
			Parameters: openai.FunctionParameters{
				"type": "object",
				"properties": map[string]interface{}{
					"callSummarizer": map[string]interface{}{
						"type":        "boolean",
						"description": "Sometimes, the final answer to user's question won't be in the last skill call result. This is unlikely but possible. If that's the case, set this to True. If the last skill call result answers the user's question, set this to False.",
					},
				},
				"required": []string{"callSummarizer"},
			},
		},
	}
}

// TODO - we probably need to have a custom made description for the tool that uses skill.description
func (a *Agent) ConvertSkillsToTools() []openai.ChatCompletionToolParam {
	tools := []openai.ChatCompletionToolParam{}
	for _, skill := range a.skills {
		tools = append(tools, openai.ChatCompletionToolParam{
			Function: openai.FunctionDefinitionParam{
				Name:        skill.Name,
				Description: param.Opt[string]{Value: skill.Description},
				Parameters:  openai.FunctionParameters{},
			},
		})
	}
	return tools
}

// decideNextAction gets the initial response from the LLM that decides whether to use skills or stop execution
func (a *Agent) decideNextAction(ctx context.Context, llm *LLM, clonedMessages *MessageList, memoryBlock *MemoryBlock) (*openai.ChatCompletion, error) {
	skillFunctions := make([]string, len(a.skills))
	for i, skill := range a.skills {
		skillFunctions[i] = skill.Name
	}

	systemPromptData := prompts.SkillSelectionPromptData{
		MainAgentSystemPrompt: a.prompt,
		MemoryBlocks:          memoryBlock.Parse(),
		SkillFunctions:        skillFunctions,
	}
	systemPrompt, err := prompts.SkillSelectionPrompt(systemPromptData)
	if err != nil {
		a.logger.Error("Error getting system prompt", "error", err)
		return nil, err
	}

	clonedMessages.AddFirst(systemPrompt)

	tools := []openai.ChatCompletionToolParam{}
	if len(a.ConvertSkillsToTools()) > 0 {
		tools = append([]openai.ChatCompletionToolParam{a.StopTool()}, a.ConvertSkillsToTools()...)
	}
	// TODO make it strict to call the tool when the openai sdk supports passing the option 'required'
	params := openai.ChatCompletionNewParams{
		Messages:   clonedMessages.All(),
		Model:      llm.GenerationModel,
		ToolChoice: openai.ChatCompletionToolChoiceOptionUnionParam{OfAuto: param.Opt[string]{Value: "auto"}},
		Tools:      tools,
	}

	completion, err := llm.New(ctx, params)
	if err != nil {
		a.logger.Error("Error getting initial response", "error", err, "params", params)
		return nil, err
	}

	if len(completion.Choices) == 0 {
		a.logger.Error("No completion choices")
		return completion, fmt.Errorf("no completion choices")
	}

	// Check for duplicate skills in tool calls
	if len(completion.Choices[0].Message.ToolCalls) > 1 {
		// Create a map to track seen skill names
		seenSkills := make(map[string]bool)
		var uniqueToolCalls []openai.ChatCompletionMessageToolCall

		// Keep only the first occurrence of each skill
		for _, toolCall := range completion.Choices[0].Message.ToolCalls {
			skillName := toolCall.Function.Name
			if !seenSkills[skillName] {
				seenSkills[skillName] = true
				uniqueToolCalls = append(uniqueToolCalls, toolCall)
			} else {
				a.logger.Warn("Removing duplicate skill from completion", "skill", skillName)
			}
		}

		// If duplicates were found, update the tool calls in the completion object
		if len(uniqueToolCalls) < len(completion.Choices[0].Message.ToolCalls) {
			completion.Choices[0].Message.ToolCalls = uniqueToolCalls
		}
	}

	return completion, nil
}

// handleLLMError handles errors from LLM API calls
func (a *Agent) handleLLMError(err error, outUserChannel chan Response) {
	content := "Error occurred!"
	a.logger.Error("Error streaming", "error", err)
	if strings.Contains(err.Error(), "ContentPolicyViolationError") {
		a.logger.Error("Content policy violation!", "error", err)
		content = "Content policy violation! If this was a mistake, please reach out to the support. Consecutive violations may result in a temporary/permanent ban."
	}
	outUserChannel <- Response{
		Content: content,
		Type:    ResponseTypeError,
	}
}

// sendThoughtsAboutSkills generates "thinking" messages to keep the user engaged while skills are processing
func (a *Agent) sendThoughtsAboutSkills(ctx context.Context, llm *LLM, messageHistory *MessageList, toolsToCall []openai.ChatCompletionMessageToolCall, outUserChannel chan Response) {
	if len(toolsToCall) == 0 {
		return
	}

	allSpecSystemPrompt := `You have these tools available for you to use. But first you need to send a response to the user about what you are planning to do. Make sure to strategize in details.
	
	Notes:
	- Do not mention about the tools or details about the tools like SQL, Python API etc. 
	- You can mention about what you are trying to achieve by mentioning what these tools enable you to do. For example, if an SQL table enable you to get latest whether, you can say "I am getting whether data" instead of "I'll look at the SQL database for whether data".
	- Make it very detailed.
	- Strictly do not answer the question. You are just planning.

	Here are the details about the tools:
	`
	for _, tool := range toolsToCall {
		skill, err := a.GetSkill(tool.Function.Name)
		if err != nil {
			a.logger.Error("Error getting skill", "error", err)
			continue
		}
		allSpecSystemPrompt += fmt.Sprintf("\n%s\n", skill.Spec())
	}

	outUserChannel <- Response{
		Type: ResponseTypeThinkingStart,
	}
	// making sure we send the end response when the agent is done
	defer func() {
		defer func() {
			if r := recover(); r != nil {
				a.logger.Error("Panic when sending end response", "error", r)
			}
		}()
		outUserChannel <- Response{
			Type: ResponseTypeThinkingEnd,
		}
	}()

	messageHistory.AddFirst(allSpecSystemPrompt)
	stream := llm.NewStreaming(ctx, openai.ChatCompletionNewParams{
		Messages: messageHistory.All(),
		Model:    llm.SmallGenerationModel,
	})
	defer stream.Close()

	for stream.Next() {
		chunk := stream.Current()
		if chunk.Choices[0].Delta.Content != "" {
			outUserChannel <- Response{
				Content: chunk.Choices[0].Delta.Content,
				Type:    ResponseTypeThinking,
			}
		}
	}

}

// sendThoughtsAboutSkills generates "thinking" messages to keep the user engaged while skills are processing
func (a *Agent) sendThoughtsAboutTools(ctx context.Context, llm *LLM, messageHistory *MessageList, toolsToCall []openai.ChatCompletionMessageToolCall, outUserChannel chan Response) {
	if len(toolsToCall) == 0 {
		return
	}

	systemPrompt := `Assistant has recommended to run a few functions. Now you need send status update to the user before you executing the request from assistant.
	
	Notes:
	- Do not mention tools/functions in details (like what it is going to do technically like using SQL, Python API etc)
	- You should mention about what you are assistant is trying to achieve by executing the functions.
	- You should not mention about the "assistant" at all. Only focus on what assistant has asked to do.
	`

	outUserChannel <- Response{
		Type: ResponseTypeThinkingStart,
	}
	// making sure we send the end response when the agent is done
	defer func() {
		defer func() {
			if r := recover(); r != nil {
				a.logger.Error("Panic when sending end response", "error", r)
			}
		}()
		outUserChannel <- Response{
			Type: ResponseTypeThinkingEnd,
		}
	}()

	messageHistory.AddFirst(systemPrompt)
	assistantMessage := "Execute below functions and get me the results so I can answer you better.\n\n"
	for _, tool := range toolsToCall {
		assistantMessage += fmt.Sprintf("Function Name: %s\nFunction Args: %s\n\n", tool.Function.Name, tool.Function.Arguments)
	}
	messageHistory.Add(AssistantMessage(assistantMessage))

	stream := llm.NewStreaming(ctx, openai.ChatCompletionNewParams{
		Messages: messageHistory.All(),
		Model:    llm.SmallGenerationModel,
	})
	defer stream.Close()

	for stream.Next() {
		chunk := stream.Current()
		if chunk.Choices[0].Delta.Content != "" {
			outUserChannel <- Response{
				Content: chunk.Choices[0].Delta.Content,
				Type:    ResponseTypeThinking,
			}
		}
	}

}

// runWithoutSkills handles the case when no skills are available by directly calling the LLM
func (a *Agent) runWithoutSkills(ctx context.Context, llm *LLM, messageHistory *MessageList, memoryBlock *MemoryBlock, outUserChannel chan Response) {
	// Create a system prompt using the NoSkillsPrompt function
	systemPromptData := prompts.NoSkillsPromptData{
		MainAgentSystemPrompt: a.prompt,
		MemoryBlocks:          memoryBlock.Parse(),
	}
	systemPrompt, err := prompts.NoSkillsPrompt(systemPromptData)
	if err != nil {
		a.logger.Error("Error getting system prompt", "error", err)
		a.handleLLMError(err, outUserChannel)
		return
	}

	// Clone the message history and add the system prompt
	clonedMessages := messageHistory.Clone()
	clonedMessages.AddFirst(systemPrompt)

	params := openai.ChatCompletionNewParams{
		Messages: clonedMessages.All(),
		Model:    llm.GenerationModel,
	}

	completion, err := llm.New(ctx, params)
	if err != nil {
		a.handleLLMError(err, outUserChannel)
		return
	}

	if len(completion.Choices) == 0 {
		a.logger.Error("No completion choices")
		a.handleLLMError(fmt.Errorf("no completion choices"), outUserChannel)
		return
	}

	outUserChannel <- Response{
		Content: completion.Choices[0].Message.Content,
		Type:    ResponseTypePartialText,
	}
}

// Run processes a user message through the LLM, executes any requested skills. It returns only after the agent is done.
// The intermediary messages are sent to the outUserChannel.
func (a *Agent) Run(ctx context.Context, meta Meta, llm *LLM, messageHistory *MessageList, memoryBlock *MemoryBlock, outUserChannel chan Response, isConversational bool) {
	if a.logger == nil {
		panic("logger is not set")
	}

	// Create a cancel function from the context
	ctx, cancel := context.WithCancel(ctx)

	// making sure we send the end response when the agent is done and cancel the context
	defer func() {
		defer func() {
			if r := recover(); r != nil {
				a.logger.Error("Panic when sending end response", "error", r)
			}
		}()
		// Cancel the context to stop any in-flight requests
		cancel()
		outUserChannel <- Response{
			Type: ResponseTypeEnd,
		}
	}()

	var finalSkillCallResults map[string]*openai.ChatCompletionToolMessageParam
	var hasStopTool bool
	var callSummarizer bool

	if len(a.skills) == 0 {
		// If no skills are available, use the runWithoutSkills function
		a.runWithoutSkills(ctx, llm, messageHistory.Clone(), memoryBlock, outUserChannel)
		return
	}

	for {
		completion, err := a.decideNextAction(ctx, llm, messageHistory.Clone(), memoryBlock)
		if err != nil {
			a.handleLLMError(err, outUserChannel)
			return
		}

		// If no tool calls were requested, we're done
		if completion.Choices[0].Message.ToolCalls == nil {
			break
		}

		// Separate stop tools from skill tools
		skillToolCalls := []openai.ChatCompletionMessageToolCall{}
		for _, toolCall := range completion.Choices[0].Message.ToolCalls {
			if toolCall.Function.Name == "stop" {
				hasStopTool = true
				// Parse the callSummarizer parameter from the stop tool
				var args map[string]interface{}
				if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
					a.logger.Error("Error parsing stop tool arguments", "error", err)
				} else {
					if val, ok := args["callSummarizer"].(bool); ok {
						callSummarizer = val
					}
				}
			} else {
				skillToolCalls = append(skillToolCalls, toolCall)
			}
		}

		// Execute all skill tools in the current response
		skillCallResults := make(map[string]*openai.ChatCompletionToolMessageParam)
		var wg sync.WaitGroup
		var mu sync.Mutex

		if isConversational {
			// sending fake thoughts to the user to keep the user engaged
			go a.sendThoughtsAboutSkills(ctx, llm, messageHistory.Clone(), skillToolCalls, outUserChannel)
		}

		for _, tool := range skillToolCalls {
			skill, err := a.GetSkill(tool.Function.Name)
			if err != nil {
				a.logger.Error("Error getting skill", "error", err)
				continue
			}

			wg.Add(1)
			go func(skill *Skill, toolID string) {
				defer wg.Done()
				// Clone the messages again so all goroutines get different message history
				result, err := a.SkillContextRunner(ctx, meta, messageHistory.Clone(), llm, outUserChannel, memoryBlock, skill, tool.ID, isConversational)
				if err != nil {
					a.logger.Error("Error running skill", "error", err)
					return
				}

				mu.Lock()
				skillCallResults[toolID] = result
				mu.Unlock()
			}(skill, tool.ID)
		}

		wg.Wait()

		// Add the completion message to history, but filter out the stop tool call
		messageToAdd := completion.Choices[0].Message
		if messageToAdd.ToolCalls != nil {
			filteredToolCalls := []openai.ChatCompletionMessageToolCall{}
			for _, toolCall := range messageToAdd.ToolCalls {
				if toolCall.Function.Name != "stop" {
					filteredToolCalls = append(filteredToolCalls, toolCall)
				}
			}
			// Only update and add the message if there are non-stop tool calls. We have this specific condition here because
			// we tinker with the tool calls to filter out one of the call.
			if len(filteredToolCalls) > 0 {
				messageToAdd.ToolCalls = filteredToolCalls
				messageHistory.Add(messageToAdd.ToParam())
			}
		} else {
			messageHistory.Add(messageToAdd.ToParam())
		}
		// Add tool results to message history
		for _, result := range skillCallResults {
			messageHistory.Add(openai.ChatCompletionMessageParamUnion{OfTool: result})
		}

		// Store results for final processing
		if len(skillToolCalls) > 0 {
			// Only update finalResults and lastCompletion if there were skill tool calls in this iteration
			finalSkillCallResults = skillCallResults
		}

		// If stop tool was called, break the loop
		if hasStopTool {
			break
		}
	}

	// if not conversational, rest of the code is not needed as that is for sending the final message to the user
	if !isConversational {
		return
	}

	// Handle final results based on the callSummarizer parameter from the stop tool or if multiple skills were called
	if callSummarizer || len(finalSkillCallResults) > 1 {
		// If callSummarizer is true, summarize the results
		summary, err := a.summarizeMultipleToolResults(ctx, messageHistory.Clone(), llm)
		if err != nil {
			a.handleLLMError(err, outUserChannel)
			return
		}
		outUserChannel <- Response{
			Content: summary,
			Type:    ResponseTypePartialText,
		}
		return
	} else if len(finalSkillCallResults) == 1 {
		// If callSummarizer is false, return the final skill result directly
		// Get the last skill result
		// Get keys from the map
		var lastResult *openai.ChatCompletionToolMessageParam
		for _, result := range finalSkillCallResults {
			lastResult = result
		}

		// Extract the text content using the existing GetMessageText function
		contentString, err := GetMessageText(openai.ChatCompletionMessageParamUnion{OfTool: lastResult})
		if err != nil {
			a.logger.Error("Error extracting content from tool result", "error", err)
			outUserChannel <- Response{
				Content: "Error processing the result.",
				Type:    ResponseTypeError,
			}
			return
		}

		outUserChannel <- Response{
			Content: contentString,
			Type:    ResponseTypePartialText,
		}
		return
	} else {
		// If there are no skill results, we return an error
		a.logger.Warn("No skill results available to return")
		outUserChannel <- Response{
			Content: "I encountered an error while processing the results.",
			Type:    ResponseTypeError,
		}
	}
}
