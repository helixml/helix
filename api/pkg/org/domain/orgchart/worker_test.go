package orgchart_test

import (
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
)

func TestNewHumanWorker(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		id       orgchart.WorkerID
		position orgchart.PositionID
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
			w, err := orgchart.NewHumanWorker(tc.id, tc.position, tc.identity, "org-test")
			gotErr := err != nil
			if gotErr != tc.wantErr {
				t.Fatalf("orgchart.NewHumanWorker error = %v, wantErr = %v", err, tc.wantErr)
			}
			if !gotErr {
				if w.Kind() != orgchart.WorkerKindHuman {
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

	w, err := orgchart.NewAIWorker("w-ai", "p-docs", "you are the docs editor", "org-test")
	if err != nil {
		t.Fatalf("orgchart.NewAIWorker: %v", err)
	}
	if w.Kind() != orgchart.WorkerKindAI {
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

	w, err := orgchart.NewAIWorker("w-1", "p-1", "old", "org-test")
	if err != nil {
		t.Fatalf("orgchart.NewAIWorker: %v", err)
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
	_ orgchart.Worker = (*orgchart.HumanWorker)(nil)
	_ orgchart.Worker = (*orgchart.AIWorker)(nil)
)

// TestWorkerOrganizationIDFromConstructor: the OrganizationID accessor
// returns the orgID supplied at construction time.
func TestWorkerOrganizationIDFromConstructor(t *testing.T) {
	ai, err := orgchart.NewAIWorker("w-bot", "p-eng", "# bot", "org-acme")
	if err != nil {
		t.Fatalf("orgchart.NewAIWorker: %v", err)
	}
	if got := ai.OrganizationID(); got != "org-acme" {
		t.Errorf("AIWorker.OrganizationID() = %q, want org-acme", got)
	}

	hu, err := orgchart.NewHumanWorker("w-alice", "p-eng", "# alice", "org-acme")
	if err != nil {
		t.Fatalf("orgchart.NewHumanWorker: %v", err)
	}
	if got := hu.OrganizationID(); got != "org-acme" {
		t.Errorf("HumanWorker.OrganizationID() = %q, want org-acme", got)
	}
}
