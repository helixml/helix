// Package projects is the org application service backing the MCP
// project-discovery tools (list_projects, get_project). It is a thin
// policy layer over the runtime.Projects port: it extracts the caller's
// org + worker identity from the authenticated invocation (never from
// tool args), optionally verifies the caller Bot is a member of that org,
// and delegates to the port — which scopes every read to that org.
package projects

import (
	"context"
	"errors"
	"fmt"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
)

// MemberVerifier confirms a Bot exists as a member of an org. *queries.Queries
// satisfies it (via GetBot). Kept tiny so the service is unit-testable with a
// fake and this package doesn't depend on queries.
type MemberVerifier interface {
	GetBot(ctx context.Context, orgID string, id orgchart.BotID) (orgchart.Bot, error)
}

// Service is the project-discovery application service.
type Service struct {
	port    runtime.Projects
	members MemberVerifier
}

// New constructs the service over the given port. The port is required;
// callers pass runtime.NoopProjects{} when no real runtime is wired. members
// is optional: when set, every call verifies the caller Bot is a member of its
// org (defensive depth — the MCP mount already enforces this); nil skips it.
func New(port runtime.Projects, members MemberVerifier) *Service {
	return &Service{port: port, members: members}
}

// callerIdentity extracts and validates the caller's org + worker IDs and,
// when a MemberVerifier is wired, confirms the Bot is a member of that org.
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

// List returns the projects in the caller's org.
func (s *Service) List(ctx context.Context, caller tool.Caller) ([]runtime.ProjectView, error) {
	orgID, _, err := s.callerIdentity(ctx, caller)
	if err != nil {
		return nil, err
	}
	return s.port.List(ctx, orgID)
}

// Get returns one project by id, scoped to the caller's org.
func (s *Service) Get(ctx context.Context, caller tool.Caller, projectID string) (runtime.ProjectView, error) {
	orgID, _, err := s.callerIdentity(ctx, caller)
	if err != nil {
		return runtime.ProjectView{}, err
	}
	return s.port.Get(ctx, orgID, projectID)
}
