package domain

import (
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/event"
	"github.com/helixml/helix/api/pkg/org/stream"
	"github.com/helixml/helix/api/pkg/org/worker"
)

func TestNewEvent(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		id      event.ID
		stream  stream.ID
		source  worker.ID
		body    string
		ts      time.Time
		wantErr bool
	}{
		{"valid worker event", "e-1", "s-1", "w-1", "hello", now, false},
		{"valid system event", "e-1", "s-1", "", "it is 9am monday", now, false},
		{"empty id", "", "s-1", "w-1", "hello", now, true},
		{"empty stream", "e-1", "", "w-1", "hello", now, true},
		{"empty body", "e-1", "s-1", "w-1", "", now, true},
		{"zero time", "e-1", "s-1", "w-1", "hello", time.Time{}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e, err := NewEvent(tc.id, tc.stream, tc.source, tc.body, tc.ts, "org-test")
			gotErr := err != nil
			if gotErr != tc.wantErr {
				t.Fatalf("NewEvent error = %v, wantErr = %v", err, tc.wantErr)
			}
			if !gotErr && e.Body != tc.body {
				t.Fatalf("body = %q", e.Body)
			}
		})
	}
}
