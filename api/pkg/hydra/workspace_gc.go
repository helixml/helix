package hydra

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// workspacesBaseDir is the base directory for per-task / per-session workspace
// checkouts on the helix-workspaces filesystem dataset (mounted at /data inside
// the sandbox). These are PLAIN DIRECTORIES, not zvols — reap with os.RemoveAll,
// never any zfs command. Var (not const) so tests can override it.
var workspacesBaseDir = "/data/workspaces"

// dirSizeBytes returns the on-disk size of a directory in bytes (du -sb style).
// Returns 0 on error.
func dirSizeBytes(path string) int64 {
	out, err := exec.Command("du", "-sb", path).Output()
	if err != nil {
		return 0
	}
	var size int64
	fmt.Sscanf(string(out), "%d", &size)
	return size
}

// reconcileWorkspaceSubdir reaps orphaned directories under
// workspacesBaseDir/<subdir>. A dir is reaped iff its name has the required
// prefix, its name is NOT in liveSet, and its mtime is older than the grace
// period. dryRun records candidates without removing them.
//
// Safety: only ever os.RemoveAll a path of the exact form
// workspacesBaseDir/<subdir>/<validated-name>. The name must be a single path
// element (no separators, no "..") and carry the expected prefix, so we can
// never escape the subtree.
func reconcileWorkspaceSubdir(subdir, requiredPrefix string, liveSet map[string]bool, grace time.Duration, dryRun bool) (reaped []string, skipped []GCSkip, freed int64) {
	base := filepath.Join(workspacesBaseDir, subdir)
	entries, err := os.ReadDir(base)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Warn().Err(err).Str("dir", base).Msg("ReconcileOrphanWorkspaces: failed to read subdir")
		}
		return nil, nil, 0
	}

	cutoff := time.Now().Add(-grace)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()

		// Guard against path traversal — name must be a single, well-formed
		// element carrying the expected prefix.
		if !strings.HasPrefix(name, requiredPrefix) ||
			strings.ContainsAny(name, "/\\") ||
			name == "." || name == ".." {
			skipped = append(skipped, GCSkip{Name: filepath.Join(subdir, name), Reason: "name not eligible"})
			continue
		}

		// id == directory name (full ses_… / spt_… string).
		if liveSet[name] {
			skipped = append(skipped, GCSkip{Name: filepath.Join(subdir, name), Reason: "live"})
			continue
		}

		info, err := entry.Info()
		if err != nil {
			skipped = append(skipped, GCSkip{Name: filepath.Join(subdir, name), Reason: "stat error: " + err.Error()})
			continue
		}
		if info.ModTime().After(cutoff) {
			skipped = append(skipped, GCSkip{Name: filepath.Join(subdir, name), Reason: "grace"})
			continue
		}

		dir := filepath.Join(base, name)
		size := dirSizeBytes(dir)

		if dryRun {
			reaped = append(reaped, dir)
			freed += size
			continue
		}

		if err := os.RemoveAll(dir); err != nil {
			log.Warn().Err(err).Str("dir", dir).Msg("ReconcileOrphanWorkspaces: failed to remove workspace dir")
			skipped = append(skipped, GCSkip{Name: filepath.Join(subdir, name), Reason: "error: " + err.Error()})
			continue
		}
		reaped = append(reaped, dir)
		freed += size
		log.Info().
			Str("dir", dir).
			Int64("size_bytes", size).
			Dur("age", time.Since(info.ModTime())).
			Msg("Reaped orphaned workspace dir")
	}

	return reaped, skipped, freed
}

// ReconcileOrphanWorkspaces reaps per-task and per-session workspace checkout
// directories that are no longer referenced by any live spec-task / session and
// have aged past the grace period.
//
// Layout under workspacesBaseDir (/data/workspaces):
//
//	spec-tasks/<spt_id>   ← per-task checkouts (reaped against liveSpecTaskIDs)
//	sessions/<ses_id>     ← per-session checkouts (reaped against liveSessionIDs)
//	sandboxes/            ← NEVER descended into or removed (separate lifecycle)
//
// All entries are plain directories on the helix-workspaces filesystem dataset,
// so reaping is os.RemoveAll — never a zfs command.
func ReconcileOrphanWorkspaces(liveSessionIDs, liveSpecTaskIDs map[string]bool, grace time.Duration, dryRun bool) (reaped []string, skipped []GCSkip, freed int64) {
	// spec-tasks/<spt_…>
	r, s, f := reconcileWorkspaceSubdir("spec-tasks", "spt_", liveSpecTaskIDs, grace, dryRun)
	reaped = append(reaped, r...)
	skipped = append(skipped, s...)
	freed += f

	// sessions/<ses_…>
	r, s, f = reconcileWorkspaceSubdir("sessions", "ses_", liveSessionIDs, grace, dryRun)
	reaped = append(reaped, r...)
	skipped = append(skipped, s...)
	freed += f

	// NOTE: the "sandboxes/" subtree is intentionally never touched here.

	return reaped, skipped, freed
}
