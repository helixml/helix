package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func pinProjectRequest(userID, projectID string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID+"/pin", nil)
	req = req.WithContext(setRequestUser(context.Background(), types.User{ID: userID}))
	return mux.SetURLVars(req, map[string]string{"id": projectID})
}

// Pinning another tenant's project must be refused without touching the
// caller's pin list, and look exactly like pinning a project that does not
// exist.
func TestPinProject_RefusesProjectOfAnotherTenant(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	s := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: st}

	st.EXPECT().GetProject(gomock.Any(), "prj_other").Return(&types.Project{ID: "prj_other", UserID: "user-owner"}, nil)
	// No EnsureUserMeta/UpdateUserMeta expectations: the pin list must not change.

	_, herr := s.pinProject(nil, pinProjectRequest("user-intruder", "prj_other"))
	require.NotNil(t, herr)
	require.Equal(t, http.StatusNotFound, herr.StatusCode)

	st.EXPECT().GetProject(gomock.Any(), "prj_missing").Return(nil, store.ErrNotFound)
	_, herrMissing := s.pinProject(nil, pinProjectRequest("user-intruder", "prj_missing"))
	require.NotNil(t, herrMissing)
	require.Equal(t, herr.StatusCode, herrMissing.StatusCode)
	require.Equal(t, herr.Message, herrMissing.Message)
}

func TestPinProject_OwnerCanPin(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	s := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: st}

	st.EXPECT().GetProject(gomock.Any(), "prj_mine").Return(&types.Project{ID: "prj_mine", UserID: "user-owner"}, nil)
	st.EXPECT().EnsureUserMeta(gomock.Any(), gomock.Any()).Return(&types.UserMeta{ID: "user-owner"}, nil)
	st.EXPECT().UpdateUserMeta(gomock.Any(), gomock.Any()).Return(&types.UserMeta{ID: "user-owner"}, nil)

	resp, herr := s.pinProject(nil, pinProjectRequest("user-owner", "prj_mine"))
	require.Nil(t, herr)
	require.Equal(t, []string{"prj_mine"}, resp.PinnedProjectIDs)
}
