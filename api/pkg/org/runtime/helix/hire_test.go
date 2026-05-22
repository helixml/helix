package helix

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/org/runtime"
	"github.com/helixml/helix/api/pkg/org/domain"
	"github.com/helixml/helix/api/pkg/org/store/sqlite"
)

func TestHireRecorderPersistsHiringUser(t *testing.T) {
	t.Parallel()
	st, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	w, _ := domain.NewAIWorker("w-alice", "p-x", "# Alice")
	if err := st.Workers.Create(context.Background(), w); err != nil {
		t.Fatalf("create worker: %v", err)
	}

	var h runtime.HireHook = &Hire{Store: st}
	if err := h.OnHire(context.Background(), w.ID(), "u-phil"); err != nil {
		t.Fatalf("OnHire: %v", err)
	}
	state, err := LoadState(context.Background(), st, w.ID())
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if state.HiringUserID != "u-phil" {
		t.Errorf("HiringUserID = %q, want u-phil", state.HiringUserID)
	}
}

func TestHireRecorderEmptyUserIDIsNoop(t *testing.T) {
	t.Parallel()
	st, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	w, _ := domain.NewAIWorker("w-alice", "p-x", "# Alice")
	if err := st.Workers.Create(context.Background(), w); err != nil {
		t.Fatalf("create worker: %v", err)
	}
	h := &Hire{Store: st}
	if err := h.OnHire(context.Background(), w.ID(), ""); err != nil {
		t.Errorf("OnHire with empty userID should be a no-op: %v", err)
	}
	state, _ := LoadState(context.Background(), st, w.ID())
	if state.HiringUserID != "" {
		t.Errorf("HiringUserID = %q, want empty", state.HiringUserID)
	}
}
