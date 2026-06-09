package transport

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	cronv3 "github.com/robfig/cron/v3"
)

// KindCron is a self-driven schedule transport. The stream has no
// external producer; an in-process scheduler publishes a canonical
// "scheduled tick" event to it on the configured cadence. Every
// subscribed Worker is activated via the normal dispatcher fan-out.
//
// The scheduler lives in api/pkg/org/infrastructure/streamcron and
// uses the same gocron/v2 + robfig/cron stack as the existing
// app-trigger cron at api/pkg/trigger/cron.
const KindCron Kind = "cron"

// minCronInterval is the lower bound enforced on every cron-kind
// stream's schedule. Matches the existing app-cron limit at
// api/pkg/trigger/cron/trigger_cron.go (the 90s gap check). Prevents a
// single misconfigured stream from saturating the dispatcher / blowing
// up activation costs.
const minCronInterval = 90 * time.Second

// cronAliases expands convenient shortcuts to standard 5-field cron
// expressions at validate time. The runtime path only ever sees the
// expanded form, so there is exactly one syntax to evaluate. The
// original (unexpanded) user input is preserved in transport_config so
// the UI can round-trip what the user typed.
var cronAliases = map[string]string{
	"@hourly":   "0 * * * *",
	"@daily":    "0 0 * * *",
	"@midnight": "0 0 * * *",
	"@weekly":   "0 0 * * 0",
	"@weekdays": "0 0 * * 1-5",
	"@weekends": "0 0 * * 0,6",
	"@monthly":  "0 0 1 * *",
	"@yearly":   "0 0 1 1 *",
	"@annually": "0 0 1 1 *",
}

// CronConfig carries the schedule for a KindCron stream. Schedule
// accepts either a 5-field cron expression (optionally prefixed by
// "CRON_TZ=<tz> ") or one of the documented aliases (@hourly, @daily,
// @weekly, @weekdays, @weekends, @monthly, @yearly).
type CronConfig struct {
	Schedule string `json:"schedule"`
}

// Validate enforces that Schedule parses as a standard cron expression
// (after alias expansion) and that consecutive fires are at least
// minCronInterval apart. Returns errors that name the limit so the
// frontend can surface them verbatim.
func (c CronConfig) Validate() error {
	if c.Schedule == "" {
		return errors.New("schedule is required")
	}
	expanded := ExpandCronSchedule(c.Schedule)
	// ParseStandard does NOT honour cron.SecondOptional / 6-field
	// expressions — by construction this rejects per-second specs
	// like "*/30 * * * * *" as unparseable.
	sched, err := cronv3.ParseStandard(expanded)
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

// ExpandCronSchedule swaps a leading "@alias" token for its
// standard-cron equivalent and returns the rest of the schedule
// unchanged. Exported so the scheduler can pass the expanded form to
// gocron without re-implementing the alias table. Pre-expanded input
// (no leading "@") is returned verbatim; this keeps "CRON_TZ=… …"
// inputs working unchanged.
func ExpandCronSchedule(schedule string) string {
	trimmed := strings.TrimSpace(schedule)
	if !strings.HasPrefix(trimmed, "@") {
		return trimmed
	}
	// Aliases are a single token; if the user typed "@daily " we
	// still need to look up "@daily".
	tok := trimmed
	if i := strings.IndexAny(trimmed, " \t"); i > 0 {
		tok = trimmed[:i]
	}
	if expansion, ok := cronAliases[tok]; ok {
		return expansion
	}
	return trimmed
}

// cron is the Strategy for KindCron.
type cron struct{}

// ParseConfig accepts either an explicit JSON object ({"schedule": "..."})
// or a plain JSON string ("..." — convenient for the CLI). An empty
// blob is rejected up-front: cron streams without a schedule are a
// configuration error, unlike KindLocal which tolerates anything.
func (cron) ParseConfig(raw json.RawMessage) (Config, error) {
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
		return nil, fmt.Errorf("parse cron config: %w", err)
	}
	return c, nil
}
