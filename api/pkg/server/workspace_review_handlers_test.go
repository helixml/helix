package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

// WorkspaceReviewHandlersSuite covers the authorization boundary in front of
// the workspace review endpoints. The desktop side is exercised by
// pkg/desktop's real-git tests; what only exists here is the rule that a
// historical turn diff is resolved from the stored receipt after session
// authorization, and never from anything the browser supplied.
type WorkspaceReviewHandlersSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	store  *store.MockStore
	server *HelixAPIServer
	user   types.User
}

func TestWorkspaceReviewHandlersSuite(t *testing.T) {
	suite.Run(t, new(WorkspaceReviewHandlersSuite))
}

func (s *WorkspaceReviewHandlersSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.server = &HelixAPIServer{Store: s.store}
	s.user = types.User{ID: "usr_owner"}
}

func (s *WorkspaceReviewHandlersSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *WorkspaceReviewHandlersSuite) turnRequest(sessionID, interactionID, rawQuery string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/external-agents/"+sessionID+"/workspace-review/turn/"+interactionID+"?"+rawQuery, nil)
	req = mux.SetURLVars(req, map[string]string{"sessionID": sessionID, "interactionID": interactionID})
	req = req.WithContext(setRequestUser(req.Context(), s.user))
	w := httptest.NewRecorder()
	s.server.getWorkspaceTurnReview(w, req)
	return w
}

func (s *WorkspaceReviewHandlersSuite) TestTurnReviewRejectsAnonymousCaller() {
	req := httptest.NewRequest(http.MethodGet, "/workspace-review/turn/int_1", nil)
	req = mux.SetURLVars(req, map[string]string{"sessionID": "ses_1", "interactionID": "int_1"})
	w := httptest.NewRecorder()

	s.server.getWorkspaceTurnReview(w, req)

	s.Equal(http.StatusUnauthorized, w.Code)
}

func (s *WorkspaceReviewHandlersSuite) TestTurnReviewRejectsUserWithoutSessionAccess() {
	s.store.EXPECT().GetSession(gomock.Any(), "ses_1").
		Return(&types.Session{ID: "ses_1", Owner: "usr_someone_else"}, nil)

	s.Equal(http.StatusForbidden, s.turnRequest("ses_1", "int_1", "").Code)
}

// An interaction ID from another session must not become readable just because
// the caller owns some session: the receipt is only honoured when the
// interaction actually belongs to the authorized session.
func (s *WorkspaceReviewHandlersSuite) TestTurnReviewRejectsInteractionFromAnotherSession() {
	s.store.EXPECT().GetSession(gomock.Any(), "ses_mine").
		Return(&types.Session{ID: "ses_mine", Owner: s.user.ID}, nil)
	s.store.EXPECT().GetInteraction(gomock.Any(), "int_theirs").Return(&types.Interaction{
		ID:        "int_theirs",
		SessionID: "ses_theirs",
		CodeChanges: &types.InteractionCodeChanges{
			Status:    types.CodeChangesStatusReady,
			BeforeRef: "refs/helix/checkpoints/ses_theirs/int_theirs/before",
			AfterRef:  "refs/helix/checkpoints/ses_theirs/int_theirs/after",
		},
	}, nil)

	s.Equal(http.StatusNotFound, s.turnRequest("ses_mine", "int_theirs", "").Code)
}

func (s *WorkspaceReviewHandlersSuite) TestTurnReviewRefusesReceiptsThatAreNotReady() {
	for _, changes := range []*types.InteractionCodeChanges{
		nil,
		{Status: types.CodeChangesStatusCapturing, BeforeRef: "refs/helix/checkpoints/ses_1/int_1/before"},
		{Status: types.CodeChangesStatusError, Error: "container not ready"},
		{Status: types.CodeChangesStatusMissing},
		// Ready but incomplete: a half-written receipt must not be diffed.
		{Status: types.CodeChangesStatusReady, BeforeRef: "refs/helix/checkpoints/ses_1/int_1/before"},
		{Status: types.CodeChangesStatusReady, AfterRef: "refs/helix/checkpoints/ses_1/int_1/after"},
	} {
		s.store.EXPECT().GetSession(gomock.Any(), "ses_1").
			Return(&types.Session{ID: "ses_1", Owner: s.user.ID}, nil)
		s.store.EXPECT().GetInteraction(gomock.Any(), "int_1").
			Return(&types.Interaction{ID: "int_1", SessionID: "ses_1", CodeChanges: changes}, nil)

		s.Equal(http.StatusNotFound, s.turnRequest("ses_1", "int_1", "").Code,
			"receipt %+v must not produce a turn diff", changes)
	}
}

// The handler must take checkpoint refs only from the persisted receipt.
// Refs supplied on the query string are inert — a caller cannot point the
// desktop at an arbitrary ref by naming it in the URL. With no connection
// manager wired up the desktop call fails, which is exactly the point: the
// request got as far as dialing using the stored receipt, not the query.
func (s *WorkspaceReviewHandlersSuite) TestTurnReviewIgnoresClientSuppliedRefs() {
	s.store.EXPECT().GetSession(gomock.Any(), "ses_1").
		Return(&types.Session{ID: "ses_1", Owner: s.user.ID}, nil)
	s.store.EXPECT().GetInteraction(gomock.Any(), "int_1").Return(&types.Interaction{
		ID:        "int_1",
		SessionID: "ses_1",
		CodeChanges: &types.InteractionCodeChanges{
			Status:    types.CodeChangesStatusReady,
			Workspace: "primary",
			BeforeRef: "refs/helix/checkpoints/ses_1/int_1/before",
			AfterRef:  "refs/helix/checkpoints/ses_1/int_1/after",
		},
	}, nil)

	w := s.turnRequest("ses_1", "int_1",
		"before_ref=refs/heads/main&after_ref=refs/heads/secret&workspace=../../etc")

	// 503, not 200: no desktop is connected. The refs never reached a git
	// command, and no path exists by which the query string could supply them.
	s.Equal(http.StatusServiceUnavailable, w.Code)
	s.NotContains(w.Body.String(), "refs/heads/secret")
}

func (s *WorkspaceReviewHandlersSuite) TestWorkspaceProxiesRejectUnauthorizedCallers() {
	for name, handler := range map[string]func(http.ResponseWriter, *http.Request){
		"review": s.server.getWorkspaceReview,
		"files":  s.server.getWorkspaceFiles,
		"file":   s.server.getWorkspaceFile,
		"write":  s.server.putWorkspaceFile,
	} {
		s.store.EXPECT().GetSession(gomock.Any(), "ses_1").
			Return(&types.Session{ID: "ses_1", Owner: "usr_someone_else"}, nil)

		req := httptest.NewRequest(http.MethodGet, "/workspace?workspace=primary", nil)
		req = mux.SetURLVars(req, map[string]string{"sessionID": "ses_1"})
		req = req.WithContext(setRequestUser(req.Context(), s.user))
		w := httptest.NewRecorder()

		handler(w, req)

		s.Equal(http.StatusForbidden, w.Code, "%s must not serve a session the caller cannot read", name)
	}
}

func (s *WorkspaceReviewHandlersSuite) TestWorkspaceFileWriteAuthorizesBeforeReadingBody() {
	s.store.EXPECT().GetSession(gomock.Any(), "ses_1").
		Return(&types.Session{ID: "ses_1", Owner: s.user.ID}, nil)
	req := httptest.NewRequest(http.MethodPut, "/workspace-file", nil)
	req = mux.SetURLVars(req, map[string]string{"sessionID": "ses_1"})
	req = req.WithContext(setRequestUser(req.Context(), s.user))
	w := httptest.NewRecorder()

	s.server.putWorkspaceFile(w, req)

	s.Equal(http.StatusBadRequest, w.Code)
}
