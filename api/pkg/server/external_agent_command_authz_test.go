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
