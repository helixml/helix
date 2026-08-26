package server

import (
	"reflect"
	"testing"
)

func TestBranchTipWasMerged(t *testing.T) {
	mergedHeads := map[string]map[string]struct{}{
		"feature/merged": {"merged-sha": {}},
	}

	tests := []struct {
		name      string
		branch    string
		sha       string
		wasMerged bool
	}{
		{name: "matching merged tip", branch: "feature/merged", sha: "merged-sha", wasMerged: true},
		{name: "merged branch advanced", branch: "feature/merged", sha: "new-sha", wasMerged: false},
		{name: "fresh branch at default tip", branch: "feature/fresh", sha: "default-sha", wasMerged: false},
		{name: "branch without merge evidence", branch: "feature/unknown", sha: "unknown-sha", wasMerged: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := branchTipWasMerged(tt.branch, tt.sha, mergedHeads); got != tt.wasMerged {
				t.Fatalf("branchTipWasMerged() = %v, want %v", got, tt.wasMerged)
			}
		})
	}
}

func TestFilterActiveBranches(t *testing.T) {
	branches := []string{"main", "helix-specs", "feature/fresh", "feature/merged", "feature/reactivated", "feature/unknown"}
	branchSHAs := map[string]string{
		"feature/fresh":       "main-sha",
		"feature/merged":      "merged-sha",
		"feature/reactivated": "new-sha",
		"feature/unknown":     "unknown-sha",
	}
	mergedHeads := map[string]map[string]struct{}{
		"feature/merged":      {"merged-sha": {}},
		"feature/reactivated": {"old-sha": {}},
	}

	got, err := filterActiveBranches(branches, "main", mergedHeads, func(branch string) (string, error) {
		return branchSHAs[branch], nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"feature/fresh", "feature/reactivated", "feature/unknown"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filterActiveBranches() = %v, want %v", got, want)
	}
}
