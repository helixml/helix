package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

const (
	zedTestTask     = "spt_a"
	zedTestInstance = "zed_instance_spt_a_1"
	zedTestThread   = "stzt_a"
	zedTestExternal = "zed-thread-a"
)

// Callers of the Zed ingestion routes: the task's creator, the in-sandbox
// agent holding the session key minted for the task's project, someone else,
// and the creator's own key confined to a different project.
var (
	zedOwner      = types.User{ID: "usr_owner"}
	zedSandboxKey = types.User{ID: "usr_owner", ProjectID: "prj_a", SessionID: "ses_a", TokenType: types.TokenTypeAPIKey}
	zedStranger   = types.User{ID: "usr_other"}
	zedOtherScope = types.User{ID: "usr_owner", ProjectID: "prj_b", TokenType: types.TokenTypeAPIKey}
)

// newZedAuthzTestServer serves one spec task with one Zed thread. Thread
// writes are counted; gomock fails the test on any store call not listed
// here, so a refused request cannot reach a write unnoticed.
func newZedAuthzTestServer(t *testing.T) (*HelixAPIServer, *int) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)

	task := &types.SpecTask{ID: zedTestTask, ProjectID: "prj_a", CreatedBy: "usr_owner", ZedInstanceID: zedTestInstance}
	thread := &types.SpecTaskZedThread{ID: zedTestThread, SpecTaskID: zedTestTask, ZedThreadID: zedTestExternal, Status: types.SpecTaskZedStatusActive}

	st.EXPECT().GetSpecTask(gomock.Any(), zedTestTask).Return(task, nil).AnyTimes()
	st.EXPECT().GetSpecTask(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound).AnyTimes()
	st.EXPECT().GetProject(gomock.Any(), "prj_a").Return(&types.Project{ID: "prj_a", UserID: "usr_owner"}, nil).AnyTimes()
	st.EXPECT().GetSpecTaskZedThread(gomock.Any(), zedTestThread).DoAndReturn(
		func(context.Context, string) (*types.SpecTaskZedThread, error) {
			copied := *thread
			return &copied, nil
		}).AnyTimes()
	st.EXPECT().GetSpecTaskZedThreadByZedThreadID(gomock.Any(), zedTestExternal).Return(thread, nil).AnyTimes()
	st.EXPECT().ListSpecTaskZedThreads(gomock.Any(), zedTestTask).Return([]*types.SpecTaskZedThread{thread}, nil).AnyTimes()

	writes := 0
	st.EXPECT().UpdateSpecTaskZedThread(gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, *types.SpecTaskZedThread) error {
			writes++
			return nil
		}).AnyTimes()

	srv := &HelixAPIServer{
		Store: st,
		Cfg:   &config.ServerConfig{},
		specDrivenTaskService: &services.SpecDrivenTaskService{
			ZedIntegrationService: services.NewZedIntegrationService(st, pubsub.NewNoop()),
		},
	}
	return srv, &writes
}

func zedRequest(user types.User, path, body string, vars map[string]string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req = mux.SetURLVars(req, vars)
	return req.WithContext(setRequestUser(req.Context(), user))
}

func TestZedInstanceEvent_BindsSpecTaskToCaller(t *testing.T) {
	body := `{"instance_id":"` + zedTestInstance + `","event_type":"instance_error","spec_task_id":"` + zedTestTask + `"}`
	for _, tc := range []struct {
		name string
		user types.User
		want int
	}{
		{"owner", zedOwner, http.StatusOK},
		{"sandbox session key", zedSandboxKey, http.StatusOK},
		{"other user", zedStranger, http.StatusNotFound},
		{"key scoped to another project", zedOtherScope, http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, writes := newZedAuthzTestServer(t)
			rec := httptest.NewRecorder()
			srv.handleZedInstanceEvent(rec, zedRequest(tc.user, "/api/v1/zed/events", body, nil))
			require.Equal(t, tc.want, rec.Code, rec.Body.String())
			if tc.want == http.StatusOK {
				require.Equal(t, 1, *writes)
			} else {
				require.Zero(t, *writes, "a refused event must not touch the task's threads")
			}
		})
	}
}

func TestZedInstanceEvent_RequiresSpecTaskAndMatchingInstance(t *testing.T) {
	srv, writes := newZedAuthzTestServer(t)

	rec := httptest.NewRecorder()
	srv.handleZedInstanceEvent(rec, zedRequest(zedOwner, "/api/v1/zed/events",
		`{"instance_id":"`+zedTestInstance+`","event_type":"instance_error"}`, nil))
	require.Equal(t, http.StatusBadRequest, rec.Code)

	rec = httptest.NewRecorder()
	srv.handleZedInstanceEvent(rec, zedRequest(zedOwner, "/api/v1/zed/events",
		`{"instance_id":"zed_instance_other","event_type":"instance_error","spec_task_id":"`+zedTestTask+`"}`, nil))
	require.Equal(t, http.StatusConflict, rec.Code)

	rec = httptest.NewRecorder()
	srv.handleZedInstanceEvent(rec, zedRequest(zedOwner, "/api/v1/zed/events",
		`{"instance_id":"`+zedTestInstance+`","event_type":"instance_error","spec_task_id":"spt_missing"}`, nil))
	require.Equal(t, http.StatusNotFound, rec.Code)

	require.Zero(t, *writes)
}

func TestZedThreadEvent_BindsSpecTaskToCaller(t *testing.T) {
	vars := map[string]string{"instanceId": zedTestInstance, "threadId": zedTestExternal}
	path := "/api/v1/zed/instances/" + zedTestInstance + "/threads/" + zedTestExternal + "/events"
	for _, tc := range []struct {
		name string
		user types.User
		body string
		want int
	}{
		{"owner, task resolved from thread", zedOwner, `{"status":"completed"}`, http.StatusOK},
		{"sandbox session key", zedSandboxKey, `{"status":"completed","spec_task_id":"` + zedTestTask + `"}`, http.StatusOK},
		{"other user, task resolved from thread", zedStranger, `{"status":"failed"}`, http.StatusNotFound},
		{"other user, task named in body", zedStranger, `{"status":"failed","spec_task_id":"` + zedTestTask + `"}`, http.StatusNotFound},
		{"key scoped to another project", zedOtherScope, `{"status":"failed"}`, http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, writes := newZedAuthzTestServer(t)
			rec := httptest.NewRecorder()
			srv.handleZedThreadEvent(rec, zedRequest(tc.user, path, tc.body, vars))
			require.Equal(t, tc.want, rec.Code, rec.Body.String())
			if tc.want == http.StatusOK {
				require.Equal(t, 1, *writes)
			} else {
				require.Zero(t, *writes)
			}
		})
	}
}

func TestZedThreadActivity_BindsThreadToCaller(t *testing.T) {
	vars := map[string]string{"threadId": zedTestThread}
	path := "/api/v1/zed/threads/" + zedTestThread + "/activity"
	for _, tc := range []struct {
		name string
		user types.User
		want int
	}{
		{"owner", zedOwner, http.StatusOK},
		{"sandbox session key", zedSandboxKey, http.StatusOK},
		{"other user", zedStranger, http.StatusNotFound},
		{"key scoped to another project", zedOtherScope, http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, writes := newZedAuthzTestServer(t)
			rec := httptest.NewRecorder()
			srv.updateZedThreadActivity(rec, zedRequest(tc.user, path, `{"status":"failed","note":"x"}`, vars))
			require.Equal(t, tc.want, rec.Code, rec.Body.String())
			if tc.want == http.StatusOK {
				require.Equal(t, 1, *writes)
				var got types.SpecTaskZedThread
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
				require.Equal(t, zedTestThread, got.ID)
			} else {
				require.Zero(t, *writes)
				require.NotContains(t, rec.Body.String(), zedTestTask, "a refused caller must not see the thread row")
			}
		})
	}
}

func TestZedHeartbeat_BindsSpecTaskToCaller(t *testing.T) {
	vars := map[string]string{"instanceId": zedTestInstance}
	path := "/api/v1/zed/instances/" + zedTestInstance + "/heartbeat"
	body := `{"spec_task_id":"` + zedTestTask + `"}`
	for _, tc := range []struct {
		name string
		user types.User
		want int
	}{
		{"owner", zedOwner, http.StatusOK},
		{"sandbox session key", zedSandboxKey, http.StatusOK},
		{"other user", zedStranger, http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, writes := newZedAuthzTestServer(t)
			rec := httptest.NewRecorder()
			srv.handleZedConnectionHeartbeat(rec, zedRequest(tc.user, path, body, vars))
			require.Equal(t, tc.want, rec.Code, rec.Body.String())
			require.Zero(t, *writes, "heartbeats store nothing")
		})
	}
}
