package helix

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
)

func TestHireRecorderPersistsHiringUser(t *testing.T) {
	t.Parallel()
	st := orggorm.GetOrgTestDB(t)
	b, _ := orgchart.NewBot("b-alice", "# Alice", nil, nil, time.Now().UTC(), "org-test")
	if err := st.Bots.Create(context.Background(), b); err != nil {
		t.Fatalf("create bot: %v", err)
	}

	var h runtime.HireHook = &Hire{Store: st}
	if err := h.OnHire(context.Background(), "org-test", b.ID, "u-phil"); err != nil {
		t.Fatalf("OnHire: %v", err)
	}
	state, err := LoadState(context.Background(), st, "org-test", b.ID)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if state.HiringUserID != "u-phil" {
		t.Errorf("HiringUserID = %q, want u-phil", state.HiringUserID)
	}
}

func TestHireRecorderEmptyUserIDIsNoop(t *testing.T) {
	t.Parallel()
	st := orggorm.GetOrgTestDB(t)
	b, _ := orgchart.NewBot("b-alice", "# Alice", nil, nil, time.Now().UTC(), "org-test")
	if err := st.Bots.Create(context.Background(), b); err != nil {
		t.Fatalf("create bot: %v", err)
	}
	h := &Hire{Store: st}
	if err := h.OnHire(context.Background(), "org-test", b.ID, ""); err != nil {
		t.Errorf("OnHire with empty userID should be a no-op: %v", err)
	}
	state, _ := LoadState(context.Background(), st, "org-test", b.ID)
	if state.HiringUserID != "" {
		t.Errorf("HiringUserID = %q, want empty", state.HiringUserID)
	}
}
