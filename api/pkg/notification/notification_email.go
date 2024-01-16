package notification

import (
	"context"

	"github.com/nikoksr/notify/service/mailgun"
)

type Email struct {
	cfg *EmailConfig
	mg  *mailgun.Mailgun
}

func NewEmail(cfg *EmailConfig) (*Email, error) {
	e := &Email{
		cfg: cfg,
	}

	if cfg.Mailgun.APIKey != "" {
		var opts []mailgun.Option
		if cfg.Mailgun.Europe {
			opts = append(opts, mailgun.WithEurope())
		}

		e.mg = mailgun.New(cfg.Mailgun.Domain, cfg.Mailgun.APIKey, cfg.SenderAddress, opts...)
	}

	// TODO: SMTP fallback

	return e, nil
}

func (e *Email) Notify(ctx context.Context, n *Notification) error {
	// TODO: get user email
	return nil
}
