package prompts

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImplementationApprovedPushInstruction(t *testing.T) {
	branchName := "test-branch"
	baseBranch := "main"
	prompt, err := ImplementationApprovedPushInstruction(branchName, "my-project", baseBranch, "000123-my-task", nil)
	require.NoError(t, err)
	assert.NotEmpty(t, prompt)
	assert.Contains(t, prompt, branchName)
	assert.Contains(t, prompt, "Your implementation has been approved")
	assert.Contains(t, prompt, "git fetch origin main")
	assert.Contains(t, prompt, "git merge origin/main")
	assert.Contains(t, prompt, "git fetch origin helix-specs")
	assert.Contains(t, prompt, "git rebase origin/helix-specs")
	assert.Contains(t, prompt, "git push origin helix-specs")
	assert.Contains(t, prompt, "/home/retro/work/helix-specs/design/tasks/000123-my-task/pull_request.md")
	assert.Contains(t, prompt, "The first line is the PR title")
	assert.Contains(t, prompt, "This metadata step is required even if every code change is already committed and pushed")
	assert.Contains(t, prompt, "Helix opens and updates it automatically")
}

func TestImplementationApprovedPushInstructionMultiRepo(t *testing.T) {
	prompt, err := ImplementationApprovedPushInstruction(
		"feature/my-task",
		"helix",
		"main",
		"000123-my-task",
		[]string{"qwen-code", "zed"},
	)
	require.NoError(t, err)

	assert.Contains(t, prompt, "/home/retro/work/helix-specs/design/tasks/000123-my-task/pull_request_helix.md")
	assert.Contains(t, prompt, "/home/retro/work/helix-specs/design/tasks/000123-my-task/pull_request_qwen-code.md")
	assert.Contains(t, prompt, "/home/retro/work/helix-specs/design/tasks/000123-my-task/pull_request_zed.md")
	assert.NotContains(t, prompt, "/home/retro/work/helix-specs/design/tasks/000123-my-task/pull_request.md`.")
}

func TestImplementationApprovedPushInstructionRequiresTaskDir(t *testing.T) {
	_, err := ImplementationApprovedPushInstruction("feature/my-task", "helix", "main", "", nil)
	require.EqualError(t, err, "task directory name is required")
}
