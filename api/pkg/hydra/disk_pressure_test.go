package hydra

import (
	"fmt"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// DiskPressureSuite exercises the disk-pressure admission-control guard.
//
// It mocks execCmdOutput directly (the zpool command path) rather than going
// through mockZFS, because mockZFS doesn't model `zpool`. ZFS availability is
// forced via zfsAvailableFlag/zfsParentDataset so we never shell out for real.
type DiskPressureSuite struct {
	suite.Suite

	origOutput func(name string, args ...string) ([]byte, error)
	// zpoolOut is the stdout returned for a `zpool list` invocation.
	zpoolOut string
	// zpoolErr, if set, is returned instead of zpoolOut for `zpool list`.
	zpoolErr error
}

func TestDiskPressureSuite(t *testing.T) {
	suite.Run(t, new(DiskPressureSuite))
}

func (s *DiskPressureSuite) SetupTest() {
	resetZFSState()
	resetDiskPressureConfig()

	// Force ZFS "available" so poolFreePercent() doesn't shell out. Consume the
	// sync.Once (with a no-op) so ZFSAvailable() returns these forced values
	// instead of re-running the real `zfs list` probe, which would clobber both
	// zfsAvailableFlag and zfsParentDataset.
	zfsAvailableFlag = true
	zfsParentDataset = "testpool/helix-zvols"
	zfsAvailableOnce.Do(func() {})

	s.zpoolOut = ""
	s.zpoolErr = nil

	s.origOutput = execCmdOutput
	execCmdOutput = func(name string, args ...string) ([]byte, error) {
		if name == "zpool" && len(args) >= 1 && args[0] == "list" {
			if s.zpoolErr != nil {
				return nil, s.zpoolErr
			}
			return []byte(s.zpoolOut), nil
		}
		return nil, fmt.Errorf("unexpected command in test: %s %s", name, strings.Join(args, " "))
	}
}

func (s *DiskPressureSuite) TearDownTest() {
	execCmdOutput = s.origOutput
	resetZFSState()
	resetDiskPressureConfig()
}

// -----------------------------------------------------------------------
// poolName
// -----------------------------------------------------------------------

func (s *DiskPressureSuite) TestPoolName_FirstComponent() {
	zfsParentDataset = "prod/helix-zvols"
	assert.Equal(s.T(), "prod", poolName())
}

func (s *DiskPressureSuite) TestPoolName_Empty() {
	zfsParentDataset = ""
	assert.Equal(s.T(), "", poolName())
}

// -----------------------------------------------------------------------
// poolFreePercent
// -----------------------------------------------------------------------

func (s *DiskPressureSuite) TestPoolFreePercent_Parses() {
	// size=1000, free=20 → 2.0%
	s.zpoolOut = "1000\t20\n"
	pct, err := poolFreePercent()
	require.NoError(s.T(), err)
	assert.InDelta(s.T(), 2.0, pct, 0.0001)
}

func (s *DiskPressureSuite) TestPoolFreePercent_ParsesHalf() {
	// size=200, free=100 → 50%
	s.zpoolOut = "200 100"
	pct, err := poolFreePercent()
	require.NoError(s.T(), err)
	assert.InDelta(s.T(), 50.0, pct, 0.0001)
}

func (s *DiskPressureSuite) TestPoolFreePercent_CommandError() {
	s.zpoolErr = fmt.Errorf("zpool: no such pool")
	_, err := poolFreePercent()
	require.Error(s.T(), err)
}

func (s *DiskPressureSuite) TestPoolFreePercent_SizeZero() {
	s.zpoolOut = "0\t0\n"
	_, err := poolFreePercent()
	require.Error(s.T(), err)
}

func (s *DiskPressureSuite) TestPoolFreePercent_UnparsableOutput() {
	s.zpoolOut = "not-a-number"
	_, err := poolFreePercent()
	require.Error(s.T(), err)
}

func (s *DiskPressureSuite) TestPoolFreePercent_ZFSUnavailable() {
	zfsAvailableFlag = false
	_, err := poolFreePercent()
	require.Error(s.T(), err)
}

func (s *DiskPressureSuite) TestPoolFreePercent_EmptyPoolName() {
	zfsParentDataset = ""
	_, err := poolFreePercent()
	require.Error(s.T(), err)
}

// -----------------------------------------------------------------------
// poolFreeBytes
// -----------------------------------------------------------------------

func (s *DiskPressureSuite) TestPoolFreeBytes_Parses() {
	s.zpoolOut = "123456789\n"
	free, err := poolFreeBytes()
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(123456789), free)
}

func (s *DiskPressureSuite) TestPoolFreeBytes_CommandError() {
	s.zpoolErr = fmt.Errorf("zpool: no such pool")
	_, err := poolFreeBytes()
	require.Error(s.T(), err)
}

func (s *DiskPressureSuite) TestPoolFreeBytes_EmptyOutput() {
	s.zpoolOut = "  \n"
	_, err := poolFreeBytes()
	require.Error(s.T(), err)
}

func (s *DiskPressureSuite) TestPoolFreeBytes_UnparsableOutput() {
	s.zpoolOut = "not-a-number"
	_, err := poolFreeBytes()
	require.Error(s.T(), err)
}

func (s *DiskPressureSuite) TestPoolFreeBytes_ZFSUnavailable() {
	zfsAvailableFlag = false
	_, err := poolFreeBytes()
	require.Error(s.T(), err)
}

func (s *DiskPressureSuite) TestPoolFreeBytes_EmptyPoolName() {
	zfsParentDataset = ""
	_, err := poolFreeBytes()
	require.Error(s.T(), err)
}

// -----------------------------------------------------------------------
// checkDiskPressureForStart
// -----------------------------------------------------------------------

func (s *DiskPressureSuite) TestCheckStart_RefusesWhenLow() {
	// free = 2% (at threshold) → refuse
	s.zpoolOut = "1000\t20\n"
	err := checkDiskPressureForStart()
	require.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "disk space critically low")
}

func (s *DiskPressureSuite) TestCheckStart_RefusesBelowThreshold() {
	// free = 0.5% → refuse
	s.zpoolOut = "1000\t5\n"
	err := checkDiskPressureForStart()
	require.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "disk space critically low")
}

func (s *DiskPressureSuite) TestCheckStart_AllowsWhenAmple() {
	// free = 10% → allow
	s.zpoolOut = "1000\t100\n"
	err := checkDiskPressureForStart()
	require.NoError(s.T(), err)
}

func (s *DiskPressureSuite) TestCheckStart_AllowsJustAboveThreshold() {
	// free = 2.5% → allow (strictly above 2% refuse threshold)
	s.zpoolOut = "1000\t25\n"
	err := checkDiskPressureForStart()
	require.NoError(s.T(), err)
}

func (s *DiskPressureSuite) TestCheckStart_FailOpenOnMeasurementError() {
	// Command errors → fail open (allow start).
	s.zpoolErr = fmt.Errorf("zpool: no such pool")
	err := checkDiskPressureForStart()
	require.NoError(s.T(), err)
}

func (s *DiskPressureSuite) TestCheckStart_FailOpenOnSizeZero() {
	s.zpoolOut = "0\t0\n"
	err := checkDiskPressureForStart()
	require.NoError(s.T(), err)
}

func (s *DiskPressureSuite) TestCheckStart_AllowsWhenDisabled() {
	s.T().Setenv("HELIX_DISK_PRESSURE_ENABLED", "false")
	resetDiskPressureConfig()

	// Even with critically low free space, disabled → allow.
	s.zpoolOut = "1000\t1\n"
	err := checkDiskPressureForStart()
	require.NoError(s.T(), err)
}

// -----------------------------------------------------------------------
// emergency-stop decision
// -----------------------------------------------------------------------

// shouldEmergencyStop mirrors the monitor's per-tick decision: stop when the
// measurement succeeds AND free% is at or below the stop threshold. A failed
// measurement (err != nil) never triggers a stop (fail-open).
func shouldEmergencyStop(freePct float64, measureErr error, stopPct float64) bool {
	if measureErr != nil {
		return false
	}
	return freePct <= stopPct
}

func (s *DiskPressureSuite) TestEmergencyStopDecision_Triggers() {
	cfg := getDiskPressureConfig()
	// free = 1% (at stop threshold) → stop
	assert.True(s.T(), shouldEmergencyStop(1.0, nil, cfg.stopFreePct))
	// free = 0.5% → stop
	assert.True(s.T(), shouldEmergencyStop(0.5, nil, cfg.stopFreePct))
}

func (s *DiskPressureSuite) TestEmergencyStopDecision_DoesNotTrigger() {
	cfg := getDiskPressureConfig()
	// free = 1.5% → no stop (above 1% stop threshold)
	assert.False(s.T(), shouldEmergencyStop(1.5, nil, cfg.stopFreePct))
	// free = 5% → no stop
	assert.False(s.T(), shouldEmergencyStop(5.0, nil, cfg.stopFreePct))
}

func (s *DiskPressureSuite) TestEmergencyStopDecision_FailOpenOnError() {
	cfg := getDiskPressureConfig()
	// Even at 0% free, a measurement error must NOT trigger a stop.
	assert.False(s.T(), shouldEmergencyStop(0.0, fmt.Errorf("boom"), cfg.stopFreePct))
}

// emergencyStopAllDevContainers with an empty container set must be a safe no-op
// (no panic, no docker calls).
func (s *DiskPressureSuite) TestEmergencyStop_EmptySetIsNoop() {
	dm := &DevContainerManager{
		containers: make(map[string]*DevContainer),
	}
	assert.NotPanics(s.T(), func() {
		dm.emergencyStopAllDevContainers(0.5)
	})
}

// -----------------------------------------------------------------------
// config defaults
// -----------------------------------------------------------------------

func (s *DiskPressureSuite) TestConfigDefaults() {
	cfg := getDiskPressureConfig()
	assert.True(s.T(), cfg.enabled)
	assert.InDelta(s.T(), defaultDiskPressureRefuseFreePct, cfg.refuseFreePct, 0.0001)
	assert.InDelta(s.T(), defaultDiskPressureStopFreePct, cfg.stopFreePct, 0.0001)
	assert.Equal(s.T(), defaultDiskPressureCheckInterval, cfg.checkInterval)
}

func (s *DiskPressureSuite) TestConfigEnvOverrides() {
	s.T().Setenv("HELIX_DISK_PRESSURE_REFUSE_FREE_PCT", "5")
	s.T().Setenv("HELIX_DISK_PRESSURE_STOP_FREE_PCT", "3")
	s.T().Setenv("HELIX_DISK_PRESSURE_CHECK_INTERVAL", "10s")
	resetDiskPressureConfig()

	cfg := getDiskPressureConfig()
	assert.InDelta(s.T(), 5.0, cfg.refuseFreePct, 0.0001)
	assert.InDelta(s.T(), 3.0, cfg.stopFreePct, 0.0001)
	assert.Equal(s.T(), "10s", cfg.checkInterval.String())
}

// =======================================================================
// statfs fallback (non-ZFS hosts: K8s on ext4/xfs, the actual deployment
// shape where the original implementation silently failed open).
// =======================================================================

// DiskPressureStatfsSuite exercises the non-ZFS code path: ZFS is forced
// "unavailable" so measureDisk() picks the statfs backend, and statfsFn is
// swapped for a table-driven mock so we can pin free percentages and inject
// ENOENT without touching the real filesystem.
type DiskPressureStatfsSuite struct {
	suite.Suite

	origStatfs func(path string, stat *syscall.Statfs_t) error
	origOutput func(name string, args ...string) ([]byte, error)

	// statfsBlocks/statfsBavail/statfsBsize define the synthetic filesystem
	// returned by the mock. Free % = Bavail/Blocks.
	statfsBlocks uint64
	statfsBavail uint64
	statfsBsize  int64
	statfsErr    error
}

func TestDiskPressureStatfsSuite(t *testing.T) {
	suite.Run(t, new(DiskPressureStatfsSuite))
}

func (s *DiskPressureStatfsSuite) SetupTest() {
	resetZFSState()
	resetDiskPressureConfig()

	// Force ZFS UNAVAILABLE so zfsBackendAvailable() returns false and
	// measureDisk() takes the statfs branch.
	zfsAvailableFlag = false
	zfsParentDataset = ""
	zfsAvailableOnce.Do(func() {})

	// Default: 1000 blocks total, 100 available, 1-byte blocks => 10% free.
	s.statfsBlocks = 1000
	s.statfsBavail = 100
	s.statfsBsize = 1
	s.statfsErr = nil

	s.origStatfs = statfsFn
	statfsFn = func(_ string, stat *syscall.Statfs_t) error {
		if s.statfsErr != nil {
			return s.statfsErr
		}
		stat.Blocks = s.statfsBlocks
		stat.Bavail = s.statfsBavail
		setStatfsBsize(stat, s.statfsBsize)
		return nil
	}

	// Trip wires: ZFS should NEVER be queried when the backend is statfs. If
	// these fire, the routing is wrong.
	s.origOutput = execCmdOutput
	execCmdOutput = func(name string, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("statfs suite should not invoke %s %s", name, strings.Join(args, " "))
	}
}

func (s *DiskPressureStatfsSuite) TearDownTest() {
	statfsFn = s.origStatfs
	execCmdOutput = s.origOutput
	resetZFSState()
	resetDiskPressureConfig()
}

// -----------------------------------------------------------------------
// statfsFreePercent / statfsFreeBytes
// -----------------------------------------------------------------------

func (s *DiskPressureStatfsSuite) TestStatfsFreePercent_TenPercent() {
	pct, err := statfsFreePercent("/anywhere")
	require.NoError(s.T(), err)
	assert.InDelta(s.T(), 10.0, pct, 0.0001)
}

func (s *DiskPressureStatfsSuite) TestStatfsFreePercent_ZeroBlocksError() {
	s.statfsBlocks = 0
	_, err := statfsFreePercent("/anywhere")
	require.Error(s.T(), err)
}

func (s *DiskPressureStatfsSuite) TestStatfsFreePercent_ENOENT() {
	s.statfsErr = syscall.ENOENT
	_, err := statfsFreePercent("/no/such/path")
	require.Error(s.T(), err)
}

func (s *DiskPressureStatfsSuite) TestStatfsFreeBytes_Computes() {
	// 100 available blocks * 4096 = 409,600 bytes
	s.statfsBavail = 100
	s.statfsBsize = 4096
	free, err := statfsFreeBytes("/anywhere")
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(100*4096), free)
}

// -----------------------------------------------------------------------
// measureDisk: backend selection
// -----------------------------------------------------------------------

func (s *DiskPressureStatfsSuite) TestMeasureDisk_PicksStatfsWhenNoZFS() {
	m, err := measureDisk()
	require.NoError(s.T(), err)
	assert.Equal(s.T(), "statfs", m.backend)
	assert.True(s.T(), m.hasPct)
	assert.InDelta(s.T(), 10.0, m.freePct, 0.0001)
}

func (s *DiskPressureStatfsSuite) TestMeasureDisk_StatfsENOENTPropagates() {
	s.statfsErr = syscall.ENOENT
	m, err := measureDisk()
	require.Error(s.T(), err)
	assert.Equal(s.T(), "statfs", m.backend)
	assert.True(s.T(), m.pathENOENT, "ENOENT must surface so the start guard can fail closed")
}

// -----------------------------------------------------------------------
// checkDiskPressureForStart on the statfs backend
// -----------------------------------------------------------------------

func (s *DiskPressureStatfsSuite) TestCheckStart_StatfsRefusesAtThreshold() {
	// Bavail/Blocks = 20/1000 = 2.0%, refuse threshold default 2% => refuse.
	s.statfsBavail = 20
	err := checkDiskPressureForStart()
	require.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "disk space critically low")
}

func (s *DiskPressureStatfsSuite) TestCheckStart_StatfsRefusesBelowThreshold() {
	// 5/1000 = 0.5%
	s.statfsBavail = 5
	err := checkDiskPressureForStart()
	require.Error(s.T(), err)
}

func (s *DiskPressureStatfsSuite) TestCheckStart_StatfsAllowsAmple() {
	// 100/1000 = 10%
	s.statfsBavail = 100
	err := checkDiskPressureForStart()
	require.NoError(s.T(), err)
}

func (s *DiskPressureStatfsSuite) TestCheckStart_StatfsAllowsJustAbove() {
	// 25/1000 = 2.5%, strictly above 2% refuse threshold
	s.statfsBavail = 25
	err := checkDiskPressureForStart()
	require.NoError(s.T(), err)
}

func (s *DiskPressureStatfsSuite) TestCheckStart_StatfsFailsClosedOnENOENT() {
	// Misconfigured path: refuse, do NOT silently allow. This is the bug the
	// fallback exists to fix.
	s.statfsErr = syscall.ENOENT
	err := checkDiskPressureForStart()
	require.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "does not exist")
}

func (s *DiskPressureStatfsSuite) TestCheckStart_StatfsFailsOpenOnOtherErr() {
	// EIO and friends are transient hardware / FUSE errors. Fail open here so
	// a flaky filesystem doesn't lock out all sandbox starts; the warn log
	// surfaces the issue for operators.
	s.statfsErr = syscall.EIO
	err := checkDiskPressureForStart()
	require.NoError(s.T(), err)
}

func (s *DiskPressureStatfsSuite) TestCheckStart_StatfsAllowsWhenDisabled() {
	s.T().Setenv("HELIX_DISK_PRESSURE_ENABLED", "false")
	resetDiskPressureConfig()

	// Even with 0% free, disabled => allow.
	s.statfsBavail = 0
	err := checkDiskPressureForStart()
	require.NoError(s.T(), err)
}

// -----------------------------------------------------------------------
// poolFreeBytes fallback on non-ZFS hosts
// -----------------------------------------------------------------------

func (s *DiskPressureStatfsSuite) TestPoolFreeBytes_StatfsFallback() {
	// 250 blocks * 4096 byte blocks = 1,024,000 bytes available.
	s.statfsBavail = 250
	s.statfsBsize = 4096
	free, err := poolFreeBytes()
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(250*4096), free)
}

// -----------------------------------------------------------------------
// diskPressurePaths: env parsing + defaults
// -----------------------------------------------------------------------

func (s *DiskPressureStatfsSuite) TestDiskPressurePaths_DefaultIsK8sLayout() {
	// Default must cover the three PVC mount points the helix-sandbox chart
	// provisions. Without /var/lib/docker the original ENOSPC-during-pull
	// failure mode would still slip past the guard.
	assert.Equal(s.T(), []string{"/var/lib/docker", "/hydra-data", "/data"}, diskPressurePaths())
}

func (s *DiskPressureStatfsSuite) TestDiskPressurePaths_SingularBackCompat() {
	// Operators on the previous single-path env var keep working.
	s.T().Setenv("HELIX_DISK_PRESSURE_PATH", "/var/lib/hydra")
	assert.Equal(s.T(), []string{"/var/lib/hydra"}, diskPressurePaths())
}

func (s *DiskPressureStatfsSuite) TestDiskPressurePaths_PluralWins() {
	// When both are set the plural form takes precedence.
	s.T().Setenv("HELIX_DISK_PRESSURE_PATH", "/single")
	s.T().Setenv("HELIX_DISK_PRESSURE_PATHS", "/a,/b,/c")
	assert.Equal(s.T(), []string{"/a", "/b", "/c"}, diskPressurePaths())
}

func (s *DiskPressureStatfsSuite) TestDiskPressurePaths_TrimsAndDropsEmpties() {
	s.T().Setenv("HELIX_DISK_PRESSURE_PATHS", "  /a , ,/b ,  ,/c  ")
	assert.Equal(s.T(), []string{"/a", "/b", "/c"}, diskPressurePaths())
}

func (s *DiskPressureStatfsSuite) TestDiskPressurePaths_EmptyPluralFallsThroughToSingular() {
	// Plural set to whitespace-only entries -> treated as unset; singular wins.
	s.T().Setenv("HELIX_DISK_PRESSURE_PATHS", " , , ")
	s.T().Setenv("HELIX_DISK_PRESSURE_PATH", "/var/lib/hydra")
	assert.Equal(s.T(), []string{"/var/lib/hydra"}, diskPressurePaths())
}

// -----------------------------------------------------------------------
// statfsFreePercentMulti
// -----------------------------------------------------------------------

// perPathResult describes the synthetic statfs reply for a single path.
type perPathResult struct {
	blocks uint64
	bavail uint64
	bsize  int64
	err    error
}

// installPerPathStatfs swaps statfsFn for a map-driven mock keyed by path.
// Returns a cleanup the suite should invoke in TearDown order; for these
// tests we just rely on the suite's TearDownTest which restores origStatfs.
func (s *DiskPressureStatfsSuite) installPerPathStatfs(results map[string]perPathResult) {
	statfsFn = func(path string, stat *syscall.Statfs_t) error {
		r, ok := results[path]
		if !ok {
			return fmt.Errorf("unexpected statfs path in test: %s", path)
		}
		if r.err != nil {
			return r.err
		}
		stat.Blocks = r.blocks
		stat.Bavail = r.bavail
		setStatfsBsize(stat, r.bsize)
		return nil
	}
}

func (s *DiskPressureStatfsSuite) TestStatfsMulti_PicksMinFreePct() {
	// /a 50%, /b 10%, /c 30%  -> min is /b at 10%.
	s.installPerPathStatfs(map[string]perPathResult{
		"/a": {blocks: 1000, bavail: 500, bsize: 1},
		"/b": {blocks: 1000, bavail: 100, bsize: 1},
		"/c": {blocks: 1000, bavail: 300, bsize: 1},
	})
	pct, trigger, enoent, err := statfsFreePercentMulti([]string{"/a", "/b", "/c"})
	require.NoError(s.T(), err)
	assert.False(s.T(), enoent)
	assert.Equal(s.T(), "/b", trigger)
	assert.InDelta(s.T(), 10.0, pct, 0.0001)
}

func (s *DiskPressureStatfsSuite) TestStatfsMulti_TriggerPathIsTheConstrainingOne() {
	// Single path: trigger must be that path.
	s.installPerPathStatfs(map[string]perPathResult{
		"/only": {blocks: 1000, bavail: 17, bsize: 1},
	})
	_, trigger, _, err := statfsFreePercentMulti([]string{"/only"})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), "/only", trigger)
}

func (s *DiskPressureStatfsSuite) TestStatfsMulti_AnyENOENTIsFailClosed() {
	// /a measures fine, /b is ENOENT. Overall must surface pathENOENT=true
	// AND return an error so the start guard refuses.
	s.installPerPathStatfs(map[string]perPathResult{
		"/a": {blocks: 1000, bavail: 500, bsize: 1},
		"/b": {err: syscall.ENOENT},
	})
	_, _, enoent, err := statfsFreePercentMulti([]string{"/a", "/b"})
	// At least one measurement succeeded so freePct is returned (no error),
	// but pathENOENT must be set so callers can fail closed on misconfig.
	require.NoError(s.T(), err)
	assert.True(s.T(), enoent, "any ENOENT path must surface pathENOENT=true")
}

func (s *DiskPressureStatfsSuite) TestStatfsMulti_TransientErrorSkipped() {
	// /a is EIO (transient), /b is fine. Multi should use /b and NOT fail.
	s.installPerPathStatfs(map[string]perPathResult{
		"/a": {err: syscall.EIO},
		"/b": {blocks: 1000, bavail: 250, bsize: 1},
	})
	pct, trigger, enoent, err := statfsFreePercentMulti([]string{"/a", "/b"})
	require.NoError(s.T(), err)
	assert.False(s.T(), enoent)
	assert.Equal(s.T(), "/b", trigger)
	assert.InDelta(s.T(), 25.0, pct, 0.0001)
}

func (s *DiskPressureStatfsSuite) TestStatfsMulti_AllErrorsReturnError() {
	// All paths error (non-ENOENT): no measurement, caller falls open.
	s.installPerPathStatfs(map[string]perPathResult{
		"/a": {err: syscall.EIO},
		"/b": {err: syscall.EACCES},
	})
	_, _, _, err := statfsFreePercentMulti([]string{"/a", "/b"})
	require.Error(s.T(), err)
}

func (s *DiskPressureStatfsSuite) TestStatfsMulti_EmptyPathsErrors() {
	_, _, _, err := statfsFreePercentMulti(nil)
	require.Error(s.T(), err)
}

// -----------------------------------------------------------------------
// admission refusal surfaces the triggering path
// -----------------------------------------------------------------------

func (s *DiskPressureStatfsSuite) TestCheckStart_RefusalIncludesTriggerPath() {
	// /var/lib/docker is the full one. The refusal log + error must name it
	// so operators can fix the right volume.
	s.T().Setenv("HELIX_DISK_PRESSURE_PATHS", "/var/lib/docker,/hydra-data,/data")
	s.installPerPathStatfs(map[string]perPathResult{
		"/var/lib/docker": {blocks: 1000, bavail: 13, bsize: 1}, // 1.3% free
		"/hydra-data":     {blocks: 1000, bavail: 800, bsize: 1},
		"/data":           {blocks: 1000, bavail: 700, bsize: 1},
	})
	err := checkDiskPressureForStart()
	require.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "/var/lib/docker", "operator must see which volume is constraining")
	assert.Contains(s.T(), err.Error(), "disk space critically low")
}
