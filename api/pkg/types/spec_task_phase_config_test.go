package types

import "testing"

func TestSpecTaskCodeAgentConfigForPhase(t *testing.T) {
	implementation := &CodeAgentExecutionConfig{Runtime: CodeAgentRuntimeCodexCLI, Model: "implementer"}
	planning := &CodeAgentExecutionConfig{Runtime: CodeAgentRuntimeClaudeCode, Model: "planner"}
	task := &SpecTask{CodeAgentConfig: implementation, PlanningCodeAgentConfig: planning}

	if got := task.CodeAgentConfigForPhase(SpecTaskPhasePlanning); got != planning {
		t.Fatalf("planning config = %#v, want %#v", got, planning)
	}
	if got := task.CodeAgentConfigForPhase(SpecTaskPhaseImplementation); got != implementation {
		t.Fatalf("implementation config = %#v, want %#v", got, implementation)
	}

	task.PlanningCodeAgentConfig = nil
	if got := task.CodeAgentConfigForPhase(SpecTaskPhasePlanning); got != implementation {
		t.Fatalf("legacy planning config = %#v, want implementation fallback %#v", got, implementation)
	}
}

func TestSpecTaskActiveCodeAgentConfigFollowsStatus(t *testing.T) {
	implementation := &CodeAgentExecutionConfig{Model: "implementer"}
	planning := &CodeAgentExecutionConfig{Model: "planner"}
	task := &SpecTask{CodeAgentConfig: implementation, PlanningCodeAgentConfig: planning}

	for _, status := range []SpecTaskStatus{
		TaskStatusBacklog,
		TaskStatusSpecGeneration,
		TaskStatusSpecReview,
		TaskStatusSpecRevision,
		TaskStatusSpecApproved,
	} {
		task.Status = status
		if got := task.ActiveCodeAgentConfig(); got != planning {
			t.Fatalf("status %s selected %#v, want planner", status, got)
		}
	}

	for _, status := range []SpecTaskStatus{
		TaskStatusImplementationQueued,
		TaskStatusImplementation,
		TaskStatusImplementationReview,
		TaskStatusPullRequest,
		TaskStatusDone,
		TaskStatusImplementationFailed,
	} {
		task.Status = status
		if got := task.ActiveCodeAgentConfig(); got != implementation {
			t.Fatalf("status %s selected %#v, want implementer", status, got)
		}
	}
}

func TestSpecTaskGooseRecipeForPhase(t *testing.T) {
	task := &SpecTask{
		CodeAgentConfig:           &CodeAgentExecutionConfig{Runtime: CodeAgentRuntimeGooseCode},
		PlanningCodeAgentConfig:   &CodeAgentExecutionConfig{Runtime: CodeAgentRuntimeGooseCode},
		GooseRecipeName:           "implement",
		GooseRecipeParams:         map[string]string{"scope": "code"},
		PlanningGooseRecipeName:   "plan",
		PlanningGooseRecipeParams: map[string]string{"scope": "design"},
	}
	name, params := task.GooseRecipeForPhase(SpecTaskPhasePlanning)
	if name != "plan" || params["scope"] != "design" {
		t.Fatalf("planning recipe = %q %#v", name, params)
	}
	name, params = task.GooseRecipeForPhase(SpecTaskPhaseImplementation)
	if name != "implement" || params["scope"] != "code" {
		t.Fatalf("implementation recipe = %q %#v", name, params)
	}

	task.PlanningCodeAgentConfig = nil
	name, _ = task.GooseRecipeForPhase(SpecTaskPhasePlanning)
	if name != "implement" {
		t.Fatalf("legacy planning recipe = %q, want implementation fallback", name)
	}
}
