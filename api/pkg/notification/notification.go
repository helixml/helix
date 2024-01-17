package notification

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

type Config struct {
	Email EmailConfig
	// TODO: Slack, Discord, etc.
}

type EmailConfig struct {
	SenderAddress string `envconfig:"EMAIL_SENDER_ADDRESS" default:"karolis@helix.ml"`

	SMTP struct {
		Host     string `envconfig:"EMAIL_SMTP_HOST"`
		Port     string `envconfig:"EMAIL_SMTP_PORT"`
		Identity string `envconfig:"EMAIL_SMTP_IDENTITY"`
		Username string `envconfig:"EMAIL_SMTP_USERNAME"`
		Password string `envconfig:"EMAIL_SMTP_PASSWORD"`
	}

	Mailgun struct {
		Domain string `envconfig:"EMAIL_MAILGUN_DOMAIN"`
		APIKey string `envconfig:"EMAIL_MAILGUN_API_KEY"`
		Europe bool   `envconfig:"EMAIL_MAILGUN_EUROPE" default:"false"` // use EU region
	}
}

type Provider string

const (
	ProviderEmail Provider = "email"
)

type Event int

const (
	EventFinetuningStarted  Event = 1
	EventFinetuningComplete Event = 2
)

func (e Event) String() string {
	switch e {
	case EventFinetuningStarted:
		return "finetuning_started"
	case EventFinetuningComplete:
		return "finetuning_complete"
	default:
		return "unknown_event"
	}
}

type Notification struct {
	Event   Event
	Session *types.Session

	// Populated by the provider
	Email string
}

type Notifier interface {
	Notify(ctx context.Context, n *Notification) error
}

type NotificationsProvider struct {
	authenticator auth.Authenticator

	email *Email
}

func New(cfg *Config, authenticator auth.Authenticator) (Notifier, error) {
	email, err := NewEmail(&cfg.Email)
	if err != nil {
		return nil, err
	}

	return &NotificationsProvider{
		authenticator: authenticator,
		email:         email,
	}, nil
}

func (n *NotificationsProvider) Notify(ctx context.Context, notification *Notification) error {
	user, err := n.authenticator.GetUserByID(ctx, notification.Session.Owner)
	if err != nil {
		return fmt.Errorf("failed to get user '%s' details: %w", notification.Session.Owner, err)
	}

	log.Debug().
		Str("email", user.Email).Str("notification", notification.Event.String()).Msg("sending notification")

	notification.Email = user.Email

	if n.email.Enabled() {
		err := n.email.Notify(ctx, notification)
		if err != nil {
			return err
		}
	}

	return nil
}
