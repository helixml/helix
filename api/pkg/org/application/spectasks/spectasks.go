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
	"fmt"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
)

// MemberVerifier confirms a Bot exists as a member of an org. *queries.Queries
// satisfies it (via GetBot). Kept as a tiny interface so the service can be
// unit-tested with a fake and so this package doesn't depend on queries.
type MemberVerifier interface {
	GetBot(ctx context.Context, orgID string, id orgchart.BotID) (orgchart.Bot, error)
}

// Service is the spec-task application service. It is a thin policy layer
// over the runtime.SpecTasks port. The caller's org + worker identity is
// taken only from the authenticated invocation (never from tool args); an
// optional projectID selects which project to act on (empty = the Worker's
// own project), and the runtime port enforces that the project belongs to
// the caller's org.
type Service struct {
	port    runtime.SpecTasks
	members MemberVerifier
}

// New constructs the service over the given port. The port is required;
// callers pass runtime.NoopSpecTasks{} when no real runtime is wired. members
// is optional: when set, every call verifies the caller Bot is a member of
// its org (defensive depth — the MCP mount already enforces this); nil skips
// the check (tests, non-MCP callers).
func New(port runtime.SpecTasks, members MemberVerifier) *Service {
	return &Service{port: port, members: members}
}

// callerIdentity extracts and validates the caller's org + worker IDs and,
// when a MemberVerifier is wired, confirms the Bot is a member of that org.
// Identity is taken from the authenticated caller, never from tool args.
func (s *Service) callerIdentity(ctx context.Context, caller tool.Caller) (string, orgchart.BotID, error) {
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
	if s.members != nil {
		if _, err := s.members.GetBot(ctx, orgID, orgchart.BotID(workerID)); err != nil {
			return "", "", fmt.Errorf("caller bot %s is not a member of org %s: %w", workerID, orgID, err)
		}
	}
	return orgID, orgchart.BotID(workerID), nil
}

func (s *Service) Create(ctx context.Context, caller tool.Caller, projectID string, in runtime.CreateSpecTaskInput) (runtime.SpecTaskView, error) {
	orgID, workerID, err := s.callerIdentity(ctx, caller)
	if err != nil {
		return runtime.SpecTaskView{}, err
	}
	return s.port.Create(ctx, orgID, workerID, projectID, in)
}

func (s *Service) List(ctx context.Context, caller tool.Caller, projectID string, filter runtime.ListSpecTasksFilter) ([]runtime.SpecTaskView, error) {
	orgID, workerID, err := s.callerIdentity(ctx, caller)
	if err != nil {
		return nil, err
	}
	return s.port.List(ctx, orgID, workerID, projectID, filter)
}

func (s *Service) Get(ctx context.Context, caller tool.Caller, projectID, taskID string) (runtime.SpecTaskView, error) {
	orgID, workerID, err := s.callerIdentity(ctx, caller)
	if err != nil {
		return runtime.SpecTaskView{}, err
	}
	return s.port.Get(ctx, orgID, workerID, projectID, taskID)
}

func (s *Service) Update(ctx context.Context, caller tool.Caller, projectID, taskID string, in runtime.UpdateSpecTaskInput) (runtime.SpecTaskView, error) {
	orgID, workerID, err := s.callerIdentity(ctx, caller)
	if err != nil {
		return runtime.SpecTaskView{}, err
	}
	return s.port.Update(ctx, orgID, workerID, projectID, taskID, in)
}

func (s *Service) StartPlanning(ctx context.Context, caller tool.Caller, projectID, taskID string) (runtime.SpecTaskView, error) {
	orgID, workerID, err := s.callerIdentity(ctx, caller)
	if err != nil {
		return runtime.SpecTaskView{}, err
	}
	return s.port.StartPlanning(ctx, orgID, workerID, projectID, taskID)
}

func (s *Service) StopAgent(ctx context.Context, caller tool.Caller, projectID, taskID string) (runtime.SpecTaskView, error) {
	orgID, workerID, err := s.callerIdentity(ctx, caller)
	if err != nil {
		return runtime.SpecTaskView{}, err
	}
	return s.port.StopAgent(ctx, orgID, workerID, projectID, taskID)
}

func (s *Service) ReviewSpec(ctx context.Context, caller tool.Caller, projectID, taskID string) (runtime.SpecReviewView, error) {
	orgID, workerID, err := s.callerIdentity(ctx, caller)
	if err != nil {
		return runtime.SpecReviewView{}, err
	}
	return s.port.ReviewSpec(ctx, orgID, workerID, projectID, taskID)
}

func (s *Service) ApproveSpec(ctx context.Context, caller tool.Caller, projectID, taskID string) (runtime.SpecTaskView, error) {
	orgID, workerID, err := s.callerIdentity(ctx, caller)
	if err != nil {
		return runtime.SpecTaskView{}, err
	}
	return s.port.ApproveSpec(ctx, orgID, workerID, projectID, taskID)
}

func (s *Service) RequestChanges(ctx context.Context, caller tool.Caller, projectID, taskID, comment string) (runtime.SpecTaskView, error) {
	orgID, workerID, err := s.callerIdentity(ctx, caller)
	if err != nil {
		return runtime.SpecTaskView{}, err
	}
	return s.port.RequestChanges(ctx, orgID, workerID, projectID, taskID, comment)
}

func (s *Service) CreatePullRequests(ctx context.Context, caller tool.Caller, projectID, taskID string) (runtime.SpecTaskView, error) {
	orgID, workerID, err := s.callerIdentity(ctx, caller)
	if err != nil {
		return runtime.SpecTaskView{}, err
	}
	return s.port.CreatePullRequests(ctx, orgID, workerID, projectID, taskID)
}
