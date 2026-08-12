package transport

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	cronv3 "github.com/robfig/cron/v3"
)

// KindCron is a self-driven schedule transport. The topic has no
// external producer; an in-process scheduler publishes a canonical
// "scheduled tick" event to it on the configured cadence. Every
// subscribed Worker is activated via the normal dispatcher fan-out.
//
// The scheduler lives in api/pkg/org/infrastructure/streamcron and
// uses the same gocron/v2 + robfig/cron stack as the existing
// app-trigger cron at api/pkg/trigger/cron.
const KindCron Kind = "cron"

// minCronInterval is the lower bound enforced on every cron-kind
// topic's schedule. Matches the existing app-cron limit at
// api/pkg/trigger/cron/trigger_cron.go (the 90s gap check). Prevents a
// single misconfigured topic from saturating the dispatcher / blowing
// up activation costs.
const minCronInterval = 90 * time.Second

// CronConfig carries the schedule and optional message for a KindCron topic. The UI
// always submits a standard 5-field cron expression (optionally
// prefixed by "CRON_TZ=<tz> " to pin the timezone, defaulting to UTC).
// Preset buttons in the UI inject the literal cron form so users
// never need to learn an alias syntax.
//
// Hand-edits via the API may additionally use the well-known
// descriptors that robfig/cron's ParseStandard recognises natively
// (@hourly, @daily, @weekly, @monthly, @yearly, @midnight) — we don't
// actively reject them, but the UI never produces them.
type CronConfig struct {
	Schedule string `json:"schedule"`
	Message  string `json:"message,omitempty"`
}

// Validate enforces that Schedule parses as a standard 5-field cron
// expression and that consecutive fires are at least minCronInterval
// apart. Returns errors that name the limit so the frontend can
// surface them verbatim.
func (c CronConfig) Validate() error {
	trimmed := strings.TrimSpace(c.Schedule)
	if trimmed == "" {
		return errors.New("schedule is required")
	}
	// ParseStandard expects the 5-field form. It does NOT honour
	// cron.SecondOptional, so per-second specs like "*/30 * * * * *"
	// are rejected as unparseable. It also rejects any @-alias —
	// this package no longer expands them; the UI presets supply the
	// literal cron form.
	sched, err := cronv3.ParseStandard(trimmed)
	if err != nil {
		return fmt.Errorf("invalid cron schedule %q: %w", c.Schedule, err)
	}
	// Use a fixed reference time so validation is deterministic and
	// reproducible across machines / clocks.
	ref := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	next := sched.Next(ref)
	second := sched.Next(next)
	if second.Sub(next) < minCronInterval {
		return fmt.Errorf("schedule %q fires more often than the %s minimum", c.Schedule, minCronInterval)
	}
	return nil
}

// CronConfig is the typed accessor for KindCron topics. Returns the
// zero value with no error when Config is empty (which Validate then
// rejects as "schedule is required"). Same accessor pattern as
// Transport.WebhookConfig() — see webhook.go.
func (t Transport) CronConfig() (CronConfig, error) {
	if t.Kind != KindCron {
		return CronConfig{}, fmt.Errorf("transport kind is %q, not cron", t.Kind)
	}
	return parseCronConfig(t.Config)
}

// cron is the Strategy for KindCron.
type cron struct{}

// ParseConfig satisfies Strategy. Delegates to the typed parser so the
// umbrella Transport.CronConfig() accessor and Strategy dispatch share
// one implementation.
func (cron) ParseConfig(raw json.RawMessage) (Config, error) {
	c, err := parseCronConfig(raw)
	return c, err
}

// parseCronConfig accepts either an explicit JSON object
// ({"schedule": "..."}) or a bare JSON string ("..." — convenient for
// the CLI). An empty blob round-trips to a zero CronConfig — Validate
// then surfaces the missing-schedule error.
func parseCronConfig(raw json.RawMessage) (CronConfig, error) {
	if len(raw) == 0 {
		return CronConfig{}, nil
	}
	var c CronConfig
	if err := json.Unmarshal(raw, &c); err != nil {
		// Allow a bare JSON string for ergonomics: `"0 9 * * 1"`.
		var s string
		if err2 := json.Unmarshal(raw, &s); err2 == nil {
			return CronConfig{Schedule: s}, nil
		}
		return CronConfig{}, fmt.Errorf("parse cron config: %w", err)
	}
	return c, nil
}
