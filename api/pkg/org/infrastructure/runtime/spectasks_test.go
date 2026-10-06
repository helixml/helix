package runtime

import (
	"context"
	"errors"
	"testing"
)

// TestNoopSpecTasks_AllVerbsUnsupported pins the noop contract: every
// verb returns ErrSpecTasksUnsupported so the MCP tools surface a clear
// "spec tasks not wired on this runtime" message instead of a nil-deref
// or a silent success. Mirrors NoopProjectConfig.
func TestNoopSpecTasks_AllVerbsUnsupported(t *testing.T) {
	t.Parallel()
	var st SpecTasks = NoopSpecTasks{}
	ctx := context.Background()
	org := "org-test"
	wid := "w-alice"

	if _, err := st.Create(ctx, org, wid, "", CreateSpecTaskInput{Name: "x", Description: "y"}); !errors.Is(err, ErrSpecTasksUnsupported) {
		t.Errorf("Create err = %v, want ErrSpecTasksUnsupported", err)
	}
	if _, err := st.List(ctx, org, wid, "", ListSpecTasksFilter{}); !errors.Is(err, ErrSpecTasksUnsupported) {
		t.Errorf("List err = %v, want ErrSpecTasksUnsupported", err)
	}
	if _, err := st.Get(ctx, org, wid, "", "task_1"); !errors.Is(err, ErrSpecTasksUnsupported) {
		t.Errorf("Get err = %v, want ErrSpecTasksUnsupported", err)
	}
	if _, err := st.StartPlanning(ctx, org, wid, "", "task_1"); !errors.Is(err, ErrSpecTasksUnsupported) {
		t.Errorf("StartPlanning err = %v, want ErrSpecTasksUnsupported", err)
	}
	if _, err := st.SendAgentMessage(ctx, org, wid, "", "task_1", SpecTaskMessageInput{Content: "status?"}); !errors.Is(err, ErrSpecTasksUnsupported) {
		t.Errorf("SendAgentMessage err = %v, want ErrSpecTasksUnsupported", err)
	}
	if _, err := st.ListAgentMessages(ctx, org, wid, "", "task_1", 20); !errors.Is(err, ErrSpecTasksUnsupported) {
		t.Errorf("ListAgentMessages err = %v, want ErrSpecTasksUnsupported", err)
	}
	if _, err := st.StartAgent(ctx, org, wid, "", "task_1"); !errors.Is(err, ErrSpecTasksUnsupported) {
		t.Errorf("StartAgent err = %v, want ErrSpecTasksUnsupported", err)
	}
	if _, err := st.StopAgent(ctx, org, wid, "", "task_1"); !errors.Is(err, ErrSpecTasksUnsupported) {
		t.Errorf("StopAgent err = %v, want ErrSpecTasksUnsupported", err)
	}
	if _, err := st.RestartAgent(ctx, org, wid, "", "task_1"); !errors.Is(err, ErrSpecTasksUnsupported) {
		t.Errorf("RestartAgent err = %v, want ErrSpecTasksUnsupported", err)
	}
	if _, err := st.ReviewSpec(ctx, org, wid, "", "task_1"); !errors.Is(err, ErrSpecTasksUnsupported) {
		t.Errorf("ReviewSpec err = %v, want ErrSpecTasksUnsupported", err)
	}
	if _, err := st.ApproveSpec(ctx, org, wid, "", "task_1"); !errors.Is(err, ErrSpecTasksUnsupported) {
		t.Errorf("ApproveSpec err = %v, want ErrSpecTasksUnsupported", err)
	}
	if _, err := st.RequestChanges(ctx, org, wid, "", "task_1", "please fix"); !errors.Is(err, ErrSpecTasksUnsupported) {
		t.Errorf("RequestChanges err = %v, want ErrSpecTasksUnsupported", err)
	}
	if _, err := st.CreatePullRequests(ctx, org, wid, "", "task_1"); !errors.Is(err, ErrSpecTasksUnsupported) {
		t.Errorf("CreatePullRequests err = %v, want ErrSpecTasksUnsupported", err)
	}
}
