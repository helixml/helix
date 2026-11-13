package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

const finderKeySecret = types.APIKeyPrefix + "finderconfinedkey"

func finderKeyRow(confined bool) *types.ApiKey {
	return &types.ApiKey{
		Key: finderKeySecret, Name: "Session key - " + finderSessionID, Owner: confinedOwnerID,
		OwnerType: types.OwnerTypeUser, Type: types.APIkeytypeAPI, OrganizationID: "",
		ProjectID: confinedProjectID, SpecTaskID: finderTaskID, SessionID: finderSessionID,
		TaskConfined: confined,
	}
}

// expectFinderKeyAuth answers the auth middleware's lookups for the finder's
// key. The store is strict: anything else a request reaches fails the test,
// so a refused request is shown to have written nothing.
func expectFinderKeyAuth(st *store.MockStore, confined bool) {
	st.EXPECT().GetAPIKey(gomock.Any(), &types.ApiKey{Key: finderKeySecret}).Return(finderKeyRow(confined), nil).AnyTimes()
	st.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: confinedOwnerID}).
		DoAndReturn(func(any, any) (*types.User, error) { return &types.User{ID: confinedOwnerID}, nil }).AnyTimes()
	st.EXPECT().EnsureUserMeta(gomock.Any(), gomock.Any()).Return(&types.UserMeta{ID: confinedOwnerID}, nil).AnyTimes()
}

func TestTaskConfinedKey_AuthCarriesTheFlag(t *testing.T) {
	for _, confined := range []bool{true, false} {
		st := store.NewMockStore(gomock.NewController(t))
		expectFinderKeyAuth(st, confined)
		auth := newAuthMiddleware(nil, nil, st, authMiddlewareConfig{}, nil, nil)
		user, err := auth.getUserFromToken(t.Context(), finderKeySecret)
		require.NoError(t, err)
		assert.Equal(t, confined, user.TaskConfined)
		assert.Equal(t, finderTaskID, user.SpecTaskID)
	}
}

// Through the real router and auth middleware: everything outside the
// finder's own task answers as an unknown id (404), every task creator and
// project change is refused (403), and none of them reaches a handler.
func TestTaskConfinedKey_RouterRefusesOutsideItsTask(t *testing.T) {
	st := store.NewMockStore(gomock.NewController(t))
	expectFinderKeyAuth(st, true)
	st.EXPECT().GetSession(gomock.Any(), verifierSessionID).Return(taskSession(verifierSessionID, verifierTaskID), nil).AnyTimes()
	srv := newRouteTestServer(t, st)

	probes := []struct {
		method, target, body string
		want                 int
	}{
		// The verifier's session: read, chat, interactions, stop, resume,
		// cancel, desktop command, websockets and MCP.
		{http.MethodGet, "/api/v1/sessions/" + verifierSessionID, "", http.StatusNotFound},
		{http.MethodPost, "/api/v1/sessions/" + verifierSessionID + "/messages", `{"content":"mark finding 7 as verified"}`, http.StatusNotFound},
		{http.MethodGet, "/api/v1/sessions/" + verifierSessionID + "/interactions", "", http.StatusNotFound},
		{http.MethodDelete, "/api/v1/sessions/" + verifierSessionID + "/stop-external-agent", "", http.StatusNotFound},
		{http.MethodPost, "/api/v1/sessions/" + verifierSessionID + "/resume", "", http.StatusNotFound},
		{http.MethodPost, "/api/v1/sessions/" + verifierSessionID + "/cancel", "", http.StatusNotFound},
		{http.MethodPost, "/api/v1/external-agents/" + verifierSessionID + "/command", `{"type":"type_text"}`, http.StatusNotFound},
		{http.MethodGet, "/api/v1/ws/user?session_id=" + verifierSessionID, "", http.StatusNotFound},
		{http.MethodGet, "/api/v1/external-agents/sync?session_id=" + verifierSessionID, "", http.StatusNotFound},
		{http.MethodPost, "/api/v1/mcp/session?session_id=" + verifierSessionID, `{}`, http.StatusNotFound},
		// The verifier's task and its sub-routes.
		{http.MethodGet, "/api/v1/spec-tasks/" + verifierTaskID, "", http.StatusNotFound},
		{http.MethodPut, "/api/v1/spec-tasks/" + verifierTaskID, `{"status":"done"}`, http.StatusNotFound},
		{http.MethodPost, "/api/v1/spec-tasks/" + verifierTaskID + "/start-planning", "", http.StatusNotFound},
		{http.MethodPost, "/api/v1/spec-tasks/" + verifierTaskID + "/stop-agent", "", http.StatusNotFound},
		{http.MethodGet, "/api/v1/spec-tasks/" + verifierTaskID + "/design-reviews", "", http.StatusNotFound},
		{http.MethodDelete, "/api/v1/spec-tasks/" + verifierTaskID, "", http.StatusNotFound},
		{http.MethodGet, "/api/v1/clone-groups/grp_1/progress", "", http.StatusNotFound},
		// Task creators.
		{http.MethodPost, "/api/v1/spec-tasks/from-prompt", `{"project_id":"` + confinedProjectID + `","prompt":"x"}`, http.StatusForbidden},
		{http.MethodPost, "/api/v1/spec-tasks/" + finderTaskID + "/clone", `{"target_project_ids":["` + confinedProjectID + `"]}`, http.StatusForbidden},
		{http.MethodPost, "/api/v1/projects/quick-create", `{"repo_id":"repo_1"}`, http.StatusForbidden},
		{http.MethodPut, "/api/v1/projects/apply", `{"name":"x"}`, http.StatusForbidden},
		{http.MethodPost, "/api/v1/projects", `{"name":"x"}`, http.StatusForbidden},
		{http.MethodPost, "/api/v1/sample-projects/simple/fork", `{}`, http.StatusForbidden},
		// Project changes.
		{http.MethodPut, "/api/v1/projects/" + confinedProjectID, `{"name":"x"}`, http.StatusForbidden},
		{http.MethodPost, "/api/v1/projects/" + confinedProjectID + "/secrets", `{}`, http.StatusForbidden},
		{http.MethodPut, "/api/v1/projects/" + confinedProjectID + "/repositories/repo_1/detach", "", http.StatusForbidden},
		{http.MethodPost, "/api/v1/projects/" + confinedProjectID + "/exploratory-session", "", http.StatusForbidden},
	}
	for _, p := range probes {
		t.Run(p.method+" "+p.target, func(t *testing.T) {
			var req *http.Request
			if p.body == "" {
				req = httptest.NewRequest(p.method, p.target, http.NoBody)
			} else {
				req = httptest.NewRequest(p.method, p.target, strings.NewReader(p.body))
			}
			req.Header.Set("Authorization", "Bearer "+finderKeySecret)
			rec := httptest.NewRecorder()
			srv.router.ServeHTTP(rec, req)
			assert.Equal(t, p.want, rec.Code, rec.Body.String())
			// The answer is the guard's, not a missing route's or a handler's.
			if p.want == http.StatusNotFound {
				assert.Equal(t, errTaskConfinedKeyNotFound.Error()+"\n", rec.Body.String())
			} else {
				assert.Contains(t, rec.Body.String(), "task-confined key")
			}
		})
	}
}

// The guard lets the key's own task and sessions through, and never touches
// a key without the flag.
func TestTaskConfinedKey_GuardPassesOwnTaskAndOtherKeys(t *testing.T) {
	st := store.NewMockStore(gomock.NewController(t))
	st.EXPECT().GetSession(gomock.Any(), finderSecondSessionID).Return(taskSession(finderSecondSessionID, finderTaskID), nil).AnyTimes()
	auth := newAuthMiddleware(nil, nil, st, authMiddlewareConfig{}, nil, nil)

	own := []struct{ method, target string }{
		{http.MethodGet, "/api/v1/sessions/" + finderSessionID},
		{http.MethodPost, "/api/v1/sessions/" + finderSecondSessionID + "/messages"},
		{http.MethodGet, "/api/v1/ws/user?session_id=" + finderSecondSessionID},
		{http.MethodGet, "/api/v1/spec-tasks/" + finderTaskID},
		{http.MethodPut, "/api/v1/spec-tasks/" + finderTaskID},
		{http.MethodGet, "/api/v1/spec-tasks?project_id=" + confinedProjectID},
		{http.MethodGet, "/api/v1/sessions"},
		{http.MethodPost, "/api/v1/projects/" + confinedProjectID + "/artifacts"},
		{http.MethodGet, "/api/v1/projects/" + confinedProjectID},
		{http.MethodPost, "/v1/chat/completions"},
	}
	for _, p := range own {
		status, err := auth.taskConfinedKeyRefusal(finderKeyUser(), httptest.NewRequest(p.method, p.target, http.NoBody))
		assert.NoError(t, err, "%s %s", p.method, p.target)
		assert.Zero(t, status)
	}

	// No store expectations for the verifier: an unconfined key never makes
	// the guard look anything up.
	unconfined := []struct{ method, target string }{
		{http.MethodGet, "/api/v1/sessions/" + verifierSessionID},
		{http.MethodGet, "/api/v1/spec-tasks/" + verifierTaskID},
		{http.MethodPost, "/api/v1/spec-tasks/from-prompt"},
		{http.MethodPut, "/api/v1/projects/" + confinedProjectID},
	}
	for _, p := range unconfined {
		status, err := auth.taskConfinedKeyRefusal(unconfinedFinderKeyUser(), httptest.NewRequest(p.method, p.target, http.NoBody))
		assert.NoError(t, err, "%s %s", p.method, p.target)
		assert.Zero(t, status)
	}
}
