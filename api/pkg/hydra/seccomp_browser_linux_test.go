//go:build linux

package hydra

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/docker/docker/profiles/seccomp"
	"github.com/stretchr/testify/require"
)

func TestBuildHostConfigBrowserSandbox(t *testing.T) {
	dm := &DevContainerManager{manager: &Manager{dataDir: t.TempDir()}}
	for _, containerType := range []DevContainerType{DevContainerTypeHeadless, DevContainerTypeUbuntu} {
		hostConfig, err := dm.buildHostConfig(&CreateDevContainerRequest{ContainerType: containerType, BrowserSandbox: true})
		require.NoError(t, err)
		require.False(t, hostConfig.Privileged)
		require.Empty(t, hostConfig.CapAdd)
		require.Contains(t, hostConfig.CapDrop, "SYS_ADMIN")
		require.Empty(t, hostConfig.Resources.Devices)
		require.Len(t, hostConfig.SecurityOpt, 1)

		var profile seccomp.Seccomp
		require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(hostConfig.SecurityOpt[0], "seccomp=")), &profile))
		require.Equal(t, seccomp.DefaultProfile().DefaultAction, profile.DefaultAction)
		allowed := map[string]bool{}
		for _, rule := range profile.Syscalls {
			if rule.Includes != nil && len(rule.Includes.Caps) > 0 {
				continue // needs a capability the container doesn't have
			}
			for _, name := range rule.Names {
				if rule.Action == "SCMP_ACT_ALLOW" && len(rule.Args) == 0 {
					allowed[name] = true
				}
			}
		}
		require.True(t, allowed["unshare"], "Chrome's sandbox needs unshare")
		require.True(t, allowed["clone"], "Chrome's sandbox needs clone with namespace flags")
		for _, blocked := range []string{"mount", "bpf", "setns", "keyctl", "init_module", "kexec_load"} {
			require.False(t, allowed[blocked], "%s must stay blocked", blocked)
		}
	}
}

func TestBuildHostConfigRejectsBrowserSandboxWithEngine(t *testing.T) {
	dm := &DevContainerManager{manager: &Manager{dataDir: t.TempDir()}}
	for _, req := range []*CreateDevContainerRequest{
		{ContainerType: DevContainerTypeUbuntu, BrowserSandbox: true, Privileged: true},
		{ContainerType: DevContainerTypeHeadless, BrowserSandbox: true, RootlessContainerEngine: true},
	} {
		_, err := dm.buildHostConfig(req)
		require.EqualError(t, err, "browser sandbox is for unprivileged containers without a container engine")
	}
}
