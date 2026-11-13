package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDesktopAgentApplySandboxDisplay(t *testing.T) {
	cases := []struct {
		name          string
		display       *SandboxDisplay
		width, height int
		resolution    string
		zoom          int
	}{
		{"nil defaults to 1080p", nil, 1920, 1080, "1080p", 100},
		{"empty defaults to 1080p", &SandboxDisplay{}, 1920, 1080, "1080p", 100},
		{"4k implies 200% zoom", &SandboxDisplay{Resolution: "4k"}, 3840, 2160, "4k", 200},
		{"5k implies 200% zoom", &SandboxDisplay{Resolution: "5k"}, 5120, 2880, "5k", 200},
		{"explicit zoom wins", &SandboxDisplay{Resolution: "4k", ZoomLevel: 100}, 3840, 2160, "4k", 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			agent := &DesktopAgent{DisplayWidth: 2560, DisplayHeight: 1600}
			agent.ApplySandboxDisplay(tc.display)
			require.Equal(t, tc.width, agent.DisplayWidth)
			require.Equal(t, tc.height, agent.DisplayHeight)
			require.Equal(t, tc.resolution, agent.Resolution)
			require.Equal(t, tc.zoom, agent.ZoomLevel)
			w, h, fps := agent.GetEffectiveResolution()
			require.Equal(t, []int{tc.width, tc.height, 60}, []int{w, h, fps})
		})
	}
}

func TestSandboxDisplayValidate(t *testing.T) {
	require.NoError(t, (*SandboxDisplay)(nil).Validate())
	require.NoError(t, (&SandboxDisplay{}).Validate())
	require.NoError(t, (&SandboxDisplay{Resolution: "4k", ZoomLevel: 150}).Validate())
	require.Error(t, (&SandboxDisplay{Resolution: "8k"}).Validate())
	require.Error(t, (&SandboxDisplay{Resolution: "3840x2160"}).Validate())
	require.Error(t, (&SandboxDisplay{ZoomLevel: 50}).Validate())
}

// withNativeDisplay installs a host display for one test and restores none.
func withNativeDisplay(t *testing.T, d NativeDisplay) {
	t.Helper()
	require.NoError(t, SetNativeDisplay(d))
	t.Cleanup(func() { require.NoError(t, SetNativeDisplay(NativeDisplay{})) })
}

// A MacBook Pro on "More Space" (1800x1169 points at 2x) reports a 3600-wide
// 16:9 desktop at 200% zoom.
var macBookMoreSpace = NativeDisplay{Width: 3600, Height: 2026, ZoomLevel: 200}

func TestNativeDisplayIsTheDefaultWhenReported(t *testing.T) {
	withNativeDisplay(t, macBookMoreSpace)

	for _, d := range []*SandboxDisplay{nil, {}, {Resolution: "native"}} {
		agent := &DesktopAgent{}
		agent.ApplySandboxDisplay(d)
		require.Equal(t, "native", agent.Resolution)
		require.Equal(t, 3600, agent.DisplayWidth)
		require.Equal(t, 2026, agent.DisplayHeight)
		require.Equal(t, 200, agent.ZoomLevel)
		w, h, _ := agent.GetEffectiveResolution()
		require.Equal(t, []int{3600, 2026}, []int{w, h})
	}

	// An explicit preset still wins over the native default.
	agent := &DesktopAgent{}
	agent.ApplySandboxDisplay(&SandboxDisplay{Resolution: "1080p"})
	require.Equal(t, []int{1920, 1080, 100}, []int{agent.DisplayWidth, agent.DisplayHeight, agent.ZoomLevel})

	// Sessions configured through an app's external agent config default the same way.
	cfg := &ExternalAgentConfig{}
	w, h := cfg.GetEffectiveResolution()
	require.Equal(t, []int{3600, 2026, 200}, []int{w, h, cfg.GetEffectiveZoomLevel()})
	cfg = &ExternalAgentConfig{DisplayWidth: 2560, DisplayHeight: 1440}
	w, h = cfg.GetEffectiveResolution()
	require.Equal(t, []int{2560, 1440, 100}, []int{w, h, cfg.GetEffectiveZoomLevel()})

	require.NoError(t, (&SandboxDisplay{Resolution: "native"}).Validate())
}

func TestNativeDisplayUnreportedKeeps1080p(t *testing.T) {
	require.Nil(t, GetNativeDisplay())
	require.Equal(t, "1080p", DefaultSandboxResolution())
	require.Error(t, (&SandboxDisplay{Resolution: "native"}).Validate())

	// A task saved as native on a Mac falls back rather than starting at 0x0
	// if the host stops reporting a display.
	agent := &DesktopAgent{}
	agent.ApplySandboxDisplay(&SandboxDisplay{Resolution: "native"})
	require.Equal(t, []int{1920, 1080, 100}, []int{agent.DisplayWidth, agent.DisplayHeight, agent.ZoomLevel})
	require.Equal(t, "1080p", agent.Resolution)

	cfg := &ExternalAgentConfig{}
	w, h := cfg.GetEffectiveResolution()
	require.Equal(t, []int{1920, 1080, 100}, []int{w, h, cfg.GetEffectiveZoomLevel()})
}

func TestSetNativeDisplayValidates(t *testing.T) {
	t.Cleanup(func() { require.NoError(t, SetNativeDisplay(NativeDisplay{})) })
	require.Error(t, SetNativeDisplay(NativeDisplay{Width: 6016, Height: 3384, ZoomLevel: 200}), "wider than the VM's 5120 EDID")
	require.Error(t, SetNativeDisplay(NativeDisplay{Width: 3600, Height: 2026, ZoomLevel: 50}))
	require.Error(t, SetNativeDisplay(NativeDisplay{Width: 100, Height: 50, ZoomLevel: 100}))
	require.NoError(t, SetNativeDisplay(macBookMoreSpace))
	require.Equal(t, &macBookMoreSpace, GetNativeDisplay())
}
