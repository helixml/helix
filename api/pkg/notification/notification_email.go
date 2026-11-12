package notification

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"html/template"
	"regexp"
	"strings"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/microcosm-cc/bluemonday"
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

//go:embed templates/waitlist_approved.html
var waitlistApprovedTemplate string

//go:embed templates/trial_activated.html
var trialActivatedTemplate string

//go:embed templates/org_invitation.html
var orgInvitationTemplate string

var cronTriggerCompleteTmpl = template.Must(template.New("taskComplete").Parse(taskCompleteTemplate))
var cronTriggerFailedTmpl = template.Must(template.New("taskFailed").Parse(taskFailedTemplate))
var passwordResetRequestTmpl = template.Must(template.New("passwordResetRequest").Parse(passwordResetRequestTemplate))
var waitlistApprovedTmpl = template.Must(template.New("waitlistApproved").Parse(waitlistApprovedTemplate))
var trialActivatedTmpl = template.Must(template.New("trialActivated").Parse(trialActivatedTemplate))
var orgInvitationTmpl = template.Must(template.New("orgInvitation").Parse(orgInvitationTemplate))

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
		log.Ctx(ctx).Warn().Msg("no email address provided for notification")
		return nil
	}

	client := e.getClient(n.Email)

	title, message, err := e.getEmailMessage(n)
	if err != nil {
		return err
	}

	log.Ctx(ctx).Debug().Str("email", n.Email).Str("title", title).Str("event", n.Event.String()).Msg("dispatching email via transport")
	err = client.Send(ctx, title, message)
	if err != nil {
		return fmt.Errorf("failed to send email to %s: %w", n.Email, err)
	}
	log.Ctx(ctx).Info().Str("email", n.Email).Str("title", title).Str("event", n.Event.String()).Msg("email dispatched")

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

// dividerPlaceholder stands in for a divider line while the markdown is
// rendered, so the renderer can stay in safe mode (raw HTML in the message is
// not passed through) and the divider is swapped for <hr> afterwards.
const dividerPlaceholder = "HELIXEMAILDIVIDERPLACEHOLDER"

// dividerRegex matches divider patterns like "------------", "********",
// "========", including Unicode box drawing characters like "─────", made of
// 3 or more such characters on their own line.
var dividerRegex = regexp.MustCompile(`(?m)^[\s]*([-*=_─━═]{3,})[\s]*$`)

// emailHTMLPolicy is applied to rendered message HTML before it is placed in
// an email template. Message content comes from model output and user input,
// so only ordinary formatting markup is kept.
var emailHTMLPolicy = bluemonday.UGCPolicy()

// renderMarkdown converts message markdown to HTML that is safe to embed in
// an email: raw HTML in the source is not passed through, and the output is
// sanitized.
func (e *Email) renderMarkdown(message string) (string, error) {
	message = dividerRegex.ReplaceAllString(message, dividerPlaceholder)

	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithXHTML(),
		),
	)
	var buf bytes.Buffer
	if err := md.Convert([]byte(message), &buf); err != nil {
		return "", fmt.Errorf("failed to convert markdown to HTML: %w", err)
	}

	rendered := strings.ReplaceAll(buf.String(), dividerPlaceholder, "<hr>")
	return emailHTMLPolicy.Sanitize(rendered), nil
}

// messageHTML returns the notification message as HTML for the email body.
// Markdown messages are rendered and sanitized; anything else is escaped as
// plain text.
func (e *Email) messageHTML(n *Notification) (template.HTML, error) {
	if !n.RenderMarkdown {
		return template.HTML(template.HTMLEscapeString(n.Message)), nil // #nosec G203 -- escaped above
	}
	rendered, err := e.renderMarkdown(n.Message)
	if err != nil {
		return "", err
	}
	return template.HTML(rendered), nil // #nosec G203 -- sanitized by renderMarkdown
}

func (e *Email) getEmailMessage(n *Notification) (title, message string, err error) {

	switch n.Event {
	case types.EventCronTriggerComplete, types.EventCronTriggerFailed:
		if n.Session == nil {
			return "", "", fmt.Errorf("session missing for %s", n.Event.String())
		}
	}

	messageHTML, err := e.messageHTML(n)
	if err != nil {
		return "", "", fmt.Errorf("failed to convert markdown to HTML: %w", err)
	}

	switch n.Event {
	case types.EventCronTriggerComplete:
		var buf bytes.Buffer

		err = cronTriggerCompleteTmpl.Execute(&buf, &templateData{
			Message:     messageHTML,
			SessionURL:  e.sessionURL(n.Session),
			SessionName: n.Session.Name,
		})
		if err != nil {
			return "", "", fmt.Errorf("failed to execute template: %w", err)
		}

		return n.Session.Name, buf.String(), nil
	case types.EventCronTriggerFailed:
		var buf bytes.Buffer

		err = cronTriggerFailedTmpl.Execute(&buf, &templateData{
			Message:      messageHTML,
			SessionURL:   e.sessionURL(n.Session),
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
			Message: messageHTML,
		})
		if err != nil {
			return "", "", fmt.Errorf("failed to execute template: %w", err)
		}

		return "Password Reset Request", buf.String(), nil
	case types.EventWaitlistApproved:
		var buf bytes.Buffer

		err = waitlistApprovedTmpl.Execute(&buf, &templateData{
			FirstName:    n.FirstName,
			AppURL:       e.cfg.AppURL,
			TrialDays:    n.TrialDays,
			TrialPending: n.TrialPending,
		})
		if err != nil {
			return "", "", fmt.Errorf("failed to execute template: %w", err)
		}

		subject := "Welcome to Helix - You're Approved!"
		if n.TrialDays > 0 {
			subject = fmt.Sprintf("Welcome to Helix - %d-day trial activated", n.TrialDays)
		}
		return subject, buf.String(), nil
	case types.EventTrialActivated:
		var buf bytes.Buffer

		err = trialActivatedTmpl.Execute(&buf, &templateData{
			FirstName:    n.FirstName,
			AppURL:       e.cfg.AppURL,
			TrialDays:    n.TrialDays,
			TrialPending: n.TrialPending,
		})
		if err != nil {
			return "", "", fmt.Errorf("failed to execute template: %w", err)
		}

		return fmt.Sprintf("Your Helix trial is active - %d days included", n.TrialDays), buf.String(), nil
	case types.EventOrgInvitation:
		if n.OrgInvitation == nil {
			return "", "", fmt.Errorf("org_invitation payload missing for EventOrgInvitation")
		}
		var buf bytes.Buffer
		err = orgInvitationTmpl.Execute(&buf, n.OrgInvitation)
		if err != nil {
			return "", "", fmt.Errorf("failed to execute template: %w", err)
		}
		displayName := n.OrgInvitation.OrganizationDisplayName
		if displayName == "" {
			displayName = n.OrgInvitation.OrganizationName
		}
		return fmt.Sprintf("You've been invited to %s on Helix", displayName), buf.String(), nil
	default:
		return "", "", fmt.Errorf("unknown event '%s'", n.Event.String())
	}
}

// sessionURL links to the session in the app, or to the app itself when the
// notification is not tied to a chat session.
func (e *Email) sessionURL(s *types.Session) string {
	if s.ID == "" {
		return e.cfg.AppURL
	}
	return fmt.Sprintf("%s/orgs/%s/session/%s", e.cfg.AppURL, s.OrganizationID, s.ID)
}

type templateData struct {
	SessionURL   string
	Message      template.HTML
	SessionName  string
	ErrorMessage string
	FirstName    string
	AppURL       string
	TrialDays    int
	TrialPending bool
}
