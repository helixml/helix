//go:build unix

package hydra

import (
	"errors"
	"fmt"
	"syscall"

	"github.com/rs/zerolog/log"
)

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
