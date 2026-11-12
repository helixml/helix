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

	zedTestWorkSession = "stws_a"
	zedForeignTask     = "spt_b"
	zedForeignExternal = "zed-thread-b"
	zedNoInstanceTask  = "spt_c"
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

	writes := 0

	// Another user's task and thread, and a task of the owner's with no Zed
	// instance yet.
	foreignTask := &types.SpecTask{ID: zedForeignTask, ProjectID: "prj_b", CreatedBy: "usr_other", ZedInstanceID: "zed_instance_spt_b_1"}
	foreignThread := &types.SpecTaskZedThread{ID: "stzt_b", SpecTaskID: zedForeignTask, ZedThreadID: zedForeignExternal, Status: types.SpecTaskZedStatusActive}
	noInstanceTask := &types.SpecTask{ID: zedNoInstanceTask, ProjectID: "prj_a", CreatedBy: "usr_owner"}

	st.EXPECT().GetSpecTask(gomock.Any(), zedTestTask).Return(task, nil).AnyTimes()
	st.EXPECT().GetSpecTask(gomock.Any(), zedForeignTask).Return(foreignTask, nil).AnyTimes()
	st.EXPECT().GetSpecTask(gomock.Any(), zedNoInstanceTask).Return(noInstanceTask, nil).AnyTimes()
	st.EXPECT().GetSpecTask(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound).AnyTimes()
	st.EXPECT().GetProject(gomock.Any(), "prj_a").Return(&types.Project{ID: "prj_a", UserID: "usr_owner"}, nil).AnyTimes()
	st.EXPECT().GetProject(gomock.Any(), "prj_b").Return(&types.Project{ID: "prj_b", UserID: "usr_other"}, nil).AnyTimes()
	st.EXPECT().GetSpecTaskZedThreadByZedThreadID(gomock.Any(), zedForeignExternal).Return(foreignThread, nil).AnyTimes()
	st.EXPECT().ListSpecTaskZedThreads(gomock.Any(), zedNoInstanceTask).Return(nil, nil).AnyTimes()
	st.EXPECT().GetSpecTaskWorkSession(gomock.Any(), zedTestWorkSession).Return(&types.SpecTaskWorkSession{ID: zedTestWorkSession, SpecTaskID: zedTestTask}, nil).AnyTimes()
	st.EXPECT().CreateSpecTaskZedThread(gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, *types.SpecTaskZedThread) error {
			writes++
			return nil
		}).AnyTimes()
	st.EXPECT().GetSpecTaskZedThread(gomock.Any(), zedTestThread).DoAndReturn(
		func(context.Context, string) (*types.SpecTaskZedThread, error) {
			copied := *thread
			return &copied, nil
		}).AnyTimes()
	st.EXPECT().GetSpecTaskZedThread(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound).AnyTimes()
	st.EXPECT().GetSpecTaskZedThreadByZedThreadID(gomock.Any(), zedTestExternal).Return(thread, nil).AnyTimes()
	st.EXPECT().GetSpecTaskZedThreadByZedThreadID(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound).AnyTimes()
	st.EXPECT().ListSpecTaskZedThreads(gomock.Any(), zedTestTask).DoAndReturn(
		func(context.Context, string) ([]*types.SpecTaskZedThread, error) {
			copied := *thread
			return []*types.SpecTaskZedThread{&copied}, nil
		}).AnyTimes()

	st.EXPECT().UpdateSpecTaskZedThread(gomock.Any(), gomock.Any()).DoAndReturn(
		func(context.Context, *types.SpecTaskZedThread) error {
			writes++
			return nil
		}).AnyTimes()

	zed := services.NewZedIntegrationService(st, pubsub.NewNoop())
	zed.SetTestMode(true) // no instance launch/shutdown messages
	srv := &HelixAPIServer{
		Store:                 st,
		Cfg:                   &config.ServerConfig{},
		specDrivenTaskService: &services.SpecDrivenTaskService{ZedIntegrationService: zed},
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

// A caller naming its own task must not reach a thread of another task, and
// is told no more than for a missing thread.
func TestZedThreadEvent_OwnTaskWithForeignThreadIsRefused(t *testing.T) {
	srv, writes := newZedAuthzTestServer(t)
	vars := map[string]string{"instanceId": zedTestInstance, "threadId": zedForeignExternal}
	rec := httptest.NewRecorder()
	srv.handleZedThreadEvent(rec, zedRequest(zedOwner, "/api/v1/zed/instances/x/threads/y/events",
		`{"status":"failed","spec_task_id":"`+zedTestTask+`"}`, vars))
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	require.Equal(t, errZedSpecTaskNotFound+"\n", rec.Body.String())
	require.Zero(t, *writes)
}

// Missing threads and unreachable tasks give the same answer, so the routes
// do not reveal which ids exist.
func TestZedIngestion_NotFoundAnswersAreUniform(t *testing.T) {
	srv, writes := newZedAuthzTestServer(t)

	unknownThread := httptest.NewRecorder()
	srv.handleZedThreadEvent(unknownThread, zedRequest(zedOwner, "/api/v1/zed/instances/x/threads/y/events",
		`{"status":"failed"}`, map[string]string{"instanceId": zedTestInstance, "threadId": "zed-thread-missing"}))

	foreignThread := httptest.NewRecorder()
	srv.handleZedThreadEvent(foreignThread, zedRequest(zedOwner, "/api/v1/zed/instances/x/threads/y/events",
		`{"status":"failed"}`, map[string]string{"instanceId": zedTestInstance, "threadId": zedForeignExternal}))

	missingActivity := httptest.NewRecorder()
	srv.updateZedThreadActivity(missingActivity, zedRequest(zedOwner, "/api/v1/zed/threads/x/activity",
		`{"status":"failed"}`, map[string]string{"threadId": "stzt_missing"}))

	foreignActivity := httptest.NewRecorder()
	srv.updateZedThreadActivity(foreignActivity, zedRequest(zedStranger, "/api/v1/zed/threads/x/activity",
		`{"status":"failed"}`, map[string]string{"threadId": zedTestThread}))

	for name, rec := range map[string]*httptest.ResponseRecorder{
		"unknown thread": unknownThread, "foreign thread": foreignThread,
		"missing activity thread": missingActivity, "foreign activity thread": foreignActivity,
	} {
		require.Equal(t, http.StatusNotFound, rec.Code, name)
		require.Equal(t, errZedSpecTaskNotFound+"\n", rec.Body.String(), name)
	}
	require.Zero(t, *writes)
}

// Before a task has a Zed instance, only stateless announcements are accepted.
func TestZedInstanceEvent_TaskWithoutInstance(t *testing.T) {
	for _, tc := range []struct {
		eventType string
		want      int
	}{
		{"instance_ready", http.StatusOK},
		{"heartbeat", http.StatusOK},
		{"instance_error", http.StatusConflict},
		{"instance_disconnected", http.StatusConflict},
		{"thread_status_changed", http.StatusConflict},
		{"thread_created", http.StatusConflict},
	} {
		t.Run(tc.eventType, func(t *testing.T) {
			srv, writes := newZedAuthzTestServer(t)
			body := `{"instance_id":"zed_instance_any","event_type":"` + tc.eventType + `","spec_task_id":"` + zedNoInstanceTask + `","thread_id":"t"}`
			rec := httptest.NewRecorder()
			srv.handleZedInstanceEvent(rec, zedRequest(zedOwner, "/api/v1/zed/events", body, nil))
			require.Equal(t, tc.want, rec.Code, rec.Body.String())
			require.Zero(t, *writes)
		})
	}
}

// The spec-task Zed routes check access through the task's project: read for
// status, update for shutdown and thread creation.
func TestZedSpecTaskRoutes_AuthorizeThroughProject(t *testing.T) {
	cases := []struct {
		name string
		user types.User
		ok   bool
	}{
		{"owner", zedOwner, true},
		{"sandbox session key", zedSandboxKey, true},
		{"other user", zedStranger, false},
		{"key scoped to another project", zedOtherScope, false},
	}
	routes := []struct {
		name   string
		okCode int
		writes int
		call   func(*HelixAPIServer, http.ResponseWriter, *http.Request)
		req    func(types.User) *http.Request
	}{
		{"status", http.StatusOK, 0, (*HelixAPIServer).getZedInstanceStatus, func(u types.User) *http.Request {
			return zedRequest(u, "/api/v1/spec-tasks/spt_a/zed-instance", "", map[string]string{"taskId": zedTestTask})
		}},
		{"shutdown", http.StatusOK, 1, (*HelixAPIServer).shutdownZedInstance, func(u types.User) *http.Request {
			return zedRequest(u, "/api/v1/spec-tasks/spt_a/zed-instance", "", map[string]string{"taskId": zedTestTask})
		}},
		{"create thread", http.StatusCreated, 1, (*HelixAPIServer).createZedThreadForWorkSession, func(u types.User) *http.Request {
			return zedRequest(u, "/api/v1/work-sessions/stws_a/zed-thread", `{}`, map[string]string{"sessionId": zedTestWorkSession})
		}},
	}
	for _, route := range routes {
		for _, tc := range cases {
			t.Run(route.name+"/"+tc.name, func(t *testing.T) {
				srv, writes := newZedAuthzTestServer(t)
				rec := httptest.NewRecorder()
				route.call(srv, rec, route.req(tc.user))
				if tc.ok {
					require.Equal(t, route.okCode, rec.Code, rec.Body.String())
					require.Equal(t, route.writes, *writes)
				} else {
					require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
					require.Zero(t, *writes)
				}
			})
		}
	}
}
