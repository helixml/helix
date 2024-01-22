package notification

import (
	"bytes"
	"context"
	"fmt"
	"html/template"

	"github.com/nikoksr/notify"
	"github.com/nikoksr/notify/service/mail"
	"github.com/nikoksr/notify/service/mailgun"
	"github.com/rs/zerolog/log"
)

type Email struct {
	cfg     *Config
	enabled bool
}

func NewEmail(cfg *Config) (*Email, error) {
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
		var opts []mailgun.Option
		if e.cfg.Email.Mailgun.Europe {
			opts = append(opts, mailgun.WithEurope())
		}

		mg := mailgun.New(e.cfg.Email.Mailgun.Domain, e.cfg.Email.Mailgun.APIKey, e.cfg.Email.SenderAddress, opts...)
		mg.AddReceivers(email)

		ntf.UseServices(mg)
	}

	if e.cfg.Email.SMTP.Host != "" {
		smtp := mail.New(e.cfg.Email.SenderAddress, e.cfg.Email.SMTP.Host+":"+e.cfg.Email.SMTP.Port)
		smtp.AuthenticateSMTP(e.cfg.Email.SMTP.Identity, e.cfg.Email.SMTP.Username, e.cfg.Email.SMTP.Password, e.cfg.Email.SMTP.Host)

		smtp.AddReceivers(email)

		ntf.UseServices(smtp)
	}
	return ntf
}

func (e *Email) getEmailMessage(n *Notification) (title, message string, err error) {
	switch n.Event {
	case EventFinetuningStarted:
		var buf bytes.Buffer

		err = finetuningStartedTmpl.Execute(&buf, &templateData{
			FirstName:  n.FirstName,
			SessionURL: fmt.Sprintf("%s/session/%s", e.cfg.AppURL, n.Session.ID),
		})
		if err != nil {
			return "", "", fmt.Errorf("failed to execute template: %w", err)
		}

		return fmt.Sprintf("Your Finetuning Process Has Begun [%s]", n.Session.Name), buf.String(), nil
	case EventFinetuningComplete:
		var buf bytes.Buffer

		err = finetuningCompletedTmpl.Execute(&buf, &templateData{
			FirstName:   n.FirstName,
			SessionURL:  fmt.Sprintf("%s/session/%s", e.cfg.AppURL, n.Session.ID),
			SessionName: n.Session.Name,
		})
		if err != nil {
			return "", "", fmt.Errorf("failed to execute template: %w", err)
		}

		return fmt.Sprintf("Finetuning Complete - Ready for Action [%s]", n.Session.Name), buf.String(), nil
	default:
		return "", "", fmt.Errorf("unknown event '%s'", n.Event.String())
	}
}

type templateData struct {
	SessionURL  string
	FirstName   string
	SessionName string
}

var (
	finetuningStartedTmpl   = template.Must(template.New("").Parse(finetuningStartedTemplate))
	finetuningCompletedTmpl = template.Must(template.New("").Parse(finetuningCompletedTemplate))
)

var finetuningStartedTemplate = `
Dear {{ .FirstName }},
<br/><br/>
We are pleased to inform you that your finetuning process has commenced. Rest assured, we will send you a follow-up email once it is successfully completed.
<br/><br/>
In the meantime, you can track the progress of your session at the following link: <a href="{{ .SessionURL }}" target="_blank">{{ .SessionURL }}</a>.
<br/><br/>
Thank you for choosing us. Should you have any questions, feel free to reach out.
<br/><br/>
Warm regards,<br/>
The Helix Team
`

var finetuningCompletedTemplate = `
Dear {{ .FirstName }},
<br/><br/>
Great news! Your finetuning process for session '{{ .SessionName }}' is now complete. You can start experiencing the enhanced features immediately.
<br/><br/>
To view and interact with your session, please visit: <a href="{{ .SessionURL }}" target="_blank">{{ .SessionURL }}</a>.
<br/><br/>
We hope you enjoy the improvements. If you have any feedback or require assistance, our team is here to help.
<br/><br/>
Best regards,<br/><br/>
The Helix Team
`
