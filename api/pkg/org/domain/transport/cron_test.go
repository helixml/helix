package transport_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

// TestCronConfig_Validate_Aliases verifies that every documented alias
// expands to a parseable, ≥ 90s-interval cron expression. New aliases
// added to the map should pick this up automatically when listed here.
func TestCronConfig_Validate_Aliases(t *testing.T) {
	t.Parallel()

	aliases := []string{
		"@hourly",
		"@daily",
		"@midnight",
		"@weekly",
		"@weekdays",
		"@weekends",
		"@monthly",
		"@yearly",
		"@annually",
	}
	for _, alias := range aliases {
		alias := alias
		t.Run(alias, func(t *testing.T) {
			t.Parallel()
			c := transport.CronConfig{Schedule: alias}
			if err := c.Validate(); err != nil {
				t.Fatalf("Validate(%q) = %v, want nil", alias, err)
			}
		})
	}
}

func TestCronConfig_Validate_OK(t *testing.T) {
	t.Parallel()

	cases := []string{
		"0 9 * * 1",                            // 9am Monday
		"0 18 * * 5",                           // 6pm Friday
		"0 0 * * 0,6",                          // weekends midnight
		"0 0 * * 1-5",                          // weekdays midnight
		"CRON_TZ=Europe/London 0 9 * * 1",      // explicit timezone
		"CRON_TZ=America/New_York */15 * * * *", // every 15 min, ≥ 90s
	}
	for _, sched := range cases {
		sched := sched
		t.Run(sched, func(t *testing.T) {
			t.Parallel()
			c := transport.CronConfig{Schedule: sched}
			if err := c.Validate(); err != nil {
				t.Fatalf("Validate(%q) = %v, want nil", sched, err)
			}
		})
	}
}

// TestCronConfig_Validate_RejectsSubMinimumIntervals proves the DoS
// floor is enforced. Every-minute is the canonical sub-90s case in
// standard cron syntax; per-second specs are caught earlier by
// ParseStandard refusing 6-field input.
func TestCronConfig_Validate_RejectsSubMinimumIntervals(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		schedule string
		wantErr  string
	}{
		{"every minute", "* * * * *", "more often than the 1m30s minimum"},
		{"every minute via */1", "*/1 * * * *", "more often than the 1m30s minimum"},
		{"per-second 6-field", "*/30 * * * * *", "invalid cron schedule"},
		{"per-second with explicit second", "30 * * * * *", "invalid cron schedule"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := (transport.CronConfig{Schedule: tc.schedule}).Validate()
			if err == nil {
				t.Fatalf("Validate(%q) = nil, want error containing %q", tc.schedule, tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate(%q) = %q, want substring %q", tc.schedule, err, tc.wantErr)
			}
		})
	}
}

func TestCronConfig_Validate_RejectsMalformed(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		schedule string
	}{
		{"empty", ""},
		{"gibberish", "not a cron"},
		{"too few fields", "0 9 *"},
		{"unknown alias", "@never"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := (transport.CronConfig{Schedule: tc.schedule}).Validate()
			if err == nil {
				t.Fatalf("Validate(%q) = nil, want error", tc.schedule)
			}
		})
	}
}

// TestCronConfigParse accepts both the object form and a bare string
// (the latter for CLI ergonomics). An empty blob round-trips to a
// CronConfig that fails Validate — the missing-schedule error is the
// surface, not a parse-time crash.
func TestCronConfigParse(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"object form", `{"schedule":"0 9 * * 1"}`, "0 9 * * 1"},
		{"bare string", `"0 9 * * 1"`, "0 9 * * 1"},
		{"object form with timezone", `{"schedule":"CRON_TZ=UTC 0 9 * * 1"}`, "CRON_TZ=UTC 0 9 * * 1"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tr := transport.Transport{Kind: transport.KindCron, Config: json.RawMessage(tc.raw)}
			if err := tr.Validate(); err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}

func TestExpandCronSchedule(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in, want string
	}{
		{"@daily", "0 0 * * *"},
		{"@weekly", "0 0 * * 0"},
		{"@weekdays", "0 0 * * 1-5"},
		{"@weekends", "0 0 * * 0,6"},
		{"  @hourly  ", "0 * * * *"},
		{"0 9 * * 1", "0 9 * * 1"}, // standard cron passes through
		{"CRON_TZ=UTC 0 9 * * 1", "CRON_TZ=UTC 0 9 * * 1"},
		{"@unknown", "@unknown"}, // unrecognised alias passes through unchanged (validator rejects)
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			got := transport.ExpandCronSchedule(tc.in)
			if got != tc.want {
				t.Fatalf("ExpandCronSchedule(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
