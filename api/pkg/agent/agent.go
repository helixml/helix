// Package agent provides the main Agent orchestrator, which uses LLM & Skills to process data.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"

	"github.com/helixml/helix/api/pkg/agent/prompts"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/types"

	pkg_errors "github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
	"github.com/tmc/langchaingo/jsonschema"
)

const defaultMaxIterations = 10

var ignErr *IgnorableError
var retErr *RetryableError

// Agent orchestrates calls to the LLM, uses Skills/Tools, and determines how to respond.
type Agent struct {
	prompt        string
	skills        []Skill
	emitter       StepInfoEmitter // Send information about various steps
	maxIterations int             // Max number of iterations to run the agent per loop
}

// NewAgent creates an Agent by adding the prompt as a DeveloperMessage.
func NewAgent(emitter StepInfoEmitter, prompt string, skills []Skill, maxIterations int) *Agent {
	// Validate that all skills have both Description and SystemPrompt set
	for _, skill := range skills {
		if skill.Description == "" {
			// TODO: return error
			panic(fmt.Sprintf("skill '%s' is missing a Description", skill.Name))
		}
		if skill.SystemPrompt == "" {
			// TODO: return error
			panic(fmt.Sprintf("skill '%s' is missing a SystemPrompt", skill.Name))
		}
	}

	// If not set or set to less than 2, use default. Otherwise agent will not be able to interpret tool results.
	if maxIterations <= 2 {
		maxIterations = defaultMaxIterations
	}

	return &Agent{
		prompt:        prompt,
		skills:        skills,
		emitter:       emitter,
		maxIterations: maxIterations,
	}
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

	model := llm.GenerationModel

	params := openai.ChatCompletionRequest{
		Messages: clonedMessages.All(),
		Model:    model.Model,
	}

	ctx = oai.SetStep(ctx, &oai.Step{
		Step: types.LLMCallStep("summarize_multiple_tool_results"),
	})

	stream, err := llm.NewStreaming(ctx, model, params)
	if err != nil {
		return "", err
	}
	defer stream.Close()

	var fullResponse strings.Builder

	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			log.Error().Err(err).Msg("Error streaming")
			return "", err
		}

		if len(chunk.Choices) == 0 {
			log.Debug().Any("chunk", chunk).Msg("No choices in chunk")
			continue
		}

		if chunk.Choices[0].Delta.Content != "" {
			fullResponse.WriteString(chunk.Choices[0].Delta.Content)
		}
	}

	return fullResponse.String(), nil
}

func (a *Agent) StopTool() openai.Tool {
	return openai.Tool{
		Type: openai.ToolTypeFunction,
		Function: &openai.FunctionDefinition{
			Name: "stop",
			Description: `Request a stop after tool execution when one of the below is true
1. You have answer for user request
2. You have completed the task
3. You don't know what to do next with the given tools or information`,
			Parameters: map[string]interface{}{
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
func (a *Agent) ConvertSkillsToTools() []openai.Tool {
	tools := []openai.Tool{}
	for _, skill := range a.skills {
		tools = append(tools, openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        skill.Name,
				Description: skill.Description,
				Parameters: jsonschema.Definition{
					Type:       jsonschema.Object,
					Required:   []string{""},
					Properties: map[string]jsonschema.Definition{},
				},
			},
		})
	}
	return tools
}

// decideNextAction gets the initial response from the LLM that decides whether to use skills or stop execution
func (a *Agent) decideNextAction(ctx context.Context, llm *LLM, clonedMessages *MessageList, memoryBlock *MemoryBlock) (*openai.ChatCompletionResponse, error) {
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
		log.Error().Err(err).Msg("Error getting system prompt")
		return nil, err
	}

	clonedMessages.AddFirst(systemPrompt)

	tools := []openai.Tool{}
	if len(a.ConvertSkillsToTools()) > 0 {
		tools = append([]openai.Tool{a.StopTool()}, a.ConvertSkillsToTools()...)
	}

	model := llm.GenerationModel

	// TODO make it strict to call the tool when the openai sdk supports passing the option 'required'
	params := openai.ChatCompletionRequest{
		Messages:   clonedMessages.All(),
		Model:      model.Model,
		ToolChoice: "auto",
		Tools:      tools,
	}

	ctx = oai.SetStep(ctx, &oai.Step{
		Step: types.LLMCallStep("decide_next_action"),
	})

	completion, err := llm.New(ctx, model, params)
	if err != nil {
		log.Error().Err(err).Interface("params", params).Msg("Error getting initial response")
		return nil, err
	}

	if len(completion.Choices) == 0 {
		log.Error().Msg("No completion choices")
		return &completion, fmt.Errorf("no completion choices")
	}

	// Check for duplicate skills in tool calls
	if len(completion.Choices[0].Message.ToolCalls) > 1 {
		// Create a map to track seen skill names
		seenSkills := make(map[string]bool)
		var uniqueToolCalls []openai.ToolCall

		// Keep only the first occurrence of each skill
		for _, toolCall := range completion.Choices[0].Message.ToolCalls {
			skillName := toolCall.Function.Name
			if !seenSkills[skillName] {
				seenSkills[skillName] = true
				uniqueToolCalls = append(uniqueToolCalls, toolCall)
			} else {
				log.Warn().Str("skill", skillName).Msg("Removing duplicate skill from completion")
			}
		}

		// If duplicates were found, update the tool calls in the completion object
		if len(uniqueToolCalls) < len(completion.Choices[0].Message.ToolCalls) {
			completion.Choices[0].Message.ToolCalls = uniqueToolCalls
		}
	}

	return &completion, nil
}

// handleLLMError handles errors from LLM API calls
func (a *Agent) handleLLMError(err error, outUserChannel chan Response) {
	content := err.Error()
	log.Error().Err(pkg_errors.WithStack(err)).Msg("Error streaming")
	if strings.Contains(err.Error(), "ContentPolicyViolationError") {
		log.Error().Err(err).Msg("Content policy violation!")
		content = "Content policy violation! If this was a mistake, please reach out to the support. Consecutive violations may result in a temporary/permanent ban."
	}
	outUserChannel <- Response{
		Content: content,
		Type:    ResponseTypeError,
	}
}

// sendThoughtsAboutSkills generates "thinking" messages to keep the user engaged while skills are processing
func (a *Agent) sendThoughtsAboutSkills(ctx context.Context, llm *LLM, messageHistory *MessageList, toolsToCall []openai.ToolCall, outUserChannel chan Response) {
	if len(toolsToCall) == 0 {
		return
	}

	allSpecSystemPrompt := `You have these tools available for you to use. But first you need to send a response to the user about what you are planning to do. Make sure to strategize in details.

	Notes:
	- Do not mention about the tools or details about the tools like API integrations, Python API etc. 
	- You can mention about what you are trying to achieve by mentioning what these tools enable you to do. For example, if an SQL table enable you to get latest whether, you can say "I am getting whether data" instead of "I'll look at the SQL database for whether data".
	- Make it very detailed.
	- Strictly do not answer the question. You are just planning.

	Here are the details about the tools:
	`
	for _, tool := range toolsToCall {
		skill, err := a.GetSkill(tool.Function.Name)
		if err != nil {
			log.Error().Err(err).Msg("Error getting skill")
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
				log.Error().Interface("error", r).Msg("Panic when sending end response")
			}
		}()
		outUserChannel <- Response{
			Type: ResponseTypeThinkingEnd,
		}
	}()

	messageHistory.AddFirst(allSpecSystemPrompt)

	ctx = oai.SetStep(ctx, &oai.Step{
		Step: types.LLMCallStep("send_thoughts_about_skills"),
	})

	model := llm.SmallGenerationModel

	stream, err := llm.NewStreaming(ctx, model, openai.ChatCompletionRequest{
		Messages: messageHistory.All(),
		Model:    model.Model,
	})
	if err != nil {
		log.Error().Err(err).Msg("Error creating streaming")
		return
	}

	defer stream.Close()

	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			log.Error().Err(err).Msg("Error streaming")
			return
		}
		if len(chunk.Choices) == 0 {
			log.Error().Interface("chunk", chunk).Msg("No choices in chunk")
			continue
		}
		if chunk.Choices[0].Delta.Content != "" {
			outUserChannel <- Response{
				Content: chunk.Choices[0].Delta.Content,
				Type:    ResponseTypeThinking,
			}
		}
	}
}

// sendThoughtsAboutTools generates "thinking" messages to keep the user engaged while skills are processing
func (a *Agent) sendThoughtsAboutTools(ctx context.Context, llm *LLM, messageHistory *MessageList, toolsToCall []openai.ToolCall, outUserChannel chan Response) {
	if len(toolsToCall) == 0 {
		return
	}

	systemPrompt := `Assistant has recommended to run a few functions. Now you need send status update to the user before you executing the request from assistant.
	
	Notes:
	- Do not mention tools/functions in details (like what it is going to do technically like using SQL, Python API etc)
	- You should not mention about what you as an assistant is trying to achieve by executing the functions.
	- You should not mention about the "assistant" at all. Only focus on what assistant has asked to do.
	`

	outUserChannel <- Response{
		Type: ResponseTypeThinkingStart,
	}
	// making sure we send the end response when the agent is done
	defer func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error().Any("error", r).Msg("Panic when sending end response")
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

	ctx = oai.SetStep(ctx, &oai.Step{
		Step: types.LLMCallStep("send_thoughts_about_tools"),
	})

	model := llm.SmallGenerationModel

	stream, err := llm.NewStreaming(ctx, model, openai.ChatCompletionRequest{
		Messages: messageHistory.All(),
		Model:    model.Model,
	})
	if err != nil {
		log.Error().Err(err).Msg("Error creating streaming")
		return
	}
	defer stream.Close()

	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			log.Error().Err(err).Msg("Error streaming")
			return
		}
		if len(chunk.Choices) == 0 {
			log.Error().Any("chunk", chunk).Msg("No choices in chunk")
			continue
		}
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
		log.Error().Err(err).Msg("Error getting system prompt")
		a.handleLLMError(err, outUserChannel)
		return
	}

	// Clone the message history and add the system prompt
	clonedMessages := messageHistory.Clone()
	clonedMessages.AddFirst(systemPrompt)

	model := llm.GenerationModel

	params := openai.ChatCompletionRequest{
		Messages: clonedMessages.All(),
		Model:    model.Model,
	}

	ctx = oai.SetStep(ctx, &oai.Step{
		Step: types.LLMCallStep("run_without_skills"),
	})

	completion, err := llm.New(ctx, model, params)
	if err != nil {
		a.handleLLMError(err, outUserChannel)
		return
	}

	if len(completion.Choices) == 0 {
		log.Error().Msg("No completion choices")
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
	// Create a cancel function from the context
	ctx, cancel := context.WithCancel(ctx)

	// making sure we send the end response when the agent is done and cancel the context
	defer func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error().Interface("error", r).Msg("Panic when sending end response")
			}
		}()
		// Cancel the context to stop any in-flight requests
		cancel()
		outUserChannel <- Response{
			Type: ResponseTypeEnd,
		}
	}()

	var (
		finalSkillCallResults map[string]*openai.ChatCompletionMessage
		finalCompletion       *openai.ChatCompletionResponse
		hasStopTool           bool
		callSummarizer        bool
	)

	if len(a.skills) == 0 {
		// If no skills are available, use the runWithoutSkills function
		a.runWithoutSkills(ctx, llm, messageHistory.Clone(), memoryBlock, outUserChannel)
		return
	}

	iterationNumber := 0

	for {
		if iterationNumber >= a.maxIterations {
			outUserChannel <- Response{
				Content: fmt.Sprintf("max iterations (%d) reached. Try adjusting model, system prompt, reasoning effort or max iterations.",
					a.maxIterations,
				),
				Type: ResponseTypeError,
			}
			return
		}

		iterationNumber++

		completion, err := a.decideNextAction(ctx, llm, messageHistory.Clone(), memoryBlock)
		if err != nil {
			a.handleLLMError(err, outUserChannel)
			return
		}

		finalCompletion = completion

		// If no tool calls were requested, we're done
		if len(completion.Choices[0].Message.ToolCalls) == 0 {
			break
		}

		// Separate stop tools from skill tools
		skillToolCalls := []openai.ToolCall{}
		for _, toolCall := range completion.Choices[0].Message.ToolCalls {
			if toolCall.Function.Name == "stop" {
				hasStopTool = true
				// Parse the callSummarizer parameter from the stop tool
				var args map[string]interface{}
				if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
					log.Error().Err(err).Msg("Error parsing stop tool arguments")
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
		skillCallResults := make(map[string]*openai.ChatCompletionMessage)
		var wg sync.WaitGroup
		var mu sync.Mutex

		if isConversational {
			// sending fake thoughts to the user to keep the user engaged
			go a.sendThoughtsAboutSkills(ctx, llm, messageHistory.Clone(), skillToolCalls, outUserChannel)
		}

		for _, tool := range skillToolCalls {
			skill, err := a.GetSkill(tool.Function.Name)
			if err != nil {
				log.Error().Err(err).Msg("Error getting skill")
				continue
			}

			wg.Add(1)
			go func(skill *Skill, toolID string) {
				defer wg.Done()
				// Clone the messages again so all goroutines get different message history
				result, err := a.SkillContextRunner(ctx, meta, messageHistory.Clone(), llm, outUserChannel, memoryBlock, skill, tool.ID, isConversational)
				if err != nil {
					log.Error().Err(err).Msg("Error running skill")
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
			filteredToolCalls := []openai.ToolCall{}
			for _, toolCall := range messageToAdd.ToolCalls {
				if toolCall.Function.Name != "stop" {
					filteredToolCalls = append(filteredToolCalls, toolCall)
				}
			}
			// Only update and add the message if there are non-stop tool calls. We have this specific condition here because
			// we tinker with the tool calls to filter out one of the call.
			if len(filteredToolCalls) > 0 {
				messageToAdd.ToolCalls = filteredToolCalls
				messageHistory.Add(&messageToAdd)
			}
		} else {
			messageHistory.Add(&messageToAdd)
		}
		// Add tool results to message history
		for _, result := range skillCallResults {
			messageHistory.Add(result)
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
		var lastResult *openai.ChatCompletionMessage
		for _, result := range finalSkillCallResults {
			lastResult = result
		}

		// Extract the text content using the existing GetMessageText function
		contentString, err := types.GetMessageText(lastResult)
		if err != nil {
			log.Error().Err(err).Msg("Error extracting content from tool result")
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
	}

	// If we have the response in the finalCompletion, we return it
	if finalCompletion != nil {
		outUserChannel <- Response{
			Content: finalCompletion.Choices[0].Message.Content,
			Type:    ResponseTypePartialText,
		}
		return
	}

	// If there are no skill results, we return an error
	log.Warn().Msg("No skill results available to return")
	outUserChannel <- Response{
		Content: "I encountered an error while processing the results.",
		Type:    ResponseTypeError,
	}
}

var sanitizeToolNameRegex = regexp.MustCompile("[^a-zA-Z0-9]+")

// OpenAI tool names can only contain alphanumeric characters and underscores, otherwise you will get an error:
// Invalid 'tools[1].function.name': string does not match pattern. Expected a string that matches the pattern '^[a-zA-Z0-9_-]+$'.
func SanitizeToolName(name string) string {
	// Replace all non-alphanumeric characters with underscores
	return sanitizeToolNameRegex.ReplaceAllString(name, "_")
}
