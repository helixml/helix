package domain

import (
	"testing"
	"time"
)

func TestNewChannel(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		id        ChannelID
		chName    string
		createdBy WorkerID
		createdAt time.Time
		wantErr   bool
	}{
		{"valid", "c-1", "general", "w-owner", now, false},
		{"empty id", "", "general", "w-owner", now, true},
		{"empty name", "c-1", "", "w-owner", now, true},
		{"empty createdBy", "c-1", "general", "", now, true},
		{"zero createdAt", "c-1", "general", "w-owner", time.Time{}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ch, err := NewChannel(tc.id, tc.chName, "desc", tc.createdBy, tc.createdAt)
			gotErr := err != nil
			if gotErr != tc.wantErr {
				t.Fatalf("NewChannel error = %v, wantErr = %v", err, tc.wantErr)
			}
			if !gotErr {
				if ch.ID != tc.id {
					t.Fatalf("ID = %q, want %q", ch.ID, tc.id)
				}
				if !ch.CreatedAt.Equal(tc.createdAt) {
					t.Fatalf("CreatedAt = %v, want %v", ch.CreatedAt, tc.createdAt)
				}
			}
		})
	}
}

func TestNewChannelNormalisesTimezone(t *testing.T) {
	t.Parallel()
	loc := time.FixedZone("UTC+5", 5*3600)
	ts := time.Date(2026, 4, 24, 17, 0, 0, 0, loc)
	ch, err := NewChannel("c-1", "general", "", "w-owner", ts)
	if err != nil {
		t.Fatalf("NewChannel: %v", err)
	}
	if ch.CreatedAt.Location() != time.UTC {
		t.Fatalf("CreatedAt location = %v, want UTC", ch.CreatedAt.Location())
	}
}
