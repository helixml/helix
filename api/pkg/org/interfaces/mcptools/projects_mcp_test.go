package mcptools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
	"github.com/helixml/helix/api/pkg/org/interfaces/mcptools"
	"github.com/helixml/helix/api/pkg/org/interfaces/server"
)

// stubProjectsPort is a runtime.Projects returning canned org projects, so
// the MCP e2e can prove list_projects/get_project work through the whole
// server → registry → tool → projects service → port path.
type stubProjectsPort struct{ views []runtime.ProjectView }

func (s stubProjectsPort) List(_ context.Context, _ string) ([]runtime.ProjectView, error) {
	return s.views, nil
}
func (s stubProjectsPort) Get(_ context.Context, _, projectID string) (runtime.ProjectView, error) {
	for _, v := range s.views {
		if v.ID == projectID {
			return v, nil
		}
	}
	return runtime.ProjectView{}, fmt.Errorf("project %s not found", projectID)
}

// TestProjectDiscoveryOverMCP is the end-to-end check for the project
// discovery surface: a Bot granted the tools connects to the real HTTP MCP
// server and both tools are advertised and return the org's projects.
func TestProjectDiscoveryOverMCP(t *testing.T) {
	t.Parallel()

	s := orggorm.GetOrgTestDB(t)

	reg := mcptools.NewRegistry()
	cfg := mcptools.DefaultDeps(s)
	cfg.Projects = stubProjectsPort{views: []runtime.ProjectView{
		{ID: "prj_1", Name: "Alpha", Status: "active"},
		{ID: "prj_2", Name: "Beta", Status: "active"},
	}}
	if err := mcptools.RegisterBuiltins(reg, cfg.Build()); err != nil {
		t.Fatalf("register builtins: %v", err)
	}
	srv := httptest.NewServer(server.NewFromStore(s, reg, nil, nil, nil).Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()

	// Seed a PM bot granted the discovery tools.
	bot, err := orgchart.NewBot(
		"b-pm",
		"# PM\nProject manager bot.",
		[]tool.Name{mcptools.ListProjectsName, mcptools.GetProjectName},
		time.Now().UTC(),
		"org-test",
	)
	if err != nil {
		t.Fatalf("new bot: %v", err)
	}
	mustCreate(t, s.Bots.Create(ctx, bot))

	session := connectMCP(t, srv.URL, "b-pm")

	// Both tools advertised.
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	advertised := map[string]bool{}
	for _, tl := range tools.Tools {
		advertised[tl.Name] = true
	}
	if !advertised[string(mcptools.ListProjectsName)] || !advertised[string(mcptools.GetProjectName)] {
		t.Fatalf("project tools not advertised: %v", advertised)
	}

	// list_projects returns the org's projects.
	out, err := invokeTool(t, session, mcptools.ListProjectsName, map[string]any{})
	if err != nil {
		t.Fatalf("list_projects: %v", err)
	}
	var views []runtime.ProjectView
	if err := json.Unmarshal(out, &views); err != nil {
		t.Fatalf("unmarshal list_projects: %v (%s)", err, out)
	}
	if len(views) != 2 {
		t.Fatalf("list_projects returned %d, want 2: %s", len(views), out)
	}

	// get_project returns the named project.
	got, err := invokeTool(t, session, mcptools.GetProjectName, map[string]any{"project_id": "prj_2"})
	if err != nil {
		t.Fatalf("get_project: %v", err)
	}
	if !strings.Contains(string(got), "Beta") {
		t.Errorf("get_project prj_2 missing name Beta: %s", got)
	}

	// get_project for an unknown id surfaces an error, not another project.
	if _, err := invokeTool(t, session, mcptools.GetProjectName, map[string]any{"project_id": "prj_missing"}); err == nil {
		t.Error("expected error for unknown project id")
	}
}
