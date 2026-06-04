package streaming_test

import (
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

func TestNewSubscription(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		position orgchart.PositionID
		stream   streaming.StreamID
		ts       time.Time
		wantErr  bool
	}{
		{"valid", "p-1", "s-1", now, false},
		{"empty position", "", "s-1", now, true},
		{"empty stream", "p-1", "", now, true},
		{"zero time", "p-1", "s-1", time.Time{}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, err := streaming.NewSubscription(string(tc.position), tc.stream, tc.ts, "org-test")
			gotErr := err != nil
			if gotErr != tc.wantErr {
				t.Fatalf("streaming.NewSubscription error = %v, wantErr = %v", err, tc.wantErr)
			}
			if !gotErr && (s.PositionID != string(tc.position) || s.StreamID != tc.stream) {
				t.Fatalf("subscription = %+v", s)
			}
		})
	}
}
