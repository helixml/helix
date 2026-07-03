package mcptools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// This file holds the MCP project-discovery tools — list_projects and
// get_project — the surface an org-wide project-manager Bot uses to find
// out which Helix projects exist in its org before deciding which to
// manage. Both are thin adapters over the projects application service,
// which scopes every read to the caller's org.

// --- list_projects --------------------------------------------------------

const ListProjectsName tool.Name = "list_projects"

type ListProjects struct{ deps Deps }

func NewListProjects(deps Deps) *ListProjects { return &ListProjects{deps: deps} }

// listProjectsArgs is intentionally empty today (the org is taken from the
// caller). Kept as a struct so the schema is a stable empty object and new
// filters can be added later without changing the tool shape.
type listProjectsArgs struct{}

var listProjectsSchema = mustSchema[listProjectsArgs]()

func (t *ListProjects) Name() tool.Name                 { return ListProjectsName }
func (t *ListProjects) InputSchema() *jsonschema.Schema { return listProjectsSchema }
func (t *ListProjects) Description() string {
	return "List the Helix projects in your organization — id, name, description, and status. " +
		"Use this to discover which projects exist so you can manage their spec tasks " +
		"(pass a project's id as project_id to the spec-task tools)."
}
func (t *ListProjects) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	if t.deps.Projects == nil {
		return nil, errors.New("project access not wired on this server")
	}
	views, err := t.deps.Projects.List(ctx, inv.Caller)
	if err != nil {
		return nil, err
	}
	return json.Marshal(views)
}

// --- get_project ----------------------------------------------------------

const GetProjectName tool.Name = "get_project"

type GetProject struct{ deps Deps }

func NewGetProject(deps Deps) *GetProject { return &GetProject{deps: deps} }

type getProjectArgs struct {
	ProjectID string `json:"project_id"`
}

var getProjectSchema = mustSchema[getProjectArgs]()

func (t *GetProject) Name() tool.Name                 { return GetProjectName }
func (t *GetProject) InputSchema() *jsonschema.Schema { return getProjectSchema }
func (t *GetProject) Description() string {
	return "Get one Helix project in your organization by its project_id. Returns not-found " +
		"for a project in another organization."
}
func (t *GetProject) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args getProjectArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	if args.ProjectID == "" {
		return nil, errors.New("project_id is required")
	}
	if t.deps.Projects == nil {
		return nil, errors.New("project access not wired on this server")
	}
	view, err := t.deps.Projects.Get(ctx, inv.Caller, args.ProjectID)
	if err != nil {
		return nil, err
	}
	return json.Marshal(view)
}
