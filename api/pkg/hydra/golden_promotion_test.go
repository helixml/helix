package hydra

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -----------------------------------------------------------------------
// waitForZvolDevice
// -----------------------------------------------------------------------

func (s *GoldenZvolSuite) TestWaitForZvolDevice_AppearsAfterDelay() {
	s.mock.addDataset("testpool/helix-zvols/golden-prj_abc")
	s.mock.deviceDelay["testpool/helix-zvols/golden-prj_abc"] = 20

	require.NoError(s.T(), waitForZvolDevice("testpool/helix-zvols/golden-prj_abc"))
	assert.Equal(s.T(), 21, s.mock.deviceStats["testpool/helix-zvols/golden-prj_abc"])
	assert.True(s.T(), s.mock.hasCommand("udevadm settle"))
}

func (s *GoldenZvolSuite) TestWaitForZvolDevice_PresentSkipsSettle() {
	s.mock.addDataset("testpool/helix-zvols/golden-prj_abc")

	require.NoError(s.T(), waitForZvolDevice("testpool/helix-zvols/golden-prj_abc"))
	assert.False(s.T(), s.mock.hasCommand("udevadm"))
}

func (s *GoldenZvolSuite) TestWaitForZvolDevice_TimesOut() {
	s.mock.addDataset("testpool/helix-zvols/golden-prj_abc")
	s.mock.deviceDelay["testpool/helix-zvols/golden-prj_abc"] = -1

	start := time.Now()
	err := waitForZvolDevice("testpool/helix-zvols/golden-prj_abc")
	require.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "/dev/zvol/testpool/helix-zvols/golden-prj_abc did not appear")
	assert.GreaterOrEqual(s.T(), time.Since(start), zvolDeviceWaitTimeout)
}

func (s *GoldenZvolSuite) TestWaitForZvolDevice_RejectsNonBlockDevice() {
	regular := filepath.Join(s.tmpDir, "not-a-device")
	require.NoError(s.T(), os.WriteFile(regular, nil, 0644))
	statZvolDevice = func(string) (os.FileInfo, error) { return os.Stat(regular) }

	err := waitForZvolDevice("testpool/helix-zvols/golden-prj_abc")
	require.Error(s.T(), err)
}

func (s *GoldenZvolSuite) TestMountZvol_WaitsForDeviceBeforeMount() {
	s.mock.addDataset("testpool/helix-zvols/ses-ses_001")
	s.mock.deviceDelay["testpool/helix-zvols/ses-ses_001"] = 5

	require.NoError(s.T(), mountZvol("testpool/helix-zvols/ses-ses_001", "/container-docker/zvol-mounts/ses_001"))
	assert.Equal(s.T(), 6, s.mock.deviceStats["testpool/helix-zvols/ses-ses_001"])
	assert.True(s.T(), s.mock.hasCommand("mount -o discard /dev/zvol/testpool/helix-zvols/ses-ses_001"))
}

func (s *GoldenZvolSuite) TestMountZvol_DeviceNeverAppearsDoesNotMount() {
	s.mock.addDataset("testpool/helix-zvols/ses-ses_001")
	s.mock.deviceDelay["testpool/helix-zvols/ses-ses_001"] = -1

	require.Error(s.T(), mountZvol("testpool/helix-zvols/ses-ses_001", "/container-docker/zvol-mounts/ses_001"))
	assert.False(s.T(), s.mock.hasCommand("mount "))
}

func (s *GoldenZvolSuite) TestCreateSessionZvol_WaitsForDeviceBeforeMkfs() {
	// Device is missing for the first stats after `zfs create`
	s.mock.deviceDelay["testpool/helix-zvols/ses-ses_001"] = 3

	_, err := CreateSessionZvol("ses_001")
	require.NoError(s.T(), err)
	assert.True(s.T(), s.mock.hasCommand("mkfs.xfs -f -q /dev/zvol/testpool/helix-zvols/ses-ses_001"))
}

func (s *GoldenZvolSuite) TestCreateSessionZvol_DeviceNeverAppearsCleansUp() {
	s.mock.deviceDelay["testpool/helix-zvols/ses-ses_001"] = -1

	_, err := CreateSessionZvol("ses_001")
	require.Error(s.T(), err)
	assert.False(s.T(), s.mock.hasCommand("mkfs.xfs"))
	assert.False(s.T(), s.mock.datasets["testpool/helix-zvols/ses-ses_001"])
}

// -----------------------------------------------------------------------
// Promotion races /dev/zvol after rename
// -----------------------------------------------------------------------

// The meta incident: the renamed golden's /dev/zvol symlink shows up after
// the mount attempt. The promotion must wait for it and finish.
func (s *GoldenZvolSuite) TestPromote_WaitsForRenamedGoldenDevice() {
	golden := "testpool/helix-zvols/golden-prj_abc"
	s.mock.addDataset(golden)
	s.mock.addSnapshot(golden, "gen14")
	s.mock.addDataset("testpool/helix-zvols/ses-ses_002")
	// Device of the renamed golden lags behind the rename
	s.mock.deviceDelay["testpool/helix-zvols/ses-ses_002"] = -1
	s.mock.deviceDelay[golden] = 10

	require.NoError(s.T(), PromoteSessionToGoldenZvol("prj_abc", "ses_002"))

	assert.True(s.T(), s.mock.datasets[golden+"@gen15"])
	assert.Equal(s.T(), golden+"@gen15", latestGoldenSnapshot("prj_abc"))
	assert.Empty(s.T(), s.mock.props[golden][goldenPendingPromotionProp], "pending mark must be cleared")
}

// If the device never appears the promotion fails, but leaves a mark so the
// next reconcile finishes it instead of leaving it half-promoted.
func (s *GoldenZvolSuite) TestPromote_MountFailureIsResumedByReconcile() {
	golden := "testpool/helix-zvols/golden-prj_abc"
	s.mock.addDataset(golden)
	s.mock.addSnapshot(golden, "gen14")
	s.mock.addDataset("testpool/helix-zvols/ses-ses_002")
	s.mock.deviceDelay[golden] = -1

	err := PromoteSessionToGoldenZvol("prj_abc", "ses_002")
	require.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "failed to mount golden for purge")

	// Half-promoted: renamed, but sessions would still clone gen14
	assert.False(s.T(), s.mock.datasets["testpool/helix-zvols/ses-ses_002"])
	assert.Equal(s.T(), golden+"@gen14", latestGoldenSnapshot("prj_abc"))
	assert.Equal(s.T(), "15,ses_002", s.mock.props[golden][goldenPendingPromotionProp])
	assert.False(s.T(), s.mock.mountedPaths["/container-docker/zvol-mounts/golden-prj_abc"])

	// udev catches up; the periodic/startup reconcile completes the promotion
	delete(s.mock.deviceDelay, golden)
	assert.Equal(s.T(), 1, ReconcilePendingGoldenPromotions())

	assert.Equal(s.T(), golden+"@gen15", latestGoldenSnapshot("prj_abc"))
	assert.Empty(s.T(), s.mock.props[golden][goldenPendingPromotionProp])
	assert.False(s.T(), s.mock.mountedPaths["/container-docker/zvol-mounts/golden-prj_abc"])

	// Nothing left to do
	assert.Equal(s.T(), 0, ReconcilePendingGoldenPromotions())
}

// The next golden build finishes an interrupted promotion before its own,
// keeping generations monotonic.
func (s *GoldenZvolSuite) TestPromote_FinishesPreviousInterruptedPromotionFirst() {
	golden := "testpool/helix-zvols/golden-prj_abc"
	s.mock.addDataset(golden)
	s.mock.addSnapshot(golden, "gen14")
	s.mock.props[golden] = map[string]string{goldenPendingPromotionProp: "15,ses_002"}
	s.mock.addDataset("testpool/helix-zvols/ses-ses_003")

	require.NoError(s.T(), PromoteSessionToGoldenZvol("prj_abc", "ses_003"))

	snaps := s.mock.commandsMatching("zfs snapshot")
	require.Len(s.T(), snaps, 2)
	assert.Equal(s.T(), "zfs snapshot "+golden+"@gen15", snaps[0].String())
	assert.Equal(s.T(), "zfs snapshot "+golden+"@gen16", snaps[1].String())
	assert.Empty(s.T(), s.mock.props[golden][goldenPendingPromotionProp])
}

// First-ever golden build interrupted after the rename: the golden has no
// snapshot at all, so GoldenZvolExists is false and nothing could clone it.
func (s *GoldenZvolSuite) TestPromote_FirstBuildInterruptedIsResumed() {
	golden := "testpool/helix-zvols/golden-prj_abc"
	s.mock.addDataset("testpool/helix-zvols/ses-ses_001")
	s.mock.deviceDelay[golden] = -1

	require.Error(s.T(), PromoteSessionToGoldenZvol("prj_abc", "ses_001"))
	assert.False(s.T(), GoldenZvolExists("prj_abc"))
	assert.Equal(s.T(), "1,ses_001", s.mock.props[golden][goldenPendingPromotionProp])

	delete(s.mock.deviceDelay, golden)
	assert.Equal(s.T(), 1, ReconcilePendingGoldenPromotions())
	assert.True(s.T(), GoldenZvolExists("prj_abc"))
	assert.Equal(s.T(), golden+"@gen1", latestGoldenSnapshot("prj_abc"))
}

// Resuming after the snapshot was taken but before the mark was cleared only
// clears the mark — no remount, no second snapshot.
func (s *GoldenZvolSuite) TestReconcile_SnapshotAlreadyTakenOnlyClearsMark() {
	golden := "testpool/helix-zvols/golden-prj_abc"
	s.mock.addDataset(golden)
	s.mock.addSnapshot(golden, "gen15")
	s.mock.props[golden] = map[string]string{goldenPendingPromotionProp: "15,ses_002"}

	assert.Equal(s.T(), 1, ReconcilePendingGoldenPromotions())
	assert.False(s.T(), s.mock.hasCommand("mount "))
	assert.False(s.T(), s.mock.hasCommand("zfs snapshot"))
	assert.Empty(s.T(), s.mock.props[golden][goldenPendingPromotionProp])
}

// Session clones marked before a failed promote/rename are not goldens;
// reconcile leaves them to the orphan GC.
func (s *GoldenZvolSuite) TestReconcile_IgnoresMarkedSessionClones() {
	clone := "testpool/helix-zvols/ses-ses_002"
	s.mock.addDataset(clone)
	s.mock.props[clone] = map[string]string{goldenPendingPromotionProp: "15,ses_002"}

	assert.Equal(s.T(), 0, ReconcilePendingGoldenPromotions())
	assert.False(s.T(), s.mock.hasCommand("zfs rename"))
}

// MigrateGoldenToZvol used to destroy any golden zvol without a snapshot as a
// "partial migration" — which would throw away an interrupted first build.
func (s *GoldenZvolSuite) TestMigrateGoldenToZvol_FinishesPendingPromotionInsteadOfDestroying() {
	golden := "testpool/helix-zvols/golden-prj_abc"
	s.mock.addDataset(golden)
	s.mock.props[golden] = map[string]string{goldenPendingPromotionProp: "1,ses_001"}

	require.NoError(s.T(), MigrateGoldenToZvol("prj_abc"))
	assert.False(s.T(), s.mock.hasCommand("zfs destroy -r "+golden))
	assert.Equal(s.T(), golden+"@gen1", latestGoldenSnapshot("prj_abc"))
}

// -----------------------------------------------------------------------
// Golden build outcome
// -----------------------------------------------------------------------

func (s *GoldenZvolSuite) newGoldenBuildManager() *DevContainerManager {
	zfsAvailableFlag = true
	zfsAvailableOnce.Do(func() {})
	return &DevContainerManager{goldenBuildResults: make(map[string]*GoldenBuildResult)}
}

// A build whose script exited 0 but whose promotion failed must be reported
// failed with the promotion error, not as a fresh cache.
func (s *GoldenZvolSuite) TestCompleteGoldenBuild_PromotionFailureIsFailedResult() {
	dm := s.newGoldenBuildManager()
	golden := "testpool/helix-zvols/golden-prj_abc"
	s.mock.addDataset(golden)
	s.mock.addSnapshot(golden, "gen14")
	s.mock.addDataset("testpool/helix-zvols/ses-ses_002")
	s.mock.deviceDelay[golden] = -1

	res := dm.completeGoldenBuild(&DevContainer{SessionID: "ses_002", ProjectID: "prj_abc"}, "0", time.Minute)

	assert.False(s.T(), res.Success)
	assert.Equal(s.T(), "0", res.ExitCode)
	assert.True(s.T(), strings.HasPrefix(res.Error, "Golden cache promotion failed: "), res.Error)
	assert.Contains(s.T(), res.Error, "did not appear")
	assert.Same(s.T(), res, dm.GetGoldenBuildResult("prj_abc"))
}

func (s *GoldenZvolSuite) TestCompleteGoldenBuild_SuccessfulPromotion() {
	dm := s.newGoldenBuildManager()
	golden := "testpool/helix-zvols/golden-prj_abc"
	s.mock.addDataset(golden)
	s.mock.addSnapshot(golden, "gen14")
	s.mock.addDataset("testpool/helix-zvols/ses-ses_002")

	res := dm.completeGoldenBuild(&DevContainer{SessionID: "ses_002", ProjectID: "prj_abc"}, "0", time.Minute)

	assert.True(s.T(), res.Success)
	assert.Empty(s.T(), res.Error)
	assert.Equal(s.T(), golden+"@gen15", latestGoldenSnapshot("prj_abc"))
}
