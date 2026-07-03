package mcptools_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/org/application/spectasks"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
	"github.com/helixml/helix/api/pkg/org/interfaces/mcptools"
)

// recordingPort is a configurable runtime.SpecTasks for the tool tests.
type recordingPort struct {
	runtime.NoopSpecTasks
	createIn    runtime.CreateSpecTaskInput
	lastProject string
	lastTaskID  string
	lastComment string
	lastFilter  runtime.ListSpecTasksFilter
	view        runtime.SpecTaskView
	review      runtime.SpecReviewView
	err         error
}

func (p *recordingPort) Create(_ context.Context, _ string, _ orgchart.BotID, projectID string, in runtime.CreateSpecTaskInput) (runtime.SpecTaskView, error) {
	p.lastProject, p.createIn = projectID, in
	return p.view, p.err
}
func (p *recordingPort) List(_ context.Context, _ string, _ orgchart.BotID, projectID string, f runtime.ListSpecTasksFilter) ([]runtime.SpecTaskView, error) {
	p.lastProject, p.lastFilter = projectID, f
	if p.err != nil {
		return nil, p.err
	}
	return []runtime.SpecTaskView{p.view}, nil
}
func (p *recordingPort) Get(_ context.Context, _ string, _ orgchart.BotID, projectID, id string) (runtime.SpecTaskView, error) {
	p.lastProject, p.lastTaskID = projectID, id
	return p.view, p.err
}
func (p *recordingPort) StartPlanning(_ context.Context, _ string, _ orgchart.BotID, projectID, id string) (runtime.SpecTaskView, error) {
	p.lastProject, p.lastTaskID = projectID, id
	return p.view, p.err
}
func (p *recordingPort) ReviewSpec(_ context.Context, _ string, _ orgchart.BotID, projectID, id string) (runtime.SpecReviewView, error) {
	p.lastProject, p.lastTaskID = projectID, id
	return p.review, p.err
}
func (p *recordingPort) ApproveSpec(_ context.Context, _ string, _ orgchart.BotID, projectID, id string) (runtime.SpecTaskView, error) {
	p.lastProject, p.lastTaskID = projectID, id
	return p.view, p.err
}
func (p *recordingPort) RequestChanges(_ context.Context, _ string, _ orgchart.BotID, projectID, id, comment string) (runtime.SpecTaskView, error) {
	p.lastProject, p.lastTaskID, p.lastComment = projectID, id, comment
	return p.view, p.err
}
func (p *recordingPort) CreatePullRequests(_ context.Context, _ string, _ orgchart.BotID, projectID, id string) (runtime.SpecTaskView, error) {
	p.lastProject, p.lastTaskID = projectID, id
	return p.view, p.err
}

func depsWithPort(p runtime.SpecTasks) mcptools.Deps {
	return mcptools.Deps{SpecTasks: spectasks.New(p, nil)}
}

func callerInv(args string) tool.Invocation {
	return tool.Invocation{
		Caller: fakeWorker{id: "w-alice", org: "org-1"},
		Args:   json.RawMessage(args),
	}
}

func TestCreateSpecTaskTool(t *testing.T) {
	t.Parallel()
	p := &recordingPort{view: runtime.SpecTaskView{ID: "task_1", Name: "Add login", Status: "backlog"}}
	tl := mcptools.NewCreateSpecTask(depsWithPort(p))
	if tl.Name() != mcptools.CreateSpecTaskName {
		t.Errorf("Name = %q", tl.Name())
	}
	if tl.InputSchema() == nil {
		t.Error("InputSchema nil")
	}
	out, err := tl.Invoke(context.Background(), callerInv(`{"name":"Add login","description":"add it","priority":"high"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if p.createIn.Name != "Add login" || p.createIn.Priority != "high" {
		t.Errorf("create input = %+v", p.createIn)
	}
	if !strings.Contains(string(out), "task_1") {
		t.Errorf("output missing task id: %s", out)
	}
}

func TestCreateSpecTaskTool_PropagatesError(t *testing.T) {
	t.Parallel()
	p := &recordingPort{err: errors.New("boom")}
	tl := mcptools.NewCreateSpecTask(depsWithPort(p))
	if _, err := tl.Invoke(context.Background(), callerInv(`{"name":"x","description":"y"}`)); err == nil {
		t.Error("expected error propagated from port")
	}
}

func TestListSpecTasksTool(t *testing.T) {
	t.Parallel()
	p := &recordingPort{view: runtime.SpecTaskView{ID: "task_1", Status: "backlog"}}
	tl := mcptools.NewListSpecTasks(depsWithPort(p))
	if tl.Name() != mcptools.ListSpecTasksName {
		t.Errorf("Name = %q", tl.Name())
	}
	out, err := tl.Invoke(context.Background(), callerInv(`{"status":"backlog"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if p.lastFilter.Status != "backlog" {
		t.Errorf("filter status = %q", p.lastFilter.Status)
	}
	if !strings.Contains(string(out), "task_1") {
		t.Errorf("output missing task: %s", out)
	}
}

func TestGetSpecTaskTool_ForwardsProjectID(t *testing.T) {
	t.Parallel()
	p := &recordingPort{view: runtime.SpecTaskView{ID: "task_9", Status: "backlog"}}
	tl := mcptools.NewGetSpecTask(depsWithPort(p))
	if _, err := tl.Invoke(context.Background(), callerInv(`{"project_id":"prj_other","task_id":"task_9"}`)); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if p.lastProject != "prj_other" {
		t.Errorf("project id = %q, want prj_other", p.lastProject)
	}
}

func TestGetSpecTaskTool_RequiresTaskID(t *testing.T) {
	t.Parallel()
	tl := mcptools.NewGetSpecTask(depsWithPort(&recordingPort{}))
	if _, err := tl.Invoke(context.Background(), callerInv(`{}`)); err == nil {
		t.Error("expected error when task_id missing")
	}
}

func TestGetSpecTaskTool(t *testing.T) {
	t.Parallel()
	p := &recordingPort{view: runtime.SpecTaskView{ID: "task_9", Status: "backlog"}}
	tl := mcptools.NewGetSpecTask(depsWithPort(p))
	if _, err := tl.Invoke(context.Background(), callerInv(`{"task_id":"task_9"}`)); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if p.lastTaskID != "task_9" {
		t.Errorf("task id = %q", p.lastTaskID)
	}
}

func TestStartSpecTaskPlanningTool(t *testing.T) {
	t.Parallel()
	p := &recordingPort{view: runtime.SpecTaskView{ID: "task_1", Status: "queued_spec_generation"}}
	tl := mcptools.NewStartSpecTaskPlanning(depsWithPort(p))
	if tl.Name() != mcptools.StartSpecTaskPlanningName {
		t.Errorf("Name = %q", tl.Name())
	}
	if _, err := tl.Invoke(context.Background(), callerInv(`{"task_id":"task_1"}`)); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if p.lastTaskID != "task_1" {
		t.Errorf("task id = %q", p.lastTaskID)
	}
}

func TestReviewSpecTaskSpecTool(t *testing.T) {
	t.Parallel()
	p := &recordingPort{review: runtime.SpecReviewView{TaskID: "task_1", Requirements: "reqs"}}
	tl := mcptools.NewReviewSpecTaskSpec(depsWithPort(p))
	out, err := tl.Invoke(context.Background(), callerInv(`{"task_id":"task_1"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !strings.Contains(string(out), "reqs") {
		t.Errorf("output missing requirements: %s", out)
	}
}

func TestApproveSpecTaskSpecTool(t *testing.T) {
	t.Parallel()
	p := &recordingPort{view: runtime.SpecTaskView{ID: "task_1", Status: "spec_approved"}}
	tl := mcptools.NewApproveSpecTaskSpec(depsWithPort(p))
	if _, err := tl.Invoke(context.Background(), callerInv(`{"task_id":"task_1"}`)); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if p.lastTaskID != "task_1" {
		t.Errorf("task id = %q", p.lastTaskID)
	}
}

func TestRequestSpecTaskChangesTool(t *testing.T) {
	t.Parallel()
	p := &recordingPort{view: runtime.SpecTaskView{ID: "task_1", Status: "spec_revision"}}
	tl := mcptools.NewRequestSpecTaskChanges(depsWithPort(p))
	if _, err := tl.Invoke(context.Background(), callerInv(`{"task_id":"task_1","comment":"tighten scope"}`)); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if p.lastComment != "tighten scope" {
		t.Errorf("comment = %q", p.lastComment)
	}
}

func TestRequestSpecTaskChangesTool_RequiresComment(t *testing.T) {
	t.Parallel()
	tl := mcptools.NewRequestSpecTaskChanges(depsWithPort(&recordingPort{}))
	if _, err := tl.Invoke(context.Background(), callerInv(`{"task_id":"task_1"}`)); err == nil {
		t.Error("expected error when comment missing")
	}
}

func TestCreateSpecTaskPRsTool_MapsMultiplePRs(t *testing.T) {
	t.Parallel()
	p := &recordingPort{view: runtime.SpecTaskView{
		ID:     "task_1",
		Status: "pull_request",
		PullRequests: []runtime.PullRequestView{
			{RepositoryName: "helix", URL: "https://x/pr/1", State: "open"},
			{RepositoryName: "docs", URL: "https://x/pr/2", State: "open"},
		},
	}}
	tl := mcptools.NewCreateSpecTaskPRs(depsWithPort(p))
	if tl.Name() != mcptools.CreateSpecTaskPRsName {
		t.Errorf("Name = %q", tl.Name())
	}
	out, err := tl.Invoke(context.Background(), callerInv(`{"task_id":"task_1"}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !strings.Contains(string(out), "pr/1") || !strings.Contains(string(out), "pr/2") {
		t.Errorf("output missing both PRs: %s", out)
	}
}
