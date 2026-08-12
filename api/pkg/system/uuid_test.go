package system

import (
	"testing"

	"github.com/helixml/helix/api/pkg/types"
)

// TestGenerateGitRepositoryID_NoCollisionUnderLoad is the regression test
// for the cross-tenant collision bug: two orgs hiring identically-named
// workers in the same wall-clock second used to mint the identical repo id
// (the suffix was `time.Now().Unix()`), and the second org's INSERT failed
// with `git_repositories_pkey` SQLSTATE 23505. Switching the suffix to a
// ULID removes the second-granularity collision window.
//
// 10,000 iterations with the same (repoType, name) inputs trivially blow
// through the old one-per-second budget; the test fails immediately on the
// old code and passes on the new code.
func TestGenerateGitRepositoryID_NoCollisionUnderLoad(t *testing.T) {
	const n = 10000
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		id := GenerateGitRepositoryID(types.GitRepositoryTypeCode, "w-mt")
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate repo id minted at iteration %d: %s", i, id)
		}
		seen[id] = struct{}{}
	}
}
