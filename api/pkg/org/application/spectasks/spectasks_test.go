package spectasks

import (
	"context"
	"errors"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
)

// fakePort records the (orgID, workerID, projectID) it was called with and
// returns canned results, so the service's caller-extraction and pass-through
// can be asserted without a real runtime.
type fakePort struct {
	lastOrg     string
	lastWorker  orgchart.NodeID
	lastProject string
	createIn    runtime.CreateSpecTaskInput
	view        runtime.SpecTaskView
	review      runtime.SpecReviewView
	err         error
	lastTaskID  string
	lastComment string
	lastMessage runtime.SpecTaskMessageInput
	action      runtime.SpecTaskAgentActionView
	message     runtime.SpecTaskMessageView
	messages    []runtime.SpecTaskAgentMessageView
	lastLimit   int
}

func (f *fakePort) Create(_ context.Context, org string, w orgchart.NodeID, projectID string, in runtime.CreateSpecTaskInput) (runtime.SpecTaskView, error) {
	f.lastOrg, f.lastWorker, f.lastProject, f.createIn = org, w, projectID, in
	return f.view, f.err
}
func (f *fakePort) List(_ context.Context, org string, w orgchart.NodeID, projectID string, _ runtime.ListSpecTasksFilter) ([]runtime.SpecTaskView, error) {
	f.lastOrg, f.lastWorker, f.lastProject = org, w, projectID
	if f.err != nil {
		return nil, f.err
	}
	return []runtime.SpecTaskView{f.view}, nil
}
func (f *fakePort) Get(_ context.Context, org string, w orgchart.NodeID, projectID, taskID string) (runtime.SpecTaskView, error) {
	f.lastOrg, f.lastWorker, f.lastProject, f.lastTaskID = org, w, projectID, taskID
	return f.view, f.err
}
func (f *fakePort) Update(_ context.Context, org string, w orgchart.NodeID, projectID, taskID string, _ runtime.UpdateSpecTaskInput) (runtime.SpecTaskView, error) {
	f.lastOrg, f.lastWorker, f.lastProject, f.lastTaskID = org, w, projectID, taskID
	return f.view, f.err
}
func (f *fakePort) StartPlanning(_ context.Context, org string, w orgchart.NodeID, projectID, taskID string) (runtime.SpecTaskView, error) {
	f.lastOrg, f.lastWorker, f.lastProject, f.lastTaskID = org, w, projectID, taskID
	return f.view, f.err
}
func (f *fakePort) SendAgentMessage(_ context.Context, org string, w orgchart.NodeID, projectID, taskID string, in runtime.SpecTaskMessageInput) (runtime.SpecTaskMessageView, error) {
	f.lastOrg, f.lastWorker, f.lastProject, f.lastTaskID, f.lastMessage = org, w, projectID, taskID, in
	return f.message, f.err
}
func (f *fakePort) ListAgentMessages(_ context.Context, org string, w orgchart.NodeID, projectID, taskID string, limit int) ([]runtime.SpecTaskAgentMessageView, error) {
	f.lastOrg, f.lastWorker, f.lastProject, f.lastTaskID, f.lastLimit = org, w, projectID, taskID, limit
	return f.messages, f.err
}
func (f *fakePort) StartAgent(_ context.Context, org string, w orgchart.NodeID, projectID, taskID string) (runtime.SpecTaskAgentActionView, error) {
	f.lastOrg, f.lastWorker, f.lastProject, f.lastTaskID = org, w, projectID, taskID
	return f.action, f.err
}
func (f *fakePort) StopAgent(_ context.Context, org string, w orgchart.NodeID, projectID, taskID string) (runtime.SpecTaskView, error) {
	f.lastOrg, f.lastWorker, f.lastProject, f.lastTaskID = org, w, projectID, taskID
	return f.view, f.err
}
func (f *fakePort) RestartAgent(_ context.Context, org string, w orgchart.NodeID, projectID, taskID string) (runtime.SpecTaskAgentActionView, error) {
	f.lastOrg, f.lastWorker, f.lastProject, f.lastTaskID = org, w, projectID, taskID
	return f.action, f.err
}
func (f *fakePort) ReviewSpec(_ context.Context, org string, w orgchart.NodeID, projectID, taskID string) (runtime.SpecReviewView, error) {
	f.lastOrg, f.lastWorker, f.lastProject, f.lastTaskID = org, w, projectID, taskID
	return f.review, f.err
}
func (f *fakePort) ApproveSpec(_ context.Context, org string, w orgchart.NodeID, projectID, taskID string) (runtime.SpecTaskView, error) {
	f.lastOrg, f.lastWorker, f.lastProject, f.lastTaskID = org, w, projectID, taskID
	return f.view, f.err
}
func (f *fakePort) RequestChanges(_ context.Context, org string, w orgchart.NodeID, projectID, taskID, comment string) (runtime.SpecTaskView, error) {
	f.lastOrg, f.lastWorker, f.lastProject, f.lastTaskID, f.lastComment = org, w, projectID, taskID, comment
	return f.view, f.err
}
func (f *fakePort) CreatePullRequests(_ context.Context, org string, w orgchart.NodeID, projectID, taskID string) (runtime.SpecTaskView, error) {
	f.lastOrg, f.lastWorker, f.lastProject, f.lastTaskID = org, w, projectID, taskID
	return f.view, f.err
}

// fakeMembers satisfies MemberVerifier. err controls whether the caller Bot
// is treated as a member of its org.
type fakeMembers struct {
	err        error
	lastOrg    string
	lastWorker orgchart.NodeID
}

func (m *fakeMembers) GetBot(_ context.Context, org string, id orgchart.NodeID) (orgchart.Node, error) {
	m.lastOrg, m.lastWorker = org, id
	if m.err != nil {
		return orgchart.Node{}, m.err
	}
	return orgchart.Node{ID: id, OrganizationID: org}, nil
}

// fakeCaller satisfies the minimal Worker surface (ID + OrganizationID).
type fakeCaller struct {
	id  string
	org string
}

func (c fakeCaller) ID() string             { return c.id }
func (c fakeCaller) OrganizationID() string { return c.org }

func TestService_CreateExtractsCallerIdentity(t *testing.T) {
	t.Parallel()
	fp := &fakePort{view: runtime.SpecTaskView{ID: "task_1", Name: "x", Status: "backlog"}}
	svc := New(fp, nil)
	view, err := svc.Create(context.Background(), fakeCaller{id: "w-alice", org: "org-1"}, "", runtime.CreateSpecTaskInput{Name: "x", Description: "y"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if fp.lastOrg != "org-1" || fp.lastWorker != "w-alice" {
		t.Errorf("identity passed = (%q, %q), want (org-1, w-alice)", fp.lastOrg, fp.lastWorker)
	}
	if view.ID != "task_1" {
		t.Errorf("view.ID = %q, want task_1", view.ID)
	}
}

// TestService_ForwardsProjectID pins the cross-project pass-through: an
// explicit project_id reaches the port unchanged (the port is where the
// org-ownership check lives).
func TestService_ForwardsProjectID(t *testing.T) {
	t.Parallel()
	fp := &fakePort{view: runtime.SpecTaskView{ID: "task_9"}}
	svc := New(fp, nil)
	if _, err := svc.Get(context.Background(), fakeCaller{id: "w-pm", org: "org-1"}, "prj_other", "task_9"); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if fp.lastProject != "prj_other" {
		t.Errorf("project passed = %q, want prj_other", fp.lastProject)
	}
}

func TestService_RejectsCallerWithoutOrg(t *testing.T) {
	t.Parallel()
	svc := New(&fakePort{}, nil)
	if _, err := svc.Create(context.Background(), fakeCaller{id: "w-alice", org: ""}, "", runtime.CreateSpecTaskInput{Name: "x", Description: "y"}); err == nil {
		t.Error("expected error when caller has no organization")
	}
}

func TestService_RejectsNilCaller(t *testing.T) {
	t.Parallel()
	svc := New(&fakePort{}, nil)
	if _, err := svc.Get(context.Background(), nil, "", "task_1"); err == nil {
		t.Error("expected error on nil caller")
	}
}

// TestService_RejectsNonMemberBot pins the defensive membership check: when a
// MemberVerifier is wired and the caller Bot is not found in its org, the call
// is rejected before touching the port.
func TestService_RejectsNonMemberBot(t *testing.T) {
	t.Parallel()
	fp := &fakePort{}
	svc := New(fp, &fakeMembers{err: errors.New("bot not found")})
	if _, err := svc.Get(context.Background(), fakeCaller{id: "w-intruder", org: "org-1"}, "", "task_1"); err == nil {
		t.Error("expected error when caller bot is not a member of the org")
	}
	if fp.lastTaskID != "" {
		t.Errorf("port should not be called when membership fails, got taskID=%q", fp.lastTaskID)
	}
}

func TestService_PropagatesPortError(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("boom")
	svc := New(&fakePort{err: sentinel}, nil)
	if _, err := svc.Get(context.Background(), fakeCaller{id: "w-alice", org: "org-1"}, "", "task_1"); !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want sentinel", err)
	}
}

func TestService_RequestChangesPassesComment(t *testing.T) {
	t.Parallel()
	fp := &fakePort{view: runtime.SpecTaskView{ID: "task_1", Status: "spec_revision"}}
	svc := New(fp, nil)
	if _, err := svc.RequestChanges(context.Background(), fakeCaller{id: "w-alice", org: "org-1"}, "", "task_1", "fix scope"); err != nil {
		t.Fatalf("RequestChanges: %v", err)
	}
	if fp.lastComment != "fix scope" || fp.lastTaskID != "task_1" {
		t.Errorf("got comment=%q taskID=%q", fp.lastComment, fp.lastTaskID)
	}
}

func TestService_AgentControlsPassCallerIdentity(t *testing.T) {
	t.Parallel()
	fp := &fakePort{
		message: runtime.SpecTaskMessageView{TaskID: "task_1", SessionID: "ses_1", PromptID: "pmt_1"},
		action:  runtime.SpecTaskAgentActionView{TaskID: "task_1", SessionID: "ses_1", Status: "started"},
	}
	svc := New(fp, nil)
	caller := fakeCaller{id: "w-pm", org: "org-1"}

	msg, err := svc.SendAgentMessage(context.Background(), caller, "prj_target", "task_1", runtime.SpecTaskMessageInput{Content: "status?", Interrupt: true})
	if err != nil {
		t.Fatalf("SendAgentMessage: %v", err)
	}
	if msg.PromptID != "pmt_1" || fp.lastMessage.Content != "status?" || !fp.lastMessage.Interrupt {
		t.Errorf("message result/input = %+v / %+v", msg, fp.lastMessage)
	}
	fp.messages = []runtime.SpecTaskAgentMessageView{{InteractionID: "int_1", AgentMessage: "done"}}
	messages, err := svc.ListAgentMessages(context.Background(), caller, "prj_target", "task_1", 7)
	if err != nil {
		t.Fatalf("ListAgentMessages: %v", err)
	}
	if len(messages) != 1 || messages[0].AgentMessage != "done" || fp.lastLimit != 7 {
		t.Errorf("messages/limit = %+v/%d", messages, fp.lastLimit)
	}
	if _, err := svc.StartAgent(context.Background(), caller, "prj_target", "task_1"); err != nil {
		t.Fatalf("StartAgent: %v", err)
	}
	if _, err := svc.RestartAgent(context.Background(), caller, "prj_target", "task_1"); err != nil {
		t.Fatalf("RestartAgent: %v", err)
	}
	if fp.lastOrg != "org-1" || fp.lastWorker != "w-pm" || fp.lastProject != "prj_target" || fp.lastTaskID != "task_1" {
		t.Errorf("last target = %q/%q/%q/%q", fp.lastOrg, fp.lastWorker, fp.lastProject, fp.lastTaskID)
	}
}

func TestService_ReviewSpecReturnsReviewView(t *testing.T) {
	t.Parallel()
	fp := &fakePort{review: runtime.SpecReviewView{TaskID: "task_1", Requirements: "reqs"}}
	svc := New(fp, nil)
	rv, err := svc.ReviewSpec(context.Background(), fakeCaller{id: "w-alice", org: "org-1"}, "", "task_1")
	if err != nil {
		t.Fatalf("ReviewSpec: %v", err)
	}
	if rv.Requirements != "reqs" {
		t.Errorf("Requirements = %q, want reqs", rv.Requirements)
	}
}
