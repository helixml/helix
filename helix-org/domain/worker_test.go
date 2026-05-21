package domain

import (
	"testing"

	"github.com/helixml/helix/api/pkg/org/position"
	"github.com/helixml/helix/api/pkg/org/worker"
)

func TestNewHumanWorker(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		id        worker.ID
		positions []position.ID
		identity  string
		wantErr   bool
	}{
		{"valid", "w-1", []position.ID{"p-ceo"}, "i am the ceo", false},
		{"valid empty identity", "w-1", []position.ID{"p-ceo"}, "", false},
		{"empty id", "", []position.ID{"p-ceo"}, "", true},
		{"no positions (vacated)", "w-1", nil, "", false},
		{"empty position id", "w-1", []position.ID{""}, "", true},
		{"duplicate positions", "w-1", []position.ID{"p-ceo", "p-ceo"}, "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w, err := NewHumanWorker(tc.id, tc.positions, tc.identity)
			gotErr := err != nil
			if gotErr != tc.wantErr {
				t.Fatalf("NewHumanWorker error = %v, wantErr = %v", err, tc.wantErr)
			}
			if !gotErr {
				if w.Kind() != worker.KindHuman {
					t.Fatalf("Kind = %q, want human", w.Kind())
				}
				if w.ID() != tc.id {
					t.Fatalf("ID = %q, want %q", w.ID(), tc.id)
				}
				if w.IdentityContent() != tc.identity {
					t.Fatalf("IdentityContent = %q, want %q", w.IdentityContent(), tc.identity)
				}
			}
		})
	}
}

func TestNewAIWorker(t *testing.T) {
	t.Parallel()

	w, err := NewAIWorker("w-ai", []position.ID{"p-docs"}, "you are the docs editor")
	if err != nil {
		t.Fatalf("NewAIWorker: %v", err)
	}
	if w.Kind() != worker.KindAI {
		t.Fatalf("Kind = %q, want ai", w.Kind())
	}
	if got := w.Positions(); len(got) != 1 || got[0] != "p-docs" {
		t.Fatalf("Positions = %v, want [p-docs]", got)
	}
	if w.IdentityContent() != "you are the docs editor" {
		t.Fatalf("IdentityContent = %q", w.IdentityContent())
	}
}

func TestWorkerWithIdentityContent(t *testing.T) {
	t.Parallel()

	w, err := NewAIWorker("w-1", []position.ID{"p-1"}, "old")
	if err != nil {
		t.Fatalf("NewAIWorker: %v", err)
	}
	updated := w.WithIdentityContent("new")
	if w.IdentityContent() != "old" {
		t.Fatalf("original mutated: %q", w.IdentityContent())
	}
	if updated.IdentityContent() != "new" {
		t.Fatalf("updated identity = %q, want %q", updated.IdentityContent(), "new")
	}
	if updated.ID() != w.ID() {
		t.Fatalf("ID changed: %q vs %q", updated.ID(), w.ID())
	}
	if updated.Kind() != w.Kind() {
		t.Fatalf("Kind changed: %q vs %q", updated.Kind(), w.Kind())
	}
}

func TestWorkerPositionsIsolation(t *testing.T) {
	t.Parallel()

	positions := []position.ID{"p-ceo"}
	w, err := NewHumanWorker("w-1", positions, "")
	if err != nil {
		t.Fatalf("NewHumanWorker: %v", err)
	}
	positions[0] = "mutated"
	if got := w.Positions(); got[0] != "p-ceo" {
		t.Fatalf("Positions leaked: %v", got)
	}
	got := w.Positions()
	got[0] = "also mutated"
	if got2 := w.Positions(); got2[0] != "p-ceo" {
		t.Fatalf("Positions getter leaked: %v", got2)
	}
}

// interface conformance at compile time
var (
	_ Worker = (*HumanWorker)(nil)
	_ Worker = (*AIWorker)(nil)
)
