package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/connman"
	"github.com/helixml/helix/api/pkg/controller"
	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// The command route writes the body verbatim to a session's agent and can
// start its container. A user who cannot change the session must be refused
// before any of that happens.
func TestSendCommandToExternalAgent_RequiresSessionAccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	server := &HelixAPIServer{
		Store:                 st,
		externalAgentExecutor: external_agent.NewMockExecutor(ctrl),
		connman:               connman.New(),
		Cfg:                   &config.ServerConfig{},
		Controller: &controller.Controller{
			Options: controller.Options{Store: st, PubSub: pubsub.NewNoop()},
		},
	}

	st.EXPECT().GetSession(gomock.Any(), "ses_victim").Return(&types.Session{
		ID:       "ses_victim",
		Owner:    "usr_owner",
		Metadata: types.SessionMetadata{AgentType: "zed_external"},
	}, nil).AnyTimes()

	body := `{"type":"chat_message","data":{"message":"hello"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/external-agents/ses_victim/command", strings.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"sessionID": "ses_victim"})
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "usr_attacker"}))
	rec := httptest.NewRecorder()

	server.sendCommandToExternalAgentHandler(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
}

// authorizeDesktopID gates watching (Get) and driving (Update) a desktop.
// The owner passes both; anyone else is refused before any I/O.
func TestAuthorizeDesktopID_OwnerAndStranger(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: st, Cfg: &config.ServerConfig{}}
	st.EXPECT().GetSession(gomock.Any(), "ses_1").Return(&types.Session{ID: "ses_1", Owner: "usr_owner"}, nil).AnyTimes()

	for _, tc := range []struct {
		user   string
		action types.Action
		want   bool
	}{
		{"usr_owner", types.ActionGet, true},
		{"usr_owner", types.ActionUpdate, true},
		{"usr_other", types.ActionGet, false},
		{"usr_other", types.ActionUpdate, false},
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/external-agents/ses_1/ws/input", nil)
		req = req.WithContext(setRequestUser(req.Context(), types.User{ID: tc.user}))
		rec := httptest.NewRecorder()
		got := server.authorizeDesktopID(rec, req, "ses_1", tc.action)
		require.Equal(t, tc.want, got, "%s %s", tc.user, tc.action)
		if !tc.want {
			require.Equal(t, http.StatusForbidden, rec.Code)
		}
	}
}

// The command reaches the agent only for callers who may change the session:
// its owner and the session key minted for its project. Anyone else, including
// the owner's key confined to another project, is refused and nothing is
// delivered.
func TestSendCommandToExternalAgent_DeliversOnlyForSessionUpdaters(t *testing.T) {
	for _, tc := range []struct {
		name string
		user types.User
		want int
	}{
		{"owner", types.User{ID: "usr_owner"}, http.StatusOK},
		{"sandbox session key", types.User{ID: "usr_owner", ProjectID: "prj_a", SessionID: "ses_a", TokenType: types.TokenTypeAPIKey}, http.StatusOK},
		{"other user", types.User{ID: "usr_other"}, http.StatusForbidden},
		{"key scoped to another project", types.User{ID: "usr_owner", ProjectID: "prj_b", TokenType: types.TokenTypeAPIKey}, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			st := store.NewMockStore(ctrl)
			st.EXPECT().GetSession(gomock.Any(), "ses_a").Return(&types.Session{
				ID:        "ses_a",
				Owner:     "usr_owner",
				ProjectID: "prj_a",
				Metadata:  types.SessionMetadata{AgentType: "zed_external"},
			}, nil).AnyTimes()

			sendChan := make(chan types.ExternalAgentCommand, 1)
			manager := NewExternalAgentWSManager()
			manager.registerConnection("ses_a", &ExternalAgentWSConnection{SessionID: "ses_a", SendChan: sendChan})
			server := &HelixAPIServer{Store: st, Cfg: &config.ServerConfig{}, externalAgentWSManager: manager}

			req := httptest.NewRequest(http.MethodPost, "/api/v1/external-agents/ses_a/command",
				strings.NewReader(`{"type":"chat_message","data":{"message":"hello"}}`))
			req = mux.SetURLVars(req, map[string]string{"sessionID": "ses_a"})
			req = req.WithContext(setRequestUser(req.Context(), tc.user))
			rec := httptest.NewRecorder()
			server.sendCommandToExternalAgentHandler(rec, req)

			require.Equal(t, tc.want, rec.Code, rec.Body.String())
			if tc.want == http.StatusOK {
				require.Len(t, sendChan, 1, "the command reaches the agent")
			} else {
				require.Empty(t, sendChan, "a refused command must not reach the agent")
			}
		})
	}
}
