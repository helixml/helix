package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

func TestAuthorizeExternalAgentSync(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}
	mockStore.EXPECT().GetSession(gomock.Any(), "ses_owned").
		Return(&types.Session{ID: "ses_owned", Owner: "user-1"}, nil).AnyTimes()
	mockStore.EXPECT().GetSession(gomock.Any(), "ses_missing").
		Return(nil, fmt.Errorf("not found")).AnyTimes()

	tests := []struct {
		name      string
		user      *types.User
		sessionID string
		want      int
	}{
		{"owner", &types.User{ID: "user-1"}, "ses_owned", http.StatusOK},
		{"another user", &types.User{ID: "user-2"}, "ses_owned", http.StatusForbidden},
		{"unknown session", &types.User{ID: "user-1"}, "ses_missing", http.StatusForbidden},
		{"non-session id", &types.User{ID: "user-1"}, "agent-1", http.StatusForbidden},
		{"runner token", &types.User{TokenType: types.TokenTypeRunner}, "ses_owned", http.StatusOK},
		{"unauthenticated", nil, "ses_owned", http.StatusUnauthorized},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/external-agents/sync?session_id="+tc.sessionID, nil)
			if tc.user != nil {
				req = req.WithContext(context.WithValue(req.Context(), userKey, *tc.user))
			}
			rec := httptest.NewRecorder()
			server.authorizeExternalAgentSync(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})(rec, req)
			require.Equal(t, tc.want, rec.Code)
		})
	}
}
