package helix

import (
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/types"
)

// SessionLaunchConfig is the sandbox runtime and size a bot session's
// container is launched with. The spawner resolves it from the Bot on every
// activation and persists it on the session, so every launch path — fresh
// start, message auto-start, resume, auto-wake cold start, dead-container
// reconcile — reads the same values. This is the org-bot analogue of the
// spec task being the source of truth for its own container.
type SessionLaunchConfig struct {
	SandboxRuntime   types.SandboxRuntime
	SandboxResources types.SandboxResourceOverrides
}

// Headless reports whether the config selects the agent-only runtime.
func (c SessionLaunchConfig) Headless() bool {
	return c.SandboxRuntime == types.SandboxRuntimeHeadlessUbuntu
}

// EffectiveLaunchConfig resolves bot → org default → global default, using
// the same helpers spec tasks use so there is exactly one preset ladder and
// one default runtime in the system.
func EffectiveLaunchConfig(bot orgchart.Node, orgRuntime types.SandboxRuntime, orgResources *types.SandboxResourceOverrides) SessionLaunchConfig {
	runtime := types.SandboxRuntime(bot.SandboxRuntime)
	if runtime == "" {
		runtime = orgRuntime
	}
	var resources *types.SandboxResourceOverrides
	if bot.SandboxVCPUs > 0 && bot.SandboxMemoryMB > 0 {
		resources = &types.SandboxResourceOverrides{VCPUs: bot.SandboxVCPUs, MemoryMB: bot.SandboxMemoryMB}
	} else if orgResources != nil {
		copied := *orgResources
		resources = &copied
	}
	return SessionLaunchConfig{
		SandboxRuntime:   types.EffectiveSpecTaskSandboxRuntime(runtime),
		SandboxResources: types.EffectiveSpecTaskSandboxResources(resources),
	}
}
