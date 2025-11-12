package notification

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"html/template"
	"regexp"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/nikoksr/notify"
	"github.com/nikoksr/notify/service/mail"
	"github.com/nikoksr/notify/service/mailgun"
	"github.com/rs/zerolog/log"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

//go:embed templates/task_complete.html
var taskCompleteTemplate string

//go:embed templates/task_failed.html
var taskFailedTemplate string

//go:embed templates/password_reset_request.html
var passwordResetRequestTemplate string

var cronTriggerCompleteTmpl = template.Must(template.New("taskComplete").Parse(taskCompleteTemplate))
var cronTriggerFailedTmpl = template.Must(template.New("taskFailed").Parse(taskFailedTemplate))
var passwordResetRequestTmpl = template.Must(template.New("passwordResetRequest").Parse(passwordResetRequestTemplate))

type Email struct {
	cfg     *config.Notifications
	enabled bool
}

func NewEmail(cfg *config.Notifications) (*Email, error) {
	e := &Email{
		cfg: cfg,
	}

	if cfg.Email.Mailgun.APIKey != "" {
		e.enabled = true
	}

	if cfg.Email.SMTP.Host != "" {
		e.enabled = true
	}

	return e, nil
}

func (e *Email) Enabled() bool {
	return e.enabled
}

func (e *Email) Notify(ctx context.Context, n *Notification) error {
	if n.Email == "" {
		// Nothing to do
		log.Ctx(ctx).Warn().Str("session_id", n.Session.ID).Msg("no email address provided for notification")
		return nil
	}

	client := e.getClient(n.Email)

	title, message, err := e.getEmailMessage(n)
	if err != nil {
		return err
	}

	err = client.Send(ctx, title, message)
	if err != nil {
		return fmt.Errorf("failed to send email to %s: %w", n.Email, err)
	}

	return nil
}

func (e *Email) getClient(email string) *notify.Notify {

	ntf := notify.New()

	if e.cfg.Email.Mailgun.APIKey != "" {
		log.Debug().Msg("using Mailgun")
		var opts []mailgun.Option
		if e.cfg.Email.Mailgun.Europe {
			opts = append(opts, mailgun.WithEurope())
		}

		mg := mailgun.New(e.cfg.Email.Mailgun.Domain, e.cfg.Email.Mailgun.APIKey, e.cfg.Email.SenderAddress, opts...)
		mg.AddReceivers(email)

		ntf.UseServices(mg)
	}

	if e.cfg.Email.SMTP.Host != "" {
		log.Debug().Msg("using SMTP")
		smtp := mail.New(e.cfg.Email.SenderAddress, e.cfg.Email.SMTP.Host+":"+e.cfg.Email.SMTP.Port)
		smtp.AuthenticateSMTP(e.cfg.Email.SMTP.Identity, e.cfg.Email.SMTP.Username, e.cfg.Email.SMTP.Password, e.cfg.Email.SMTP.Host)

		smtp.AddReceivers(email)

		ntf.UseServices(smtp)
	}
	return ntf
}

func (e *Email) renderMarkdown(message string) (string, error) {
	// Preprocess the message to convert divider patterns to HTML dividers
	message = e.convertDividers(message)

	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithXHTML(),
			html.WithUnsafe(),
		),
	)
	var buf bytes.Buffer
	if err := md.Convert([]byte(message), &buf); err != nil {
		return "", fmt.Errorf("failed to convert markdown to HTML: %w", err)
	}

	return buf.String(), nil
}

func (e *Email) convertDividers(message string) string {
	// Regex to match divider patterns like "------------", "********", "========", etc.
	// Also matches Unicode box drawing characters like "─────────────────"
	// Matches 3 or more consecutive dashes, asterisks, equals signs, underscores, or box drawing characters on their own line
	dividerRegex := regexp.MustCompile(`(?m)^[\s]*([-*=_─━═]{3,})[\s]*$`)

	// Replace divider patterns with HTML hr tags
	return dividerRegex.ReplaceAllString(message, "<hr>")
}

func (e *Email) getEmailMessage(n *Notification) (title, message string, err error) {

	if n.RenderMarkdown {
		message, err = e.renderMarkdown(n.Message)
		if err != nil {
			return "", "", fmt.Errorf("failed to convert markdown to HTML: %w", err)
		}
		n.Message = message
	}

	switch n.Event {
	case types.EventCronTriggerComplete:
		var buf bytes.Buffer

		err = cronTriggerCompleteTmpl.Execute(&buf, &templateData{
			Message:     template.HTML(n.Message),
			SessionURL:  fmt.Sprintf("%s/session/%s", e.cfg.AppURL, n.Session.ID),
			SessionName: n.Session.Name,
		})
		if err != nil {
			return "", "", fmt.Errorf("failed to execute template: %w", err)
		}

		return n.Session.Name, buf.String(), nil
	case types.EventCronTriggerFailed:
		var buf bytes.Buffer

		err = cronTriggerFailedTmpl.Execute(&buf, &templateData{
			Message:      template.HTML(n.Message),
			SessionURL:   fmt.Sprintf("%s/session/%s", e.cfg.AppURL, n.Session.ID),
			SessionName:  n.Session.Name,
			ErrorMessage: n.Message,
		})
		if err != nil {
			return "", "", fmt.Errorf("failed to execute template: %w", err)
		}

		return n.Session.Name, buf.String(), nil
	case types.EventPasswordResetRequest:
		var buf bytes.Buffer

		err = passwordResetRequestTmpl.Execute(&buf, &templateData{
			Message: template.HTML(n.Message),
		})
		if err != nil {
			return "", "", fmt.Errorf("failed to execute template: %w", err)
		}

		return "Password Reset Request", buf.String(), nil
	default:
		return "", "", fmt.Errorf("unknown event '%s'", n.Event.String())
	}
}

type templateData struct {
	SessionURL   string
	Message      template.HTML
	SessionName  string
	ErrorMessage string
}
