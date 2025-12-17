package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestAdminResetPassword(t *testing.T) {
	tests := []struct {
		name           string
		adminUser      *types.User
		targetUserID   string
		requestBody    map[string]string
		setupMocks     func(*store.MockStore, *auth.MockAuthenticator)
		expectedStatus int
		expectedError  string
	}{
		{
			name: "successful password reset",
			adminUser: &types.User{
				ID:    "admin-123",
				Email: "admin@example.com",
				Admin: true,
			},
			targetUserID: "user-456",
			requestBody: map[string]string{
				"new_password": "newSecurePassword123",
			},
			setupMocks: func(mockStore *store.MockStore, mockAuth *auth.MockAuthenticator) {
				targetUser := &types.User{
					ID:    "user-456",
					Email: "user@example.com",
				}
				mockStore.EXPECT().
					GetUser(gomock.Any(), &store.GetUserQuery{ID: "user-456"}).
					Return(targetUser, nil).Times(2)
				mockAuth.EXPECT().
					UpdatePassword(gomock.Any(), "user-456", "newSecurePassword123").
					Return(nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "non-admin cannot reset password",
			adminUser: &types.User{
				ID:    "user-123",
				Email: "user@example.com",
				Admin: false,
			},
			targetUserID: "user-456",
			requestBody: map[string]string{
				"new_password": "newSecurePassword123",
			},
			setupMocks:     func(mockStore *store.MockStore, mockAuth *auth.MockAuthenticator) {},
			expectedStatus: http.StatusForbidden,
			expectedError:  "only admins can reset user passwords",
		},
		{
			name: "missing user ID",
			adminUser: &types.User{
				ID:    "admin-123",
				Email: "admin@example.com",
				Admin: true,
			},
			targetUserID: "",
			requestBody: map[string]string{
				"new_password": "newSecurePassword123",
			},
			setupMocks:     func(mockStore *store.MockStore, mockAuth *auth.MockAuthenticator) {},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "user ID is required",
		},
		{
			name: "empty password",
			adminUser: &types.User{
				ID:    "admin-123",
				Email: "admin@example.com",
				Admin: true,
			},
			targetUserID: "user-456",
			requestBody: map[string]string{
				"new_password": "",
			},
			setupMocks:     func(mockStore *store.MockStore, mockAuth *auth.MockAuthenticator) {},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "new password is required",
		},
		{
			name: "user not found",
			adminUser: &types.User{
				ID:    "admin-123",
				Email: "admin@example.com",
				Admin: true,
			},
			targetUserID: "nonexistent-user",
			requestBody: map[string]string{
				"new_password": "newSecurePassword123",
			},
			setupMocks: func(mockStore *store.MockStore, mockAuth *auth.MockAuthenticator) {
				mockStore.EXPECT().
					GetUser(gomock.Any(), &store.GetUserQuery{ID: "nonexistent-user"}).
					Return(nil, store.ErrNotFound)
			},
			expectedStatus: http.StatusNotFound,
			expectedError:  "user not found",
		},
		{
			name: "password too short",
			adminUser: &types.User{
				ID:    "admin-123",
				Email: "admin@example.com",
				Admin: true,
			},
			targetUserID: "user-456",
			requestBody: map[string]string{
				"new_password": "short",
			},
			setupMocks: func(mockStore *store.MockStore, mockAuth *auth.MockAuthenticator) {
				targetUser := &types.User{
					ID:    "user-456",
					Email: "user@example.com",
				}
				mockStore.EXPECT().
					GetUser(gomock.Any(), &store.GetUserQuery{ID: "user-456"}).
					Return(targetUser, nil)
				mockAuth.EXPECT().
					UpdatePassword(gomock.Any(), "user-456", "short").
					Return(assert.AnError)
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "failed to update password",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStore := store.NewMockStore(ctrl)
			mockAuth := auth.NewMockAuthenticator(ctrl)

			tt.setupMocks(mockStore, mockAuth)

			server := &HelixAPIServer{
				Store:         mockStore,
				authenticator: mockAuth,
			}

			body, err := json.Marshal(tt.requestBody)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/"+tt.targetUserID+"/password", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			// Set up mux vars
			req = mux.SetURLVars(req, map[string]string{"id": tt.targetUserID})

			// Set user in context
			ctx := setTestRequestUser(req.Context(), tt.adminUser)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()

			result, httpErr := server.adminResetPassword(rr, req)

			if tt.expectedError != "" {
				require.NotNil(t, httpErr)
				assert.Contains(t, httpErr.Error(), tt.expectedError)
				httpError, ok := httpErr.(*system.HTTPError)
				require.True(t, ok, "expected *system.HTTPError")
				assert.Equal(t, tt.expectedStatus, httpError.StatusCode)
			} else {
				require.Nil(t, httpErr)
				require.NotNil(t, result)
				assert.Equal(t, tt.targetUserID, result.ID)
			}
		})
	}
}

func TestAdminResetPassword_InvalidJSON(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	mockAuth := auth.NewMockAuthenticator(ctrl)

	server := &HelixAPIServer{
		Store:         mockStore,
		authenticator: mockAuth,
	}

	adminUser := &types.User{
		ID:    "admin-123",
		Email: "admin@example.com",
		Admin: true,
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/user-456/password", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	req = mux.SetURLVars(req, map[string]string{"id": "user-456"})
	ctx := setTestRequestUser(req.Context(), adminUser)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()

	_, httpErr := server.adminResetPassword(rr, req)

	require.NotNil(t, httpErr)
	assert.Contains(t, httpErr.Error(), "failed to decode request")
	httpError, ok := httpErr.(*system.HTTPError)
	require.True(t, ok, "expected *system.HTTPError")
	assert.Equal(t, http.StatusBadRequest, httpError.StatusCode)
}

// Helper function to set request user in context (mirrors the actual implementation)
func setTestRequestUser(ctx context.Context, user *types.User) context.Context {
	return context.WithValue(ctx, userKey, *user)
}

func TestAdminDeleteUser(t *testing.T) {
	tests := []struct {
		name           string
		adminUser      *types.User
		targetUserID   string
		setupMocks     func(*store.MockStore)
		expectedStatus int
		expectedError  string
	}{
		{
			name: "successful user deletion",
			adminUser: &types.User{
				ID:    "admin-123",
				Email: "admin@example.com",
				Admin: true,
			},
			targetUserID: "user-456",
			setupMocks: func(mockStore *store.MockStore) {
				targetUser := &types.User{
					ID:    "user-456",
					Email: "user@example.com",
				}
				mockStore.EXPECT().
					GetUser(gomock.Any(), &store.GetUserQuery{ID: "user-456"}).
					Return(targetUser, nil)
				mockStore.EXPECT().
					DeleteUser(gomock.Any(), "user-456").
					Return(nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "non-admin cannot delete user",
			adminUser: &types.User{
				ID:    "user-123",
				Email: "user@example.com",
				Admin: false,
			},
			targetUserID:   "user-456",
			setupMocks:     func(mockStore *store.MockStore) {},
			expectedStatus: http.StatusForbidden,
			expectedError:  "only admins can delete users",
		},
		{
			name: "cannot delete own account",
			adminUser: &types.User{
				ID:    "admin-123",
				Email: "admin@example.com",
				Admin: true,
			},
			targetUserID:   "admin-123",
			setupMocks:     func(mockStore *store.MockStore) {},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "cannot delete your own account",
		},
		{
			name: "missing user ID",
			adminUser: &types.User{
				ID:    "admin-123",
				Email: "admin@example.com",
				Admin: true,
			},
			targetUserID:   "",
			setupMocks:     func(mockStore *store.MockStore) {},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "user ID is required",
		},
		{
			name: "user not found",
			adminUser: &types.User{
				ID:    "admin-123",
				Email: "admin@example.com",
				Admin: true,
			},
			targetUserID: "nonexistent-user",
			setupMocks: func(mockStore *store.MockStore) {
				mockStore.EXPECT().
					GetUser(gomock.Any(), &store.GetUserQuery{ID: "nonexistent-user"}).
					Return(nil, store.ErrNotFound)
			},
			expectedStatus: http.StatusNotFound,
			expectedError:  "user not found",
		},
		{
			name: "delete fails",
			adminUser: &types.User{
				ID:    "admin-123",
				Email: "admin@example.com",
				Admin: true,
			},
			targetUserID: "user-456",
			setupMocks: func(mockStore *store.MockStore) {
				targetUser := &types.User{
					ID:    "user-456",
					Email: "user@example.com",
				}
				mockStore.EXPECT().
					GetUser(gomock.Any(), &store.GetUserQuery{ID: "user-456"}).
					Return(targetUser, nil)
				mockStore.EXPECT().
					DeleteUser(gomock.Any(), "user-456").
					Return(assert.AnError)
			},
			expectedStatus: http.StatusInternalServerError,
			expectedError:  "failed to delete user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStore := store.NewMockStore(ctrl)
			tt.setupMocks(mockStore)

			server := &HelixAPIServer{
				Store: mockStore,
			}

			req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/users/"+tt.targetUserID, nil)

			// Set up mux vars
			req = mux.SetURLVars(req, map[string]string{"id": tt.targetUserID})

			// Set user in context
			ctx := setTestRequestUser(req.Context(), tt.adminUser)
			req = req.WithContext(ctx)

			rr := httptest.NewRecorder()

			result, httpErr := server.adminDeleteUser(rr, req)

			if tt.expectedError != "" {
				require.NotNil(t, httpErr)
				assert.Contains(t, httpErr.Error(), tt.expectedError)
				httpError, ok := httpErr.(*system.HTTPError)
				require.True(t, ok, "expected *system.HTTPError")
				assert.Equal(t, tt.expectedStatus, httpError.StatusCode)
			} else {
				require.Nil(t, httpErr)
				require.NotNil(t, result)
				assert.Equal(t, "user deleted successfully", result["message"])
				assert.Equal(t, tt.targetUserID, result["user_id"])
			}
		})
	}
}
