package services

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	zeroRev = "0000000000000000000000000000000000000000"
	someRev = "1111111111111111111111111111111111111111"
)

// runHook executes the real pre-receive script with the given ref update and
// allow-list, returning its stderr and whether it accepted the push. The script
// is the shipped artifact, so running it beats asserting on the Go string.
func runHook(t *testing.T, allowedBranches, refLine string) (string, bool) {
	t.Helper()

	// preReceiveHookScript is already a fully-substituted string at runtime (the
	// version is concatenated in), so it can be written out and run as-is.
	path := filepath.Join(t.TempDir(), "pre-receive")
	if err := os.WriteFile(path, []byte(preReceiveHookScript), 0o755); err != nil {
		t.Fatalf("write hook: %v", err)
	}

	cmd := exec.Command("sh", path)
	cmd.Stdin = strings.NewReader(refLine + "\n")
	cmd.Env = append(os.Environ(), "HELIX_ALLOWED_BRANCHES="+allowedBranches)
	out, err := cmd.CombinedOutput()
	return string(out), err == nil
}

// An agent whose task never reached implementation has no BranchName, so its
// allow-list is helix-specs alone. Telling it to "push to your assigned feature
// branch" is then actively wrong — there is no such branch — and it sent a bot,
// and then two people, hunting a credentials fault that did not exist. The hint
// must name the real cause.
func TestPreReceiveHookExplainsMissingFeatureBranch(t *testing.T) {
	out, ok := runHook(t, "helix-specs", zeroRev+" "+someRev+" refs/heads/feature/mac-app-self-serve-playbook")
	if ok {
		t.Fatal("push should have been refused")
	}
	for _, want := range []string{
		"no feature branch assigned yet",
		"not a credentials problem",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("hint missing %q, got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Push to your assigned feature branch instead") {
		t.Errorf("must not advise pushing to a branch that was never assigned, got:\n%s", out)
	}
}

// When a feature branch DOES exist, the original advice is correct — and worth
// adding that the same name is used in every repo of the project, since the
// allow-list is not repo-scoped.
func TestPreReceiveHookAdvisesAssignedBranch(t *testing.T) {
	out, ok := runHook(t, "helix-specs,feature/002667-x", zeroRev+" "+someRev+" refs/heads/fix/other")
	if ok {
		t.Fatal("push should have been refused")
	}
	if !strings.Contains(out, "Push to your assigned feature branch instead") {
		t.Errorf("expected the assigned-branch hint, got:\n%s", out)
	}
	if strings.Contains(out, "no feature branch assigned") {
		t.Errorf("wrong branch of the hint fired, got:\n%s", out)
	}
}

func TestPreReceiveHookAllowsAssignedAndSpecsBranches(t *testing.T) {
	for _, branch := range []string{"feature/002667-x", "helix-specs"} {
		if out, ok := runHook(t, "helix-specs,feature/002667-x", zeroRev+" "+someRev+" refs/heads/"+branch); !ok {
			t.Errorf("push to %s should be allowed, got:\n%s", branch, out)
		}
	}
}

// Guard the pre-existing protections against regressions in the hint change.
func TestPreReceiveHookKeepsExistingProtections(t *testing.T) {
	if out, ok := runHook(t, "", someRev+" "+zeroRev+" refs/heads/helix-specs"); ok {
		t.Errorf("deleting helix-specs must be refused, got:\n%s", out)
	}
	if _, ok := runHook(t, "", zeroRev+" "+someRev+" refs/heads/anything"); !ok {
		t.Error("an unrestricted key should be able to push any branch")
	}
	if _, ok := runHook(t, "helix-specs", zeroRev+" "+someRev+" refs/tags/v1"); !ok {
		t.Error("tag pushes are not branch updates and must not be restricted")
	}
}

// The installer no-ops when the on-disk hook already contains the current version
// string, so a change to the script that forgets to bump the version silently
// never reaches existing repositories.
func TestPreReceiveHookVersionIsStamped(t *testing.T) {
	if !strings.Contains(preReceiveHookScript, "Helix pre-receive hook v"+PreReceiveHookVersion) {
		t.Fatal("hook script must stamp its version — InstallPreReceiveHook keys upgrades off it")
	}
}
