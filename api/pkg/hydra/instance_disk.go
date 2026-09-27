package hydra

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/rs/zerolog/log"
)

const (
	// instanceAgentCacheDir is where the settings-sync daemon installs
	// admin-pinned agent binaries. A quota-home container gets its own copy on
	// its disk instead of the host-wide shared cache.
	instanceAgentCacheDir = "/opt/helix/agent-cache"
	instanceUID           = 1000
)

// instanceDisksBaseDir holds one directory per quota-home session: the ext4
// image and its mountpoint. Var (not const) so tests can override it.
var instanceDisksBaseDir = "/data/instance-disks"

type instanceDiskPaths struct {
	dir   string
	image string
	home  string
	// initialized marks a seeded home. It lives beside the image, outside
	// the tenant-writable home, so the instance cannot trigger a re-seed.
	initialized string
}

func instanceDisk(sessionID string) instanceDiskPaths {
	dir := filepath.Join(instanceDisksBaseDir, sessionID)
	return instanceDiskPaths{
		dir:         dir,
		image:       filepath.Join(dir, "home.ext4"),
		home:        filepath.Join(dir, "home"),
		initialized: filepath.Join(dir, "initialized"),
	}
}

// instanceDiskLocks serializes disk operations per session, so concurrent
// starts, stops and deletes of one instance never format, mount or remove
// its disk at the same time. Entries are dropped once no caller holds them.
var instanceDiskLocks = struct {
	sync.Mutex
	held map[string]*instanceDiskLock
}{held: map[string]*instanceDiskLock{}}

type instanceDiskLock struct {
	sync.Mutex
	refs int
}

func lockInstanceDisk(sessionID string) (unlock func()) {
	instanceDiskLocks.Lock()
	l := instanceDiskLocks.held[sessionID]
	if l == nil {
		l = &instanceDiskLock{}
		instanceDiskLocks.held[sessionID] = l
	}
	l.refs++
	instanceDiskLocks.Unlock()

	l.Lock()
	return func() {
		l.Unlock()
		instanceDiskLocks.Lock()
		if l.refs--; l.refs == 0 {
			delete(instanceDiskLocks.held, sessionID)
		}
		instanceDiskLocks.Unlock()
	}
}

// prepareInstanceDisk mounts a capacity-limited filesystem and makes it the
// container's complete home. Work, tool state, caches and pinned agent
// binaries all live on it, so moving data between writable paths cannot
// evade the limit.
func prepareInstanceDisk(ctx context.Context, dockerClient *client.Client, image string, req *CreateDevContainerRequest) error {
	if !isResourceID(req.SessionID, "ses_") {
		return fmt.Errorf("disk quota requires a valid session id, got %q", req.SessionID)
	}
	defer lockInstanceDisk(req.SessionID)()
	disk := instanceDisk(req.SessionID)
	if err := os.MkdirAll(disk.home, 0o700); err != nil {
		return fmt.Errorf("create instance disk directory: %w", err)
	}
	if err := ensureInstanceDiskImage(ctx, disk.image, req.DiskSizeGB); err != nil {
		return err
	}
	mounted, err := isMountpoint(ctx, disk.home)
	if err != nil {
		return err
	}
	if !mounted {
		if output, err := exec.CommandContext(ctx, "mount", "-o", "loop,nodev,nosuid", disk.image, disk.home).CombinedOutput(); err != nil {
			return fmt.Errorf("mount instance disk: %w: %s", err, output)
		}
	}

	if _, err := os.Stat(disk.initialized); errors.Is(err, os.ErrNotExist) {
		if err := initializeInstanceHome(ctx, dockerClient, image, req.SessionID, disk.home, legacyWorkspace(req.Mounts)); err != nil {
			return err
		}
		if err := os.WriteFile(disk.initialized, nil, 0o600); err != nil {
			return fmt.Errorf("write instance disk marker: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("inspect instance disk marker: %w", err)
	}

	workPath := filepath.Join(disk.home, "work")
	agentCachePath := filepath.Join(disk.home, ".cache", "helix-agent-cache")
	for _, dir := range []string{workPath, filepath.Join(disk.home, ".cache"), agentCachePath, filepath.Join(agentCachePath, "opencode")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
		if err := os.Chown(dir, instanceUID, instanceUID); err != nil {
			return fmt.Errorf("own %s: %w", dir, err)
		}
	}

	mounts := []MountConfig{
		{Source: disk.home, Destination: "/home/retro"},
		{Source: agentCachePath, Destination: instanceAgentCacheDir},
	}
	for _, m := range req.Mounts {
		switch {
		case m.Destination == "/home/retro" || m.Destination == instanceAgentCacheDir:
			continue
		case m.Destination == "/home/retro/work" || m.Destination == "/workspace",
			m.Destination == m.Source && filepath.Base(m.Source) == req.SessionID:
			m.Source = workPath
		}
		mounts = append(mounts, m)
	}
	req.Mounts = mounts
	return nil
}

// ensureInstanceDiskImage creates and formats the disk image once. The image
// is formatted under a temporary name and renamed into place, so an
// interrupted creation is retried from scratch instead of leaving an
// unformatted image that can never be mounted.
func ensureInstanceDiskImage(ctx context.Context, imagePath string, sizeGB int) error {
	wantBytes := int64(sizeGB) << 30
	info, err := os.Stat(imagePath)
	if err == nil {
		if info.Size() != wantBytes {
			return fmt.Errorf("instance disk size is immutable: stored %d GB, requested %d GB", info.Size()>>30, sizeGB)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect instance disk image: %w", err)
	}

	tmpPath := imagePath + ".tmp"
	if err := os.Remove(tmpPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove incomplete instance disk image: %w", err)
	}
	file, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("create instance disk image: %w", err)
	}
	if err := file.Truncate(wantBytes); err != nil {
		file.Close()
		return fmt.Errorf("size instance disk image: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close instance disk image: %w", err)
	}
	if output, err := exec.CommandContext(ctx, "mkfs.ext4", "-q", "-F", "-m", "0", tmpPath).CombinedOutput(); err != nil {
		return fmt.Errorf("format instance disk: %w: %s", err, output)
	}
	if err := os.Rename(tmpPath, imagePath); err != nil {
		return fmt.Errorf("install instance disk image: %w", err)
	}
	return nil
}

// quotaHomeTmpfs bounds every path a quota-home container writes outside its
// disk. With the root filesystem closed to the agent user, these mounts and
// the disk are the only writable storage it has.
func quotaHomeTmpfs(mounts []MountConfig) map[string]string {
	tmpfs := map[string]string{
		"/tmp":     "rw,nosuid,nodev,size=256m,mode=1777",
		"/var/tmp": "rw,nosuid,nodev,size=64m,mode=1777",
		// Claude Code managed settings, rewritten on every settings sync.
		"/etc/claude-code": "rw,nosuid,nodev,noexec,size=1m,uid=1000,gid=1000,mode=0755",
	}
	// Desktop containers bind a per-session runtime dir for PipeWire here.
	if !slices.ContainsFunc(mounts, func(m MountConfig) bool { return m.Destination == "/run/user/1000" }) {
		tmpfs["/run/user/1000"] = "rw,nosuid,nodev,size=64m,uid=1000,gid=1000,mode=0700"
	}
	return tmpfs
}

// quotaHomeConfigChanged reports whether an existing container was created
// with different limits, mounts or security options than a quota-home
// container now requires, so it must be recreated rather than restarted.
func quotaHomeConfigChanged(existing, desired *container.HostConfig) bool {
	if existing == nil || desired == nil {
		return true
	}
	if !equalPtr(existing.Resources.PidsLimit, desired.Resources.PidsLimit) ||
		!sameElements(existing.SecurityOpt, desired.SecurityOpt) ||
		!maps.Equal(existing.Tmpfs, desired.Tmpfs) {
		return true
	}
	type bind struct {
		source, target string
		readOnly       bool
	}
	binds := func(mounts []mount.Mount) []bind {
		out := make([]bind, 0, len(mounts))
		for _, m := range mounts {
			out = append(out, bind{m.Source, m.Target, m.ReadOnly})
		}
		return out
	}
	return !sameElements(binds(existing.Mounts), binds(desired.Mounts))
}

func equalPtr[T comparable](a, b *T) bool {
	return (a == nil) == (b == nil) && (a == nil || *a == *b)
}

func sameElements[T comparable](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[T]int, len(a))
	for _, v := range a {
		counts[v]++
	}
	for _, v := range b {
		if counts[v] == 0 {
			return false
		}
		counts[v]--
	}
	return true
}

// legacyWorkspace returns the host workspace a session used before it had a
// disk, if it holds anything worth migrating.
func legacyWorkspace(mounts []MountConfig) string {
	for _, m := range mounts {
		if m.Destination != "/home/retro/work" {
			continue
		}
		entries, err := os.ReadDir(m.Source)
		if err != nil || len(entries) == 0 {
			return ""
		}
		return m.Source
	}
	return ""
}

// initializeInstanceHome fills a fresh disk with the image's /home/retro and,
// for a session that predates its disk, the contents of its old workspace.
// The copy runs inside a throwaway container of the image, so paths and
// symlinks in either source resolve there rather than on the sandbox host.
func initializeInstanceHome(ctx context.Context, dockerClient *client.Client, image, sessionID, home, legacyWork string) error {
	entries, err := os.ReadDir(home)
	if err != nil {
		return fmt.Errorf("read uninitialized instance disk: %w", err)
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(home, entry.Name())); err != nil {
			return fmt.Errorf("clear uninitialized instance disk: %w", err)
		}
	}

	script := "cp -a /home/retro/. /seed/"
	mounts := []mount.Mount{{Type: mount.TypeBind, Source: home, Target: "/seed"}}
	if legacyWork != "" {
		script += " && mkdir -p /seed/work && cp -a /legacy/. /seed/work/"
		mounts = append(mounts, mount.Mount{Type: mount.TypeBind, Source: legacyWork, Target: "/legacy", ReadOnly: true})
		log.Info().Str("session_id", sessionID).Str("legacy_workspace", legacyWork).Msg("Migrating workspace onto new instance disk")
	}

	name := "helix-home-seed-" + sessionID
	if old, err := dockerClient.ContainerInspect(ctx, name); err == nil {
		if err := dockerClient.ContainerRemove(ctx, old.ID, container.RemoveOptions{Force: true}); err != nil {
			return fmt.Errorf("remove stale home seed container: %w", err)
		}
	}
	created, err := dockerClient.ContainerCreate(ctx, &container.Config{
		Image:           image,
		User:            "0:0",
		Entrypoint:      []string{"/bin/sh", "-c", script},
		NetworkDisabled: true,
	}, &container.HostConfig{NetworkMode: "none", Mounts: mounts}, nil, nil, name)
	if err != nil {
		return fmt.Errorf("create home seed container: %w", err)
	}
	defer func() {
		removeCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := dockerClient.ContainerRemove(removeCtx, created.ID, container.RemoveOptions{Force: true}); err != nil {
			log.Warn().Err(err).Str("container", name).Msg("Failed to remove home seed container")
		}
	}()

	waitCh, errCh := dockerClient.ContainerWait(ctx, created.ID, container.WaitConditionNextExit)
	if err := dockerClient.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("start home seed container: %w", err)
	}
	select {
	case err := <-errCh:
		return fmt.Errorf("wait for home seed container: %w", err)
	case result := <-waitCh:
		if result.StatusCode == 0 {
			return nil
		}
		return fmt.Errorf("seed instance home: exit %d: %s", result.StatusCode, containerOutput(ctx, dockerClient, created.ID))
	}
}

func containerOutput(ctx context.Context, dockerClient *client.Client, containerID string) string {
	logs, err := dockerClient.ContainerLogs(ctx, containerID, container.LogsOptions{ShowStdout: true, ShowStderr: true, Tail: "20"})
	if err != nil {
		return "logs unavailable: " + err.Error()
	}
	defer logs.Close()
	var out bytes.Buffer
	if _, err := stdcopy.StdCopy(&out, &out, logs); err != nil {
		return "logs unavailable: " + err.Error()
	}
	return string(bytes.TrimSpace(out.Bytes()))
}

func isMountpoint(ctx context.Context, path string) (bool, error) {
	err := exec.CommandContext(ctx, "mountpoint", "-q", path).Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		switch exitErr.ExitCode() {
		case 1, 32:
			return false, nil
		}
	}
	return false, fmt.Errorf("check instance disk mountpoint: %w", err)
}

// unmountInstanceDisk releases a session's disk and its loop device. Call it
// only once no container uses the disk: mounting the image again while an
// old container still holds it would mount one ext4 filesystem twice.
func unmountInstanceDisk(ctx context.Context, sessionID string) error {
	if !isResourceID(sessionID, "ses_") {
		return fmt.Errorf("invalid session id %q", sessionID)
	}
	defer lockInstanceDisk(sessionID)()
	return unmountInstanceDiskDir(ctx, instanceDisk(sessionID).home)
}

func unmountInstanceDiskDir(ctx context.Context, home string) error {
	mounted, err := isMountpoint(ctx, home)
	if err != nil || !mounted {
		return err
	}
	if output, err := exec.CommandContext(ctx, "umount", home).CombinedOutput(); err != nil {
		return fmt.Errorf("unmount instance disk: %w: %s", err, output)
	}
	return nil
}

// removeInstanceDiskDir unmounts and deletes one base/<ses_id> directory.
func removeInstanceDiskDir(ctx context.Context, dir string) error {
	defer lockInstanceDisk(filepath.Base(dir))()
	if err := unmountInstanceDiskDir(ctx, filepath.Join(dir, "home")); err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove instance disk: %w", err)
	}
	return nil
}

func destroyInstanceDisk(ctx context.Context, sessionID string) error {
	if !isResourceID(sessionID, "ses_") {
		return fmt.Errorf("invalid session id %q", sessionID)
	}
	return removeInstanceDiskDir(ctx, instanceDisk(sessionID).dir)
}

// ReconcileOrphanInstanceDisks reaps the disks of sessions that are no longer
// live, so a delete that could not reach this host does not leak the image
// and its loop device.
func ReconcileOrphanInstanceDisks(ctx context.Context, liveSessionIDs map[string]bool, grace time.Duration, dryRun bool) (reaped []string, skipped []GCSkip) {
	return reconcileIDDirs(instanceDisksBaseDir, "instance-disks", "ses_", liveSessionIDs, grace, dryRun, func(dir string) error {
		return removeInstanceDiskDir(ctx, dir)
	})
}
