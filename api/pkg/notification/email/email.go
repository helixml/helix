package email

import (
	"context"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/nikoksr/notify"
	"github.com/nikoksr/notify/service/mail"
	"github.com/nikoksr/notify/service/mailgun"
)

type Email struct {
	cfg *config.ServerConfig
}

func New(cfg *config.ServerConfig) *Email {
	return &Email{
		cfg: cfg,
	}
}

func (e *Email) Send(ctx context.Context, to, subject, body string) error {
	ntf := e.getClient(to)

	return ntf.Send(ctx, subject, body)
}

func (e *Email) getClient(email string) *notify.Notify {

	ntf := notify.New()

	if e.cfg.Notifications.Email.Mailgun.APIKey != "" {
		var opts []mailgun.Option
		if e.cfg.Notifications.Email.Mailgun.Europe {
			opts = append(opts, mailgun.WithEurope())
		}

		mg := mailgun.New(e.cfg.Notifications.Email.Mailgun.Domain, e.cfg.Notifications.Email.Mailgun.APIKey, e.cfg.Notifications.Email.SenderAddress, opts...)
		mg.AddReceivers(email)

		ntf.UseServices(mg)
	}

	if e.cfg.Notifications.Email.SMTP.Host != "" {
		smtp := mail.New(e.cfg.Notifications.Email.SenderAddress, e.cfg.Notifications.Email.SMTP.Host+":"+e.cfg.Notifications.Email.SMTP.Port)
		smtp.AuthenticateSMTP(e.cfg.Notifications.Email.SMTP.Identity, e.cfg.Notifications.Email.SMTP.Username, e.cfg.Notifications.Email.SMTP.Password, e.cfg.Notifications.Email.SMTP.Host)
		smtp.AddReceivers(email)

		ntf.UseServices(smtp)
	}

	return ntf
}

func Enabled(cfg *config.ServerConfig) bool {
	return cfg.Notifications.Email.Mailgun.APIKey != "" || cfg.Notifications.Email.SMTP.Host != ""
}
