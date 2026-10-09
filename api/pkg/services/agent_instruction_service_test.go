package services

import (
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
)

func TestBuildApprovalInstructionPromptRecoversSharedSpecsPush(t *testing.T) {
	task := &types.SpecTask{
		ID:             "spt_test",
		Name:           "Update dependencies",
		OriginalPrompt: "Upgrade the database driver without changing behavior.",
	}

	prompt := BuildApprovalInstructionPrompt(
		task,
		"feature/update-dependencies",
		"main",
		"",
		"app",
		"",
		"",
		"",
		nil,
		"",
		false,
	)

	for _, want := range []string{
		"git fetch origin helix-specs",
		"git rebase origin/helix-specs",
		"git push origin helix-specs",
		"Do not stop and do not force-push",
		"continue with the code",
		"Before each follow-up pull request, replace the relevant `pull_request*.md` title and body",
		"Describe only the changes that are not already merged",
		"Upgrade the database driver without changing behavior.",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("approval prompt is missing %q", want)
		}
	}

	if strings.Contains(prompt, "If `git push` fails: paste the full verbatim stderr") {
		t.Fatal("approval prompt still tells the agent to stop on every push failure")
	}
}

// TestBuildApprovalInstructionPrompt_HelixSkills mirrors
// TestBuildPlanningPrompt_HelixSkills for the implementation phase.
func TestBuildApprovalInstructionPrompt_HelixSkills(t *testing.T) {
	task := &types.SpecTask{ID: "spt_test", ProjectID: "prj_test", Name: "x", DesignDocPath: "000001_x"}

	out := BuildApprovalInstructionPrompt(task, "feature/x", "main", "", "repo", "", "", "", nil, "", false)

	for _, want := range []string{"## Helix skills", "`helix-cli`", "`helix-artifacts`"} {
		if !strings.Contains(out, want) {
			t.Errorf("approval prompt is missing required snippet %q", want)
		}
	}
}

// External-repo projects ship work only as agent-proposed pull requests; the
// prompt must teach that flow and must not promise an automatic PR.
func TestBuildApprovalInstructionPrompt_PullRequestsViaProposals(t *testing.T) {
	task := &types.SpecTask{ID: "spt_test", ProjectID: "prj_test", Name: "x", DesignDocPath: "000001_x"}

	external := BuildApprovalInstructionPrompt(task, "feature/x", "main", "", "app", "", "", "", []string{"lib"}, "", true)
	for _, want := range []string{
		"`propose_pull_request`",
		"Propose each further slice with its own new `head_branch` BEFORE pushing to it",
		"Propose one pull request per repository you changed",
		"`list_pull_request_proposals`",
	} {
		if !strings.Contains(external, want) {
			t.Errorf("external-repo prompt is missing %q", want)
		}
	}
	for _, banned := range []string{"creates the GitHub PR automatically", "pull_request.md"} {
		if strings.Contains(external, banned) {
			t.Errorf("external-repo prompt still contains %q", banned)
		}
	}

	internal := BuildApprovalInstructionPrompt(task, "feature/x", "main", "", "app", "", "", "", nil, "", false)
	if strings.Contains(internal, "propose_pull_request") {
		t.Error("internal-only projects have no pull requests to propose")
	}
}

// Tasks never finish on their own, so every implementation prompt must tell the
// agent how to finish — and, with pull requests, not to while they are open.
func TestBuildApprovalInstructionPrompt_MarkTaskComplete(t *testing.T) {
	task := &types.SpecTask{ID: "spt_test", ProjectID: "prj_test", Name: "x", DesignDocPath: "000001_x"}

	external := BuildApprovalInstructionPrompt(task, "feature/x", "main", "", "app", "", "", "", nil, "", true)
	for _, want := range []string{"7. **Finishing the task:**", "`mark_task_complete`", "never marked done automatically", "Do not call it while your pull requests are still open"} {
		if !strings.Contains(external, want) {
			t.Errorf("external-repo prompt is missing %q", want)
		}
	}

	internal := BuildApprovalInstructionPrompt(task, "feature/x", "main", "", "app", "", "", "", nil, "", false)
	for _, want := range []string{"7. **Finishing the task:**", "Accept in Helix", "`mark_task_complete`"} {
		if !strings.Contains(internal, want) {
			t.Errorf("internal-repo prompt is missing %q", want)
		}
	}
	if strings.Contains(internal, "pull requests are still open") {
		t.Error("internal-only projects have no pull requests to wait for")
	}
}
