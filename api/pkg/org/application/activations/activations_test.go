package activations

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

func TestPrepareManual_CreatesRow(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := New(Deps{
		Repo:  st.Activations,
		Now:   func() time.Time { return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC) },
		NewID: func() string { return "fixed" },
	})
	id, err := svc.PrepareManual(context.Background(), "org-test", "w-mark")
	if err != nil {
		t.Fatalf("PrepareManual: %v", err)
	}
	if id != "a-fixed" {
		t.Fatalf("id = %q, want a-fixed", id)
	}
	got, err := st.Activations.Get(context.Background(), "org-test", id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("activation row not persisted")
	}
}

func TestPrepareManual_NoRepoIsNoOp(t *testing.T) {
	t.Parallel()
	svc := New(Deps{NewID: func() string { return "x" }}) // no Repo
	id, err := svc.PrepareManual(context.Background(), "org-test", "w-mark")
	if err != nil {
		t.Fatalf("PrepareManual: %v", err)
	}
	if id != "" {
		t.Fatalf("id = %q, want empty (no-op)", id)
	}
}
