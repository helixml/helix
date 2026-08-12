package services

import (
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
)

// TestBuildPlanningPrompt_TitleFormatRule guards the explicit "## CRITICAL:
// Title Format" instruction in the planning prompt. The instruction tells the
// agent to title requirements.md as `# Requirements: <Descriptive Title>` so
// the title can be parsed back into task.Name and task.BranchName. If a
// future edit silently drops the rule, agents will start writing generic
// `# Requirements` H1s again and downstream naming will break (see
// SpecTitleFromRequirements in git_helpers.go).
func TestBuildPlanningPrompt_TitleFormatRule(t *testing.T) {
	task := &types.SpecTask{
		ID:            "spt_test",
		ProjectID:     "prj_test",
		Name:          "Add Dark Mode",
		Type:          "feature",
		Priority:      types.SpecTaskPriorityMedium,
		DesignDocPath: "000001_add-dark-mode",
	}

	out := BuildPlanningPrompt(task, "", "", "", "")

	mustContain := []string{
		"## CRITICAL: Title Format",
		"`# Requirements: <Descriptive Title>`",
		"`# Design: <Descriptive Title>`",
		"`# Implementation Tasks: <Descriptive Title>`",
		"# Implementation Tasks: <Descriptive Title>", // also in the tasks.md example block
	}
	for _, want := range mustContain {
		if !strings.Contains(out, want) {
			t.Errorf("planning prompt is missing required snippet %q", want)
		}
	}
}

// TestBuildPlanningPrompt_OpenQuestions guards the instruction that tells the
// planning agent to add an "## Open Questions" section to requirements.md. This
// surfaces the agent's guesses for user review instead of letting invented
// requirements silently become the spec. If a future edit drops the rule,
// agents will stop listing their open questions.
func TestBuildPlanningPrompt_OpenQuestions(t *testing.T) {
	task := &types.SpecTask{
		ID:            "spt_test",
		ProjectID:     "prj_test",
		Name:          "Add Dark Mode",
		Type:          "feature",
		Priority:      types.SpecTaskPriorityMedium,
		DesignDocPath: "000001_add-dark-mode",
	}

	out := BuildPlanningPrompt(task, "", "", "", "")

	mustContain := []string{
		"## Open Questions (requirements.md)",
		"`## Open Questions`",
	}
	for _, want := range mustContain {
		if !strings.Contains(out, want) {
			t.Errorf("planning prompt is missing required snippet %q", want)
		}
	}
}
