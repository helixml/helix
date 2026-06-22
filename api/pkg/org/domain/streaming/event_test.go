package streaming_test

import (
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

func TestNewEvent(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		id      streaming.EventID
		topic  streaming.TopicID
		source  orgchart.WorkerID
		body    string
		ts      time.Time
		wantErr bool
	}{
		{"valid worker event", "e-1", "s-1", "w-1", "hello", now, false},
		{"valid system event", "e-1", "s-1", "", "it is 9am monday", now, false},
		{"empty id", "", "s-1", "w-1", "hello", now, true},
		{"empty topic", "e-1", "", "w-1", "hello", now, true},
		{"empty body", "e-1", "s-1", "w-1", "", now, true},
		{"zero time", "e-1", "s-1", "w-1", "hello", time.Time{}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e, err := streaming.NewEvent(tc.id, tc.topic, tc.source, tc.body, tc.ts, "org-test")
			gotErr := err != nil
			if gotErr != tc.wantErr {
				t.Fatalf("streaming.NewEvent error = %v, wantErr = %v", err, tc.wantErr)
			}
			if !gotErr && e.Body != tc.body {
				t.Fatalf("body = %q", e.Body)
			}
		})
	}
}
