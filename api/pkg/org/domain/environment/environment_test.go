package environment_test

import (
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/environment"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
)

func TestNewEnvironment(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		worker  orgchart.WorkerID
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
			env, err := environment.New(tc.worker, tc.path, tc.ts, "org-test")
			gotErr := err != nil
			if gotErr != tc.wantErr {
				t.Fatalf("New error = %v, wantErr = %v", err, tc.wantErr)
			}
			if !gotErr && (env.WorkerID != tc.worker || env.Path != tc.path) {
				t.Fatalf("env = %+v", env)
			}
		})
	}
}
