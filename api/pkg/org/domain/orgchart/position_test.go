package orgchart_test

import (
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
)

func positionID(s string) *orgchart.PositionID {
	p := orgchart.PositionID(s)
	return &p
}

func TestNewPosition(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		id       orgchart.PositionID
		roleID   orgchart.RoleID
		parentID *orgchart.PositionID
		wantErr  bool
	}{
		{"root", "p-root", "r-owner", nil, false},
		{"child", "p-ceo", "r-ceo", positionID("p-root"), false},
		{"empty id", "", "r-ceo", nil, true},
		{"empty role id", "p-ceo", "", nil, true},
		{"empty parent", "p-ceo", "r-ceo", positionID(""), true},
		{"self as parent", "p-ceo", "r-ceo", positionID("p-ceo"), true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pos, err := orgchart.NewPosition(tc.id, tc.roleID, tc.parentID, "org-test")
			gotErr := err != nil
			if gotErr != tc.wantErr {
				t.Fatalf("orgchart.NewPosition error = %v, wantErr = %v", err, tc.wantErr)
			}
			if !gotErr && pos.ID != tc.id {
				t.Fatalf("pos.ID = %q, want %q", pos.ID, tc.id)
			}
			if !gotErr && tc.parentID == nil && !pos.IsRoot() {
				t.Fatalf("expected root position")
			}
		})
	}
}

func TestNewPositionParentIsCopied(t *testing.T) {
	t.Parallel()

	parent := orgchart.PositionID("p-root")
	pos, err := orgchart.NewPosition("p-ceo", "r-ceo", &parent, "org-test")
	if err != nil {
		t.Fatalf("orgchart.NewPosition: %v", err)
	}
	parent = "mutated"
	if *pos.ParentID != "p-root" {
		t.Fatalf("pos.ParentID = %q, expected caller mutation not to leak", *pos.ParentID)
	}
}
