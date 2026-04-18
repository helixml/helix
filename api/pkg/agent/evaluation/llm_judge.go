package evaluation

import (
	"context"
	"fmt"
	"strings"

	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/sashabaranov/go-openai"
)

// controllerLLMJudge implements LLMJudge using the controller's ChatCompletion
type controllerLLMJudge struct {
	completer ChatCompleter
	user      *types.User
	model     string
	provider  string
	orgID     string
}

// NewControllerLLMJudge creates an LLM judge that uses the controller's chat completion
// with the specified model from the agent config
func NewControllerLLMJudge(ctrl ChatCompleter, user *types.User, app *types.App) LLMJudge {
	model, provider := pickJudgeModel(app)
	if model == "" {
		return nil
	}

	return &controllerLLMJudge{
		completer: ctrl,
		user:      user,
		model:     model,
		provider:  provider,
		orgID:     app.OrganizationID,
	}
}

// pickJudgeModel selects the best model for judging from the agent config.
// Prefers small generation model, then generation model, then the main model.
func pickJudgeModel(app *types.App) (model, provider string) {
	if len(app.Config.Helix.Assistants) == 0 {
		return "", ""
	}
	a := app.Config.Helix.Assistants[0]

	if a.SmallGenerationModel != "" {
		return a.SmallGenerationModel, a.SmallGenerationModelProvider
	}
	if a.GenerationModel != "" {
		return a.GenerationModel, a.GenerationModelProvider
	}
	if a.Model != "" {
		return a.Model, a.Provider
	}
	return "", ""
}

func (j *controllerLLMJudge) Judge(ctx context.Context, question, response, criteria string) (bool, string, error) {
	prompt := fmt.Sprintf(`You are an evaluation judge. Assess whether the following response meets the given criteria.

Response to evaluate:
%s

Criteria:
%s

Answer with exactly "PASS" or "FAIL" on the first line, followed by a brief explanation.`, response, criteria)

	req := openai.ChatCompletionRequest{
		Model: j.model,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: prompt,
			},
		},
		MaxTokens: 512,
	}

	opts := &controller.ChatCompletionOptions{
		OrganizationID: j.orgID,
		Provider:       j.provider,
	}

	resp, _, err := j.completer.ChatCompletion(ctx, j.user, req, opts)
	if err != nil {
		return false, "", fmt.Errorf("LLM judge call failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return false, "", fmt.Errorf("LLM judge returned no choices")
	}

	content := resp.Choices[0].Message.Content
	passed := strings.HasPrefix(strings.TrimSpace(strings.ToUpper(content)), "PASS")

	return passed, content, nil
}
