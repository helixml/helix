// Package spectasks is the org application service for managing the spec
// tasks in a Worker's own Helix project — the front-of-house the MCP
// spec-task tools consume, mirroring application/roles, application/workers,
// etc. It owns the org-side use-case concerns (extracting the caller's
// identity, surfacing a clear error when the runtime can't serve spec
// tasks) and depends only on the runtime.SpecTasks port — never on the
// Helix store or services directly, so org stays decoupled from the core.
package spectasks

import (
	"context"
	"errors"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
)

// Service is the spec-task application service. It is a thin policy layer
// over the runtime.SpecTasks port: it never sees a project ID (the port
// resolves that from the caller's worker state) and a Worker can only act
// on its own project's tasks.
type Service struct {
	port runtime.SpecTasks
}

// New constructs the service over the given port. The port is required;
// callers pass runtime.NoopSpecTasks{} when no real runtime is wired.
func New(port runtime.SpecTasks) *Service {
	return &Service{port: port}
}

// callerIdentity extracts and validates the caller's org + worker IDs.
// The worker is the subject of every call — there is no separate worker
// argument, so a Worker can only manage its own project's tasks.
func callerIdentity(caller tool.Worker) (string, orgchart.WorkerID, error) {
	if caller == nil {
		return "", "", errors.New("caller missing on invocation")
	}
	orgID := caller.OrganizationID()
	if orgID == "" {
		return "", "", errors.New("caller has no organization id")
	}
	workerID := caller.ID()
	if workerID == "" {
		return "", "", errors.New("caller has no worker id")
	}
	return orgID, orgchart.WorkerID(workerID), nil
}

func (s *Service) Create(ctx context.Context, caller tool.Worker, in runtime.CreateSpecTaskInput) (runtime.SpecTaskView, error) {
	orgID, workerID, err := callerIdentity(caller)
	if err != nil {
		return runtime.SpecTaskView{}, err
	}
	return s.port.Create(ctx, orgID, workerID, in)
}

func (s *Service) List(ctx context.Context, caller tool.Worker, filter runtime.ListSpecTasksFilter) ([]runtime.SpecTaskView, error) {
	orgID, workerID, err := callerIdentity(caller)
	if err != nil {
		return nil, err
	}
	return s.port.List(ctx, orgID, workerID, filter)
}

func (s *Service) Get(ctx context.Context, caller tool.Worker, taskID string) (runtime.SpecTaskView, error) {
	orgID, workerID, err := callerIdentity(caller)
	if err != nil {
		return runtime.SpecTaskView{}, err
	}
	return s.port.Get(ctx, orgID, workerID, taskID)
}

func (s *Service) StartPlanning(ctx context.Context, caller tool.Worker, taskID string) (runtime.SpecTaskView, error) {
	orgID, workerID, err := callerIdentity(caller)
	if err != nil {
		return runtime.SpecTaskView{}, err
	}
	return s.port.StartPlanning(ctx, orgID, workerID, taskID)
}

func (s *Service) ReviewSpec(ctx context.Context, caller tool.Worker, taskID string) (runtime.SpecReviewView, error) {
	orgID, workerID, err := callerIdentity(caller)
	if err != nil {
		return runtime.SpecReviewView{}, err
	}
	return s.port.ReviewSpec(ctx, orgID, workerID, taskID)
}

func (s *Service) ApproveSpec(ctx context.Context, caller tool.Worker, taskID string) (runtime.SpecTaskView, error) {
	orgID, workerID, err := callerIdentity(caller)
	if err != nil {
		return runtime.SpecTaskView{}, err
	}
	return s.port.ApproveSpec(ctx, orgID, workerID, taskID)
}

func (s *Service) RequestChanges(ctx context.Context, caller tool.Worker, taskID, comment string) (runtime.SpecTaskView, error) {
	orgID, workerID, err := callerIdentity(caller)
	if err != nil {
		return runtime.SpecTaskView{}, err
	}
	return s.port.RequestChanges(ctx, orgID, workerID, taskID, comment)
}

func (s *Service) CreatePullRequests(ctx context.Context, caller tool.Worker, taskID string) (runtime.SpecTaskView, error) {
	orgID, workerID, err := callerIdentity(caller)
	if err != nil {
		return runtime.SpecTaskView{}, err
	}
	return s.port.CreatePullRequests(ctx, orgID, workerID, taskID)
}
