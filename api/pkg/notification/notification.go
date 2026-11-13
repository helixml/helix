package notification

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/netguard"
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

	// webhookClient posts to callback URLs, which users set on cron triggers
	// and sessions; it enforces the outbound policy on every connection.
	webhookClient *http.Client

	email *Email
}

// New returns a Notifier. webhookPolicy restricts where callback URLs may
// point (they are user-supplied and posted to from the API server).
func New(cfg *config.Notifications, store store.Store, webhookPolicy netguard.Policy) (Notifier, error) {
	email, err := NewEmail(cfg)
	if err != nil {
		return nil, err
	}

	return &NotificationsProvider{
		store:         store,
		email:         email,
		webhookClient: webhookPolicy.NewClient(webhookTimeout),
	}, nil
}

// CheckRecipients splits emails into the addresses Helix may notify on behalf
// of ownerID and the rest. A recipient must be a Helix user who is either the
// owner or a member of orgID (when set). Recipient lists are user-configured
// (e.g. on cron triggers), and Helix only sends mail on a user's behalf to
// people inside that user's account or organization. Blank and duplicate
// (case-insensitive) addresses are skipped.
func CheckRecipients(ctx context.Context, st store.Store, ownerID, orgID string, emails []string) (allowed, rejected []string) {
	seen := map[string]bool{}
	for _, email := range emails {
		email = strings.TrimSpace(email)
		key := strings.ToLower(email)
		if email == "" || seen[key] {
			continue
		}
		seen[key] = true
		if ownerID != "" && recipientAllowed(ctx, st, ownerID, orgID, email) {
			allowed = append(allowed, email)
		} else {
			rejected = append(rejected, email)
		}
	}
	return allowed, rejected
}

func recipientAllowed(ctx context.Context, st store.Store, ownerID, orgID, email string) bool {
	user, err := st.GetUser(ctx, &store.GetUserQuery{Email: email})
	if err != nil || user == nil {
		return false
	}
	if user.ID == ownerID {
		return true
	}
	if orgID == "" {
		return false
	}
	_, err = st.GetOrganizationMembership(ctx, &store.GetOrganizationMembershipQuery{
		OrganizationID: orgID,
		UserID:         user.ID,
	})
	return err == nil
}

// allowedRecipients applies CheckRecipients to a notification's explicit
// recipients, using the notifying session's owner and organization. Dropped
// recipients are logged as a count, without their addresses.
func (n *NotificationsProvider) allowedRecipients(ctx context.Context, notification *Notification) []string {
	var ownerID, orgID, sessionID string
	if notification.Session != nil {
		ownerID, orgID, sessionID = notification.Session.Owner, notification.Session.OrganizationID, notification.Session.ID
	}
	allowed, rejected := CheckRecipients(ctx, n.store, ownerID, orgID, notification.Emails)
	if len(rejected) > 0 {
		log.Warn().
			Str("notification", notification.Event.String()).
			Str("owner_id", ownerID).
			Str("organization_id", orgID).
			Str("session_id", sessionID).
			Int("dropped_recipients", len(rejected)).
			Int("allowed_recipients", len(allowed)).
			Msg("dropped notification recipients outside the owner's account and organization")
	}
	return allowed
}

func (n *NotificationsProvider) Notify(ctx context.Context, notification *Notification) error {
	if n.store == nil {
		return nil
	}

	// Send webhook if callback URL is configured
	if notification.CallbackURL != "" {
		if err := sendWebhook(ctx, n.webhookClient, notification.CallbackURL, notification); err != nil {
			log.Error().Err(err).Str("callback_url", notification.CallbackURL).Msg("failed to send webhook notification")
		}
	}

	if !n.email.Enabled() {
		log.Debug().Str("notification", notification.Event.String()).Msg("email not enabled")
		return nil
	}

	// If multiple emails are specified, send to each one
	if len(notification.Emails) > 0 {
		for _, email := range n.allowedRecipients(ctx, notification) {
			perRecipient := &Notification{
				Event:          notification.Event,
				Session:        notification.Session,
				Message:        notification.Message,
				RenderMarkdown: notification.RenderMarkdown,
				Email:          email,
			}
			log.Debug().
				Str("email", email).Str("notification", perRecipient.Event.String()).Msg("sending notification")
			if err := n.email.Notify(ctx, perRecipient); err != nil {
				log.Error().Err(err).Str("email", email).Msg("failed to send notification")
			}
		}
		return nil
	}

	// If email is not pre-populated, look up from session owner
	if notification.Email == "" && notification.Session != nil {
		user, err := n.store.GetUser(ctx, &store.GetUserQuery{ID: notification.Session.Owner})
		if err != nil {
			return fmt.Errorf("failed to get user '%s' details: %w", notification.Session.Owner, err)
		}

		notification.Email = user.Email
		notification.FirstName = strings.Split(user.FullName, " ")[0]
	}

	log.Debug().
		Str("email", notification.Email).Str("notification", notification.Event.String()).Msg("sending notification")
	err := n.email.Notify(ctx, notification)
	if err != nil {
		return err
	}

	return nil
}
