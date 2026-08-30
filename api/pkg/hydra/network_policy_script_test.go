package hydra

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSandboxNetworkPolicyScopesFrameExportToVirtio(t *testing.T) {
	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "..", "sandbox", "06-setup-network-policy.sh"))
	require.NoError(t, err)

	runPolicy := func(t *testing.T, gpuVendor, framePort string) (string, string, error) {
		t.Helper()
		readyFile := filepath.Join(t.TempDir(), "network-ready")
		const harness = `
source "$1"
HELIX_NETWORK_READY_FILE="$2"
GPU_VENDOR="$3"
HELIX_FRAME_EXPORT_PORT="$4"
helix_ensure_sandbox_network() { :; }
helix_reset_iptables_chain() { :; }
helix_remove_all_iptables_rules() { :; }
helix_configure_ipv6_policy() { :; }
helix_iptables() { printf '%s\n' "$*"; }
helix_apply_sandbox_network_policy
`
		cmd := exec.Command("bash", "-c", harness, "network-policy-test", scriptPath, readyFile, gpuVendor, framePort)
		output, runErr := cmd.CombinedOutput()
		return string(output), readyFile, runErr
	}

	t.Run("linux ignores globally exported frame port", func(t *testing.T) {
		output, readyFile, runErr := runPolicy(t, "nvidia", "15937")
		require.NoError(t, runErr, output)
		require.NotContains(t, output, "10.0.2.2/32")
		require.Contains(t, output, "10.0.0.0/8")
		require.FileExists(t, readyFile)
	})

	t.Run("virtio allow precedes private range reject", func(t *testing.T) {
		output, readyFile, runErr := runPolicy(t, "virtio", "15937")
		require.NoError(t, runErr, output)
		allowIndex := strings.Index(output, "10.0.2.2/32")
		rejectIndex := strings.Index(output, "10.0.0.0/8")
		require.NotEqual(t, -1, allowIndex, output)
		require.NotEqual(t, -1, rejectIndex, output)
		require.Less(t, allowIndex, rejectIndex, output)
		require.FileExists(t, readyFile)
	})

	t.Run("virtio without frame port fails closed", func(t *testing.T) {
		output, readyFile, runErr := runPolicy(t, "virtio", "")
		require.Error(t, runErr)
		require.Contains(t, output, "HELIX_FRAME_EXPORT_PORT is required")
		_, statErr := os.Stat(readyFile)
		require.True(t, os.IsNotExist(statErr), "network ready marker unexpectedly exists")
	})
}
