package hydra

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type WorkspaceGCSuite struct {
	suite.Suite
	tmpDir  string
	oldBase string
}

func TestWorkspaceGCSuite(t *testing.T) {
	suite.Run(t, new(WorkspaceGCSuite))
}

func (s *WorkspaceGCSuite) SetupTest() {
	var err error
	s.tmpDir, err = os.MkdirTemp("", "workspace-gc-test-*")
	require.NoError(s.T(), err)

	s.oldBase = workspacesBaseDir
	workspacesBaseDir = s.tmpDir
}

func (s *WorkspaceGCSuite) TearDownTest() {
	workspacesBaseDir = s.oldBase
	os.RemoveAll(s.tmpDir)
}

// mkdirOld creates a directory and backdates its mtime by `age`.
func (s *WorkspaceGCSuite) mkdirOld(rel string, age time.Duration) string {
	dir := filepath.Join(s.tmpDir, rel)
	require.NoError(s.T(), os.MkdirAll(dir, 0755))
	old := time.Now().Add(-age)
	require.NoError(s.T(), os.Chtimes(dir, old, old))
	return dir
}

func (s *WorkspaceGCSuite) TestReconcileOrphanWorkspaces_ReapsOnlyOrphans() {
	const grace = time.Hour

	// spec-tasks
	sptLive := s.mkdirOld("spec-tasks/spt_live", 8*time.Hour) // old but live
	sptOld := s.mkdirOld("spec-tasks/spt_old", 8*time.Hour)   // old + not live → reap
	// sessions
	sesOld := s.mkdirOld("sessions/ses_old", 8*time.Hour) // old + not live → reap
	// protected / junk
	sandboxDir := s.mkdirOld("sandboxes", 8*time.Hour)        // NEVER touched
	junkDir := s.mkdirOld("spec-tasks/junkname", 8*time.Hour) // wrong prefix → skipped

	liveSessions := map[string]bool{}                 // ses_old not live
	liveSpecTasks := map[string]bool{"spt_live": true} // spt_live is live

	reaped, _, freed := ReconcileOrphanWorkspaces(liveSessions, liveSpecTasks, grace, false)

	// Only spt_old and ses_old removed.
	_, err := os.Stat(sptOld)
	assert.True(s.T(), os.IsNotExist(err), "spt_old should be reaped")
	_, err = os.Stat(sesOld)
	assert.True(s.T(), os.IsNotExist(err), "ses_old should be reaped")

	// Everything else untouched.
	_, err = os.Stat(sptLive)
	assert.NoError(s.T(), err, "spt_live (live) must be kept")
	_, err = os.Stat(sandboxDir)
	assert.NoError(s.T(), err, "sandboxes/ must never be removed")
	_, err = os.Stat(junkDir)
	assert.NoError(s.T(), err, "wrong-prefix dir must be kept")

	assert.ElementsMatch(s.T(), []string{sptOld, sesOld}, reaped)
	assert.GreaterOrEqual(s.T(), freed, int64(0))
}

func (s *WorkspaceGCSuite) TestReconcileOrphanWorkspaces_SkipsWithinGrace() {
	const grace = time.Hour

	sptFresh := s.mkdirOld("spec-tasks/spt_fresh", 10*time.Minute) // within grace

	reaped, _, _ := ReconcileOrphanWorkspaces(map[string]bool{}, map[string]bool{}, grace, false)

	_, err := os.Stat(sptFresh)
	assert.NoError(s.T(), err, "within-grace dir must be kept")
	assert.Empty(s.T(), reaped)
}

func (s *WorkspaceGCSuite) TestReconcileOrphanWorkspaces_DryRunRemovesNothing() {
	const grace = time.Hour

	sptOld := s.mkdirOld("spec-tasks/spt_old", 8*time.Hour)
	sesOld := s.mkdirOld("sessions/ses_old", 8*time.Hour)

	reaped, _, _ := ReconcileOrphanWorkspaces(map[string]bool{}, map[string]bool{}, grace, true /* dryRun */)

	// Reported as candidates...
	assert.ElementsMatch(s.T(), []string{sptOld, sesOld}, reaped)
	// ...but still present on disk.
	_, err := os.Stat(sptOld)
	assert.NoError(s.T(), err, "dry run must not remove spt_old")
	_, err = os.Stat(sesOld)
	assert.NoError(s.T(), err, "dry run must not remove ses_old")
}

func (s *WorkspaceGCSuite) TestReconcileOrphanWorkspaces_NeverTouchesSandboxesSubtree() {
	const grace = time.Hour

	// A sandbox checkout whose name happens to look like a session id — must
	// still be ignored because it lives under sandboxes/, which we never scan.
	inner := s.mkdirOld("sandboxes/ses_inside_sandboxes", 30*24*time.Hour)

	reaped, _, _ := ReconcileOrphanWorkspaces(map[string]bool{}, map[string]bool{}, grace, false)

	_, err := os.Stat(inner)
	assert.NoError(s.T(), err, "nothing under sandboxes/ may be reaped")
	assert.Empty(s.T(), reaped)
}

func (s *WorkspaceGCSuite) TestReconcileOrphanWorkspaces_NoBaseDir() {
	workspacesBaseDir = filepath.Join(s.tmpDir, "does-not-exist")

	reaped, skipped, freed := ReconcileOrphanWorkspaces(map[string]bool{}, map[string]bool{}, time.Hour, false)

	assert.Empty(s.T(), reaped)
	assert.Empty(s.T(), skipped)
	assert.Equal(s.T(), int64(0), freed)
}
