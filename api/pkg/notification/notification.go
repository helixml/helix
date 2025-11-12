package notification

import (
	"context"
	"fmt"
	"strings"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

type Provider string

const (
	ProviderEmail Provider = "email"
)

type Notification = types.Notification

//go:generate mockgen -source $GOFILE -destination notification_mocks.go -package $GOPACKAGE

type Notifier interface {
	Notify(ctx context.Context, n *Notification) error
}

type NotificationsProvider struct {
	store store.Store

	email *Email
}

func New(cfg *config.Notifications, store store.Store) (Notifier, error) {
	email, err := NewEmail(cfg)
	if err != nil {
		return nil, err
	}

	return &NotificationsProvider{
		store: store,
		email: email,
	}, nil
}

func (n *NotificationsProvider) Notify(ctx context.Context, notification *Notification) error {
	if n.store == nil {
		return nil
	}

	if !n.email.Enabled() {
		log.Debug().Str("notification", notification.Event.String()).Msg("email not enabled")
		return nil
	}

	user, err := n.store.GetUser(ctx, &store.GetUserQuery{ID: notification.Session.Owner})
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
