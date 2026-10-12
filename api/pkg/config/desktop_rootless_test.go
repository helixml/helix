package config

import (
	"testing"

	"github.com/kelseyhightower/envconfig"
	"github.com/stretchr/testify/require"
)

func TestDesktopRootlessProjectIDsFromEnvironment(t *testing.T) {
	t.Setenv("HELIX_SANDBOX_DESKTOP_ROOTLESS_PROJECT_IDS", "prj_target,prj_second")
	var cfg ServerConfig
	require.NoError(t, envconfig.Process("", &cfg))
	require.Equal(t, []string{"prj_target", "prj_second"}, cfg.Sandboxes.DesktopRootlessProjectIDs)
}

func TestDesktopRootlessForProject(t *testing.T) {
	require.True(t, DesktopRootlessForProject(true, nil, ""))
	require.False(t, DesktopRootlessForProject(false, nil, "prj_target"))
	require.True(t, DesktopRootlessForProject(false, []string{"prj_target", " prj_second "}, "prj_target"))
	require.True(t, DesktopRootlessForProject(false, []string{"prj_target", " prj_second "}, "prj_second"))
	require.False(t, DesktopRootlessForProject(true, []string{"prj_target", "prj_second"}, "prj_other"))
	require.False(t, DesktopRootlessForProject(true, []string{"prj_target"}, ""))
	require.True(t, DesktopRootlessForProject(true, []string{"", " "}, "prj_other"))
	require.False(t, DesktopRootlessProjectScopeEnabled(nil))
	require.False(t, DesktopRootlessProjectScopeEnabled([]string{"", " "}))
	require.True(t, DesktopRootlessProjectScopeEnabled([]string{"", "prj_target"}))
}
