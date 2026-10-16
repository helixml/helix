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
