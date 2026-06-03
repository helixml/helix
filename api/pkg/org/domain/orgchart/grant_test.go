package orgchart_test

import (
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

func TestNewToolGrant(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		id       orgchart.GrantID
		workerID orgchart.WorkerID
		toolName tool.Name
		wantErr  bool
	}{
		{"valid", "g-1", "w-1", "hire_worker", false},
		{"empty id", "", "w-1", "ping", true},
		{"empty worker", "g-1", "", "ping", true},
		{"empty tool", "g-1", "w-1", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, err := orgchart.NewToolGrant(tc.id, tc.workerID, tc.toolName, "org-test")
			gotErr := err != nil
			if gotErr != tc.wantErr {
				t.Fatalf("orgchart.NewToolGrant error = %v, wantErr = %v", err, tc.wantErr)
			}
			if !gotErr && g.ID != tc.id {
				t.Fatalf("orgchart.GrantID = %q, want %q", g.ID, tc.id)
			}
		})
	}
}
