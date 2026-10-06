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
	createIn     runtime.CreateSpecTaskInput
	lastProject  string
	lastTaskID   string
	lastComment  string
	lastFilter   runtime.ListSpecTasksFilter
	updateIn     runtime.UpdateSpecTaskInput
	stopCalls    int
	startCalls   int
	restartCalls int
	messageIn    runtime.SpecTaskMessageInput
	view         runtime.SpecTaskView
	review       runtime.SpecReviewView
	action       runtime.SpecTaskAgentActionView
	message      runtime.SpecTaskMessageView
	messages     []runtime.SpecTaskAgentMessageView
	lastLimit    int
	err          error
}

func (p *recordingPort) Create(_ context.Context, _ string, _ orgchart.NodeID, projectID string, in runtime.CreateSpecTaskInput) (runtime.SpecTaskView, error) {
	p.lastProject, p.createIn = projectID, in
	return p.view, p.err
}
func (p *recordingPort) List(_ context.Context, _ string, _ orgchart.NodeID, projectID string, f runtime.ListSpecTasksFilter) ([]runtime.SpecTaskView, error) {
	p.lastProject, p.lastFilter = projectID, f
	if p.err != nil {
		return nil, p.err
	}
	return []runtime.SpecTaskView{p.view}, nil
}
func (p *recordingPort) Get(_ context.Context, _ string, _ orgchart.NodeID, projectID, id string) (runtime.SpecTaskView, error) {
	p.lastProject, p.lastTaskID = projectID, id
	return p.view, p.err
}
func (p *recordingPort) Update(_ context.Context, _ string, _ orgchart.NodeID, projectID, id string, in runtime.UpdateSpecTaskInput) (runtime.SpecTaskView, error) {
	p.lastProject, p.lastTaskID, p.updateIn = projectID, id, in
	return p.view, p.err
}
func (p *recordingPort) StartPlanning(_ context.Context, _ string, _ orgchart.NodeID, projectID, id string) (runtime.SpecTaskView, error) {
	p.lastProject, p.lastTaskID = projectID, id
	return p.view, p.err
}
func (p *recordingPort) SendAgentMessage(_ context.Context, _ string, _ orgchart.NodeID, projectID, id string, in runtime.SpecTaskMessageInput) (runtime.SpecTaskMessageView, error) {
	p.lastProject, p.lastTaskID, p.messageIn = projectID, id, in
	return p.message, p.err
}
func (p *recordingPort) ListAgentMessages(_ context.Context, _ string, _ orgchart.NodeID, projectID, id string, limit int) ([]runtime.SpecTaskAgentMessageView, error) {
	p.lastProject, p.lastTaskID, p.lastLimit = projectID, id, limit
	return p.messages, p.err
}
func (p *recordingPort) StartAgent(_ context.Context, _ string, _ orgchart.NodeID, projectID, id string) (runtime.SpecTaskAgentActionView, error) {
	p.lastProject, p.lastTaskID = projectID, id
	p.startCalls++
	return p.action, p.err
}
func (p *recordingPort) StopAgent(_ context.Context, _ string, _ orgchart.NodeID, projectID, id string) (runtime.SpecTaskView, error) {
	p.lastProject, p.lastTaskID = projectID, id
	p.stopCalls++
	return p.view, p.err
}
func (p *recordingPort) RestartAgent(_ context.Context, _ string, _ orgchart.NodeID, projectID, id string) (runtime.SpecTaskAgentActionView, error) {
	p.lastProject, p.lastTaskID = projectID, id
	p.restartCalls++
	return p.action, p.err
}
func (p *recordingPort) ReviewSpec(_ context.Context, _ string, _ orgchart.NodeID, projectID, id string) (runtime.SpecReviewView, error) {
	p.lastProject, p.lastTaskID = projectID, id
	return p.review, p.err
}
func (p *recordingPort) ApproveSpec(_ context.Context, _ string, _ orgchart.NodeID, projectID, id string) (runtime.SpecTaskView, error) {
	p.lastProject, p.lastTaskID = projectID, id
	return p.view, p.err
}
func (p *recordingPort) RequestChanges(_ context.Context, _ string, _ orgchart.NodeID, projectID, id, comment string) (runtime.SpecTaskView, error) {
	p.lastProject, p.lastTaskID, p.lastComment = projectID, id, comment
	return p.view, p.err
}
func (p *recordingPort) CreatePullRequests(_ context.Context, _ string, _ orgchart.NodeID, projectID, id string) (runtime.SpecTaskView, error) {
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

func TestUpdateSpecTaskTool(t *testing.T) {
	t.Parallel()
	p := &recordingPort{view: runtime.SpecTaskView{ID: "task_9", Name: "Renamed", Status: "backlog"}}
	tl := mcptools.NewUpdateSpecTask(depsWithPort(p))
	if _, err := tl.Invoke(context.Background(), callerInv(`{"project_id":"prj_other","task_id":"task_9","name":"Renamed","priority":"high","skip_planning":true}`)); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if p.lastProject != "prj_other" || p.lastTaskID != "task_9" {
		t.Errorf("target = %q/%q", p.lastProject, p.lastTaskID)
	}
	if p.updateIn.Name == nil || *p.updateIn.Name != "Renamed" || p.updateIn.Priority == nil || *p.updateIn.Priority != "high" {
		t.Errorf("update input = %+v", p.updateIn)
	}
	if p.updateIn.SkipPlanning == nil || !*p.updateIn.SkipPlanning {
		t.Errorf("skip planning = %v", p.updateIn.SkipPlanning)
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

func TestStopSpecTaskAgentTool(t *testing.T) {
	t.Parallel()
	p := &recordingPort{view: runtime.SpecTaskView{ID: "task_1", Status: "implementation"}}
	tl := mcptools.NewStopSpecTaskAgent(depsWithPort(p))
	if _, err := tl.Invoke(context.Background(), callerInv(`{"task_id":"task_1"}`)); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if p.stopCalls != 1 || p.lastTaskID != "task_1" {
		t.Errorf("stop calls/task = %d/%q", p.stopCalls, p.lastTaskID)
	}
}

func TestSendSpecTaskAgentMessageTool(t *testing.T) {
	t.Parallel()
	p := &recordingPort{message: runtime.SpecTaskMessageView{TaskID: "task_1", SessionID: "ses_1", PromptID: "pmt_1"}}
	tl := mcptools.NewSendSpecTaskAgentMessage(depsWithPort(p))
	out, err := tl.Invoke(context.Background(), callerInv(`{"project_id":"prj_other","task_id":"task_1","content":"check CI","interrupt":true}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if p.lastProject != "prj_other" || p.lastTaskID != "task_1" || p.messageIn.Content != "check CI" || !p.messageIn.Interrupt {
		t.Errorf("target/input = %q/%q/%+v", p.lastProject, p.lastTaskID, p.messageIn)
	}
	if !strings.Contains(string(out), "pmt_1") {
		t.Errorf("output missing prompt id: %s", out)
	}
}

func TestSendSpecTaskAgentMessageTool_RequiresContent(t *testing.T) {
	t.Parallel()
	tl := mcptools.NewSendSpecTaskAgentMessage(depsWithPort(&recordingPort{}))
	if _, err := tl.Invoke(context.Background(), callerInv(`{"task_id":"task_1","content":"  "}`)); err == nil {
		t.Error("expected error when content is blank")
	}
}

func TestListSpecTaskAgentMessagesTool(t *testing.T) {
	t.Parallel()
	p := &recordingPort{messages: []runtime.SpecTaskAgentMessageView{{InteractionID: "int_1", AgentMessage: "done"}}}
	tl := mcptools.NewListSpecTaskAgentMessages(depsWithPort(p))
	out, err := tl.Invoke(context.Background(), callerInv(`{"project_id":"prj_other","task_id":"task_1","limit":5}`))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if p.lastProject != "prj_other" || p.lastTaskID != "task_1" || p.lastLimit != 5 {
		t.Errorf("target/limit = %q/%q/%d", p.lastProject, p.lastTaskID, p.lastLimit)
	}
	if !strings.Contains(string(out), "done") {
		t.Errorf("output missing agent response: %s", out)
	}
}

func TestSpecTaskAgentLifecycleTools(t *testing.T) {
	t.Parallel()
	p := &recordingPort{action: runtime.SpecTaskAgentActionView{TaskID: "task_1", SessionID: "ses_1", Status: "started"}}
	if _, err := mcptools.NewStartSpecTaskAgent(depsWithPort(p)).Invoke(context.Background(), callerInv(`{"task_id":"task_1"}`)); err != nil {
		t.Fatalf("start Invoke: %v", err)
	}
	if _, err := mcptools.NewRestartSpecTaskAgent(depsWithPort(p)).Invoke(context.Background(), callerInv(`{"task_id":"task_1"}`)); err != nil {
		t.Fatalf("restart Invoke: %v", err)
	}
	if p.startCalls != 1 || p.restartCalls != 1 {
		t.Errorf("start/restart calls = %d/%d", p.startCalls, p.restartCalls)
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
