package hydra

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/rs/zerolog/log"
)

// Disk-pressure admission control.
//
// The ZFS pool that backs every session zvol must never hit 0% free: ENOSPC
// inside a session's XFS zvol corrupts the filesystem, which cascades into
// Postgres faults and git-repository corruption. This guard watches the pool's
// FREE PERCENT and applies two protections, both keyed off thresholds that are
// configurable via env:
//
//   - Free ≤ refuse threshold (default 2%): refuse to START new dev containers,
//     surfacing a clear user-facing error.
//   - Free ≤ stop threshold (default 1%): an emergency brake STOPS (not deletes)
//     existing running dev containers so they stop writing. Sessions remain
//     resumable — the zvol/workspace is never touched.
//
// CRITICAL fail-open contract: a failed or unknowable measurement must NEVER
// refuse a start and NEVER trigger an emergency stop. When we cannot read the
// pool's free percent we do nothing — the alternative (acting on garbage) is
// worse than not acting.

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

// poolName returns the ZFS pool name — the first "/"-separated component of
// zfsParentDataset (e.g. "prod/helix-zvols" → "prod"). Empty if the parent
// dataset is unset.
func poolName() string {
	if zfsParentDataset == "" {
		return ""
	}
	return strings.SplitN(zfsParentDataset, "/", 2)[0]
}

// poolFreePercent returns the percentage of the ZFS pool that is free, computed
// from `zpool list -Hp -o size,free <pool>` (raw byte values).
//
// Returns an error if ZFS is unavailable, the pool name is empty, the command
// fails, the output can't be parsed, or size is zero. Callers MUST treat any
// error as "unknown — do nothing" (fail open): never refuse a start and never
// trigger an emergency stop on a failed/unknowable measurement.
func poolFreePercent() (float64, error) {
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

// poolFreeBytes returns the number of free bytes in the ZFS pool, read from
// `zpool list -Hp -o free <pool>` (raw byte value).
//
// Returns an error under the same conditions as poolFreePercent (ZFS
// unavailable, empty pool name, command failure, or unparsable output). Used by
// the GC reconcile path to cheaply measure reclaimed space as a before/after
// delta instead of running a per-directory `du` sweep.
func poolFreeBytes() (int64, error) {
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
// containers. It returns a non-nil error (to be surfaced to the user) ONLY when
// the pool's free percent is at or below the refuse threshold.
//
// Fail-open: if disk pressure is disabled, or the measurement errors, it returns
// nil (allow the start).
func checkDiskPressureForStart() error {
	cfg := getDiskPressureConfig()
	if !cfg.enabled {
		return nil
	}

	freePct, err := poolFreePercent()
	if err != nil {
		// Unknown measurement → fail open, allow the start.
		log.Warn().Err(err).Msg("disk-pressure: could not measure pool free percent for start guard, allowing start (fail-open)")
		return nil
	}

	if freePct <= cfg.refuseFreePct {
		log.Error().
			Str("pool", poolName()).
			Float64("free_pct", freePct).
			Float64("refuse_pct", cfg.refuseFreePct).
			Msg("disk-pressure: refusing to start dev container — pool free space critically low")
		return fmt.Errorf("refusing to start dev container: disk space critically low — pool %q is %.2f%% free (minimum %.0f%% required); free up space and retry", poolName(), freePct, cfg.refuseFreePct)
	}

	return nil
}

// runDiskPressureMonitor is the emergency-brake monitor goroutine. It polls the
// pool's free percent on a ticker and, when free percent drops to or below the
// stop threshold, gracefully STOPS all running dev containers (resumable — never
// deletes). Returns immediately if disk pressure is disabled.
func (dm *DevContainerManager) runDiskPressureMonitor() {
	cfg := getDiskPressureConfig()
	if !cfg.enabled {
		log.Info().Msg("disk-pressure: monitor disabled (HELIX_DISK_PRESSURE_ENABLED=false)")
		return
	}

	log.Info().
		Float64("refuse_free_pct", cfg.refuseFreePct).
		Float64("stop_free_pct", cfg.stopFreePct).
		Dur("check_interval", cfg.checkInterval).
		Msg("disk-pressure: monitor started")

	ticker := time.NewTicker(cfg.checkInterval)
	defer ticker.Stop()
	for range ticker.C {
		freePct, err := poolFreePercent()
		if err != nil {
			// Unknown measurement → do nothing this tick (fail-open).
			log.Debug().Err(err).Msg("disk-pressure: could not measure pool free percent this tick, skipping")
			continue
		}

		if freePct <= cfg.stopFreePct {
			log.Error().
				Str("pool", poolName()).
				Float64("free_pct", freePct).
				Float64("stop_pct", cfg.stopFreePct).
				Msg("disk-pressure: EMERGENCY BRAKE — pool free space below stop threshold, stopping all dev containers")
			dm.emergencyStopAllDevContainers(freePct)
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
