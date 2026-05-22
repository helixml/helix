package domain

import (
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/stream"
	"github.com/helixml/helix/api/pkg/org/worker"
)

func TestNewSubscription(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		worker  worker.ID
		stream  stream.ID
		ts      time.Time
		wantErr bool
	}{
		{"valid", "w-1", "s-1", now, false},
		{"empty worker", "", "s-1", now, true},
		{"empty stream", "w-1", "", now, true},
		{"zero time", "w-1", "s-1", time.Time{}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, err := NewSubscription(tc.worker, tc.stream, tc.ts)
			gotErr := err != nil
			if gotErr != tc.wantErr {
				t.Fatalf("NewSubscription error = %v, wantErr = %v", err, tc.wantErr)
			}
			if !gotErr && (s.WorkerID != tc.worker || s.StreamID != tc.stream) {
				t.Fatalf("subscription = %+v", s)
			}
		})
	}
}
