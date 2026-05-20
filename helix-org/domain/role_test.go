package domain

import (
	"testing"
	"time"
)

func TestNewRole(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name    string
		id      RoleID
		content string
		now     time.Time
		wantOK  bool
	}{
		{"valid", "r-ceo", "# CEO\nMakes calls.", now, true},
		{"empty id", "", "# CEO", now, false},
		{"empty content", "r-ceo", "", now, false},
		{"zero time", "r-ceo", "# CEO", time.Time{}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			role, err := NewRole(tc.id, tc.content, tc.now)
			gotOK := err == nil
			if gotOK != tc.wantOK {
				t.Fatalf("NewRole error = %v, wantOK = %v", err, tc.wantOK)
			}
			if gotOK {
				if role.ID != tc.id {
					t.Fatalf("role.ID = %q, want %q", role.ID, tc.id)
				}
				if role.Content != tc.content {
					t.Fatalf("role.Content = %q", role.Content)
				}
				if role.CreatedAt != tc.now || role.UpdatedAt != tc.now {
					t.Fatalf("timestamps not set: created=%v updated=%v", role.CreatedAt, role.UpdatedAt)
				}
			}
		})
	}
}
