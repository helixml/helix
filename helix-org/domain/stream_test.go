package domain

import (
	"testing"
	"time"
)

func TestNewStream(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		id      StreamID
		worker  WorkerID
		channel ChannelID
		ts      time.Time
		wantErr bool
	}{
		{"valid", "s-1", "w-1", "c-1", now, false},
		{"empty id", "", "w-1", "c-1", now, true},
		{"empty worker", "s-1", "", "c-1", now, true},
		{"empty channel", "s-1", "w-1", "", now, true},
		{"zero time", "s-1", "w-1", "c-1", time.Time{}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, err := NewStream(tc.id, tc.worker, tc.channel, tc.ts)
			gotErr := err != nil
			if gotErr != tc.wantErr {
				t.Fatalf("NewStream error = %v, wantErr = %v", err, tc.wantErr)
			}
			if !gotErr && (s.WorkerID != tc.worker || s.ChannelID != tc.channel) {
				t.Fatalf("stream = %+v", s)
			}
		})
	}
}
