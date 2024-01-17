package notification

import (
	"context"
	"fmt"

	"github.com/nikoksr/notify"
	"github.com/nikoksr/notify/service/mail"
	"github.com/nikoksr/notify/service/mailgun"
	"github.com/rs/zerolog/log"
)

type Email struct {
	cfg     *EmailConfig
	enabled bool
}

func NewEmail(cfg *EmailConfig) (*Email, error) {
	e := &Email{
		cfg: cfg,
	}

	if cfg.Mailgun.APIKey != "" {
		e.enabled = true
	}

	if cfg.SMTP.Host != "" {
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

	if e.cfg.Mailgun.APIKey != "" {
		var opts []mailgun.Option
		if e.cfg.Mailgun.Europe {
			opts = append(opts, mailgun.WithEurope())
		}

		mg := mailgun.New(e.cfg.Mailgun.Domain, e.cfg.Mailgun.APIKey, e.cfg.SenderAddress, opts...)
		mg.AddReceivers(email)

		ntf.UseServices(mg)
	}

	if e.cfg.SMTP.Host != "" {
		smtp := mail.New(e.cfg.SenderAddress, e.cfg.SMTP.Host+":"+e.cfg.SMTP.Port)
		smtp.AuthenticateSMTP(e.cfg.SMTP.Identity, e.cfg.SMTP.Username, e.cfg.SMTP.Password, e.cfg.SMTP.Host)

		smtp.AddReceivers(email)

		ntf.UseServices(smtp)
	}
	return ntf
}

func (e *Email) getEmailMessage(n *Notification) (title, message string, err error) {
	switch n.Event {
	case EventFinetuningStarted:
		return "Helix Finetuning started", "Finetuning has started, we will inform you once it has finished", nil
	case EventFinetuningComplete:
		return "Finetuning has finished", "Finetuning has finished, you can start using the model", nil
	default:
		return "", "", fmt.Errorf("unknown event '%s'", n.Event.String())
	}
}
