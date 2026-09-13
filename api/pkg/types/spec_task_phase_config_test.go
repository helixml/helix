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
