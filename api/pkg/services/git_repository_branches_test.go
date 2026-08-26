package services

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseLsRemoteBranches(t *testing.T) {
	stdout := "aaaaaaaa\trefs/heads/main\n" +
		"bbbbbbbb\trefs/heads/feature/active\n" +
		"cccccccc\trefs/tags/v1.0.0\n" +
		"malformed\n"

	// A stale local branch is intentionally absent: only authoritative
	// upstream heads returned by ls-remote can enter this list.
	want := []string{"main", "feature/active"}
	if got := parseLsRemoteBranches(stdout); !reflect.DeepEqual(got, want) {
		t.Fatalf("parseLsRemoteBranches() = %v, want %v", got, want)
	}
}

func TestParseMergeParentSHAs(t *testing.T) {
	stdout := "merge1 first-parent feature-tip\n" +
		"merge2 first-parent topic-a topic-b\n" +
		"ordinary parent\n"

	want := map[string]struct{}{
		"feature-tip": {},
		"topic-a":     {},
		"topic-b":     {},
	}
	if got := parseMergeParentSHAs(stdout); !reflect.DeepEqual(got, want) {
		t.Fatalf("parseMergeParentSHAs() = %v, want %v", got, want)
	}
}

func TestGetMergeParentSHAsFromRepository(t *testing.T) {
	repoPath := t.TempDir()
	runGitForBranchTest(t, repoPath, "init", "-b", "main")
	runGitForBranchTest(t, repoPath, "config", "user.name", "Test User")
	runGitForBranchTest(t, repoPath, "config", "user.email", "test@example.com")

	writeBranchTestFile(t, repoPath, "base.txt", "base")
	runGitForBranchTest(t, repoPath, "add", "base.txt")
	runGitForBranchTest(t, repoPath, "commit", "-m", "base")

	runGitForBranchTest(t, repoPath, "checkout", "-b", "feature/merged")
	writeBranchTestFile(t, repoPath, "feature.txt", "merged work")
	runGitForBranchTest(t, repoPath, "add", "feature.txt")
	runGitForBranchTest(t, repoPath, "commit", "-m", "feature work")
	mergedTip := runGitForBranchTest(t, repoPath, "rev-parse", "HEAD")

	runGitForBranchTest(t, repoPath, "checkout", "main")
	runGitForBranchTest(t, repoPath, "merge", "--no-ff", "feature/merged", "-m", "merge feature")
	mainTip := runGitForBranchTest(t, repoPath, "rev-parse", "HEAD")

	mergeParents, err := GetMergeParentSHAs(context.Background(), repoPath, "main")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := mergeParents[mergedTip]; !ok {
		t.Fatalf("merged feature tip %s was not detected", mergedTip)
	}
	if _, ok := mergeParents[mainTip]; ok {
		t.Fatalf("fresh branch at current main tip %s must remain active", mainTip)
	}

	runGitForBranchTest(t, repoPath, "checkout", "feature/merged")
	writeBranchTestFile(t, repoPath, "feature.txt", "new work after merge")
	runGitForBranchTest(t, repoPath, "add", "feature.txt")
	runGitForBranchTest(t, repoPath, "commit", "-m", "reactivate feature")
	advancedTip := runGitForBranchTest(t, repoPath, "rev-parse", "HEAD")
	if _, ok := mergeParents[advancedTip]; ok {
		t.Fatalf("advanced feature tip %s must remain active", advancedTip)
	}
}

func runGitForBranchTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

func writeBranchTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
