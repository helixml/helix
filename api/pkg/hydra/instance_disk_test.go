package hydra

import (
	"archive/tar"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/stretchr/testify/require"
)

func TestIsMountpointDirectory(t *testing.T) {
	mounted, err := isMountpoint(context.Background(), t.TempDir())
	require.NoError(t, err)
	require.False(t, mounted)
}

func TestExtractHomeArchive(t *testing.T) {
	var buf bytes.Buffer
	w := tar.NewWriter(&buf)
	require.NoError(t, w.WriteHeader(&tar.Header{Name: "./", Typeflag: tar.TypeDir, Mode: 0o755, Uid: os.Getuid(), Gid: os.Getgid()}))
	require.NoError(t, w.WriteHeader(&tar.Header{Name: "./.config/", Typeflag: tar.TypeDir, Mode: 0o700, Uid: os.Getuid(), Gid: os.Getgid()}))
	content := []byte("settings")
	require.NoError(t, w.WriteHeader(&tar.Header{Name: "./.config/app", Typeflag: tar.TypeReg, Mode: 0o600, Size: int64(len(content)), Uid: os.Getuid(), Gid: os.Getgid()}))
	_, err := w.Write(content)
	require.NoError(t, err)
	require.NoError(t, w.WriteHeader(&tar.Header{Name: "./config-link", Typeflag: tar.TypeSymlink, Linkname: ".config/app", Mode: 0o777, Uid: os.Getuid(), Gid: os.Getgid()}))
	require.NoError(t, w.Close())

	destination := t.TempDir()
	require.NoError(t, extractHomeArchive(&buf, destination))
	got, err := os.ReadFile(filepath.Join(destination, ".config", "app"))
	require.NoError(t, err)
	require.Equal(t, content, got)
	link, err := os.Readlink(filepath.Join(destination, "config-link"))
	require.NoError(t, err)
	require.Equal(t, ".config/app", link)
}

func TestExtractHomeArchiveRejectsTraversal(t *testing.T) {
	var buf bytes.Buffer
	w := tar.NewWriter(&buf)
	require.NoError(t, w.WriteHeader(&tar.Header{Name: "../../escape", Typeflag: tar.TypeReg, Mode: 0o600}))
	require.NoError(t, w.Close())
	require.ErrorContains(t, extractHomeArchive(&buf, t.TempDir()), "unsafe image home path")
}

func TestInstanceHardeningChanged(t *testing.T) {
	pids := int64(512)
	desired := &container.HostConfig{
		Resources:   container.Resources{PidsLimit: &pids},
		SecurityOpt: []string{"no-new-privileges", "seccomp=profile"},
		Tmpfs:       map[string]string{"/tmp": "size=256m"},
		Mounts:      []mount.Mount{{Source: "/quota/home", Target: "/home/retro"}},
	}
	existing := &container.HostConfig{
		Resources:   container.Resources{PidsLimit: &pids},
		SecurityOpt: []string{"seccomp=profile", "no-new-privileges"},
		Tmpfs:       map[string]string{"/tmp": "size=256m"},
		Mounts:      []mount.Mount{{Source: "/quota/home", Target: "/home/retro"}},
	}
	require.False(t, instanceHardeningChanged(existing, desired))

	existing.Resources.PidsLimit = nil
	require.True(t, instanceHardeningChanged(existing, desired))
	existing.Resources.PidsLimit = &pids
	existing.SecurityOpt = []string{"seccomp=profile"}
	require.True(t, instanceHardeningChanged(existing, desired))
	existing.SecurityOpt = desired.SecurityOpt
	existing.Mounts[0].Source = "/unlimited/home"
	require.True(t, instanceHardeningChanged(existing, desired))
}
