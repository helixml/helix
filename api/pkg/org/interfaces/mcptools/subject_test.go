package mcptools

import (
	"context"
	"errors"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
)

type stubCaller struct{ id, org string }

func (c stubCaller) ID() string             { return c.id }
func (c stubCaller) OrganizationID() string { return c.org }

var _ tool.Caller = stubCaller{}

func TestSubjectForCallerBotActsAsItself(t *testing.T) {
	got, err := SubjectForCaller(context.Background(), stubCaller{id: "w-1", org: "org-1"})
	if err != nil {
		t.Fatal(err)
	}
	if got != orgchart.NodeID("w-1") {
		t.Fatalf("got %q", got)
	}
	if _, err := SubjectForCaller(context.Background(), nil); err == nil {
		t.Fatal("nil caller must fail closed")
	}
}

func TestSubjectForCallerTaskActsForBoundAgent(t *testing.T) {
	ctx := runtime.WithProjectPrincipal(context.Background(), runtime.ProjectPrincipal{ProjectID: "prj-1", ActingUserID: "u-1"})
	ctx = runtime.WithBoundWorker(ctx, "w-owner")
	got, err := SubjectForCaller(ctx, stubCaller{id: "spt-1", org: "org-1"})
	if err != nil {
		t.Fatal(err)
	}
	if got != orgchart.NodeID("w-owner") {
		t.Fatalf("task must act for the bound agent, got %q", got)
	}
}

func TestSubjectForCallerUnboundTaskFailsClosed(t *testing.T) {
	ctx := runtime.WithProjectPrincipal(context.Background(), runtime.ProjectPrincipal{ProjectID: "prj-2", ActingUserID: "u-1"})
	if _, err := SubjectForCaller(ctx, stubCaller{id: "spt-2", org: "org-1"}); !errors.Is(err, ErrNoBoundWorker) {
		t.Fatalf("unbound principal must fail closed, got %v", err)
	}
}

func TestBoundWorkerContext(t *testing.T) {
	ctx := runtime.WithBoundWorker(context.Background(), "")
	if _, ok := runtime.BoundWorkerFromContext(ctx); ok {
		t.Fatal("empty id must be a no-op stash")
	}
	ctx = runtime.WithBoundWorker(ctx, "w-1")
	id, ok := runtime.BoundWorkerFromContext(ctx)
	if !ok || id != "w-1" {
		t.Fatalf("got %q %v", id, ok)
	}
}
