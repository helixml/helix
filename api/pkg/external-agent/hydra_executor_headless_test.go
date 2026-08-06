package external_agent

import (
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
)

func TestHeadlessEnvironmentAndMountsExcludeDesktopRuntime(t *testing.T) {
	executor := &HydraExecutor{
		helixAPIURL:                   "http://api:8080",
		workspaceBasePathForContainer: "/workspace",
		gpuVendor:                     "nvidia",
	}
	agent := &types.DesktopAgent{SessionID: "ses_1", UserID: "usr_1", Env: []string{"USER_API_TOKEN=token"}}
	env := executor.buildEnvVars(agent, "headless", "/data/workspaces/ses_1")
	joined := strings.Join(env, "\n")
	for _, forbidden := range []string{"GOW_REQUIRED_DEVICES", "NVIDIA_VISIBLE_DEVICES", "XDG_RUNTIME_DIR", "ZED_EXTERNAL_SYNC_ENABLED", "SETTINGS_SYNC_PORT"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("headless environment contains %s: %s", forbidden, joined)
		}
	}
	if !strings.Contains(joined, "HELIX_HEADLESS=1") || !strings.Contains(joined, "USER_API_TOKEN=token") {
		t.Fatalf("headless environment is missing runtime values: %s", joined)
	}

	mounts := executor.buildMounts(agent, "/data/workspaces/ses_1", "headless")
	for _, mount := range mounts {
		if mount.Destination == "/var/lib/docker" || mount.Destination == "/tmp/cores" || mount.Destination == "/run/user/1000" {
			t.Fatalf("headless mount includes desktop path: %+v", mount)
		}
	}
}
