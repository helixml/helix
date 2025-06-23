package skill

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/util/jsonschema"
	"github.com/sashabaranov/go-openai"

	"github.com/nikoksr/notify"
	"github.com/nikoksr/notify/service/mail"
	"github.com/nikoksr/notify/service/mailgun"
)

const emailMainPrompt = `You are an expert at sending emails. Your role is to help users by composing and sending emails on their behalf to the user you are currently interacting with.

Key responsibilities:
1.  **Email Composition**:
    *   Craft clear and concise email subjects and messages based on the user's request.
    *   Understand the context of the conversation to generate relevant email content.
2.  **Sending Emails**:
    *   Use the 'SendEmail' tool to send the email.
    *   The email will be sent to the current user.

When using the 'SendEmail' tool:
*   You must provide a 'subject' for the email.
*   You must provide a 'message' for the email body.
*   The tool will automatically handle sending the email to the user.
*   Use HTML formatting for the email body. If user has supplied the template, use it as is. For new lines, use <br/>.

Example:
If the user says "send me an email with the results of our conversation", you should summarize the conversation and use the 'SendEmail' tool with a relevant subject and the summary as the message.

Remember: Your goal is to send helpful and relevant emails to the user. Always ensure the subject and message are appropriate for the user's request.`

func NewSendEmailSkill(cfg *config.EmailConfig, templateExample string) agent.Skill {
	prompt := emailMainPrompt
	if templateExample != "" {
		prompt += "\n\nWhen composing the email message, you MUST adhere to the following template that the user has provided:\n\n" + templateExample
	}
	return agent.Skill{
		Name:         "SendEmail",
		Description:  "Send email to the current user (with whom we are currently interacting)",
		SystemPrompt: prompt,
		Tools: []agent.Tool{
			&SendEmailTool{
				cfg:             cfg,
				templateExample: templateExample,
			},
		},
	}
}

type SendEmailTool struct {
	cfg             *config.EmailConfig
	templateExample string
}

func (t *SendEmailTool) Name() string {
	return "SendEmail"
}

func (t *SendEmailTool) Description() string {
	return "Send an email to the current user (with whom we are currently interacting)"
}

func (t *SendEmailTool) String() string {
	return "SendEmail"
}

func (t *SendEmailTool) StatusMessage() string {
	return "Sending email"
}

func (t *SendEmailTool) Icon() string {
	return "EmailIcon"
}

func (t *SendEmailTool) OpenAI() []openai.Tool {
	return []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "SendEmail",
				Description: "Send an email to the current user (with whom we are currently interacting)",
				Parameters: jsonschema.Definition{
					Type: jsonschema.Object,
					Properties: map[string]jsonschema.Definition{
						"subject": {
							Type:        jsonschema.String,
							Description: "The subject of the email.",
						},
						"message": {
							Type:        jsonschema.String,
							Description: "The body of the email.",
						},
					},
					Required: []string{"subject", "message"},
				},
			},
		},
	}
}

func (t *SendEmailTool) Execute(ctx context.Context, meta agent.Meta, args map[string]interface{}) (string, error) {
	subject, ok := args["subject"].(string)
	if !ok {
		return "", fmt.Errorf("subject is required")
	}

	message, ok := args["message"].(string)
	if !ok {
		return "", fmt.Errorf("message is required")
	}

	enabled := false
	if t.cfg.Mailgun.APIKey != "" {
		enabled = true
	}

	if t.cfg.SMTP.Host != "" {
		enabled = true
	}

	if !enabled {
		return "", fmt.Errorf("no email provider configured")
	}

	ntf := notify.New()

	if t.cfg.Mailgun.APIKey != "" {
		var opts []mailgun.Option
		if t.cfg.Mailgun.Europe {
			opts = append(opts, mailgun.WithEurope())
		}

		mg := mailgun.New(t.cfg.Mailgun.Domain, t.cfg.Mailgun.APIKey, t.cfg.AgentSkillSenderAddress, opts...)
		mg.AddReceivers(meta.UserEmail)

		ntf.UseServices(mg)
	}

	if t.cfg.SMTP.Host != "" {
		smtp := mail.New(t.cfg.AgentSkillSenderAddress, t.cfg.SMTP.Host+":"+t.cfg.SMTP.Port)
		smtp.AuthenticateSMTP(t.cfg.SMTP.Identity, t.cfg.SMTP.Username, t.cfg.SMTP.Password, t.cfg.SMTP.Host)

		smtp.AddReceivers(meta.UserEmail)

		ntf.UseServices(smtp)
	}

	err := ntf.Send(ctx, subject, message)
	if err != nil {
		return "", fmt.Errorf("failed to send email: %w", err)
	}

	return "Email sent", nil
}
