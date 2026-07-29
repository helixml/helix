package notification

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

//go:embed templates/webservice_down_alert.html
var webServiceDownAlertTemplate string

var webServiceDownAlertTmpl = template.Must(template.New("webServiceDownAlert").Parse(webServiceDownAlertTemplate))

// SendWebServiceDownAlert pages admins that a hosted customer site is down.
//
// This exists because a Prometheus rule alone was not enough: during the
// we-find.ai outage (2026-07-28) the site served 503 for five days while the
// only in-code signal was a log line, and the metric the alerting stack watched
// flapped between up and down on every auto-recovery attempt — so the page
// resolved itself repeatedly and nobody was told. This path does not depend on
// scraping, Prometheus, or Alertmanager being correctly wired.
//
// Sent in a background goroutine; safe to call from any goroutine.
func (a *AdminAlerter) SendWebServiceDownAlert(ctx context.Context, data *types.WebServiceDownAlert) {
	go a.sendWebServiceAlert(ctx, data, true)
}

// SendWebServiceRecoveredAlert closes the loop on a previously-sent down alert,
// so an operator knows the incident ended without going to look.
func (a *AdminAlerter) SendWebServiceRecoveredAlert(ctx context.Context, data *types.WebServiceDownAlert) {
	go a.sendWebServiceAlert(ctx, data, false)
}

func (a *AdminAlerter) sendWebServiceAlert(ctx context.Context, data *types.WebServiceDownAlert, down bool) {
	if data == nil {
		return
	}
	delivered := false
	if a.sendWebServiceSlack(data, down) {
		delivered = true
	}
	if a.sendWebServiceEmail(ctx, data, down) {
		delivered = true
	}
	if !delivered && down {
		// Deliberately loud. A down alert that reached no channel is the exact
		// failure this whole change exists to prevent, so it must not be a
		// debug-level shrug.
		log.Error().
			Str("project_id", data.ProjectID).
			Msg("hosted web service is DOWN but no alert channel is configured (set JANITOR_SLACK_WEBHOOK_URL and/or SMTP + admin users) — nobody was paged")
	}
}

func (a *AdminAlerter) sendWebServiceSlack(data *types.WebServiceDownAlert, down bool) bool {
	if a.slack == nil {
		return false
	}
	if err := a.slack.SendMessage("", webServiceSlackMessage(data, down)); err != nil {
		log.Error().Err(err).Str("project_id", data.ProjectID).
			Msg("Failed to send web service alert to Slack")
		return false
	}
	log.Info().Str("project_id", data.ProjectID).Bool("down", down).
		Msg("Web service alert sent to Slack")
	return true
}

// webServiceSlackMessage renders the page. It leads with the customer-facing
// domains, because the first question an operator asks is "which site?".
func webServiceSlackMessage(data *types.WebServiceDownAlert, down bool) string {
	name := data.ProjectName
	if name == "" {
		name = data.ProjectID
	}
	domains := strings.Join(data.Domains, ", ")
	if domains == "" {
		domains = "(no domains configured)"
	}

	if !down {
		return fmt.Sprintf("✅ Hosted web service RECOVERED — %s (%s)\nDomains: %s\nWas down for: %s",
			name, data.ProjectID, domains, data.DownFor.Round(time.Second))
	}

	var b strings.Builder
	fmt.Fprintf(&b, "🚨 Hosted web service DOWN — %s (%s)\n", name, data.ProjectID)
	fmt.Fprintf(&b, "Domains: %s\n", domains)
	fmt.Fprintf(&b, "Down for: %s\n", data.DownFor.Round(time.Second))
	if data.ConsecutiveRecoveryFailures > 0 {
		fmt.Fprintf(&b, "Consecutive failed auto-recoveries: %d — auto-recovery cannot fix this on its own\n",
			data.ConsecutiveRecoveryFailures)
	}
	if data.DeployError != "" {
		fmt.Fprintf(&b, "Last deploy error: %s\n", data.DeployError)
	}
	if data.DeployLogURL != "" {
		fmt.Fprintf(&b, "Deploy log: %s", data.DeployLogURL)
	}
	return b.String()
}

func (a *AdminAlerter) sendWebServiceEmail(ctx context.Context, data *types.WebServiceDownAlert, down bool) bool {
	if !a.email.Enabled() {
		return false
	}
	admins, _, err := a.store.ListUsers(ctx, &store.ListUsersQuery{Admin: true})
	if err != nil {
		log.Error().Err(err).Msg("Failed to list admin users for web service alert")
		return false
	}

	name := data.ProjectName
	if name == "" {
		name = data.ProjectID
	}
	subject := fmt.Sprintf("🚨 Web service DOWN - %s", name)
	if !down {
		subject = fmt.Sprintf("✅ Web service recovered - %s", name)
	}

	var buf bytes.Buffer
	if err := webServiceDownAlertTmpl.Execute(&buf, struct {
		*types.WebServiceDownAlert
		Down        bool
		DisplayName string
		DownForText string
		DomainList  string
	}{
		WebServiceDownAlert: data,
		Down:                down,
		DisplayName:         name,
		DownForText:         data.DownFor.Round(time.Second).String(),
		DomainList:          strings.Join(data.Domains, ", "),
	}); err != nil {
		log.Error().Err(err).Msg("Failed to render web service alert email")
		return false
	}

	sent := 0
	for _, admin := range admins {
		if admin.Email == "" {
			continue
		}
		if err := a.email.getClient(admin.Email).Send(ctx, subject, buf.String()); err != nil {
			log.Error().Err(err).Str("email", admin.Email).Msg("Failed to send web service alert to admin")
			continue
		}
		sent++
	}
	if sent == 0 {
		return false
	}
	log.Info().Int("sent_count", sent).Str("project_id", data.ProjectID).Bool("down", down).
		Msg("Web service alert emailed to admins")
	return true
}
