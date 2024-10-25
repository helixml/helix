package query

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"strings"

	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/sashabaranov/go-openai"
)

const (
	variationPrompt = `
Given the following prompt:

"{{ .Prompt}}"

Generate {{.NumVariations }} variations of this prompt with similar meanings, focusing on the most important keywords. Each variation should be on a new line. The variations should be:

1. A rephrased version of the original prompt
2. A more concise version of the original prompt
3. A slightly expanded version of the original prompt, adding some context
4. A version that focuses on a specific aspect of the original prompt
5. A version containing only the most important keywords from the original prompt

Ensure that each variation maintains the core meaning and intent of the original prompt.
	`
)

var variationTemplate = template.Must(template.New("variation").Parse(variationPrompt))

type variationsTemplateParams struct {
	Prompt        string
	NumVariations int
}

// createVariations creates a list of variations of the prompt with similar meaning, focusing on the most important keywords
func (q *Query) createVariations(ctx context.Context, prompt string, numVariations int) ([]string, error) {
	req, err := q.prepareVariationChatCompletionRequest(prompt, numVariations)
	if err != nil {
		return nil, fmt.Errorf("error preparing variation chat completion request: %w", err)
	}

	ctx = q.setContextAndStep(ctx, "", "", types.LLMCallStepCreateVariations)

	resp, err := q.apiClient.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("error creating chat completion: %w", err)
	}

	return q.getVariationsFromResponse(resp)
}

func (q *Query) setContextAndStep(ctx context.Context, sessionID, interactionID string, step types.LLMCallStep) context.Context {
	ctx = oai.SetContextValues(ctx, &oai.ContextValues{
		OwnerID:       "system",
		SessionID:     sessionID,
		InteractionID: interactionID,
	})

	return oai.SetStep(ctx, &oai.Step{
		Step: step,
	})
}

func (q *Query) prepareVariationChatCompletionRequest(prompt string, numVariations int) (openai.ChatCompletionRequest, error) {
	params := variationsTemplateParams{
		Prompt:        prompt,
		NumVariations: numVariations,
	}

	var buf bytes.Buffer

	err := variationTemplate.Execute(&buf, params)
	if err != nil {
		return openai.ChatCompletionRequest{}, fmt.Errorf("error executing variation template: %w", err)
	}

	return openai.ChatCompletionRequest{
		Model: q.model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: "You are a helpful assistant that generates variations of a given prompt."},
			{Role: openai.ChatMessageRoleUser, Content: buf.String()},
		},
	}, nil
}

func (q *Query) getVariationsFromResponse(resp openai.ChatCompletionResponse) ([]string, error) {
	var variations []string

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no choices found in the response")
	}

	content := resp.Choices[0].Message.Content

	lines := strings.Split(content, "\n")

	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			variations = append(variations, strings.TrimSpace(line))
		}
	}

	return variations, nil
}
