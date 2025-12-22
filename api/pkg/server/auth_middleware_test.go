package server

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

func TestIsAdminWithContext_EmptyUserID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)

	auth := newAuthMiddleware(nil, mockStore, authMiddlewareConfig{
		adminUserIDs: nil,
	})

	result := auth.isAdminWithContext(context.Background(), "")
	assert.False(t, result, "empty userID should return false")
}

func TestIsAdminWithContext_DevMode_EveryoneIsAdmin(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	// No database calls expected when AdminUserIDs contains "all"

	auth := newAuthMiddleware(nil, mockStore, authMiddlewareConfig{
		adminUserIDs: []string{config.AdminAllUsers},
	})

	result := auth.isAdminWithContext(context.Background(), "any-user-id")
	assert.True(t, result, "with ADMIN_USER_IDS=all, any user should be admin")
}

func TestIsAdminWithContext_SpecificUserInList(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	userID := "user-123"
	// No database calls expected when user is in the admin list

	auth := newAuthMiddleware(nil, mockStore, authMiddlewareConfig{
		adminUserIDs: []string{"user-123", "user-456"},
	})

	result := auth.isAdminWithContext(context.Background(), userID)
	assert.True(t, result, "user in ADMIN_USER_IDS list should be admin")
}

func TestIsAdminWithContext_UserNotInList_ChecksDatabase(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	userID := "user-789"

	// User is not in the admin list, so database should be checked
	mockStore.EXPECT().
		GetUser(gomock.Any(), &store.GetUserQuery{ID: userID}).
		Return(&types.User{
			ID:    userID,
			Admin: true,
		}, nil)

	auth := newAuthMiddleware(nil, mockStore, authMiddlewareConfig{
		adminUserIDs: []string{"user-123", "user-456"}, // user-789 not in list
	})

	result := auth.isAdminWithContext(context.Background(), userID)
	assert.True(t, result, "user not in list but Admin=true in database should be admin")
}

func TestIsAdminWithContext_DatabaseAdmin_True(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	userID := "user-123"

	mockStore.EXPECT().
		GetUser(gomock.Any(), &store.GetUserQuery{ID: userID}).
		Return(&types.User{
			ID:    userID,
			Admin: true,
		}, nil)

	auth := newAuthMiddleware(nil, mockStore, authMiddlewareConfig{
		adminUserIDs: nil, // Empty list - use database
	})

	result := auth.isAdminWithContext(context.Background(), userID)
	assert.True(t, result, "user with Admin=true in database should be admin")
}

func TestIsAdminWithContext_DatabaseAdmin_False(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	userID := "user-456"

	mockStore.EXPECT().
		GetUser(gomock.Any(), &store.GetUserQuery{ID: userID}).
		Return(&types.User{
			ID:    userID,
			Admin: false,
		}, nil)

	auth := newAuthMiddleware(nil, mockStore, authMiddlewareConfig{
		adminUserIDs: nil, // Empty list - use database
	})

	result := auth.isAdminWithContext(context.Background(), userID)
	assert.False(t, result, "user with Admin=false in database should not be admin")
}

func TestIsAdminWithContext_UserNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	userID := "nonexistent-user"

	mockStore.EXPECT().
		GetUser(gomock.Any(), &store.GetUserQuery{ID: userID}).
		Return(nil, nil)

	auth := newAuthMiddleware(nil, mockStore, authMiddlewareConfig{
		adminUserIDs: nil, // Empty list - use database
	})

	result := auth.isAdminWithContext(context.Background(), userID)
	assert.False(t, result, "user not found should return false")
}

func TestIsAdminWithContext_DatabaseError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	userID := "user-789"

	mockStore.EXPECT().
		GetUser(gomock.Any(), &store.GetUserQuery{ID: userID}).
		Return(nil, errors.New("database connection error"))

	auth := newAuthMiddleware(nil, mockStore, authMiddlewareConfig{
		adminUserIDs: nil, // Empty list - use database
	})

	result := auth.isAdminWithContext(context.Background(), userID)
	assert.False(t, result, "database error should return false")
}
