package domain_test

import (
	"strings"
	"testing"

	"github.com/helixml/helix-org/domain"
)

func TestWorkerKindValidateAcceptsKnown(t *testing.T) {
	t.Parallel()
	for _, k := range domain.WorkerKindValues() {
		if err := k.Validate(); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", k, err)
		}
	}
}

// TestWorkerKindValidateRejectsUnknownWithList pins the contract a
// self-correcting agent relies on: when validation fails, the message
// contains the offending value AND the valid options, so the next call
// can succeed without reading source.
func TestWorkerKindValidateRejectsUnknownWithList(t *testing.T) {
	t.Parallel()
	err := domain.WorkerKind("claude").Validate()
	if err == nil {
		t.Fatal("Validate(claude) = nil, want error")
	}
	for _, want := range []string{"claude", `"human"`, `"ai"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %q, missing %q", err, want)
		}
	}
}

func TestQuotedListEmptyAndSingleAndMany(t *testing.T) {
	t.Parallel()
	type S string
	if got := domain.QuotedList([]S{}); got != "" {
		t.Errorf("empty = %q, want \"\"", got)
	}
	if got := domain.QuotedList([]S{"only"}); got != `"only"` {
		t.Errorf("single = %q", got)
	}
	if got := domain.QuotedList([]S{"a", "b", "c"}); got != `"a", "b", "c"` {
		t.Errorf("many = %q", got)
	}
}
