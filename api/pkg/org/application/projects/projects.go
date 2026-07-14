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

// OwnProjectResolver resolves the Bot's runtime-owned project. It is separate
// from the explicit Bot.ProjectIDs allowlist because runtime provisioning owns
// that project pointer.
type OwnProjectResolver interface {
	OwnProjectID(ctx context.Context, orgID string, botID orgchart.BotID) (string, error)
}

// Service is the project-discovery application service.
type Service struct {
	port    runtime.Projects
	members MemberVerifier
	access  OwnProjectResolver
}

// New constructs the service over the given port. The port is required;
// callers pass runtime.NoopProjects{} when no real runtime is wired. members
// is optional: when set, every call verifies the caller Bot is a member of its
// org (defensive depth — the MCP mount already enforces this); nil skips it.
func New(port runtime.Projects, members MemberVerifier, access ...OwnProjectResolver) *Service {
	var resolver OwnProjectResolver
	if len(access) > 0 {
		resolver = access[0]
	}
	return &Service{port: port, members: members, access: resolver}
}

// callerIdentity extracts and validates the caller's org + worker IDs and,
// when a MemberVerifier is wired, confirms the Bot is a member of that org.
func (s *Service) callerIdentity(ctx context.Context, caller tool.Caller) (string, orgchart.BotID, orgchart.Bot, error) {
	if caller == nil {
		return "", "", orgchart.Bot{}, errors.New("caller missing on invocation")
	}
	orgID := caller.OrganizationID()
	if orgID == "" {
		return "", "", orgchart.Bot{}, errors.New("caller has no organization id")
	}
	workerID := caller.ID()
	if workerID == "" {
		return "", "", orgchart.Bot{}, errors.New("caller has no worker id")
	}
	var bot orgchart.Bot
	if s.members != nil {
		var err error
		bot, err = s.members.GetBot(ctx, orgID, orgchart.BotID(workerID))
		if err != nil {
			return "", "", orgchart.Bot{}, fmt.Errorf("caller bot %s is not a member of org %s: %w", workerID, orgID, err)
		}
	}
	return orgID, orgchart.BotID(workerID), bot, nil
}

func (s *Service) allowedProjectIDs(ctx context.Context, orgID string, botID orgchart.BotID, bot orgchart.Bot) (map[string]struct{}, error) {
	allowed := make(map[string]struct{}, len(bot.ProjectIDs)+1)
	for _, projectID := range bot.ProjectIDs {
		allowed[projectID] = struct{}{}
	}
	if s.access != nil {
		ownProjectID, err := s.access.OwnProjectID(ctx, orgID, botID)
		if err != nil {
			return nil, fmt.Errorf("resolve bot's own project: %w", err)
		}
		if ownProjectID != "" {
			allowed[ownProjectID] = struct{}{}
		}
	}
	return allowed, nil
}

// List returns the projects in the caller's org.
func (s *Service) List(ctx context.Context, caller tool.Caller) ([]runtime.ProjectView, error) {
	orgID, botID, bot, err := s.callerIdentity(ctx, caller)
	if err != nil {
		return nil, err
	}
	views, err := s.port.List(ctx, orgID)
	if err != nil || s.access == nil {
		return views, err
	}
	allowed, err := s.allowedProjectIDs(ctx, orgID, botID, bot)
	if err != nil {
		return nil, err
	}
	filtered := make([]runtime.ProjectView, 0, len(views))
	for _, view := range views {
		if _, ok := allowed[view.ID]; ok {
			filtered = append(filtered, view)
		}
	}
	return filtered, nil
}

// Get returns one project by id, scoped to the caller's org.
func (s *Service) Get(ctx context.Context, caller tool.Caller, projectID string) (runtime.ProjectView, error) {
	orgID, botID, bot, err := s.callerIdentity(ctx, caller)
	if err != nil {
		return runtime.ProjectView{}, err
	}
	if s.access != nil {
		allowed, err := s.allowedProjectIDs(ctx, orgID, botID, bot)
		if err != nil {
			return runtime.ProjectView{}, err
		}
		if _, ok := allowed[projectID]; !ok {
			return runtime.ProjectView{}, fmt.Errorf("bot %s does not have access to project %s", botID, projectID)
		}
	}
	return s.port.Get(ctx, orgID, projectID)
}
