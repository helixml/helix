package notification

import "context"

type Config struct {
	Email EmailConfig
	// TODO: Slack, Discord, etc.
}

type EmailConfig struct {
	SenderAddress string `envconfig:"EMAIL_SENDER_ADDRESS" default:"info@helix.ml"`

	SMTP struct {
		Host     string `envconfig:"EMAIL_SMTP_HOST"`
		Identity string `envconfig:"EMAIL_SMTP_IDENTITY"`
		Username string `envconfig:"EMAIL_SMTP_USERNAME"`
		Password string `envconfig:"EMAIL_SMTP_PASSWORD"`
	}

	Mailgun struct {
		Domain string `envconfig:"EMAIL_MAILGUN_DOMAIN"`
		APIKey string `envconfig:"EMAIL_MAILGUN_API_KEY"`
		Europe bool   `envconfig:"EMAIL_MAILGUN_EUROPE" default:"true"` // use EU region
	}
}

type Provider string

const (
	ProviderEmail Provider = "email"
)

type Event int

const EventFinetuningComplete Event = 1

type Notification struct {
	Event  Event
	UserID string
}

type Notifier interface {
	Notify(ctx context.Context, n *Notification) error
}

type NotificationsProvider struct {
	email *Email
}

func New(cfg *Config) (Notifier, error) {
	email, err := NewEmail(&cfg.Email)
	if err != nil {
		return nil, err
	}

	return &NotificationsProvider{
		email: email,
	}, nil
}

func (n *NotificationsProvider) Notify(ctx context.Context, notification *Notification) error {
	// TODO: check user preferences

	switch notification.Event {
	case EventFinetuningComplete:
		return n.email.Notify(ctx, notification)
	}

	return nil
}
