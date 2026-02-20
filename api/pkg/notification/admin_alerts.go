package notification

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"html/template"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

//go:embed templates/disk_space_alert.html
var diskSpaceAlertTemplate string

var diskSpaceAlertTmpl = template.Must(template.New("diskSpaceAlert").Parse(diskSpaceAlertTemplate))

// SlackSender is an interface for sending Slack messages (implemented by janitor.Janitor)
type SlackSender interface {
	SendMessage(userEmail string, message string) error
}

// AdminAlerter handles sending alerts to admin users
type AdminAlerter struct {
	cfg   *config.Notifications
	store store.Store
	email *Email
	slack SlackSender
}

// NewAdminAlerter creates a new admin alerter
func NewAdminAlerter(cfg *config.Notifications, store store.Store) (*AdminAlerter, error) {
	email, err := NewEmail(cfg)
	if err != nil {
		return nil, err
	}

	return &AdminAlerter{
		cfg:   cfg,
		store: store,
		email: email,
	}, nil
}

// SetSlackSender sets the Slack sender (typically the Janitor)
func (a *AdminAlerter) SetSlackSender(slack SlackSender) {
	a.slack = slack
}

// DiskSpaceAlertData holds data for disk space alert emails
type DiskSpaceAlertData struct {
	WolfName    string
	WolfID      string
	AlertLevel  string
	DiskMetrics []DiskMetricData
	DashboardURL string
}

// DiskMetricData holds data for a single disk metric
type DiskMetricData struct {
	MountPoint  string
	UsedPercent string
	UsedGB      string
	TotalGB     string
	AvailGB     string
	AlertLevel  string
}

// SendDiskSpaceAlert sends disk space alert emails to all admin users
func (a *AdminAlerter) SendDiskSpaceAlert(ctx context.Context, data *DiskSpaceAlertData) error {
	if !a.email.Enabled() {
		log.Debug().Msg("Email not enabled, skipping admin disk space alert")
		return nil
	}

	// Get all admin users
	admins, _, err := a.store.ListUsers(ctx, &store.ListUsersQuery{Admin: true})
	if err != nil {
		return fmt.Errorf("failed to list admin users: %w", err)
	}

	if len(admins) == 0 {
		log.Warn().Msg("No admin users found to send disk space alert")
		return nil
	}

	// Build email content
	var buf bytes.Buffer
	err = diskSpaceAlertTmpl.Execute(&buf, data)
	if err != nil {
		return fmt.Errorf("failed to execute disk space alert template: %w", err)
	}

	subject := fmt.Sprintf("🚨 Disk Space Alert - %s (%s)", data.WolfName, data.AlertLevel)
	if data.AlertLevel == "warning" {
		subject = fmt.Sprintf("⚠️ Disk Space Warning - %s", data.WolfName)
	}

	// Send to each admin
	var lastErr error
	sentCount := 0
	for _, admin := range admins {
		if admin.Email == "" {
			continue
		}

		client := a.email.getClient(admin.Email)
		err = client.Send(ctx, subject, buf.String())
		if err != nil {
			log.Error().Err(err).Str("email", admin.Email).Msg("Failed to send disk space alert to admin")
			lastErr = err
			continue
		}

		log.Info().
			Str("email", admin.Email).
			Str("wolf_name", data.WolfName).
			Str("alert_level", data.AlertLevel).
			Msg("Disk space alert sent to admin")
		sentCount++
	}

	if sentCount == 0 && lastErr != nil {
		return fmt.Errorf("failed to send disk space alert to any admin: %w", lastErr)
	}

	log.Info().
		Int("sent_count", sentCount).
		Int("admin_count", len(admins)).
		Str("wolf_name", data.WolfName).
		Msg("Disk space alerts sent to admins")

	return nil
}

// SendWaitlistSignupAlert fires a Slack notification (in a background goroutine)
// when a new user signs up and is waitlisted. Safe to call from any goroutine.
func (a *AdminAlerter) SendWaitlistSignupAlert(user *types.User) {
	go a.sendWaitlistSignupAlert(user)
}

func (a *AdminAlerter) sendWaitlistSignupAlert(user *types.User) {
	if a.slack == nil {
		log.Debug().Msg("Slack not configured, skipping waitlist signup notification")
		return
	}

	msg := fmt.Sprintf("New beta signup: %s", user.Email)
	if user.FullName != "" {
		msg = fmt.Sprintf("New beta signup: %s (%s)", user.Email, user.FullName)
	}
	msg += " — waiting for approval"

	if err := a.slack.SendMessage("", msg); err != nil {
		log.Error().Err(err).Str("email", user.Email).Msg("Failed to send waitlist signup Slack notification")
	} else {
		log.Info().Str("email", user.Email).Msg("Waitlist signup Slack notification sent")
	}
}
