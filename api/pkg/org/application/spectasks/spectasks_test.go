package spectasks

import (
	"context"
	"errors"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
)

// fakePort records the (orgID, workerID) it was called with and returns
// canned results, so the service's caller-extraction and pass-through can
// be asserted without a real runtime.
type fakePort struct {
	lastOrg    string
	lastWorker orgchart.WorkerID
	createIn   runtime.CreateSpecTaskInput
	view       runtime.SpecTaskView
	review     runtime.SpecReviewView
	err        error
	lastTaskID string
	lastComment string
}

func (f *fakePort) Create(_ context.Context, org string, w orgchart.WorkerID, in runtime.CreateSpecTaskInput) (runtime.SpecTaskView, error) {
	f.lastOrg, f.lastWorker, f.createIn = org, w, in
	return f.view, f.err
}
func (f *fakePort) List(_ context.Context, org string, w orgchart.WorkerID, _ runtime.ListSpecTasksFilter) ([]runtime.SpecTaskView, error) {
	f.lastOrg, f.lastWorker = org, w
	if f.err != nil {
		return nil, f.err
	}
	return []runtime.SpecTaskView{f.view}, nil
}
func (f *fakePort) Get(_ context.Context, org string, w orgchart.WorkerID, taskID string) (runtime.SpecTaskView, error) {
	f.lastOrg, f.lastWorker, f.lastTaskID = org, w, taskID
	return f.view, f.err
}
func (f *fakePort) StartPlanning(_ context.Context, org string, w orgchart.WorkerID, taskID string) (runtime.SpecTaskView, error) {
	f.lastOrg, f.lastWorker, f.lastTaskID = org, w, taskID
	return f.view, f.err
}
func (f *fakePort) ReviewSpec(_ context.Context, org string, w orgchart.WorkerID, taskID string) (runtime.SpecReviewView, error) {
	f.lastOrg, f.lastWorker, f.lastTaskID = org, w, taskID
	return f.review, f.err
}
func (f *fakePort) ApproveSpec(_ context.Context, org string, w orgchart.WorkerID, taskID string) (runtime.SpecTaskView, error) {
	f.lastOrg, f.lastWorker, f.lastTaskID = org, w, taskID
	return f.view, f.err
}
func (f *fakePort) RequestChanges(_ context.Context, org string, w orgchart.WorkerID, taskID, comment string) (runtime.SpecTaskView, error) {
	f.lastOrg, f.lastWorker, f.lastTaskID, f.lastComment = org, w, taskID, comment
	return f.view, f.err
}
func (f *fakePort) CreatePullRequests(_ context.Context, org string, w orgchart.WorkerID, taskID string) (runtime.SpecTaskView, error) {
	f.lastOrg, f.lastWorker, f.lastTaskID = org, w, taskID
	return f.view, f.err
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
	svc := New(fp)
	view, err := svc.Create(context.Background(), fakeCaller{id: "w-alice", org: "org-1"}, runtime.CreateSpecTaskInput{Name: "x", Description: "y"})
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

func TestService_RejectsCallerWithoutOrg(t *testing.T) {
	t.Parallel()
	svc := New(&fakePort{})
	if _, err := svc.Create(context.Background(), fakeCaller{id: "w-alice", org: ""}, runtime.CreateSpecTaskInput{Name: "x", Description: "y"}); err == nil {
		t.Error("expected error when caller has no organization")
	}
}

func TestService_RejectsNilCaller(t *testing.T) {
	t.Parallel()
	svc := New(&fakePort{})
	if _, err := svc.Get(context.Background(), nil, "task_1"); err == nil {
		t.Error("expected error on nil caller")
	}
}

func TestService_PropagatesPortError(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("boom")
	svc := New(&fakePort{err: sentinel})
	if _, err := svc.Get(context.Background(), fakeCaller{id: "w-alice", org: "org-1"}, "task_1"); !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want sentinel", err)
	}
}

func TestService_RequestChangesPassesComment(t *testing.T) {
	t.Parallel()
	fp := &fakePort{view: runtime.SpecTaskView{ID: "task_1", Status: "spec_revision"}}
	svc := New(fp)
	if _, err := svc.RequestChanges(context.Background(), fakeCaller{id: "w-alice", org: "org-1"}, "task_1", "fix scope"); err != nil {
		t.Fatalf("RequestChanges: %v", err)
	}
	if fp.lastComment != "fix scope" || fp.lastTaskID != "task_1" {
		t.Errorf("got comment=%q taskID=%q", fp.lastComment, fp.lastTaskID)
	}
}

func TestService_ReviewSpecReturnsReviewView(t *testing.T) {
	t.Parallel()
	fp := &fakePort{review: runtime.SpecReviewView{TaskID: "task_1", Requirements: "reqs"}}
	svc := New(fp)
	rv, err := svc.ReviewSpec(context.Background(), fakeCaller{id: "w-alice", org: "org-1"}, "task_1")
	if err != nil {
		t.Fatalf("ReviewSpec: %v", err)
	}
	if rv.Requirements != "reqs" {
		t.Errorf("Requirements = %q, want reqs", rv.Requirements)
	}
}
