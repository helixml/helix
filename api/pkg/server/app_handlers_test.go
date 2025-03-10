package server

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"

	"go.uber.org/mock/gomock"
)

func Test_populateAppOwner_PopulateUser(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	userStore := store.NewMockStore(ctrl)
	userStore.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(&types.User{
		ID: "user1",
	}, nil)

	server := &HelixAPIServer{
		Store: userStore,
	}

	apps := []*types.App{
		{
			Owner: "user1",
		},
	}

	populatedApps := server.populateAppOwner(context.Background(), apps)

	require.Equal(t, "user1", populatedApps[0].User.ID)
}

func Test_populateAppOwner_OwnerNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	userStore := store.NewMockStore(ctrl)
	userStore.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound)

	server := &HelixAPIServer{
		Store: userStore,
	}

	apps := []*types.App{
		{
			Owner: "user1",
		},
	}

	populatedApps := server.populateAppOwner(context.Background(), apps)

	require.Equal(t, "", populatedApps[0].User.ID)
}
