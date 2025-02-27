package auth

import (
	"context"
	"fmt"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func Test_ensureStoreUser_CreateNew(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockStore(ctrl)
	authenticator := &KeycloakAuthenticator{
		store: mockStore,
	}

	mockStore.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound)
	mockStore.EXPECT().CreateUser(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, user *types.User) (*types.User, error) {
			require.Equal(t, "123", user.ID)
			require.Equal(t, "testuser", user.Username)
			return user, nil
		},
	)

	user := &types.User{
		ID:       "123",
		Username: "testuser",
		Email:    "testuser@example.com",
		FullName: "Test User",
	}

	err := authenticator.ensureStoreUser(user)
	require.NoError(t, err)
}

func Test_ensureStoreUser_CreateNew_Concurrent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockStore(ctrl)
	authenticator := &KeycloakAuthenticator{
		store: mockStore,
	}

	mockStore.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound)
	mockStore.EXPECT().CreateUser(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ *types.User) (*types.User, error) {
			return nil, fmt.Errorf("duplicate key")
		},
	)

	user := &types.User{
		ID:       "123",
		Username: "testuser",
		Email:    "testuser@example.com",
		FullName: "Test User",
	}

	err := authenticator.ensureStoreUser(user)
	require.NoError(t, err)
}

func Test_ensureStoreUser_UpdateExisting(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockStore(ctrl)
	authenticator := &KeycloakAuthenticator{
		store: mockStore,
	}

	mockStore.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(&types.User{
		ID:    "123",
		Email: "old-email@example.com",
	}, nil)
	mockStore.EXPECT().UpdateUser(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, user *types.User) (*types.User, error) {
			require.Equal(t, "123", user.ID)
			require.Equal(t, "testuser", user.Username)
			require.Equal(t, "new-email@example.com", user.Email)
			require.Equal(t, "Test User", user.FullName)
			return user, nil
		},
	)

	user := &types.User{
		ID:       "123",
		Username: "testuser",
		Email:    "new-email@example.com",
		FullName: "Test User",
	}

	err := authenticator.ensureStoreUser(user)
	require.NoError(t, err)
}

func Test_ensureStoreUser_UpToDate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockStore(ctrl)
	authenticator := &KeycloakAuthenticator{
		store: mockStore,
	}

	mockStore.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(&types.User{
		ID:       "123",
		Username: "testuser",
		Email:    "new-email@example.com",
		FullName: "Test User",
	}, nil)

	user := &types.User{
		ID:       "123",
		Username: "testuser",
		Email:    "new-email@example.com",
		FullName: "Test User",
	}

	err := authenticator.ensureStoreUser(user)
	require.NoError(t, err)
}
