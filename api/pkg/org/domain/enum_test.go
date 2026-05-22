package domain_test

import (
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain"
)

// WorkerKind tests moved to api/pkg/org/worker/kind_test.go in B3a
// alongside the lifted type. QuotedList remains in helix-org/domain
// for now (still used by other enum types); its test stays here.

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
