package services

import (
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
)

func TestBuildApprovalInstructionPromptRecoversSharedSpecsPush(t *testing.T) {
	task := &types.SpecTask{
		ID:   "spt_test",
		Name: "Update dependencies",
	}

	prompt := BuildApprovalInstructionPrompt(
		task,
		"feature/update-dependencies",
		"main",
		"",
		"app",
		"",
		"",
		nil,
		"",
	)

	for _, want := range []string{
		"git fetch origin helix-specs",
		"git rebase origin/helix-specs",
		"git push origin helix-specs",
		"Do not stop and do not force-push",
		"continue with the code",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("approval prompt is missing %q", want)
		}
	}

	if strings.Contains(prompt, "If `git push` fails: paste the full verbatim stderr") {
		t.Fatal("approval prompt still tells the agent to stop on every push failure")
	}
}
