package domain

import (
	"testing"

	"github.com/helixml/helix/api/pkg/org/position"
	"github.com/helixml/helix/api/pkg/org/worker"
)

func TestNewHumanWorker(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		id       worker.ID
		position position.ID
		identity string
		wantErr  bool
	}{
		{"valid", "w-1", "p-ceo", "i am the ceo", false},
		{"valid empty identity", "w-1", "p-ceo", "", false},
		{"empty id", "", "p-ceo", "", true},
		{"vacated (no position)", "w-1", "", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w, err := NewHumanWorker(tc.id, tc.position, tc.identity, "org-test")
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
				if w.Position() != tc.position {
					t.Fatalf("Position = %q, want %q", w.Position(), tc.position)
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

	w, err := NewAIWorker("w-ai", "p-docs", "you are the docs editor", "org-test")
	if err != nil {
		t.Fatalf("NewAIWorker: %v", err)
	}
	if w.Kind() != worker.KindAI {
		t.Fatalf("Kind = %q, want ai", w.Kind())
	}
	if got := w.Position(); got != "p-docs" {
		t.Fatalf("Position = %q, want p-docs", got)
	}
	if w.IdentityContent() != "you are the docs editor" {
		t.Fatalf("IdentityContent = %q", w.IdentityContent())
	}
}

func TestWorkerWithIdentityContent(t *testing.T) {
	t.Parallel()

	w, err := NewAIWorker("w-1", "p-1", "old", "org-test")
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
	if updated.Position() != w.Position() {
		t.Fatalf("Position changed: %q vs %q", updated.Position(), w.Position())
	}
}

// interface conformance at compile time
var (
	_ Worker = (*HumanWorker)(nil)
	_ Worker = (*AIWorker)(nil)
)

// TestWorkerOrganizationIDFromConstructor pins H5.2+: the OrganizationID
// accessor returns the orgID supplied at construction time. Workers are
// always scoped to an org under the multi-tenant graph.
func TestWorkerOrganizationIDFromConstructor(t *testing.T) {
	ai, err := NewAIWorker("w-bot", "p-eng", "# bot", "org-acme")
	if err != nil {
		t.Fatalf("NewAIWorker: %v", err)
	}
	if got := ai.OrganizationID(); got != "org-acme" {
		t.Errorf("AIWorker.OrganizationID() = %q, want org-acme", got)
	}

	hu, err := NewHumanWorker("w-alice", "p-eng", "# alice", "org-acme")
	if err != nil {
		t.Fatalf("NewHumanWorker: %v", err)
	}
	if got := hu.OrganizationID(); got != "org-acme" {
		t.Errorf("HumanWorker.OrganizationID() = %q, want org-acme", got)
	}
}
