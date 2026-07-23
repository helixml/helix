package mcptools_test

import (
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/tool"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	"github.com/helixml/helix/api/pkg/org/interfaces/mcptools"
)

// repositoryToolNames is the set the repository surface adds.
var repositoryToolNames = []tool.Name{
	mcptools.ListRepositoriesName,
	mcptools.ListBotRepositoriesName,
	mcptools.AttachRepositoryName,
	mcptools.DetachRepositoryName,
}

// TestRepositoryToolsRegistered pins that RegisterBuiltins registers the
// repository tools so a Role listing one resolves it.
func TestRepositoryToolsRegistered(t *testing.T) {
	t.Parallel()
	s := orggorm.GetOrgTestDB(t)
	reg := mcptools.NewRegistry()
	deps := mcptools.DefaultDeps(s)
	injectTestPublishing(&deps)
	if err := mcptools.RegisterBuiltins(reg, deps.Build()); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}
	for _, name := range repositoryToolNames {
		if _, err := reg.Get(name); err != nil {
			t.Errorf("tool %q not registered: %v", name, err)
		}
	}
}

// TestRepositoryToolsNotInBaseReadTools pins that repository tools are
// opt-in per Role (granted to Chief of Staff via OwnerBotTools, or
// explicitly via create_bot/attach_tool), not handed to every Bot.
func TestRepositoryToolsNotInBaseReadTools(t *testing.T) {
	t.Parallel()
	base := map[tool.Name]bool{}
	for _, n := range mcptools.BaseReadTools {
		base[n] = true
	}
	for _, name := range repositoryToolNames {
		if base[name] {
			t.Errorf("repository tool %q must not be in BaseReadTools", name)
		}
	}
}

// TestOwnerBotToolsIncludeRepositories pins that Chief of Staff seed
// tools include the repository surface.
func TestOwnerBotToolsIncludeRepositories(t *testing.T) {
	t.Parallel()
	owner := map[tool.Name]bool{}
	for _, n := range mcptools.OwnerBotTools() {
		owner[n] = true
	}
	for _, name := range repositoryToolNames {
		if !owner[name] {
			t.Errorf("OwnerBotTools missing repository tool %q", name)
		}
	}
}

// agentLifecycleToolNames is the set the agent start/stop/restart surface adds.
var agentLifecycleToolNames = []tool.Name{
	mcptools.StartBotName,
	mcptools.StopBotName,
	mcptools.RestartBotName,
}

// TestAgentLifecycleToolsRegistered pins that RegisterBuiltins registers
// start_bot / stop_bot / restart_bot.
func TestAgentLifecycleToolsRegistered(t *testing.T) {
	t.Parallel()
	s := orggorm.GetOrgTestDB(t)
	reg := mcptools.NewRegistry()
	deps := mcptools.DefaultDeps(s)
	injectTestPublishing(&deps)
	if err := mcptools.RegisterBuiltins(reg, deps.Build()); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}
	for _, name := range agentLifecycleToolNames {
		if _, err := reg.Get(name); err != nil {
			t.Errorf("tool %q not registered: %v", name, err)
		}
	}
}

// TestOwnerBotToolsIncludeAgentLifecycle pins that Chief of Staff seed
// tools include start/stop/restart.
func TestOwnerBotToolsIncludeAgentLifecycle(t *testing.T) {
	t.Parallel()
	owner := map[tool.Name]bool{}
	for _, n := range mcptools.OwnerBotTools() {
		owner[n] = true
	}
	for _, name := range agentLifecycleToolNames {
		if !owner[name] {
			t.Errorf("OwnerBotTools missing agent lifecycle tool %q", name)
		}
	}
}
