package prompts

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestImplementationApprovedPushInstruction(t *testing.T) {
	branchName := "test-branch"
	baseBranch := "main"
	prompt, err := ImplementationApprovedPushInstruction(branchName, "my-project", baseBranch, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	assert.NotEmpty(t, prompt)
	assert.Contains(t, prompt, branchName)
	assert.Contains(t, prompt, "Your implementation has been approved")
	assert.Contains(t, prompt, "git fetch origin main")
	assert.Contains(t, prompt, "git merge origin/main")
}
