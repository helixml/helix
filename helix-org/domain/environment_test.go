package domain

import (
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/worker"
)

func TestNewEnvironment(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		worker  worker.ID
		path    string
		ts      time.Time
		wantErr bool
	}{
		{"valid", "w-1", "/srv/env/w-1", now, false},
		{"empty worker", "", "/srv/env/w-1", now, true},
		{"empty path", "w-1", "", now, true},
		{"zero time", "w-1", "/srv/env/w-1", time.Time{}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env, err := NewEnvironment(tc.worker, tc.path, tc.ts)
			gotErr := err != nil
			if gotErr != tc.wantErr {
				t.Fatalf("NewEnvironment error = %v, wantErr = %v", err, tc.wantErr)
			}
			if !gotErr && (env.WorkerID != tc.worker || env.Path != tc.path) {
				t.Fatalf("env = %+v", env)
			}
		})
	}
}
