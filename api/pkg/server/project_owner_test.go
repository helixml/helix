package server

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func Test_populateProjectOwners_PopulatesUser(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "user_owner"}).Return(&types.User{
		ID:       "user_owner",
		Email:    "karolis@helix.ml",
		FullName: "karolis",
	}, nil).Times(1)

	server := &HelixAPIServer{
		Store: mockStore,
	}

	projects := []*types.Project{
		{ID: "proj_1", UserID: "user_owner"},
		{ID: "proj_2", UserID: "user_owner"},
	}

	server.populateProjectOwners(context.Background(), projects)

	require.Equal(t, "user_owner", projects[0].User.ID)
	require.Equal(t, "karolis@helix.ml", projects[0].User.Email)
	require.Equal(t, "user_owner", projects[1].User.ID)
}

func Test_populateProjectOwners_OwnerNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "missing_user"}).Return(nil, store.ErrNotFound)

	server := &HelixAPIServer{
		Store: mockStore,
	}

	projects := []*types.Project{
		{ID: "proj_1", UserID: "missing_user"},
	}

	server.populateProjectOwners(context.Background(), projects)

	require.Empty(t, projects[0].User.ID)
}
