package hydra

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/rs/zerolog/log"
)

// Disk-pressure admission control.
//
// The storage backing every session must never hit 0% free: ENOSPC inside a
// session's filesystem corrupts state, which cascades into Postgres faults and
// git-repository corruption. This guard watches the storage's FREE PERCENT and
// applies two protections, both keyed off thresholds that are configurable via
// env:
//
//   - Free <= refuse threshold (default 2%): refuse to START new dev
//     containers, surfacing a clear user-facing error.
//   - Free <= stop threshold (default 1%): an emergency brake STOPS (not
//     deletes) existing running dev containers so they stop writing. Sessions
//     remain resumable, the zvol/workspace is never touched.
//
// Two backends measure free space:
//
//   - zfs:    `zpool list -Hp -o size,free <pool>` against the ZFS pool that
//             backs zvols. Used when ZFS is available and a parent dataset is
//             configured. This is the original implementation.
//   - statfs: `syscall.Statfs(2)` against EACH of the configured data paths.
//             Used as a fallback on non-ZFS hosts (most K8s deployments,
//             which run on ext4 / xfs / overlay). The K8s helix-sandbox
//             chart provisions three separate PVCs per pod (docker-storage
//             /var/lib/docker, hydra-data /hydra-data, workspace-data
//             /data), and a fill on any one of them stops sandbox starts
//             (the original incident was a fill of /var/lib/docker mid
//             image-layer extract). The statfs backend measures every path
//             and uses the LOWEST free percentage as the admission signal,
//             so adding paths only ever makes the guard stricter.
//
// Fail policy:
//
//   - Backend reports a measurement: act on it (refuse / stop / allow per
//     thresholds).
//   - statfs fails with ENOENT for ANY configured path: the operator
//     misconfigured `HELIX_DISK_PRESSURE_PATHS`, FAIL CLOSED on start
//     (refuse) so we do not silently allow unprotected starts. Monitor
//     logs and skips the tick.
//   - statfs fails with a non-ENOENT error on a path: log and skip that
//     path; other paths may still produce a usable reading.
//   - Both backends unavailable (ZFS absent AND statfs probe errors on
//     every path for other reasons): FAIL OPEN with a prominent warn log,
//     matching pre-fallback behaviour. This should be unreachable in
//     practice on any sane host.

const (
	defaultDiskPressureRefuseFreePct = 2.0
	defaultDiskPressureStopFreePct   = 1.0
	defaultDiskPressureCheckInterval = 30 * time.Second
)

// diskPressureConfig holds the parsed env knobs. Read once on first use.
type diskPressureConfig struct {
	enabled       bool
	refuseFreePct float64
	stopFreePct   float64
	checkInterval time.Duration
}

var (
	diskPressureConfigOnce sync.Once
	diskPressureConfigVal  diskPressureConfig
)

// getDiskPressureConfig reads the disk-pressure env knobs once and caches them.
//
//	HELIX_DISK_PRESSURE_ENABLED          default true
//	HELIX_DISK_PRESSURE_REFUSE_FREE_PCT  default 2.0  (≤ this → refuse new)
//	HELIX_DISK_PRESSURE_STOP_FREE_PCT    default 1.0  (≤ this → stop existing)
//	HELIX_DISK_PRESSURE_CHECK_INTERVAL   default 30s
func getDiskPressureConfig() diskPressureConfig {
	diskPressureConfigOnce.Do(func() {
		diskPressureConfigVal = diskPressureConfig{
			enabled:       envBool("HELIX_DISK_PRESSURE_ENABLED", true),
			refuseFreePct: envFloat("HELIX_DISK_PRESSURE_REFUSE_FREE_PCT", defaultDiskPressureRefuseFreePct),
			stopFreePct:   envFloat("HELIX_DISK_PRESSURE_STOP_FREE_PCT", defaultDiskPressureStopFreePct),
			checkInterval: envDuration("HELIX_DISK_PRESSURE_CHECK_INTERVAL", defaultDiskPressureCheckInterval),
		}
	})
	return diskPressureConfigVal
}

// resetDiskPressureConfig clears the cached config so tests can re-read env.
func resetDiskPressureConfig() {
	diskPressureConfigOnce = sync.Once{}
	diskPressureConfigVal = diskPressureConfig{}
}

func envBool(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		log.Warn().Str("key", key).Str("value", v).Bool("default", def).
			Msg("disk-pressure: invalid bool env, using default")
		return def
	}
	return b
}

func envFloat(key string, def float64) float64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		log.Warn().Str("key", key).Str("value", v).Float64("default", def).
			Msg("disk-pressure: invalid float env, using default")
		return def
	}
	return f
}

func envDuration(key string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		log.Warn().Str("key", key).Str("value", v).Dur("default", def).
			Msg("disk-pressure: invalid duration env, using default")
		return def
	}
	return d
}

// poolName returns the ZFS pool name, the first "/"-separated component of
// zfsParentDataset (e.g. "prod/helix-zvols" -> "prod"). Empty if the parent
// dataset is unset.
func poolName() string {
	if zfsParentDataset == "" {
		return ""
	}
	return strings.SplitN(zfsParentDataset, "/", 2)[0]
}

// defaultDiskPressurePaths is the set of filesystem paths the statfs backend
// measures by default. They correspond 1:1 to the three PVCs the
// helix-sandbox K8s chart mounts into each pod: /var/lib/docker
// (docker-storage, image layers), /hydra-data (hydra-data, hydra state and
// zvol-equivalent storage), /data (workspace-data, per-session workspaces).
// Statfs-ing only one of these would miss fills on the other two, which is
// the specific failure mode the multi-path admission check guards against
// (helix-ubuntu pulls fill /var/lib/docker, not /hydra-data).
var defaultDiskPressurePaths = []string{
	"/var/lib/docker",
	"/hydra-data",
	"/data",
}

// diskPressurePaths returns the list of filesystem paths the statfs backend
// should measure. Selection rules (in order):
//
//  1. HELIX_DISK_PRESSURE_PATHS (plural, comma-separated): each entry is
//     whitespace-trimmed; empty entries are dropped. Use this on
//     multi-volume deployments to monitor every mount that could fill.
//  2. HELIX_DISK_PRESSURE_PATH (singular, kept for backwards compatibility
//     with deployments that adopted the original single-path fallback):
//     used as a one-element list when the plural form is unset.
//  3. Default: defaultDiskPressurePaths, the three K8s PVC mount points.
//
// The admission check uses the LOWEST free percentage across the returned
// paths, so adding paths only ever makes the guard stricter.
func diskPressurePaths() []string {
	if v := strings.TrimSpace(os.Getenv("HELIX_DISK_PRESSURE_PATHS")); v != "" {
		raw := strings.Split(v, ",")
		paths := make([]string, 0, len(raw))
		for _, p := range raw {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			paths = append(paths, p)
		}
		if len(paths) > 0 {
			return paths
		}
	}
	if v := strings.TrimSpace(os.Getenv("HELIX_DISK_PRESSURE_PATH")); v != "" {
		return []string{v}
	}
	return append([]string{}, defaultDiskPressurePaths...)
}

// statfsFn is the seam used to mock syscall.Statfs in tests. Production code
// calls syscall.Statfs directly; tests can swap it to model ENOENT, zero-block
// devices, and pinned free-percentage scenarios without touching the real
// filesystem.
var statfsFn = func(path string, stat *syscall.Statfs_t) error {
	return syscall.Statfs(path, stat)
}

// statfsFreePercent returns the percentage of `path`'s underlying filesystem
// that is free, computed from a POSIX statfs(2) call.
//
// Uses Bavail (blocks available to non-root) rather than Bfree, matching the
// semantics most operators reason about ("df -h"). Bsize is cast to uint64
// because it is int32 on Linux/Darwin (matches the existing pattern in
// cmd/sandbox-heartbeat/main.go).
//
// Returns an error when statfs itself fails (e.g. ENOENT for a misconfigured
// path) or when the filesystem reports zero total blocks (defensive, should
// not happen on a real filesystem).
func statfsFreePercent(path string) (float64, error) {
	var stat syscall.Statfs_t
	if err := statfsFn(path, &stat); err != nil {
		return 0, fmt.Errorf("statfs %q failed: %w", path, err)
	}
	total := stat.Blocks * uint64(stat.Bsize)
	avail := stat.Bavail * uint64(stat.Bsize)
	if total == 0 {
		return 0, fmt.Errorf("statfs %q reports zero total blocks", path)
	}
	return float64(avail) / float64(total) * 100, nil
}

// statfsFreeBytes returns the number of bytes available to non-root on the
// filesystem backing `path`. Mirrors statfsFreePercent's error contract.
func statfsFreeBytes(path string) (int64, error) {
	var stat syscall.Statfs_t
	if err := statfsFn(path, &stat); err != nil {
		return 0, fmt.Errorf("statfs %q failed: %w", path, err)
	}
	avail := stat.Bavail * uint64(stat.Bsize)
	return int64(avail), nil
}

// statfsFreePercentMulti runs statfsFreePercent against every path in `paths`
// and returns the LOWEST free-percentage, the path that produced it, and a
// pathENOENT flag set when any path returned ENOENT.
//
// Policy:
//   - ENOENT on any path: pathENOENT=true. The start guard treats this as
//     fail-closed because a missing path indicates operator misconfiguration
//     (HELIX_DISK_PRESSURE_PATHS lists a directory that does not exist).
//   - Non-ENOENT error on a path: log and skip that path. Other paths may
//     still measure successfully; we want one bad mount to not blind the
//     guard against the rest.
//   - At least one successful measurement: return min(freePct) and the
//     corresponding triggerPath. The caller threads triggerPath into refusal
//     logs and user-facing errors so operators see which volume is full.
//   - All paths errored without any ENOENT: return the last error so the
//     caller falls open with the existing warn log.
//
// `paths` must be non-empty; callers should pass diskPressurePaths().
func statfsFreePercentMulti(paths []string) (minPct float64, triggerPath string, pathENOENT bool, err error) {
	if len(paths) == 0 {
		return 0, "", false, fmt.Errorf("statfsFreePercentMulti: no paths configured")
	}
	have := false
	var lastErr error
	for _, p := range paths {
		pct, perr := statfsFreePercent(p)
		if perr != nil {
			if errors.Is(perr, syscall.ENOENT) {
				pathENOENT = true
				lastErr = perr
				log.Error().Err(perr).Str("path", p).
					Msg("disk-pressure: configured path does not exist (set HELIX_DISK_PRESSURE_PATHS or create the directory)")
				continue
			}
			lastErr = perr
			log.Warn().Err(perr).Str("path", p).
				Msg("disk-pressure: statfs failed for path, skipping (other paths may still measure)")
			continue
		}
		if !have || pct < minPct {
			minPct = pct
			triggerPath = p
			have = true
		}
	}
	if !have {
		if lastErr != nil {
			return 0, "", pathENOENT, fmt.Errorf("statfs probe failed for all paths: %w", lastErr)
		}
		return 0, "", pathENOENT, fmt.Errorf("statfs probe failed for all paths")
	}
	return minPct, triggerPath, pathENOENT, nil
}

// statfsFreeBytesMulti returns the free-bytes reading from the SAME path
// that statfsFreePercentMulti would identify as the constraining volume.
// Picking the min-free-pct path (rather than e.g. the sum of free bytes
// across all paths) keeps the GC reaper's before/after delta sensible: the
// reaper reclaims space on the volume that is actually under pressure, so
// the delta we surface should track that volume. ENOENT on the trigger path
// is propagated to the caller so it can fail closed exactly like the start
// guard.
func statfsFreeBytesMulti(paths []string) (int64, string, bool, error) {
	_, triggerPath, pathENOENT, err := statfsFreePercentMulti(paths)
	if err != nil {
		return 0, "", pathENOENT, err
	}
	free, ferr := statfsFreeBytes(triggerPath)
	if ferr != nil {
		if errors.Is(ferr, syscall.ENOENT) {
			pathENOENT = true
		}
		return 0, triggerPath, pathENOENT, ferr
	}
	return free, triggerPath, pathENOENT, nil
}

// measurement describes the result of a free-space probe. Backend identifies
// which probe produced it (zfs / statfs / none) so callers can log and tests
// can assert. PathENOENT is true when the statfs backend was selected but a
// configured path does not exist, which the start guard treats as
// fail-closed. TriggerPath is the path that produced the worst (lowest-free)
// reading on the statfs backend; empty for the ZFS backend. Threading it
// through to the refusal log lets operators see which volume is constraining
// (e.g. "refused: /var/lib/docker at 1.3% free") rather than a generic
// message.
type measurement struct {
	freePct     float64
	freeBytes   int64
	backend     string
	hasPct      bool
	hasBytes    bool
	pathENOENT  bool
	triggerPath string
}

// zfsBackendAvailable reports whether the ZFS backend can be queried. Both ZFS
// itself AND a configured parent dataset are required (an empty dataset means
// hydra is running in non-ZFS mode even on a host with ZFS userspace
// installed).
func zfsBackendAvailable() bool {
	return ZFSAvailable() && zfsParentDataset != ""
}

// measureDisk runs the active backend (ZFS or statfs) and returns a populated
// measurement. The error is non-nil only when NEITHER backend produced a
// usable reading.
func measureDisk() (measurement, error) {
	var m measurement
	if zfsBackendAvailable() {
		m.backend = "zfs"
		pct, pctErr := poolFreePercentZFS()
		free, freeErr := poolFreeBytesZFS()
		if pctErr == nil {
			m.freePct = pct
			m.hasPct = true
		}
		if freeErr == nil {
			m.freeBytes = free
			m.hasBytes = true
		}
		if m.hasPct || m.hasBytes {
			return m, nil
		}
		// ZFS configured but both probes failed (e.g. transient `zpool`
		// failure). Don't fall over to statfs, the operator selected ZFS, log
		// the error so they can see it.
		return m, fmt.Errorf("zfs probe failed: pct_err=%v free_err=%v", pctErr, freeErr)
	}

	// Non-ZFS host: statfs every configured path and take the worst
	// (lowest free percent) reading as the admission signal.
	m.backend = "statfs"
	paths := diskPressurePaths()
	pct, triggerPath, pctENOENT, pctErr := statfsFreePercentMulti(paths)
	if pctErr == nil {
		m.freePct = pct
		m.hasPct = true
		m.triggerPath = triggerPath
	} else if pctENOENT {
		m.pathENOENT = true
	}
	free, byteTrigger, bytesENOENT, freeErr := statfsFreeBytesMulti(paths)
	if freeErr == nil {
		m.freeBytes = free
		m.hasBytes = true
		if m.triggerPath == "" {
			m.triggerPath = byteTrigger
		}
	} else if bytesENOENT {
		m.pathENOENT = true
	}
	if m.hasPct || m.hasBytes {
		return m, nil
	}
	return m, fmt.Errorf("statfs probe failed for paths %v: %w", paths, pctErr)
}

// poolFreePercent returns the percentage of the ZFS pool that is free.
//
// Thin wrapper around poolFreePercentZFS that exists for the test suite which
// pokes at the ZFS path directly (forcing zfsAvailableFlag and mocking
// execCmdOutput). The production admission/monitor paths go through
// measureDisk() instead so they pick up the statfs fallback.
func poolFreePercent() (float64, error) {
	return poolFreePercentZFS()
}

// poolFreePercentZFS is the ZFS-only implementation, computed from
// `zpool list -Hp -o size,free <pool>` (raw byte values).
//
// Returns an error if ZFS is unavailable, the pool name is empty, the command
// fails, the output can't be parsed, or size is zero.
func poolFreePercentZFS() (float64, error) {
	if !ZFSAvailable() {
		return 0, fmt.Errorf("ZFS not available; cannot measure pool free space")
	}
	pool := poolName()
	if pool == "" {
		return 0, fmt.Errorf("pool name is empty; cannot measure pool free space")
	}

	out, err := execCmdOutput("zpool", "list", "-Hp", "-o", "size,free", pool)
	if err != nil {
		return 0, fmt.Errorf("zpool list for pool %q failed: %w", pool, err)
	}

	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) < 2 {
		return 0, fmt.Errorf("unexpected zpool list output for pool %q: %q", pool, strings.TrimSpace(string(out)))
	}

	size, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse pool size %q: %w", fields[0], err)
	}
	free, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse pool free %q: %w", fields[1], err)
	}
	if size == 0 {
		return 0, fmt.Errorf("pool %q reports size 0; cannot compute free percent", pool)
	}

	return float64(free) / float64(size) * 100, nil
}

// poolFreeBytes returns the number of free bytes for the GC reconcile path's
// before/after delta. Tries ZFS first, then falls back to statfs on the
// configured disk-pressure paths.
//
// On the statfs backend we report the free-bytes value from the SAME path
// that statfsFreePercentMulti identifies as the constraining volume (the
// lowest-free-percent path), not the sum across all paths. Summing would
// hide whether the reaper actually clawed back space on the volume that is
// under pressure: if /var/lib/docker is full and the reaper frees space on
// /data, the sum would still grow but pressure remains. Reporting the
// constraining-volume figure keeps the delta meaningful for the reaper's
// before/after comparison.
func poolFreeBytes() (int64, error) {
	if zfsBackendAvailable() {
		return poolFreeBytesZFS()
	}
	free, _, _, err := statfsFreeBytesMulti(diskPressurePaths())
	return free, err
}

// poolFreeBytesZFS is the ZFS-only implementation, read from
// `zpool list -Hp -o free <pool>` (raw byte value).
//
// Returns an error if ZFS is unavailable, the pool name is empty, the command
// fails, or output can't be parsed.
func poolFreeBytesZFS() (int64, error) {
	if !ZFSAvailable() {
		return 0, fmt.Errorf("ZFS not available; cannot measure pool free space")
	}
	pool := poolName()
	if pool == "" {
		return 0, fmt.Errorf("pool name is empty; cannot measure pool free space")
	}

	out, err := execCmdOutput("zpool", "list", "-Hp", "-o", "free", pool)
	if err != nil {
		return 0, fmt.Errorf("zpool list for pool %q failed: %w", pool, err)
	}

	field := strings.TrimSpace(string(out))
	if field == "" {
		return 0, fmt.Errorf("empty zpool list output for pool %q", pool)
	}

	free, err := strconv.ParseInt(field, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse pool free %q: %w", field, err)
	}

	return free, nil
}

// checkDiskPressureForStart is the admission-control guard for new dev
// containers. It returns a non-nil error (to be surfaced to the user) when the
// active backend's free percent is at or below the refuse threshold.
//
// Fail policy:
//   - Disabled: allow.
//   - Successful measurement above threshold: allow.
//   - Successful measurement at/below threshold: refuse.
//   - statfs path missing (ENOENT, misconfiguration): refuse. Silently
//     allowing on misconfig is exactly the bug this fallback fixes.
//   - All other measurement failures: fail-open (allow) with a warn log,
//     preserving prior behaviour for transient `zpool` failures.
func checkDiskPressureForStart() error {
	cfg := getDiskPressureConfig()
	if !cfg.enabled {
		return nil
	}

	m, err := measureDisk()
	if err != nil {
		if m.pathENOENT {
			log.Error().Err(err).
				Str("backend", m.backend).
				Strs("paths", diskPressurePaths()).
				Msg("disk-pressure: refusing to start dev container, a configured data path does not exist (set HELIX_DISK_PRESSURE_PATHS or create the directory)")
			return fmt.Errorf("refusing to start dev container: one of the disk-pressure paths %v does not exist; set HELIX_DISK_PRESSURE_PATHS to the correct mounts and retry", diskPressurePaths())
		}
		// Unknown measurement, both backends unavailable. Fail open, allow
		// the start, but log loudly so operators notice.
		log.Warn().Err(err).Str("backend", m.backend).
			Msg("disk-pressure: could not measure free space for start guard, allowing start (fail-open)")
		return nil
	}

	if !m.hasPct {
		// Successful probe but no percentage (statfs Blocks=0 only).
		log.Warn().Str("backend", m.backend).
			Msg("disk-pressure: measurement returned no percentage, allowing start (fail-open)")
		return nil
	}

	if m.freePct <= cfg.refuseFreePct {
		log.Error().
			Str("backend", m.backend).
			Str("pool", poolName()).
			Str("trigger_path", m.triggerPath).
			Float64("free_pct", m.freePct).
			Float64("refuse_pct", cfg.refuseFreePct).
			Msg("disk-pressure: refusing to start dev container, free space critically low")
		where := m.backend
		if m.triggerPath != "" {
			where = fmt.Sprintf("%s at %s", m.backend, m.triggerPath)
		}
		return fmt.Errorf("refusing to start dev container: disk space critically low, %s reports %.2f%% free (minimum %.0f%% required); free up space and retry", where, m.freePct, cfg.refuseFreePct)
	}

	return nil
}

// runDiskPressureMonitor is the emergency-brake monitor goroutine. It polls
// the active backend's free percent on a ticker and, when free percent drops
// to or below the stop threshold, gracefully STOPS all running dev containers
// (resumable, never deletes). Returns immediately if disk pressure is
// disabled.
func (dm *DevContainerManager) runDiskPressureMonitor() {
	cfg := getDiskPressureConfig()
	if !cfg.enabled {
		log.Info().Msg("disk-pressure: monitor disabled (HELIX_DISK_PRESSURE_ENABLED=false)")
		return
	}

	backend := "statfs"
	if zfsBackendAvailable() {
		backend = "zfs"
	}
	log.Info().
		Str("backend", backend).
		Str("pool", poolName()).
		Strs("paths", diskPressurePaths()).
		Float64("refuse_free_pct", cfg.refuseFreePct).
		Float64("stop_free_pct", cfg.stopFreePct).
		Dur("check_interval", cfg.checkInterval).
		Msg("disk-pressure: monitor started")

	ticker := time.NewTicker(cfg.checkInterval)
	defer ticker.Stop()
	for range ticker.C {
		m, err := measureDisk()
		if err != nil {
			// Unknown measurement, do nothing this tick (fail-open).
			log.Debug().Err(err).Str("backend", m.backend).Msg("disk-pressure: could not measure free space this tick, skipping")
			continue
		}
		if !m.hasPct {
			continue
		}

		if m.freePct <= cfg.stopFreePct {
			log.Error().
				Str("backend", m.backend).
				Str("pool", poolName()).
				Str("trigger_path", m.triggerPath).
				Float64("free_pct", m.freePct).
				Float64("stop_pct", cfg.stopFreePct).
				Msg("disk-pressure: EMERGENCY BRAKE, free space below stop threshold, stopping all dev containers")
			dm.emergencyStopAllDevContainers(m.freePct)
		}
	}
}

// emergencyStopAllDevContainers gracefully stops every tracked dev container to
// halt further writes when the pool is critically full. It NEVER removes the
// container and NEVER touches the zvol/workspace, so each session is fully
// resumable once space is recovered. Idempotent across ticks: stopping an
// already-stopped container is harmless.
func (dm *DevContainerManager) emergencyStopAllDevContainers(freePct float64) {
	// Snapshot the container set under the lock; do not hold it during the
	// (potentially slow) Docker stop calls.
	dm.mu.RLock()
	snapshot := make([]*DevContainer, 0, len(dm.containers))
	for _, dc := range dm.containers {
		snapshot = append(snapshot, dc)
	}
	dm.mu.RUnlock()

	if len(snapshot) == 0 {
		log.Warn().
			Str("pool", poolName()).
			Float64("free_pct", freePct).
			Msg("disk-pressure: emergency brake fired but no dev containers are tracked; nothing to stop")
		return
	}

	// Graceful stop timeout (seconds). Generous so the inner workloads can
	// flush, but bounded so a wedged container doesn't block recovery.
	timeout := 30
	stopped := 0
	for _, dc := range snapshot {
		// Acquire a Docker client the same way the delete path does
		// (per-container DockerSocket).
		dockerClient, err := dm.getDockerClient(dc.DockerSocket)
		if err != nil {
			log.Warn().Err(err).
				Str("session_id", dc.SessionID).
				Str("container_id", dc.ContainerID).
				Msg("disk-pressure: failed to create Docker client for emergency stop")
			continue
		}

		log.Warn().
			Str("session_id", dc.SessionID).
			Str("container_id", dc.ContainerID).
			Float64("free_pct", freePct).
			Msg("disk-pressure: emergency-stopping dev container (resumable, not deleted)")

		// ContainerStop ONLY — never ContainerRemove. Stopping an
		// already-stopped container is a no-op, so this is idempotent.
		if err := dockerClient.ContainerStop(context.Background(), dc.ContainerID, container.StopOptions{Timeout: &timeout}); err != nil {
			log.Warn().Err(err).
				Str("session_id", dc.SessionID).
				Str("container_id", dc.ContainerID).
				Msg("disk-pressure: failed to stop dev container during emergency brake")
			dockerClient.Close()
			continue
		}
		stopped++
		dockerClient.Close()
	}

	log.Error().
		Str("pool", poolName()).
		Float64("free_pct", freePct).
		Int("stopped", stopped).
		Int("total", len(snapshot)).
		Msg("disk-pressure: emergency brake complete — stopped dev containers to protect the pool (sessions are resumable)")
}
