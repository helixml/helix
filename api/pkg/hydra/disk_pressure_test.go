package hydra

import (
	"fmt"
	"strings"
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
