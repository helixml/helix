package helix

import (
	"context"
	"errors"
	"fmt"

	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// ProjectStore is the slice of the Helix store the project-discovery
// runtime impl needs. *store.PostgresStore satisfies it structurally, so
// the server wires the real store with no adapter.
type ProjectStore interface {
	ListProjects(ctx context.Context, query *store.ListProjectsQuery) ([]*types.Project, error)
	GetProject(ctx context.Context, id string) (*types.Project, error)
}

// Projects is the helix-runtime implementation of runtime.Projects. Every
// read is scoped to the caller's org: List filters by org, Get asserts the
// project belongs to the org before returning it (a project id from
// another tenant is rejected). No existing helix code is modified.
type Projects struct {
	store ProjectStore
}

// NewProjects builds the impl. The store is required.
func NewProjects(s ProjectStore) (*Projects, error) {
	if s == nil {
		return nil, errors.New("helix.NewProjects: store is nil")
	}
	return &Projects{store: s}, nil
}

var _ runtime.Projects = (*Projects)(nil)

func (p *Projects) List(ctx context.Context, orgID string) ([]runtime.ProjectView, error) {
	if orgID == "" {
		return nil, errors.New("orgID is required")
	}
	projects, err := p.store.ListProjects(ctx, &store.ListProjectsQuery{OrganizationID: orgID})
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	out := make([]runtime.ProjectView, 0, len(projects))
	for _, proj := range projects {
		out = append(out, projectView(proj))
	}
	return out, nil
}

func (p *Projects) Get(ctx context.Context, orgID, projectID string) (runtime.ProjectView, error) {
	if orgID == "" {
		return runtime.ProjectView{}, errors.New("orgID is required")
	}
	proj, err := p.store.GetProject(ctx, projectID)
	if err != nil {
		return runtime.ProjectView{}, fmt.Errorf("get project: %w", err)
	}
	// Hard org scoping: never return another tenant's project.
	if proj.OrganizationID != orgID {
		return runtime.ProjectView{}, fmt.Errorf("project %s does not belong to this organization", projectID)
	}
	return projectView(proj), nil
}

func projectView(p *types.Project) runtime.ProjectView {
	return runtime.ProjectView{
		ID:             p.ID,
		Name:           p.Name,
		Description:    p.Description,
		Status:         p.Status,
		DefaultRepoID:  p.DefaultRepoID,
		DefaultAgentID: p.DefaultHelixAppID,
	}
}
