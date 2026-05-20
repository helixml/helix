package domain

import "testing"

func TestNewToolGrant(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		id       GrantID
		workerID WorkerID
		toolName ToolName
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
			g, err := NewToolGrant(tc.id, tc.workerID, tc.toolName)
			gotErr := err != nil
			if gotErr != tc.wantErr {
				t.Fatalf("NewToolGrant error = %v, wantErr = %v", err, tc.wantErr)
			}
			if !gotErr && g.ID != tc.id {
				t.Fatalf("grant.ID = %q, want %q", g.ID, tc.id)
			}
		})
	}
}
