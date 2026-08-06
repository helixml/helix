package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestLoadAuthorizedTerminalSession(t *testing.T) {
	tests := []struct {
		name       string
		session    *types.Session
		user       types.User
		wantStatus int
		wantClient bool
	}{
		{
			name: "owner with assigned sandbox host",
			session: &types.Session{
				ID:        "ses_allowed",
				Owner:     "user-1",
				SandboxID: "runner-a",
			},
			user:       types.User{ID: "user-1"},
			wantStatus: http.StatusOK,
			wantClient: true,
		},
		{
			name: "cross-user access",
			session: &types.Session{
				ID:        "ses_forbidden",
				Owner:     "user-1",
				SandboxID: "runner-a",
			},
			user:       types.User{ID: "user-2"},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "sandbox host not assigned",
			session: &types.Session{
				ID:    "ses_starting",
				Owner: "user-1",
			},
			user:       types.User{ID: "user-1"},
			wantStatus: http.StatusServiceUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockStore := store.NewMockStore(ctrl)
			server := &HelixAPIServer{Store: mockStore}

			mockStore.EXPECT().GetSession(gomock.Any(), tt.session.ID).Return(tt.session, nil)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+tt.session.ID+"/terminal", nil)
			req = mux.SetURLVars(req, map[string]string{"id": tt.session.ID})
			req = req.WithContext(setRequestUser(req.Context(), tt.user))
			rw := httptest.NewRecorder()

			session, client := server.loadAuthorizedTerminalSession(rw, req, types.ActionUpdate)
			if tt.wantStatus == http.StatusOK {
				require.Equal(t, tt.session, session)
				require.Equal(t, tt.wantClient, client != nil)
				return
			}
			require.Nil(t, session)
			require.Nil(t, client)
			require.Equal(t, tt.wantStatus, rw.Code)
		})
	}
}
