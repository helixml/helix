package server

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/types"
)

// LiveExternalAgentStatusSuite pins the single rule that reconciles the stored
// external_agent_status against a live executor probe. getSession and listTasks
// both call the helper; they used to carry inline copies that drifted, and the
// getSession copy was the source of the "restart looks broken" bug — it
// downgraded an in-flight boot to "stopped", so the UI offered a "Start
// sandbox" button for the whole restart.
type LiveExternalAgentStatusSuite struct {
	suite.Suite
	ctrl     *gomock.Controller
	executor *external_agent.MockExecutor
	server   *HelixAPIServer
}

func TestLiveExternalAgentStatusSuite(t *testing.T) {
	suite.Run(t, new(LiveExternalAgentStatusSuite))
}

func (s *LiveExternalAgentStatusSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.executor = external_agent.NewMockExecutor(s.ctrl)
	s.server = &HelixAPIServer{externalAgentExecutor: s.executor}
}

func (s *LiveExternalAgentStatusSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *LiveExternalAgentStatusSuite) TestTable() {
	cases := []struct {
		name string
		// stored external_agent_status
		status string
		// container name on the session metadata
		container string
		// nil → no executor wired; true → executor tracks the session;
		// false → executor probe errors (container is gone)
		tracked *bool
		want    string
	}{
		// A boot in flight is never downgraded, whether or not a container
		// name is set and whether or not the executor knows about it yet.
		// This is the regression gate for the restart bug.
		{"restarting_with_dead_container", "restarting", "ubuntu-external-ses_1", boolPtr(false), "restarting"},
		{"restarting_no_container", "restarting", "", nil, "restarting"},
		{"starting_with_dead_container", "starting", "ubuntu-external-ses_1", boolPtr(false), "starting"},
		{"starting_no_container", "starting", "", nil, "starting"},
		// ...and never upgraded either: the container can be up in Docker
		// before RevDial connects, and claiming "running" early causes
		// ScreenshotViewer 503s.
		{"starting_with_live_container", "starting", "ubuntu-external-ses_1", boolPtr(true), "starting"},
		{"restarting_with_live_container", "restarting", "ubuntu-external-ses_1", boolPtr(true), "restarting"},

		// Running sessions are probed: a failed probe means the container
		// is genuinely gone.
		{"running_live", "running", "ubuntu-external-ses_1", boolPtr(true), "running"},
		{"running_dead", "running", "ubuntu-external-ses_1", boolPtr(false), "stopped"},
		{"running_no_executor", "running", "ubuntu-external-ses_1", nil, "stopped"},

		// An unlabelled session with a container is probed the same way.
		{"empty_status_live", "", "ubuntu-external-ses_1", boolPtr(true), "running"},
		{"empty_status_dead", "", "ubuntu-external-ses_1", boolPtr(false), "stopped"},

		// Nothing to probe → trust the DB.
		{"empty_status_no_container", "", "", nil, ""},
		{"stopped_no_container", "stopped", "", nil, "stopped"},
	}

	for _, tc := range cases {
		s.Run(tc.name, func() {
			s.SetupTest()
			defer s.ctrl.Finish()

			session := &types.Session{
				ID: "ses_1",
				Metadata: types.SessionMetadata{
					ExternalAgentStatus: tc.status,
					ContainerName:       tc.container,
				},
			}

			switch {
			case tc.tracked == nil:
				s.server.externalAgentExecutor = nil
			case *tc.tracked:
				s.executor.EXPECT().GetSession("ses_1").Return(&external_agent.ZedSession{SessionID: "ses_1"}, nil).AnyTimes()
			default:
				s.executor.EXPECT().GetSession("ses_1").Return(nil, errors.New("session not found")).AnyTimes()
			}

			s.Equal(tc.want, s.server.liveExternalAgentStatus(session))
		})
	}
}

func boolPtr(b bool) *bool { return &b }
