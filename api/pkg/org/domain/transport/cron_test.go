package transport_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

// TestCronConfig_Validate_PassesStandardDescriptors pins the reality
// that the underlying robfig/cron ParseStandard accepts the well-known
// @-descriptors. The UI never submits them (it injects literal 5-field
// cron through the preset buttons) but they remain valid for
// hand-edits via the API. Custom @-tokens we used to expand
// (@weekdays, @weekends) are NOT in robfig's table and are rejected by
// the next test — that's the difference between standard descriptors
// (still supported) and our removed alias layer (gone).
func TestCronConfig_Validate_PassesStandardDescriptors(t *testing.T) {
	t.Parallel()

	standard := []string{"@hourly", "@daily", "@midnight", "@weekly", "@monthly", "@yearly", "@annually"}
	for _, d := range standard {
		d := d
		t.Run(d, func(t *testing.T) {
			t.Parallel()
			if err := (transport.CronConfig{Schedule: d}).Validate(); err != nil {
				t.Fatalf("Validate(%q) = %v, want nil — robfig/cron's ParseStandard supports this natively", d, err)
			}
		})
	}
}

// TestCronConfig_Validate_RejectsRemovedAliases pins that the custom
// @-aliases the old expansion layer recognised (@weekdays, @weekends)
// are no longer accepted — they were our addition, not robfig's, and
// removing the expansion layer dropped them. Tested so a future "let's
// add aliases back" refactor doesn't silently re-enable just these two.
func TestCronConfig_Validate_RejectsRemovedAliases(t *testing.T) {
	t.Parallel()

	for _, alias := range []string{"@weekdays", "@weekends"} {
		alias := alias
		t.Run(alias, func(t *testing.T) {
			t.Parallel()
			if err := (transport.CronConfig{Schedule: alias}).Validate(); err == nil {
				t.Fatalf("Validate(%q) = nil, want error rejecting removed alias", alias)
			}
		})
	}
}

func TestCronConfig_Validate_OK(t *testing.T) {
	t.Parallel()

	cases := []string{
		"0 9 * * 1",                             // 9am Monday
		"0 18 * * 5",                            // 6pm Friday
		"0 0 * * 0,6",                           // weekends midnight
		"0 0 * * 1-5",                           // weekdays midnight
		"0 0 * * *",                             // daily midnight
		"0 * * * *",                             // hourly
		"*/15 * * * *",                          // every 15 min, ≥ 90s
		"CRON_TZ=Europe/London 0 9 * * 1",       // explicit timezone
		"CRON_TZ=America/New_York */15 * * * *", // every 15 min in a zone
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
		{"whitespace only", "   "},
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
	}{
		{"object form", `{"schedule":"0 9 * * 1"}`},
		{"bare string", `"0 9 * * 1"`},
		{"object form with timezone", `{"schedule":"CRON_TZ=UTC 0 9 * * 1"}`},
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

func TestCronConfigParseMessage(t *testing.T) {
	t.Parallel()

	tr := transport.Transport{
		Kind:   transport.KindCron,
		Config: json.RawMessage(`{"schedule":"0 9 * * 1","message":"Prepare the report"}`),
	}
	cfg, err := tr.CronConfig()
	if err != nil {
		t.Fatalf("CronConfig() = %v, want nil", err)
	}
	if cfg.Message != "Prepare the report" {
		t.Fatalf("Message = %q, want configured message", cfg.Message)
	}
}
