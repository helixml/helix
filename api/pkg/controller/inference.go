package controller

import (
	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/types"

	openai "github.com/lukemarsden/go-openai2"
	"github.com/rs/zerolog/log"
)

func (c *Controller) processTextInferenceSession(session *types.Session) {
	// TODO: ability to switch models/providers here

	chatReq, err := sessionToChatCompletion(session)
	if err != nil {
		log.Err(err).Msg("error converting session to chat completion")
		return
	}

}

func sessionToChatCompletion(session *types.Session) (*openai.ChatCompletionRequest, error) {
	var messages []openai.ChatCompletionMessage

	// Adjust length
	var interactions []*types.Interaction
	if len(session.Interactions) > 10 {
		first, err := data.GetFirstUserInteraction(session.Interactions)
		if err != nil {
			log.Err(err).Msg("error getting first user interaction")
		} else {
			interactions = append(interactions, first)
			interactions = append(interactions, data.GetLastInteractions(session, 10)...)
		}
	} else {
		interactions = session.Interactions
	}

	// Adding the system prompt first
	if session.Metadata.SystemPrompt != "" {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: session.Metadata.SystemPrompt,
		})
	}

	for _, interaction := range interactions {
		switch interaction.Creator {

		case types.CreatorTypeUser:
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleUser,
				Content: interaction.Message,
			})
		case types.CreatorTypeSystem:
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleAssistant,
				Content: interaction.Message,
			})
		case types.CreatorTypeTool:
			messages = append(messages, openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleUser,
				Content:    interaction.Message,
				ToolCalls:  interaction.ToolCalls,
				ToolCallID: interaction.ToolCallID,
			})
		}
	}

	var (
		responseFormat *openai.ChatCompletionResponseFormat
		tools          []openai.Tool
		toolChoice     any
	)

	// If the last interaction has response format, use it
	last, _ := data.GetLastSystemInteraction(interactions)
	if last != nil && last.ResponseFormat.Type == types.ResponseFormatTypeJSONObject {
		responseFormat = &openai.ChatCompletionResponseFormat{
			Type:   openai.ChatCompletionResponseFormatTypeJSONObject,
			Schema: last.ResponseFormat.Schema,
		}
	}

	if last != nil && len(last.Tools) > 0 {
		tools = last.Tools
		toolChoice = last.ToolChoice
	}

	// TODO: temperature, etc.

	return &openai.ChatCompletionRequest{
		Model:          string(session.ModelName),
		Stream:         session.Metadata.Stream,
		Messages:       messages,
		ResponseFormat: responseFormat,
		Tools:          tools,
		ToolChoice:     toolChoice,
	}, nil
}
