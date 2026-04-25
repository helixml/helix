package domain

import "testing"

func TestNewHumanWorker(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		id        WorkerID
		positions []PositionID
		wantErr   bool
	}{
		{"valid", "w-1", []PositionID{"p-ceo"}, false},
		{"empty id", "", []PositionID{"p-ceo"}, true},
		{"no positions (vacated)", "w-1", nil, false},
		{"empty position id", "w-1", []PositionID{""}, true},
		{"duplicate positions", "w-1", []PositionID{"p-ceo", "p-ceo"}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w, err := NewHumanWorker(tc.id, tc.positions)
			gotErr := err != nil
			if gotErr != tc.wantErr {
				t.Fatalf("NewHumanWorker error = %v, wantErr = %v", err, tc.wantErr)
			}
			if !gotErr {
				if w.Kind() != WorkerKindHuman {
					t.Fatalf("Kind = %q, want human", w.Kind())
				}
				if w.ID() != tc.id {
					t.Fatalf("ID = %q, want %q", w.ID(), tc.id)
				}
			}
		})
	}
}

func TestNewAIWorker(t *testing.T) {
	t.Parallel()

	w, err := NewAIWorker("w-ai", []PositionID{"p-docs"})
	if err != nil {
		t.Fatalf("NewAIWorker: %v", err)
	}
	if w.Kind() != WorkerKindAI {
		t.Fatalf("Kind = %q, want ai", w.Kind())
	}
	if got := w.Positions(); len(got) != 1 || got[0] != "p-docs" {
		t.Fatalf("Positions = %v, want [p-docs]", got)
	}
}

func TestWorkerPositionsIsolation(t *testing.T) {
	t.Parallel()

	positions := []PositionID{"p-ceo"}
	w, err := NewHumanWorker("w-1", positions)
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
