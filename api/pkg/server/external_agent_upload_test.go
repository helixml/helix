package server

import (
	"bytes"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/connman"
	"github.com/helixml/helix/api/pkg/controller"
	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

func TestDesktopUploadURL(t *testing.T) {
	t.Run("preserves chat upload options", func(t *testing.T) {
		require.Equal(
			t,
			"http://localhost:9876/upload?open_file_manager=false",
			desktopUploadURL("open_file_manager=false"),
		)
	})

	t.Run("omits an empty query", func(t *testing.T) {
		require.Equal(t, "http://localhost:9876/upload", desktopUploadURL(""))
	})
}

func TestDesktopFileURL(t *testing.T) {
	require.Equal(
		t,
		"http://localhost:9876/file?name=my+image.png",
		desktopFileURL("my image.png"),
	)
}

func TestValidWorkspaceAttachmentFilename(t *testing.T) {
	require.True(t, validWorkspaceAttachmentFilename("my image.png"))
	require.False(t, validWorkspaceAttachmentFilename(""))
	require.False(t, validWorkspaceAttachmentFilename("../secret"))
	require.False(t, validWorkspaceAttachmentFilename(`folder\\secret`))
}

type SandboxUploadSuite struct {
	suite.Suite
	ctrl     *gomock.Controller
	store    *store.MockStore
	executor *external_agent.MockExecutor
	server   *HelixAPIServer
}

func TestSandboxUploadSuite(t *testing.T) { suite.Run(t, new(SandboxUploadSuite)) }

func (s *SandboxUploadSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.executor = external_agent.NewMockExecutor(s.ctrl)
	s.server = &HelixAPIServer{
		Store:                 s.store,
		externalAgentExecutor: s.executor,
		connman:               connman.New(),
		Cfg:                   &config.ServerConfig{},
		Controller: &controller.Controller{
			Options: controller.Options{Store: s.store, PubSub: pubsub.NewNoop()},
		},
	}
}

func (s *SandboxUploadSuite) TearDownTest() { s.ctrl.Finish() }

// upload posts a one-file multipart body to the handler as the session owner.
func (s *SandboxUploadSuite) upload(sessionID string) *httptest.ResponseRecorder {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", "passport.png")
	s.Require().NoError(err)
	_, err = part.Write([]byte("image"))
	s.Require().NoError(err)
	s.Require().NoError(form.Close())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/external-agents/"+sessionID+"/upload?open_file_manager=false", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	req = mux.SetURLVars(req, map[string]string{"sessionID": sessionID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "usr_owner"}))

	rec := httptest.NewRecorder()
	s.server.uploadFileToSandbox(rec, req)
	return rec
}

// A stopped session keeps its container name in the database. Before the fix
// the handler read that name as "running", skipped the resume and failed with
// "Sandbox not connected: no connection" — every upload to a stopped org bot.
func (s *SandboxUploadSuite) TestStoppedSessionWithStaleContainerNameIsResumed() {
	session := &types.Session{
		ID:        "ses_stopped",
		Owner:     "usr_owner",
		ProjectID: "prj_bot",
		Metadata: types.SessionMetadata{
			AgentType:     "zed_external",
			ContainerName: "headless-external-stopped",
		},
	}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_stopped").Return(session, nil)
	s.executor.EXPECT().HasRunningContainer(gomock.Any(), "ses_stopped").Return(false)
	// The resume path loads the project first; failing it here proves the
	// handler tried to start the desktop instead of dialling a dead one.
	s.store.EXPECT().GetProject(gomock.Any(), "prj_bot").Return(nil, errors.New("boom"))

	rec := s.upload("ses_stopped")

	s.Equal(http.StatusServiceUnavailable, rec.Code)
	s.Contains(rec.Body.String(), "failed to start agent for upload")
}

func (s *SandboxUploadSuite) TestRunningSessionIsNotResumed() {
	session := &types.Session{
		ID:       "ses_running",
		Owner:    "usr_owner",
		Metadata: types.SessionMetadata{AgentType: "zed_external", ExternalAgentStatus: "running"},
	}
	s.store.EXPECT().GetSession(gomock.Any(), "ses_running").Return(session, nil)
	s.executor.EXPECT().HasRunningContainer(gomock.Any(), "ses_running").Return(true)

	rec := s.upload("ses_running")

	// No bridge is registered in this test, so the dial fails straight away —
	// without a resume (no GetProject call is expected).
	s.Equal(http.StatusServiceUnavailable, rec.Code)
	s.Contains(rec.Body.String(), "Sandbox not connected")
}
