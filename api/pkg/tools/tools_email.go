package tools

import (
	"context"
	"fmt"
	"html/template"
	"strings"

	"github.com/helixml/helix/api/pkg/notification/email"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/lukemarsden/go-openai2"
)

func (c *ChainStrategy) RunEmailAction(ctx context.Context, tool *types.Tool, history []*types.ToolHistoryMessage, currentMessage, action string) (*RunActionResponse, error) {
	// Generate email content
	emailContent, err := c.generateEmailContent(ctx, tool, history, currentMessage)
	if err != nil {
		return nil, fmt.Errorf("failed to generate email content: %w", err)
	}

	// Generate email title
	emailTitle, err := c.generateEmailTitle(ctx, tool, history, currentMessage)
	if err != nil {
		return nil, fmt.Errorf("failed to generate email title: %w", err)
	}

	// Get user's email address from the database
	userEmail, err := c.getUserEmail(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get user's email address: %w", err)
	}

	// Send the email
	err = c.sendEmail(ctx, userEmail, emailTitle, emailContent)
	if err != nil {
		return nil, fmt.Errorf("failed to send email: %w", err)
	}

	return &RunActionResponse{
		Message: fmt.Sprintf("Email sent successfully to your email %s with title: %s", userEmail, emailTitle),
	}, nil
}

func (c *ChainStrategy) generateEmailContent(ctx context.Context, tool *types.Tool, history []*types.ToolHistoryMessage, currentMessage string) (string, error) {
	systemPrompt, err := c.getEmailSystemPrompt(ctx, tool, history, currentMessage)
	if err != nil {
		return "", err
	}

	userPrompt, err := c.getEmailUserPrompt(ctx, tool, history, currentMessage)
	if err != nil {
		return "", err
	}
	messages := []openai.ChatCompletionMessage{
		systemPrompt,
		userPrompt,
	}

	req := openai.ChatCompletionRequest{
		Stream:   false,
		Model:    c.cfg.Tools.Model,
		Messages: messages,
	}

	resp, err := c.apiClient.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response from inference API")
	}

	return resp.Choices[0].Message.Content, nil
}

func (c *ChainStrategy) getEmailSystemPrompt(ctx context.Context, tool *types.Tool, history []*types.ToolHistoryMessage, currentMessage string) (openai.ChatCompletionMessage, error) {
	systemPrompt := openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleSystem,
		Content: emailSystemPrompt,
	}

	return systemPrompt, nil
}

func (c *ChainStrategy) getEmailUserPrompt(ctx context.Context, tool *types.Tool, history []*types.ToolHistoryMessage, currentMessage string) (openai.ChatCompletionMessage, error) {
	tmpl, err := template.New("email_content").Parse(emailUserPrompt)
	if err != nil {
		return openai.ChatCompletionMessage{}, err
	}

	// Render template
	var sb strings.Builder
	err = tmpl.Execute(&sb, struct {
		Schema       string
		Message      string
		Interactions []*types.ToolHistoryMessage
	}{
		Message:      currentMessage,
		Interactions: history,
	})

	if err != nil {
		return openai.ChatCompletionMessage{}, err
	}

	userPrompt := openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: sb.String(),
	}

	return userPrompt, nil
}

func (c *ChainStrategy) generateEmailTitle(ctx context.Context, tool *types.Tool, history []*types.ToolHistoryMessage, currentMessage string) (string, error) {
	// Implement LLM call to generate email title
	// ...

	return "Helix: TODO", nil
}

func (c *ChainStrategy) getUserEmail(ctx context.Context) (string, error) {
	vals, ok := oai.GetContextValues(ctx)
	if !ok {
		return "", fmt.Errorf("context values not found")
	}

	user, err := c.authenticator.GetUserByID(ctx, vals.OwnerID)
	if err != nil {
		return "", fmt.Errorf("failed to get user: %w", err)
	}

	return user.Email, nil
}

func (c *ChainStrategy) sendEmail(ctx context.Context, to, subject, body string) error {
	client := email.New(c.cfg)

	return client.Send(ctx, to, subject, body)
}

const emailSystemPrompt = `You are an email assistant that can help the user to write an email. User might ask you to summarize content into an email friendly format.`

const emailUserPrompt = `
Conversation so far:
{{ range $index, $interaction := .Interactions }}
{{ $interaction.Role }}: ({{ $interaction.Content }})
{{ end }}
user: ({{ .Message }})

Generate email message.
`
