package hydra

import (
	"os"
	"path/filepath"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A golden build whose container survives a Hydra restart must have its
// result monitor resumed on recovery: the build's result is detected, the
// session is promoted to a new golden snapshot, and the result is stored for
// the API — exactly as if Hydra had never restarted.
func (s *GoldenZvolSuite) TestRecoveredGoldenContainerResumesMonitorAndPromotes() {
	zfsAvailableOnce.Do(func() {})
	zfsAvailableFlag = true

	origSessions := sessionsBaseDir
	sessionsBaseDir = filepath.Join(s.tmpDir, "sessions")
	defer func() { sessionsBaseDir = origSessions }()
	origInterval := goldenBuildPollInterval
	goldenBuildPollInterval = 10 * time.Millisecond
	defer func() { goldenBuildPollInterval = origInterval }()

	for _, tc := range []struct {
		name      string
		projectID string
		sessionID string
		kind      goldenCacheKind
		kindLabel string
	}{
		{name: "legacy Docker label", projectID: "prj_docker", sessionID: "ses_docker", kind: dockerGoldenCache},
		{name: "Podman label", projectID: "prj_podman", sessionID: "ses_podman", kind: podmanGoldenCache, kindLabel: "podman"},
	} {
		s.Run(tc.name, func() {
			golden := goldenZvolNameForKind(tc.projectID, tc.kind)
			s.mock.addDataset(golden)
			s.mock.addSnapshot(golden, "gen3")
			s.mock.addDataset(sessionZvolNameForKind(tc.sessionID, tc.kind))

			resultDir := filepath.Join(sessionsBaseDir, "docker-data-"+tc.sessionID, string(tc.kind))
			require.NoError(s.T(), os.MkdirAll(resultDir, 0755))
			require.NoError(s.T(), os.WriteFile(filepath.Join(resultDir, ".golden-build-result"), []byte("0\n"), 0644))

			dm := &DevContainerManager{
				containers:         make(map[string]*DevContainer),
				goldenBuildResults: make(map[string]*GoldenBuildResult),
				goldenCopyProgress: make(map[string]*GoldenCopyProgress),
			}
			labels := map[string]string{
				containerSessionIDLabel:      tc.sessionID,
				containerGoldenBuildLabel:    "true",
				containerProjectIDLabel:      tc.projectID,
				containerGoldenDeadlineLabel: time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			}
			if tc.kindLabel != "" {
				labels[containerGoldenCacheKindLabel] = tc.kindLabel
			}
			dm.adoptRecoveredContainer(&DevContainer{
				SessionID:    tc.sessionID,
				ContainerID:  "0123456789abcdef",
				Status:       DevContainerStatusRunning,
				CreatedAt:    time.Now().Add(-20 * time.Minute),
				DockerSocket: filepath.Join(s.tmpDir, "no-docker.sock"),
			}, labels)

			dc := dm.FindDevContainerBySessionID(tc.sessionID)
			require.NotNil(s.T(), dc)
			assert.True(s.T(), dc.IsGoldenBuild)
			assert.Equal(s.T(), tc.projectID, dc.ProjectID)
			assert.Equal(s.T(), tc.kind, dc.goldenCacheKind)

			var res *GoldenBuildResult
			require.Eventually(s.T(), func() bool {
				dm.goldenBuildResultsMu.RLock()
				defer dm.goldenBuildResultsMu.RUnlock()
				res = dm.goldenBuildResults[tc.projectID]
				return res != nil
			}, 5*time.Second, 10*time.Millisecond, "resumed monitor never recorded a result")

			assert.True(s.T(), res.Success, "build should be promoted: %s", res.Error)
			assert.Equal(s.T(), tc.sessionID, res.SessionID)
			assert.Equal(s.T(), golden+"@gen4", latestGoldenSnapshotForKind(tc.projectID, tc.kind))
		})
	}
}

// Non-golden containers are re-adopted without a golden monitor.
func (s *GoldenZvolSuite) TestRecoveredNonGoldenContainerHasNoMonitor() {
	dm := &DevContainerManager{
		containers:         make(map[string]*DevContainer),
		goldenBuildResults: make(map[string]*GoldenBuildResult),
	}
	dm.adoptRecoveredContainer(&DevContainer{
		SessionID:    "ses_plain",
		ContainerID:  "fedcba9876543210",
		DockerSocket: filepath.Join(s.tmpDir, "no-docker.sock"),
	}, map[string]string{containerSessionIDLabel: "ses_plain"})

	dc := dm.FindDevContainerBySessionID("ses_plain")
	require.NotNil(s.T(), dc)
	assert.False(s.T(), dc.IsGoldenBuild)
	assert.Empty(s.T(), dc.ProjectID)
}
