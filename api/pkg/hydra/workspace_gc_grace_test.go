package hydra

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These cover the bug where the live path stamped the directory mtime that the
// grace check read, so the grace clock never advanced and nothing was ever
// reaped. Each test fails against the old behaviour.
//
// reconcileIDDirs takes base as the directory holding the id dirs (e.g.
// /data/workspaces/spec-tasks), so the tests use a temp dir as that base and
// check the sibling marks dir created next to it.

// marksPath returns the liveness marks dir used for the given id-dir base.
func marksPath(base, label string) string {
	return filepath.Join(filepath.Dir(base), liveMarksDirName, label)
}

// A live directory gets a liveness mark, and its own mtime is left untouched —
// otherwise the age the grace check reads keeps resetting.
func TestReconcileIDDirs_LiveDoesNotResetDirectoryMtime(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "spt_live")
	require.NoError(t, os.MkdirAll(dir, 0755))

	old := time.Now().Add(-8 * time.Hour)
	require.NoError(t, os.Chtimes(dir, old, old))

	_, _ = reconcileIDDirs(base, "spec-tasks", "spt_", map[string]bool{"spt_live": true}, time.Hour, false, os.RemoveAll)

	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.WithinDuration(t, old, info.ModTime(), time.Minute,
		"live reconciliation must not rewrite the workspace dir mtime")

	markInfo, err := os.Stat(filepath.Join(marksPath(base, "spec-tasks"), "spt_live"))
	require.NoError(t, err, "live dirs must be recorded in the marks dir")
	assert.WithinDuration(t, time.Now(), markInfo.ModTime(), time.Minute)
}

// Once a directory leaves the live set, its grace clock runs from when it was
// last seen live, not from "now".
func TestReconcileIDDirs_StaleAfterLiveIsReaped(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "spt_was_live")
	require.NoError(t, os.MkdirAll(dir, 0755))

	const grace = time.Hour

	// Seen live well past the grace window. The dir mtime is also old here so
	// the mark is the deciding input; the point is that the mark is consulted.
	require.NoError(t, os.MkdirAll(marksPath(base, "spec-tasks"), 0755))
	mark := filepath.Join(marksPath(base, "spec-tasks"), "spt_was_live")
	require.NoError(t, os.WriteFile(mark, nil, 0644))
	liveSeen := time.Now().Add(-8 * time.Hour)
	require.NoError(t, os.Chtimes(mark, liveSeen, liveSeen))
	require.NoError(t, os.Chtimes(dir, liveSeen, liveSeen))

	reaped, _ := reconcileIDDirs(base, "spec-tasks", "spt_", map[string]bool{}, grace, false, os.RemoveAll)

	assert.Equal(t, []string{dir}, reaped, "a dir whose liveness mark is older than grace must be reaped")
	_, err := os.Stat(dir)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(mark)
	assert.True(t, os.IsNotExist(err), "the stale mark must be cleaned up on reap")
}

// A directory still inside its grace window after being live is kept.
func TestReconcileIDDirs_RecentlyLiveIsKept(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "spt_recent_live")
	require.NoError(t, os.MkdirAll(dir, 0755))

	reaped, _ := reconcileIDDirs(base, "spec-tasks", "spt_", map[string]bool{"spt_recent_live": true}, time.Hour, false, os.RemoveAll)
	assert.Empty(t, reaped)

	reaped, skipped := reconcileIDDirs(base, "spec-tasks", "spt_", map[string]bool{}, time.Hour, false, os.RemoveAll)

	assert.Empty(t, reaped)
	_, err := os.Stat(dir)
	assert.NoError(t, err, "recently live dir must be kept")
	require.NotEmpty(t, skipped)
	assert.Equal(t, "grace", skipped[0].Reason)
}

// Repeated ticks must eventually reap a dir that stays out of the live set,
// regardless of when the last live tick marked it. This is the end-to-end shape
// of the production bug.
func TestReconcileIDDirs_RepeatedTicksEventuallyReap(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "spt_churn")
	require.NoError(t, os.MkdirAll(dir, 0755))

	// One live tick marks it at "now".
	_, _ = reconcileIDDirs(base, "spec-tasks", "spt_", map[string]bool{"spt_churn": true}, time.Hour, false, os.RemoveAll)

	// Simulate grace elapsing: both the mark and the dir move back, since a
	// workspace's own mtime also reflects its age.
	old := time.Now().Add(-2 * time.Hour)
	require.NoError(t, os.Chtimes(filepath.Join(marksPath(base, "spec-tasks"), "spt_churn"), old, old))
	require.NoError(t, os.Chtimes(dir, old, old))

	reaped, _ := reconcileIDDirs(base, "spec-tasks", "spt_", map[string]bool{}, time.Hour, false, os.RemoveAll)
	assert.Equal(t, []string{dir}, reaped)
}

// A dir that was never live still falls back to its own mtime.
func TestReconcileIDDirs_NeverLiveUsesDirMtime(t *testing.T) {
	base := t.TempDir()
	stale := filepath.Join(base, "spt_stale")
	fresh := filepath.Join(base, "spt_fresh")
	require.NoError(t, os.MkdirAll(stale, 0755))
	require.NoError(t, os.MkdirAll(fresh, 0755))

	old := time.Now().Add(-8 * time.Hour)
	require.NoError(t, os.Chtimes(stale, old, old))

	reaped, _ := reconcileIDDirs(base, "spec-tasks", "spt_", map[string]bool{}, time.Hour, false, os.RemoveAll)

	assert.Equal(t, []string{stale}, reaped)
	_, err := os.Stat(fresh)
	assert.NoError(t, err, "fresh dir must be kept")
}

// A dry run must report candidates without deleting them or writing marks.
func TestReconcileIDDirs_DryRunIsSideEffectFree(t *testing.T) {
	base := t.TempDir()
	stale := filepath.Join(base, "spt_stale")
	require.NoError(t, os.MkdirAll(stale, 0755))

	old := time.Now().Add(-8 * time.Hour)
	require.NoError(t, os.Chtimes(stale, old, old))

	reaped, _ := reconcileIDDirs(base, "spec-tasks", "spt_", map[string]bool{}, time.Hour, true, os.RemoveAll)

	assert.Equal(t, []string{stale}, reaped)
	_, err := os.Stat(stale)
	assert.NoError(t, err, "dry run must not remove the candidate")
	_, err = os.Stat(marksPath(base, "spec-tasks"))
	assert.True(t, os.IsNotExist(err), "dry run must not create the marks dir")
}
