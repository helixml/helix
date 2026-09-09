package configregistry

import (
	"context"

	"github.com/helixml/helix/api/pkg/types"
)

// Sandbox default keys. A Bot with no sandbox config of its own inherits
// these; when they are unset too, the global spec-task defaults apply
// (ubuntu-desktop, standard preset).
const (
	DefaultSandboxRuntimeKey = "worker.sandbox_runtime"
	DefaultSandboxVCPUsKey   = "worker.sandbox_vcpus"
)

// GetDefaultSandboxConfig returns the org-level default sandbox runtime and
// size preset for Bots. Empty runtime / nil resources mean "not configured".
// An unknown vCPU count is ignored rather than propagated: the preset ladder
// is validated on write, so a stale value here must not break every
// activation in the org.
func (r *Registry) GetDefaultSandboxConfig(ctx context.Context, orgID string) (types.SandboxRuntime, *types.SandboxResourceOverrides) {
	runtime := ""
	if r.IsConfigured(ctx, orgID, DefaultSandboxRuntimeKey) {
		runtime, _ = r.GetString(ctx, orgID, DefaultSandboxRuntimeKey)
	}
	var resources *types.SandboxResourceOverrides
	if r.IsConfigured(ctx, orgID, DefaultSandboxVCPUsKey) {
		if vcpus, err := r.GetInt(ctx, orgID, DefaultSandboxVCPUsKey); err == nil && vcpus > 0 {
			if preset, ok := types.SpecTaskSandboxPresetForVCPUs(int(vcpus)); ok {
				resources = preset
			}
		}
	}
	return types.SandboxRuntime(runtime), resources
}
