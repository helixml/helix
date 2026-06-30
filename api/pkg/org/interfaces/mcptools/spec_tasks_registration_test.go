package mcptools_test

import (
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/tool"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	"github.com/helixml/helix/api/pkg/org/interfaces/mcptools"
)

// specTaskToolNames is the full set the worker spec-task surface adds.
var specTaskToolNames = []tool.Name{
	mcptools.CreateSpecTaskName,
	mcptools.ListSpecTasksName,
	mcptools.GetSpecTaskName,
	mcptools.StartSpecTaskPlanningName,
	mcptools.ReviewSpecTaskSpecName,
	mcptools.ApproveSpecTaskSpecName,
	mcptools.RequestSpecTaskChangesName,
	mcptools.CreateSpecTaskPRsName,
}

// TestSpecTaskToolsRegistered pins that RegisterBuiltins registers every
// spec-task tool so a Role listing one resolves it.
func TestSpecTaskToolsRegistered(t *testing.T) {
	t.Parallel()
	s := orggorm.GetOrgTestDB(t)
	reg := mcptools.NewRegistry()
	if err := mcptools.RegisterBuiltins(reg, mcptools.DefaultDeps(s).Build()); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}
	for _, name := range specTaskToolNames {
		if _, err := reg.Get(name); err != nil {
			t.Errorf("tool %q not registered: %v", name, err)
		}
	}
}

// TestSpecTaskToolsNotInBaseReadTools pins that the mutating/approving
// spec-task tools are NOT in the universal baseline — they must be granted
// per-Role, not handed to every Worker by default.
func TestSpecTaskToolsNotInBaseReadTools(t *testing.T) {
	t.Parallel()
	base := map[tool.Name]bool{}
	for _, n := range mcptools.BaseReadTools {
		base[n] = true
	}
	for _, name := range specTaskToolNames {
		if base[name] {
			t.Errorf("spec-task tool %q must not be in BaseReadTools", name)
		}
	}
}
