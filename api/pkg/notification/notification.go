package notification

import (
	"context"
	"fmt"
	"strings"

	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

type Provider string

const (
	ProviderEmail Provider = "email"
)

type Event int

const (
	EventCronTriggerComplete Event = 1
	EventCronTriggerFailed   Event = 2
)

func (e Event) String() string {
	switch e {
	case EventCronTriggerComplete:
		return "cron_trigger_complete"
	case EventCronTriggerFailed:
		return "cron_trigger_failed"
	default:
		return "unknown_event"
	}
}

type Notification struct {
	Event   Event
	Session *types.Session
	Message string

	RenderMarkdown bool // Set to true to render markdown to HTML when sending email

	// Populated by the provider
	Email     string
	FirstName string
}

//go:generate mockgen -source $GOFILE -destination notification_mocks.go -package $GOPACKAGE

type Notifier interface {
	Notify(ctx context.Context, n *Notification) error
}

type NotificationsProvider struct {
	authenticator auth.Authenticator

	email *Email
}

func New(cfg *config.Notifications, authenticator auth.Authenticator) (Notifier, error) {
	email, err := NewEmail(cfg)
	if err != nil {
		return nil, err
	}

	return &NotificationsProvider{
		authenticator: authenticator,
		email:         email,
	}, nil
}

func (n *NotificationsProvider) Notify(ctx context.Context, notification *Notification) error {
	if n.authenticator == nil {
		return nil
	}

	if !n.email.Enabled() {
		log.Debug().Str("notification", notification.Event.String()).Msg("email not enabled")
		return nil
	}

	user, err := n.authenticator.GetUserByID(ctx, notification.Session.Owner)
	if err != nil {
		return fmt.Errorf("failed to get user '%s' details: %w", notification.Session.Owner, err)
	}

	notification.Email = user.Email
	notification.FirstName = strings.Split(user.FullName, " ")[0]

	if n.email.Enabled() {
		log.Debug().
			Str("email", user.Email).Str("notification", notification.Event.String()).Msg("sending notification")
		err := n.email.Notify(ctx, notification)
		if err != nil {
			return err
		}
	}

	return nil
}
