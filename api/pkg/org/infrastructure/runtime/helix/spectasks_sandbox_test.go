package helix

import (
	"context"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
	"github.com/helixml/helix/api/pkg/types"
)

// createInProject wires a Worker to a project and runs one create, returning
// the stored task so assertions read against what actually persisted.
func createInProject(
	t *testing.T,
	project *types.Project,
	in runtime.CreateSpecTaskInput,
) (*types.SpecTask, error) {
	t.Helper()
	wrap := newSpecTasksTestStore(t)
	wid := orgchart.NodeID("w-alice")
	saveAllPointers(t, &wrap.Store, "org-test", wid, project.ID, "app_x", "repo_y", "ses_z")

	fs := newFakeSpecTaskStore()
	fs.projects[project.ID] = project
	st, err := NewSpecTasks(&wrap.Store, fs, &fakeSpecTaskWorkflow{})
	if err != nil {
		t.Fatalf("NewSpecTasks: %v", err)
	}
	view, err := st.Create(context.Background(), "org-test", wid, "", in)
	if err != nil {
		return nil, err
	}
	stored, ok := fs.tasks[view.ID]
	if !ok {
		t.Fatalf("created task %s missing from store", view.ID)
	}
	return stored, nil
}

func plainProject() *types.Project {
	return &types.Project{ID: "prj_01abc", OrganizationID: "org-test"}
}

// A Worker asking for a bigger headless sandbox through MCP must get one.
// Before this, the tool had no way to express it and the Worker fell back to
// hand-writing a curl against the REST API — dragging a hand-written
// code-agent config along with it.
func TestSpecTasks_CreateHonoursExplicitSandboxSettings(t *testing.T) {
	t.Parallel()
	task, err := createInProject(t, plainProject(), runtime.CreateSpecTaskInput{
		Name: "Big build", Description: "needs headroom",
		SandboxVCPUs: 8, SandboxRuntime: string(types.SandboxRuntimeHeadlessUbuntu),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if task.SandboxResourceOverrides == nil {
		t.Fatal("sandbox resources not set")
	}
	if task.SandboxResourceOverrides.VCPUs != 8 || task.SandboxResourceOverrides.MemoryMB != 16384 {
		t.Fatalf("resources = %+v, want 8 vCPU / 16384 MB", *task.SandboxResourceOverrides)
	}
	// Memory is derived, never taken on trust, so the pair can never be one
	// ValidPreset would reject.
	if !task.SandboxResourceOverrides.ValidPreset() {
		t.Fatal("resolved resources are not a valid preset")
	}
	if task.SandboxRuntime != types.SandboxRuntimeHeadlessUbuntu {
		t.Fatalf("runtime = %q, want headless-ubuntu", task.SandboxRuntime)
	}
}

// Omitted values fall back to the project's defaults, so a task filed through
// MCP lands on the same sandbox it would have through the UI.
func TestSpecTasks_CreateFallsBackToProjectSandboxDefaults(t *testing.T) {
	t.Parallel()
	project := plainProject()
	project.DefaultSandboxResourceOverrides = &types.SandboxResourceOverrides{VCPUs: 4, MemoryMB: 8192}
	project.DefaultSandboxRuntime = types.SandboxRuntimeHeadlessUbuntu

	task, err := createInProject(t, project, runtime.CreateSpecTaskInput{
		Name: "Ordinary", Description: "no overrides",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if task.SandboxResourceOverrides == nil || task.SandboxResourceOverrides.VCPUs != 4 {
		t.Fatalf("resources = %+v, want the project default of 4 vCPU", task.SandboxResourceOverrides)
	}
	if task.SandboxRuntime != types.SandboxRuntimeHeadlessUbuntu {
		t.Fatalf("runtime = %q, want the project default headless-ubuntu", task.SandboxRuntime)
	}
}

// With neither an explicit value nor a project default, the row stores NO
// override and the global default is resolved at container-create time.
//
// Materializing it here is what froze every task created after 1eff4e801 at
// 4 vCPU / 8 GB: a stored override is indistinguishable from a user's choice, so
// a raised default could never reach those rows. Asserting nil is the point of
// this test, not an accident of it.
func TestSpecTasks_CreateLeavesGlobalSandboxDefaultUnmaterialized(t *testing.T) {
	t.Parallel()
	task, err := createInProject(t, plainProject(), runtime.CreateSpecTaskInput{
		Name: "Ordinary", Description: "no overrides",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if task.SandboxResourceOverrides != nil {
		t.Fatalf("resources = %+v, want nil so the live default applies at start",
			task.SandboxResourceOverrides)
	}
	// Nil still resolves to a usable preset for the container that eventually starts.
	if effective := types.EffectiveSpecTaskSandboxResources(task.SandboxResourceOverrides); !effective.ValidPreset() {
		t.Fatalf("effective resources = %+v, want a valid preset", effective)
	}
	if task.SandboxRuntime != types.SandboxRuntimeUbuntuDesktop {
		t.Fatalf("runtime = %q, want the global default ubuntu-desktop", task.SandboxRuntime)
	}
}

// A model can produce any integer. Rejecting it with a message naming the
// legal values beats silently rounding to a preset the caller did not ask for.
func TestSpecTasks_CreateRejectsUnknownSandboxSize(t *testing.T) {
	t.Parallel()
	_, err := createInProject(t, plainProject(), runtime.CreateSpecTaskInput{
		Name: "x", Description: "y", SandboxVCPUs: 2,
	})
	if err == nil {
		t.Fatal("expected an error for a vCPU count with no preset")
	}
	// Derived, not hand-copied: a Worker reads this message and retries with a
	// size from it, so a stale list here would hide a stale list in production.
	if !strings.Contains(err.Error(), types.SpecTaskSandboxVCPUList()) {
		t.Fatalf("err = %v, want it to name the legal sizes %s", err, types.SpecTaskSandboxVCPUList())
	}
}

func TestSpecTasks_CreateRejectsUnknownSandboxRuntime(t *testing.T) {
	t.Parallel()
	_, err := createInProject(t, plainProject(), runtime.CreateSpecTaskInput{
		Name: "x", Description: "y", SandboxRuntime: "macos",
	})
	if err == nil {
		t.Fatal("expected an error for an unknown sandbox runtime")
	}
	if !strings.Contains(err.Error(), "headless-ubuntu") {
		t.Fatalf("err = %v, want it to name the legal runtimes", err)
	}
}

// The tool deliberately exposes no code-agent argument: the task must inherit
// the project's configured agent, which is the Bot's own. A Worker that could
// name a harness here would reintroduce the bug this whole change exists to
// stop.
func TestSpecTasks_CreateLeavesCodeAgentToTheProject(t *testing.T) {
	t.Parallel()
	task, err := createInProject(t, plainProject(), runtime.CreateSpecTaskInput{
		Name: "x", Description: "y",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if task.CodeAgentConfig != nil {
		t.Fatalf("code-agent config = %+v, want nil so it resolves from the project at start", task.CodeAgentConfig)
	}
}

// createViewInProject is createInProject's sibling: it returns the tool-facing
// view rather than the stored row, because what the Worker is told is its own
// contract.
func createViewInProject(
	t *testing.T,
	project *types.Project,
	in runtime.CreateSpecTaskInput,
) runtime.SpecTaskView {
	t.Helper()
	wrap := newSpecTasksTestStore(t)
	wid := orgchart.NodeID("w-alice")
	saveAllPointers(t, &wrap.Store, "org-test", wid, project.ID, "app_x", "repo_y", "ses_z")

	fs := newFakeSpecTaskStore()
	fs.projects[project.ID] = project
	st, err := NewSpecTasks(&wrap.Store, fs, &fakeSpecTaskWorkflow{})
	if err != nil {
		t.Fatalf("NewSpecTasks: %v", err)
	}
	view, err := st.Create(context.Background(), "org-test", wid, "", in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return view
}

// The Worker is told what it actually got, so it can confirm the sandbox
// without a follow-up call.
func TestSpecTasks_CreateViewReportsResolvedSandbox(t *testing.T) {
	t.Parallel()
	view := createViewInProject(t, plainProject(), runtime.CreateSpecTaskInput{
		Name: "x", Description: "y",
		SandboxVCPUs: 4, SandboxRuntime: string(types.SandboxRuntimeHeadlessUbuntu),
	})
	if view.SandboxVCPUs != 4 {
		t.Fatalf("view vCPUs = %d, want 4", view.SandboxVCPUs)
	}
	if view.SandboxRuntime != string(types.SandboxRuntimeHeadlessUbuntu) {
		t.Fatalf("view runtime = %q, want headless-ubuntu", view.SandboxRuntime)
	}
}

// The task has no code-agent config of its own yet, so the view reports the
// project's — the one it will actually start on. An empty field here reads as
// "unset" and is what invites a Worker to go and set one itself.
func TestSpecTasks_CreateViewReportsInheritedCodeAgent(t *testing.T) {
	t.Parallel()
	project := plainProject()
	project.CodeAgentConfig = &types.CodeAgentExecutionConfig{
		Runtime: types.CodeAgentRuntimeOpenCode,
		Model:   "qwen3.8-27b",
	}
	view := createViewInProject(t, project, runtime.CreateSpecTaskInput{
		Name: "x", Description: "y",
	})
	if view.CodeAgentRuntime != string(types.CodeAgentRuntimeOpenCode) {
		t.Fatalf("view runtime = %q, want opencode", view.CodeAgentRuntime)
	}
	if view.CodeAgentModel != "qwen3.8-27b" {
		t.Fatalf("view model = %q, want qwen3.8-27b", view.CodeAgentModel)
	}
}
