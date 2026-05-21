// Package worker_test characterises the public behaviour of the Kind
// enum, lifted from helix-org/domain into this canonical home in B3a.
//
// Names lose the redundant "Worker" prefix on the way through
// (domain.WorkerKind -> worker.Kind, domain.WorkerKindHuman ->
// worker.KindHuman, etc.) per the B1 stutter-removal precedent. The
// test cases themselves were authored against the unmoved code (with
// a temporary upward import) and ran green before the lift; only the
// import path and symbol references changed in the lift commit.
//
// Coverage matches W1..W6 from the B3a success criteria:
//
//	W1 KindValues returns [KindHuman, KindAI] in that exact order
//	W2 Kind("human").Validate() = nil
//	W3 Kind("ai").Validate() = nil
//	W4 Kind("").Validate() returns an error
//	W5 Kind("bogus").Validate() returns an error
//	W6 the unknown-kind error contains the offending value AND every
//	   valid option in quotes — the self-correction contract a Worker
//	   relies on
//
// The legacy helix-org/domain/enum_test.go cases W2..W6 are deleted
// in the same commit (their coverage moves here); the QuotedList
// test in that file stays because QuotedList stays in domain for now.
package worker_test

import (
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/org/worker"
)

func TestKindValues_ReturnsHumanThenAIInOrder(t *testing.T) { // W1
	t.Parallel()
	got := worker.KindValues()
	if len(got) != 2 {
		t.Fatalf("WorkerKindValues() len = %d, want 2 (%v)", len(got), got)
	}
	if got[0] != worker.KindHuman {
		t.Fatalf("WorkerKindValues()[0] = %q, want %q", got[0], worker.KindHuman)
	}
	if got[1] != worker.KindAI {
		t.Fatalf("WorkerKindValues()[1] = %q, want %q", got[1], worker.KindAI)
	}
}

func TestValidate_AcceptsHuman(t *testing.T) { // W2
	t.Parallel()
	if err := worker.KindHuman.Validate(); err != nil {
		t.Fatalf("WorkerKindHuman.Validate() = %v, want nil", err)
	}
}

func TestValidate_AcceptsAI(t *testing.T) { // W3
	t.Parallel()
	if err := worker.KindAI.Validate(); err != nil {
		t.Fatalf("WorkerKindAI.Validate() = %v, want nil", err)
	}
}

func TestValidate_RejectsEmpty(t *testing.T) { // W4
	t.Parallel()
	err := worker.Kind("").Validate()
	if err == nil {
		t.Fatal("Validate(\"\") = nil, want error")
	}
}

func TestValidate_RejectsBogus(t *testing.T) { // W5
	t.Parallel()
	err := worker.Kind("bogus").Validate()
	if err == nil {
		t.Fatal("Validate(\"bogus\") = nil, want error")
	}
}

func TestValidate_UnknownErrorListsOffendingValueAndValidOptions(t *testing.T) { // W6
	t.Parallel()
	err := worker.Kind("claude").Validate()
	if err == nil {
		t.Fatal("Validate(\"claude\") = nil, want error")
	}
	for _, want := range []string{"claude", `"human"`, `"ai"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %q, missing %q", err, want)
		}
	}
}
