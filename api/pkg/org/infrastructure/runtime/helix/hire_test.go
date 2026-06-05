package helix

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
)

func TestHireRecorderPersistsHiringUser(t *testing.T) {
	t.Parallel()
	st := orggorm.GetOrgTestDB(t)
	w, _ := orgchart.NewAIWorker("w-alice", "r-x", nil, "# Alice", "org-test")
	if err := st.Workers.Create(context.Background(), w); err != nil {
		t.Fatalf("create worker: %v", err)
	}

	var h runtime.HireHook = &Hire{Store: st}
	if err := h.OnHire(context.Background(), "org-test", w.ID(), "u-phil"); err != nil {
		t.Fatalf("OnHire: %v", err)
	}
	state, err := LoadState(context.Background(), st, "org-test", w.ID())
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
	w, _ := orgchart.NewAIWorker("w-alice", "r-x", nil, "# Alice", "org-test")
	if err := st.Workers.Create(context.Background(), w); err != nil {
		t.Fatalf("create worker: %v", err)
	}
	h := &Hire{Store: st}
	if err := h.OnHire(context.Background(), "org-test", w.ID(), ""); err != nil {
		t.Errorf("OnHire with empty userID should be a no-op: %v", err)
	}
	state, _ := LoadState(context.Background(), st, "org-test", w.ID())
	if state.HiringUserID != "" {
		t.Errorf("HiringUserID = %q, want empty", state.HiringUserID)
	}
}
