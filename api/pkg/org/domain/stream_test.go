package domain

import (
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/stream"
	"github.com/helixml/helix/api/pkg/org/transport"
	"github.com/helixml/helix/api/pkg/org/worker"
)

func TestNewStream(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		id        stream.ID
		stName    string
		createdBy worker.ID
		createdAt time.Time
		wantErr   bool
	}{
		{"valid", "s-1", "general", "w-owner", now, false},
		{"empty id", "", "general", "w-owner", now, true},
		{"empty name", "s-1", "", "w-owner", now, true},
		{"empty createdBy", "s-1", "general", "", now, true},
		{"zero createdAt", "s-1", "general", "w-owner", time.Time{}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, err := NewStream(tc.id, tc.stName, "desc", tc.createdBy, tc.createdAt, transport.Transport{})
			gotErr := err != nil
			if gotErr != tc.wantErr {
				t.Fatalf("NewStream error = %v, wantErr = %v", err, tc.wantErr)
			}
			if !gotErr {
				if s.ID != tc.id {
					t.Fatalf("ID = %q, want %q", s.ID, tc.id)
				}
				if !s.CreatedAt.Equal(tc.createdAt) {
					t.Fatalf("CreatedAt = %v, want %v", s.CreatedAt, tc.createdAt)
				}
				if s.Transport.Kind != transport.KindLocal {
					t.Fatalf("Transport.Kind = %q, want %q", s.Transport.Kind, transport.KindLocal)
				}
			}
		})
	}
}

func TestNewStreamNormalisesTimezone(t *testing.T) {
	t.Parallel()
	loc := time.FixedZone("UTC+5", 5*3600)
	ts := time.Date(2026, 4, 24, 17, 0, 0, 0, loc)
	s, err := NewStream("s-1", "general", "", "w-owner", ts, transport.Transport{})
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	if s.CreatedAt.Location() != time.UTC {
		t.Fatalf("CreatedAt location = %v, want UTC", s.CreatedAt.Location())
	}
}

func TestNewStreamRejectsUnknownTransport(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	_, err := NewStream("s-1", "general", "", "w-owner", now, transport.Transport{Kind: "bogus"})
	if err == nil {
		t.Fatal("NewStream with unknown transport: want error, got nil")
	}
}
