package mcptools_test

import (
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/tool"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	"github.com/helixml/helix/api/pkg/org/interfaces/mcptools"
)

// projectToolNames is the set the project-discovery surface adds.
var projectToolNames = []tool.Name{
	mcptools.ListProjectsName,
	mcptools.GetProjectName,
}

// TestProjectToolsRegistered pins that RegisterBuiltins registers the
// project-discovery tools so a Role listing one resolves it.
func TestProjectToolsRegistered(t *testing.T) {
	t.Parallel()
	s := orggorm.GetOrgTestDB(t)
	reg := mcptools.NewRegistry()
	deps := mcptools.DefaultDeps(s)
	injectTestPublishing(&deps)
	if err := mcptools.RegisterBuiltins(reg, deps.Build()); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}
	for _, name := range projectToolNames {
		if _, err := reg.Get(name); err != nil {
			t.Errorf("tool %q not registered: %v", name, err)
		}
	}
}

// TestProjectToolsNotInBaseReadTools pins that project-discovery tools are
// opt-in per Role (granted explicitly, e.g. to a PM bot), not handed to
// every Worker via the universal baseline.
func TestProjectToolsNotInBaseReadTools(t *testing.T) {
	t.Parallel()
	base := map[tool.Name]bool{}
	for _, n := range mcptools.BaseReadTools {
		base[n] = true
	}
	for _, name := range projectToolNames {
		if base[name] {
			t.Errorf("project tool %q must not be in BaseReadTools", name)
		}
	}
}
