package hydra

import (
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog/log"
)

// workspacesBaseDir is the base directory for per-task / per-session workspace
// checkouts on the helix-workspaces filesystem dataset (mounted at /data inside
// the sandbox). These are PLAIN DIRECTORIES, not zvols — reap with os.RemoveAll,
// never any zfs command. Var (not const) so tests can override it.
var workspacesBaseDir = "/data/workspaces"

// liveMarksDirName is a sibling of the reaped subtrees that records when each
// id was last observed live. The grace period is measured from that time.
//
// It is deliberately OUTSIDE the workspace directories. The live path used to
// stamp a live directory's own mtime, which is exactly the age the grace check
// reads, so the two cancelled out: a directory that had just left the live set
// carried a timestamp of "now" and stayed within grace forever, while a live
// one had its clock reset every tick. Nothing was ever reaped — a dev host
// accumulated 278 spec-task checkouts (~466G) while the reaper logged
// workspaces_reaped=[] and a few hundred skips, tick after tick.
//
// Writing a marker inside the workspace dir does not help: creating any file
// inside a directory bumps that directory's mtime, which is the very signal we
// need to leave intact. Keeping the marks in a sibling dir leaves both
// timestamps honest.
const liveMarksDirName = ".gc-live-marks"

// reconcileWorkspaceSubdir reaps orphaned directories under
// workspacesBaseDir/<subdir>.
func reconcileWorkspaceSubdir(subdir, requiredPrefix string, liveSet map[string]bool, grace time.Duration, dryRun bool) (reaped []string, skipped []GCSkip) {
	return reconcileIDDirs(filepath.Join(workspacesBaseDir, subdir), subdir, requiredPrefix, liveSet, grace, dryRun, os.RemoveAll)
}

// reconcileIDDirs reaps orphaned directories under base whose names are
// resource ids. A dir is reaped iff its name has the required prefix, its name
// is NOT in liveSet, and its age exceeds the grace period. dryRun records
// candidates without removing them. label prefixes skip names in the report.
// remove deletes one candidate directory.
//
// Age is the newer of the directory's own mtime and the last time it was seen
// live, so a directory that was live recently keeps the remainder of its grace
// after it leaves the live set. Directories never seen live fall back to their
// own mtime.
//
// Safety: only ever remove a path of the exact form
// base/<validated-name>. The name must be a single path element (no
// separators, no "..") and carry the expected prefix, so we can never escape
// the subtree.
func reconcileIDDirs(base, label, requiredPrefix string, liveSet map[string]bool, grace time.Duration, dryRun bool, remove func(dir string) error) (reaped []string, skipped []GCSkip) {
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		log.Error().Err(err).Str("dir", base).Msg("ReconcileOrphanWorkspaces: failed to read subdir; nothing reaped")
		return nil, []GCSkip{{Name: base, Reason: "list error: " + err.Error()}}
	}

	marksDir, err := marksDirFor(base, label, dryRun)
	if err != nil && !dryRun {
		// Without a place to record liveness we would fall back to directory
		// mtimes, which is exactly how the reaper went silent. Report and stop
		// rather than risk reaping something that is actually live.
		log.Error().Err(err).Str("dir", base).Msg("ReconcileOrphanWorkspaces: cannot prepare liveness marks; nothing reaped")
		return nil, []GCSkip{{Name: base, Reason: "marks dir error: " + err.Error()}}
	}

	cutoff := time.Now().Add(-grace)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()

		// Guard against path traversal — name must be a single, well-formed
		// element carrying the expected prefix.
		if !isResourceID(name, requiredPrefix) {
			skipped = append(skipped, GCSkip{Name: filepath.Join(label, name), Reason: "name not eligible"})
			continue
		}

		dir := filepath.Join(base, name)

		// id == directory name (full ses_… / spt_… string).
		if liveSet[name] {
			// Record "seen live now" so the grace clock starts when the
			// directory stops being live. Recorded in a sibling directory —
			// never by touching dir itself, whose mtime is an input above.
			writeLiveMark(marksDir, name, time.Now())
			skipped = append(skipped, GCSkip{Name: filepath.Join(label, name), Reason: "live"})
			continue
		}

		age, err := lastActivity(dir, marksDir, name)
		if err != nil {
			skipped = append(skipped, GCSkip{Name: filepath.Join(label, name), Reason: "stat error: " + err.Error()})
			continue
		}
		if age.After(cutoff) {
			skipped = append(skipped, GCSkip{Name: filepath.Join(label, name), Reason: "grace"})
			continue
		}

		if dryRun {
			reaped = append(reaped, dir)
			continue
		}

		if err := remove(dir); err != nil {
			log.Warn().Err(err).Str("dir", dir).Msg("ReconcileOrphanWorkspaces: failed to remove workspace dir")
			skipped = append(skipped, GCSkip{Name: filepath.Join(label, name), Reason: "error: " + err.Error()})
			continue
		}
		// The id is gone; drop its stale liveness mark so the marks dir does
		// not accumulate one file per task ever created.
		if marksDir != "" {
			if err := os.Remove(filepath.Join(marksDir, name)); err != nil && !os.IsNotExist(err) {
				log.Warn().Err(err).Str("dir", dir).Msg("ReconcileOrphanWorkspaces: failed to clear liveness mark")
			}
		}
		reaped = append(reaped, dir)
		log.Info().
			Str("dir", dir).
			Dur("age", time.Since(age)).
			Msg("Reaped orphaned workspace dir")
	}

	return reaped, skipped
}

// marksDirFor returns the directory that records liveness for the id dirs under
// base. It is a sibling of base (base's parent, so spec-tasks and sessions each
// get their own), keeping the marks out of the reaped subtrees. It is created
// unless this is a dry run, so a dry run stays side-effect free and only reads
// whatever marks exist.
func marksDirFor(base, label string, dryRun bool) (string, error) {
	marks := filepath.Join(filepath.Dir(base), liveMarksDirName, label)
	if !dryRun {
		if err := os.MkdirAll(marks, 0755); err != nil {
			return "", err
		}
	}
	return marks, nil
}

// writeLiveMark records that name was live at when. Best-effort: a failure is
// logged because the next tick retries, and a missing mark only delays grace.
func writeLiveMark(marksDir, name string, when time.Time) {
	if marksDir == "" {
		return
	}
	mark := filepath.Join(marksDir, name)
	f, err := os.OpenFile(mark, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Warn().Err(err).Str("mark", mark).Msg("ReconcileOrphanWorkspaces: failed to write liveness mark")
		return
	}
	f.Close()
	if err := os.Chtimes(mark, when, when); err != nil {
		log.Warn().Err(err).Str("mark", mark).Msg("ReconcileOrphanWorkspaces: failed to refresh liveness mark")
	}
}

// lastActivity returns the reference time for dir's grace calculation, trying
// in order: the last time the reaper saw it live, then the newest mtime
// anywhere inside it, then the directory's own mtime.
//
// The mark WINS over the directory mtime rather than being maxed with it. That
// matters for recovery: versions of the reaper before this one stamped the
// directory's mtime on every tick, so a long-dead checkout can still carry an
// mtime of "now". Taking the max would keep every such directory inside grace
// forever — the exact trap this change exists to escape. A mark is written only
// by the reaper, so it is the one honest clock available.
//
// The walk exists for the first run after upgrading, when no marks exist yet
// and every directory already carries a stamped (worthless) mtime. File mtimes
// inside a checkout were never touched by the old code, so the newest of them
// is the honest estimate of when the workspace last did real work. A directory
// mtime that is newer than everything inside it is precisely the signature of
// the old stamping, so the contents decide in that case.
func lastActivity(dir, marksDir, name string) (time.Time, error) {
	if marksDir != "" {
		if markInfo, err := os.Stat(filepath.Join(marksDir, name)); err == nil {
			return markInfo.ModTime(), nil
		}
	}

	info, err := os.Stat(dir)
	if err != nil {
		return time.Time{}, err
	}

	if newest, ok := newestFileMtime(dir); ok {
		// Contents are only trusted when they predate the directory itself.
		// A directory can legitimately be newer than its files (an entry was
		// added or removed), but never by a whole grace period unless it was
		// stamped. Use the contents whenever they are older.
		if newest.Before(info.ModTime()) {
			return newest, nil
		}
	}
	return info.ModTime(), nil
}

// newestFileMtime walks dir (bounded) and returns the newest file mtime it
// finds. Returns false if the walk found no files or hit its budget.
func newestFileMtime(dir string) (time.Time, bool) {
	const maxEntries = 20000
	var newest time.Time
	seen := 0
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if seen++; seen > maxEntries {
			return filepath.SkipAll
		}
		if d.IsDir() {
			return nil
		}
		if fi, err := d.Info(); err == nil {
			if m := fi.ModTime(); m.After(newest) {
				newest = m
			}
		}
		return nil
	})
	return newest, !newest.IsZero()
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
//	.gc-live-marks/       ← liveness bookkeeping, one file per id ever seen live
//
// It also reaps sessionRuntimeBaseDir/<ses_id> (/data/sessions) against
// liveSessionIDs.
//
// All entries are plain directories on the helix-workspaces filesystem dataset,
// so reaping is os.RemoveAll — never a zfs command.
func ReconcileOrphanWorkspaces(liveSessionIDs, liveSpecTaskIDs map[string]bool, grace time.Duration, dryRun bool) (reaped []string, skipped []GCSkip) {
	// spec-tasks/<spt_…>
	r, s := reconcileWorkspaceSubdir("spec-tasks", "spt_", liveSpecTaskIDs, grace, dryRun)
	reaped = append(reaped, r...)
	skipped = append(skipped, s...)

	// sessions/<ses_…>
	r, s = reconcileWorkspaceSubdir("sessions", "ses_", liveSessionIDs, grace, dryRun)
	reaped = append(reaped, r...)
	skipped = append(skipped, s...)

	// NOTE: the "sandboxes/" subtree is intentionally never touched here.

	// Per-session runtime dirs (PipeWire, crash dumps) outside the workspaces
	// tree, keyed by the same session ids.
	r, s = reconcileIDDirs(sessionRuntimeBaseDir, "session-runtime", "ses_", liveSessionIDs, grace, dryRun, os.RemoveAll)
	reaped = append(reaped, r...)
	skipped = append(skipped, s...)

	return reaped, skipped
}
