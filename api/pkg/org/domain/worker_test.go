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
			w, err := NewHumanWorker(tc.id, tc.position, tc.identity)
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

	w, err := NewAIWorker("w-ai", "p-docs", "you are the docs editor")
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

	w, err := NewAIWorker("w-1", "p-1", "old")
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

// TestWorkerOrganizationIDDefaultsEmpty pins H5.1: the OrganizationID
// accessor returns the empty string by default. The alpha is
// single-tenant — every Worker lives in the global graph — and the
// constructors don't take OrgID so existing call sites stay
// unchanged. H5.2+ flips lookups to (org_id, worker_id) once
// bootstrap-per-org lands; until then OrgID is purely additive
// scaffolding.
func TestWorkerOrganizationIDDefaultsEmpty(t *testing.T) {
	ai, err := NewAIWorker("w-bot", "p-eng", "# bot")
	if err != nil {
		t.Fatalf("NewAIWorker: %v", err)
	}
	if got := ai.OrganizationID(); got != "" {
		t.Errorf("AIWorker.OrganizationID() = %q, want empty", got)
	}

	hu, err := NewHumanWorker("w-alice", "p-eng", "# alice")
	if err != nil {
		t.Fatalf("NewHumanWorker: %v", err)
	}
	if got := hu.OrganizationID(); got != "" {
		t.Errorf("HumanWorker.OrganizationID() = %q, want empty", got)
	}
}

// TestWorkerWithOrgIDReturnsScopedWorker pins the builder shape that
// multi-tenant call sites use to stamp an OrgID onto a Worker without
// disrupting the zero-arg constructors. The returned Worker preserves
// every other field (ID, Position, Identity, Kind).
func TestWorkerWithOrgIDReturnsScopedWorker(t *testing.T) {
	ai, _ := NewAIWorker("w-bot", "p-eng", "# bot")
	scoped := ai.WithOrgID("org-acme")
	if scoped.OrganizationID() != "org-acme" {
		t.Errorf("AIWorker.WithOrgID().OrganizationID() = %q, want org-acme", scoped.OrganizationID())
	}
	if scoped.ID() != ai.ID() {
		t.Errorf("WithOrgID changed ID: %q vs %q", scoped.ID(), ai.ID())
	}
	if scoped.Position() != ai.Position() {
		t.Errorf("WithOrgID changed Position: %q vs %q", scoped.Position(), ai.Position())
	}
	if scoped.IdentityContent() != ai.IdentityContent() {
		t.Errorf("WithOrgID changed Identity: %q vs %q", scoped.IdentityContent(), ai.IdentityContent())
	}
	if scoped.Kind() != ai.Kind() {
		t.Errorf("WithOrgID changed Kind: %q vs %q", scoped.Kind(), ai.Kind())
	}

	// Original is untouched — value semantics.
	if ai.OrganizationID() != "" {
		t.Errorf("WithOrgID mutated original: OrganizationID = %q", ai.OrganizationID())
	}

	// Human path works the same way.
	hu, _ := NewHumanWorker("w-alice", "p-eng", "# alice")
	if got := hu.WithOrgID("org-acme").OrganizationID(); got != "org-acme" {
		t.Errorf("HumanWorker.WithOrgID().OrganizationID() = %q, want org-acme", got)
	}
}
