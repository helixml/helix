package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/inferencerouter"
	"github.com/helixml/helix/api/pkg/netguard"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/tools"
	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// GET /sessions/{id} sits on the optional-auth router. An anonymous caller is
// refused before the lookup, a missing session is a 404, and a store failure
// is a 500 that does not carry the database error.
func TestGetSession_ErrorResponses(t *testing.T) {
	newReq := func(user *types.User) *http.Request {
		req := mux.SetURLVars(httptest.NewRequest(http.MethodGet, "/api/v1/sessions/ses_x", nil), map[string]string{"id": "ses_x"})
		if user != nil {
			req = req.WithContext(setRequestUser(req.Context(), *user))
		}
		return req
	}

	t.Run("anonymous", func(t *testing.T) {
		m := store.NewMockStore(gomock.NewController(t)) // no lookup expected
		rr := httptest.NewRecorder()
		(&HelixAPIServer{Store: m}).getSession(rr, newReq(nil))
		require.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("not found", func(t *testing.T) {
		m := store.NewMockStore(gomock.NewController(t))
		m.EXPECT().GetSession(gomock.Any(), "ses_x").Return(nil, store.ErrNotFound)
		rr := httptest.NewRecorder()
		(&HelixAPIServer{Store: m}).getSession(rr, newReq(&types.User{ID: "usr_a"}))
		require.Equal(t, http.StatusNotFound, rr.Code)
		require.Equal(t, "session not found\n", rr.Body.String())
	})

	t.Run("store failure", func(t *testing.T) {
		m := store.NewMockStore(gomock.NewController(t))
		m.EXPECT().GetSession(gomock.Any(), "ses_x").Return(nil, errors.New("pq: connection refused"))
		rr := httptest.NewRecorder()
		(&HelixAPIServer{Store: m}).getSession(rr, newReq(&types.User{ID: "usr_a"}))
		require.Equal(t, http.StatusInternalServerError, rr.Code)
		require.Equal(t, "failed to load session\n", rr.Body.String())
	})

	t.Run("another user's session", func(t *testing.T) {
		m := store.NewMockStore(gomock.NewController(t))
		m.EXPECT().GetSession(gomock.Any(), "ses_x").Return(&types.Session{ID: "ses_x", Owner: "usr_b"}, nil)
		rr := httptest.NewRecorder()
		(&HelixAPIServer{Store: m}).getSession(rr, newReq(&types.User{ID: "usr_a"}))
		require.Equal(t, http.StatusForbidden, rr.Code)
		require.Equal(t, "not authorized to access this session\n", rr.Body.String())
	})
}

// DELETE and PUT /sessions/{id} on a missing session answer 404, not 500.
func TestDeleteUpdateSession_NotFoundGets404(t *testing.T) {
	m := store.NewMockStore(gomock.NewController(t))
	m.EXPECT().GetSession(gomock.Any(), "ses_x").Return(nil, store.ErrNotFound).Times(2)
	srv := &HelixAPIServer{Store: m}

	for _, method := range []string{http.MethodDelete, http.MethodPut} {
		req := mux.SetURLVars(httptest.NewRequest(method, "/api/v1/sessions/ses_x", strings.NewReader(`{}`)), map[string]string{"id": "ses_x"})
		req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "usr_a"}))
		rr := httptest.NewRecorder()
		if method == http.MethodDelete {
			system.Wrapper(srv.deleteSession)(rr, req)
		} else {
			system.Wrapper(srv.updateSession)(rr, req)
		}
		require.Equal(t, http.StatusNotFound, rr.Code, method)
		require.Equal(t, "session not found\n", rr.Body.String(), method)
	}
}

// POST /v1/chat/completions validates the messages before any provider or
// model resolution: the mock store fails the test if a lookup happens.
func TestCreateChatCompletion_ValidatesBodyBeforeModelResolution(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"no messages", `{"HXG":"x"}`, "messages is required and must contain at least one message\n"},
		{"empty messages", `{"model":"m","messages":[]}`, "messages is required and must contain at least one message\n"},
		{"unknown role", `{"model":"m","messages":[{"role":"root","content":"hi"}]}`, "messages[0].role must be one of system, developer, user, assistant, tool or function\n"},
		{"wrong type", `{"model":"m","messages":"hi"}`, "Invalid request body: field \"messages\" has the wrong type\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := store.NewMockStore(gomock.NewController(t))
			srv := &HelixAPIServer{Store: m, Cfg: &config.ServerConfig{}}
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(tc.body))
			req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "usr_a", OrganizationID: "org_a"}))
			rr := httptest.NewRecorder()

			srv.createChatCompletion(rr, req)

			require.Equal(t, http.StatusBadRequest, rr.Code)
			require.Equal(t, tc.want, rr.Body.String())
		})
	}
}

// Model resolution failures are the caller's to fix: a 400 that names only
// the requested model, never the default provider or the default model.
func TestWriteChatCompletionError(t *testing.T) {
	notConfigured := fmt.Errorf("failed to get client: %w", &controller.ModelNotConfiguredError{Model: "llama3:instruct", Provider: "helix"})

	rr := httptest.NewRecorder()
	writeChatCompletionError(rr, notConfigured, "", "")
	require.Equal(t, http.StatusBadRequest, rr.Code)
	require.NotContains(t, rr.Body.String(), "llama3")
	require.NotContains(t, rr.Body.String(), "helix")

	rr = httptest.NewRecorder()
	writeChatCompletionError(rr, notConfigured, "my-model", "")
	require.Equal(t, http.StatusBadRequest, rr.Code)
	require.Contains(t, rr.Body.String(), `model "my-model" is not available`)
	require.NotContains(t, rr.Body.String(), "default provider")

	rr = httptest.NewRecorder()
	writeChatCompletionError(rr, errors.New("upstream exploded"), "my-model", "")
	require.Equal(t, http.StatusInternalServerError, rr.Code)

	// A caller that named an agent is not told to supply an app_id: it is the
	// agent's configured model that is unavailable.
	rr = httptest.NewRecorder()
	writeChatCompletionError(rr, notConfigured, "", "app_1")
	require.Equal(t, http.StatusBadRequest, rr.Code)
	require.Contains(t, rr.Body.String(), "agent's configured model is not available")
	require.NotContains(t, rr.Body.String(), "supply an app_id")
	require.NotContains(t, rr.Body.String(), "llama3")
}

func TestIsModelResolutionError(t *testing.T) {
	resolution := []error{
		fmt.Errorf("failed to get response from inference API: %w", fmt.Errorf("error enqueuing request: %w", oai.ErrMissingModel)),
		fmt.Errorf("error enqueuing request: %w", &inferencerouter.NoRunnerError{RequestedModel: "qwen3:8b"}),
		fmt.Errorf("failed to get client: %w", &controller.ModelNotConfiguredError{Model: "m", Provider: "helix"}),
		fmt.Errorf("wrapped: %w", &openai.APIError{HTTPStatusCode: http.StatusNotFound, Message: "model not found for API profile"}),
	}
	for _, err := range resolution {
		require.True(t, isModelResolutionError(err), err.Error())
	}
	other := []error{
		errors.New("boom"),
		fmt.Errorf("wrapped: %w", &openai.APIError{HTTPStatusCode: http.StatusInternalServerError}),
	}
	for _, err := range other {
		require.False(t, isModelResolutionError(err), err.Error())
	}
}

func triggerExecuteRequest(user types.User) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/triggers/trg_1/execute", nil)
	req = mux.SetURLVars(req, map[string]string{"trigger_id": "trg_1"})
	return req.WithContext(setRequestUser(req.Context(), user))
}

// POST /triggers/{id}/execute refuses a disabled or archived trigger with 409
// before anything runs; the mock store fails the test on any further call
// (reserving an execution, creating a session).
func TestExecuteAppTrigger_DisabledTriggerGets409(t *testing.T) {
	owner := types.User{ID: "usr_owner"}
	cases := map[string]*types.TriggerConfiguration{
		"disabled": {ID: "trg_1", AppID: "app_1", Owner: owner.ID, Enabled: false,
			Trigger: types.Trigger{Cron: &types.CronTrigger{Enabled: true, Schedule: "* * * * *"}}},
		"archived": {ID: "trg_1", AppID: "app_1", Owner: owner.ID, Enabled: true, Archived: true,
			Trigger: types.Trigger{Cron: &types.CronTrigger{Enabled: true, Schedule: "* * * * *"}}},
	}
	for name, trigger := range cases {
		t.Run(name, func(t *testing.T) {
			m := store.NewMockStore(gomock.NewController(t))
			m.EXPECT().GetTriggerConfiguration(gomock.Any(), &store.GetTriggerConfigurationQuery{ID: "trg_1", Owner: owner.ID}).Return(trigger, nil)
			m.EXPECT().GetAppWithTools(gomock.Any(), "app_1").Return(&types.App{ID: "app_1", Owner: owner.ID}, nil)
			srv := &HelixAPIServer{Store: m}

			rr := httptest.NewRecorder()
			system.Wrapper(srv.executeAppTrigger)(rr, triggerExecuteRequest(owner))

			require.Equal(t, http.StatusConflict, rr.Code)
			require.Equal(t, "trigger is disabled; enable it before running it\n", rr.Body.String())
		})
	}
}

// A trigger whose agent is gone is a 404, not a 500 with the store error.
func TestExecuteAppTrigger_MissingAgentGets404(t *testing.T) {
	owner := types.User{ID: "usr_owner"}
	m := store.NewMockStore(gomock.NewController(t))
	m.EXPECT().GetTriggerConfiguration(gomock.Any(), gomock.Any()).Return(&types.TriggerConfiguration{ID: "trg_1", AppID: "app_1", Owner: owner.ID, Enabled: true}, nil)
	m.EXPECT().GetAppWithTools(gomock.Any(), "app_1").Return(nil, store.ErrNotFound)

	rr := httptest.NewRecorder()
	system.Wrapper((&HelixAPIServer{Store: m}).executeAppTrigger)(rr, triggerExecuteRequest(owner))

	require.Equal(t, http.StatusNotFound, rr.Code)
	require.Equal(t, "agent not found\n", rr.Body.String())
}

// Invoking an API action whose URL the outbound policy blocks is a clean 400.
func TestAppRunAPIAction_BlockedDestinationGets400(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := store.NewMockStore(ctrl)
	planner := tools.NewMockPlanner(ctrl)
	owner := types.User{ID: "usr_owner"}
	tool := &types.Tool{Name: "internal", ToolType: types.ToolTypeAPI, Config: types.ToolConfig{API: &types.ToolAPIConfig{
		URL:     "http://169.254.169.254",
		Actions: []*types.ToolAPIAction{{Name: "meta", Method: "GET", Path: "/latest"}},
	}}}
	m.EXPECT().GetAppWithTools(gomock.Any(), "app_1").Return(&types.App{ID: "app_1", Owner: owner.ID, Config: types.AppConfig{
		Helix: types.AppHelixConfig{Assistants: []types.AssistantConfig{{Tools: []*types.Tool{tool}}}},
	}}, nil)
	m.EXPECT().ListOAuthConnections(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	planner.EXPECT().RunAPIActionWithParameters(gomock.Any(), gomock.Any()).
		Return(nil, fmt.Errorf("failed to make api call: %w", fmt.Errorf("GET giving up: %w", fmt.Errorf("%w: reserved or link-local address", netguard.ErrBlockedDestination))))

	srv := &HelixAPIServer{Store: m, Controller: &controller.Controller{ToolsPlanner: planner}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/app_1/api-actions", strings.NewReader(`{"action":"meta"}`))
	req = mux.SetURLVars(req, map[string]string{"id": "app_1"})
	req = req.WithContext(setRequestUser(req.Context(), owner))

	rr := httptest.NewRecorder()
	system.Wrapper(srv.appRunAPIAction)(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	require.NotContains(t, rr.Body.String(), "169.254")
}

// GET /sessions?org_id=… answers an unknown org and an org the caller is not
// in with the same 404, without echoing the requested reference.
func TestListSessions_ForeignOrgGets404(t *testing.T) {
	cases := map[string]func(m *store.MockStore){
		"unknown org": func(m *store.MockStore) {
			m.EXPECT().GetOrganization(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound)
		},
		"not a member": func(m *store.MockStore) {
			m.EXPECT().GetOrganization(gomock.Any(), gomock.Any()).Return(&types.Organization{ID: "org_victim"}, nil)
			m.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound)
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			m := store.NewMockStore(gomock.NewController(t))
			setup(m)
			srv := &HelixAPIServer{Store: m, Cfg: &config.ServerConfig{}}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions?org_id=probe-<b>ref</b>", nil)
			req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "usr_a"}))
			rr := httptest.NewRecorder()
			system.DefaultWrapper(srv.listSessions)(rr, req)
			require.Equal(t, http.StatusNotFound, rr.Code)
			require.Equal(t, "organization not found\n", rr.Body.String())
		})
	}
}
