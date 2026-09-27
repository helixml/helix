package hydra

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/stretchr/testify/require"
)

func TestIsMountpointDirectory(t *testing.T) {
	mounted, err := isMountpoint(context.Background(), t.TempDir())
	require.NoError(t, err)
	require.False(t, mounted)
}

func TestEnsureInstanceDiskImageReplacesIncompleteImage(t *testing.T) {
	if _, err := exec.LookPath("mkfs.ext4"); err != nil {
		t.Skip("mkfs.ext4 not available")
	}
	imagePath := filepath.Join(t.TempDir(), "home.ext4")
	// An interrupted earlier attempt leaves only the temporary image.
	require.NoError(t, os.WriteFile(imagePath+".tmp", []byte("partial"), 0o600))

	require.NoError(t, ensureInstanceDiskImage(context.Background(), imagePath, 1))
	info, err := os.Stat(imagePath)
	require.NoError(t, err)
	require.Equal(t, int64(1)<<30, info.Size())
	_, err = os.Stat(imagePath + ".tmp")
	require.ErrorIs(t, err, os.ErrNotExist)

	require.NoError(t, ensureInstanceDiskImage(context.Background(), imagePath, 1))
	require.ErrorContains(t, ensureInstanceDiskImage(context.Background(), imagePath, 2), "immutable")
}

func TestLegacyWorkspace(t *testing.T) {
	empty, populated := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(populated, "notes.md"), []byte("x"), 0o600))

	require.Empty(t, legacyWorkspace([]MountConfig{{Source: empty, Destination: "/home/retro/work"}}))
	require.Empty(t, legacyWorkspace([]MountConfig{{Source: filepath.Join(empty, "missing"), Destination: "/home/retro/work"}}))
	require.Empty(t, legacyWorkspace([]MountConfig{{Source: populated, Destination: "/workspace"}}))
	require.Equal(t, populated, legacyWorkspace([]MountConfig{{Source: populated, Destination: "/home/retro/work"}}))
}

func TestQuotaHomeTmpfs(t *testing.T) {
	headless := quotaHomeTmpfs(nil)
	require.Contains(t, headless, "/etc/claude-code")
	require.Contains(t, headless, "/run/user/1000")

	desktop := quotaHomeTmpfs([]MountConfig{{Source: "/data/sessions/ses_1/runtime", Destination: "/run/user/1000"}})
	require.NotContains(t, desktop, "/run/user/1000")
}

func TestQuotaHomeConfigChanged(t *testing.T) {
	pids := int64(4096)
	desired := &container.HostConfig{
		Resources:   container.Resources{PidsLimit: &pids},
		SecurityOpt: []string{"no-new-privileges", "seccomp=profile"},
		Tmpfs:       map[string]string{"/tmp": "size=256m"},
		Mounts:      []mount.Mount{{Source: "/quota/home", Target: "/home/retro"}, {Source: "/quota/home/work", Target: "/workspace"}},
	}
	existing := &container.HostConfig{
		Resources:   container.Resources{PidsLimit: &pids},
		SecurityOpt: []string{"seccomp=profile", "no-new-privileges"},
		Tmpfs:       map[string]string{"/tmp": "size=256m"},
		Mounts:      []mount.Mount{{Source: "/quota/home/work", Target: "/workspace"}, {Source: "/quota/home", Target: "/home/retro"}},
	}
	require.False(t, quotaHomeConfigChanged(existing, desired))

	existing.Resources.PidsLimit = nil
	require.True(t, quotaHomeConfigChanged(existing, desired))
	existing.Resources.PidsLimit = &pids

	existing.SecurityOpt = []string{"seccomp=profile"}
	require.True(t, quotaHomeConfigChanged(existing, desired))
	existing.SecurityOpt = desired.SecurityOpt

	existing.Tmpfs = map[string]string{"/tmp": "size=256m", "/etc/claude-code": "size=1m"}
	require.True(t, quotaHomeConfigChanged(existing, desired))
	existing.Tmpfs = desired.Tmpfs

	existing.Mounts[1].Source = "/unlimited/home"
	require.True(t, quotaHomeConfigChanged(existing, desired))
}

func TestReconcileOrphanInstanceDisks(t *testing.T) {
	old := instanceDisksBaseDir
	instanceDisksBaseDir = t.TempDir()
	t.Cleanup(func() { instanceDisksBaseDir = old })

	for _, id := range []string{"ses_live", "ses_dead", "ses_fresh"} {
		require.NoError(t, os.MkdirAll(filepath.Join(instanceDisksBaseDir, id, "home"), 0o700))
	}
	past := time.Now().Add(-2 * time.Hour)
	require.NoError(t, os.Chtimes(filepath.Join(instanceDisksBaseDir, "ses_live"), past, past))
	require.NoError(t, os.Chtimes(filepath.Join(instanceDisksBaseDir, "ses_dead"), past, past))

	reaped, _ := ReconcileOrphanInstanceDisks(context.Background(), map[string]bool{"ses_live": true}, time.Hour, false)
	require.Equal(t, []string{filepath.Join(instanceDisksBaseDir, "ses_dead")}, reaped)
	for id, exists := range map[string]bool{"ses_live": true, "ses_dead": false, "ses_fresh": true} {
		_, err := os.Stat(filepath.Join(instanceDisksBaseDir, id))
		require.Equal(t, exists, err == nil, id)
	}
}
