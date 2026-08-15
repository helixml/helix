package sandbox

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
)

func testRegistry(t *testing.T) *RuntimeRegistry {
	t.Helper()
	r, err := NewRuntimeRegistry(config.Sandboxes{
		Runtimes:       "headless-ubuntu=ubuntu:22.04|sleep infinity,node22=node:22-bookworm-slim",
		DefaultRuntime: "headless-ubuntu",
	})
	require.NoError(t, err)
	return r
}

// The desktop runtime is the only one that needs display hardware; everything
// configured through HELIX_SANDBOX_RUNTIMES is headless and runs anywhere.
func TestRuntimeAvailabilityWithoutDisplayHost(t *testing.T) {
	got := testRegistry(t).Availability(false)

	byName := map[string]RuntimeAvailability{}
	for _, r := range got {
		byName[r.Name] = r
	}

	desktop := byName[string(types.SandboxRuntimeUbuntuDesktop)]
	require.True(t, desktop.RequiresDisplay)
	require.False(t, desktop.Available, "desktop must be unavailable with no render-capable host")
	require.NotEmpty(t, desktop.Reason, "an unavailable runtime must say why")

	for _, name := range []string{"headless-ubuntu", "node22"} {
		r := byName[name]
		require.False(t, r.RequiresDisplay, "%s should not require a display", name)
		require.True(t, r.Available, "%s must stay available on a CPU-only fleet", name)
		require.Empty(t, r.Reason)
	}
}

func TestRuntimeAvailabilityWithDisplayHost(t *testing.T) {
	for _, r := range testRegistry(t).Availability(true) {
		require.True(t, r.Available, "%s should be available when a render-capable host exists", r.Name)
		require.Empty(t, r.Reason)
	}
}

// Availability is sorted so CLI and UI output is stable across calls; the
// registry itself is a map and would otherwise iterate in random order.
func TestRuntimeAvailabilityIsSorted(t *testing.T) {
	got := testRegistry(t).Availability(true)
	require.Len(t, got, 3)
	for i := 1; i < len(got); i++ {
		require.Less(t, got[i-1].Name, got[i].Name)
	}
}

// The built-in desktop spec must carry RequiresDisplay, otherwise placement
// would silently schedule a compositor onto a host with no render node.
func TestBuiltinDesktopRequiresDisplay(t *testing.T) {
	spec, err := testRegistry(t).Resolve(&types.CreateSandboxRequest{
		Runtime: types.SandboxRuntimeUbuntuDesktop,
	})
	require.NoError(t, err)
	require.True(t, spec.RequiresDisplay)

	headless, err := testRegistry(t).Resolve(&types.CreateSandboxRequest{Runtime: "headless-ubuntu"})
	require.NoError(t, err)
	require.False(t, headless.RequiresDisplay)
}
