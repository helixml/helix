package user

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *client.HelixClient) {
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	apiClient, err := client.NewClient(server.URL+"/api/v1", "test-api-key")
	require.NoError(t, err)

	return server, apiClient
}

func newTestCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})
	return cmd
}

func TestResetPassword_NonAdminResetsOwnPassword_Succeeds(t *testing.T) {
	currentUser := &types.User{
		ID:    "user-123",
		Email: "user@example.com",
		Admin: false,
	}

	_, apiClient := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/status":
			// Return current user info
			json.NewEncoder(w).Encode(currentUser)
		case "/api/v1/auth/password-update":
			// Verify it's a POST request
			assert.Equal(t, http.MethodPost, r.Method)

			// Verify the request body
			var req types.PasswordUpdateRequest
			err := json.NewDecoder(r.Body).Decode(&req)
			require.NoError(t, err)
			assert.Equal(t, "newpassword123", req.NewPassword)

			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("Unexpected request to %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	cmd := newTestCmd()
	err := resetPassword(context.Background(), apiClient, "user@example.com", "newpassword123", cmd)
	assert.NoError(t, err)
}

func TestResetPassword_AdminResetsOwnPassword_Succeeds(t *testing.T) {
	currentUser := &types.User{
		ID:    "admin-123",
		Email: "admin@example.com",
		Admin: true,
	}

	_, apiClient := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/status":
			json.NewEncoder(w).Encode(currentUser)
		case "/api/v1/auth/password-update":
			assert.Equal(t, http.MethodPost, r.Method)

			var req types.PasswordUpdateRequest
			err := json.NewDecoder(r.Body).Decode(&req)
			require.NoError(t, err)
			assert.Equal(t, "newadminpassword", req.NewPassword)

			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("Unexpected request to %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	cmd := newTestCmd()
	err := resetPassword(context.Background(), apiClient, "admin@example.com", "newadminpassword", cmd)
	assert.NoError(t, err)
}

func TestResetPassword_AdminResetsOtherUserPassword_Succeeds(t *testing.T) {
	currentUser := &types.User{
		ID:    "admin-123",
		Email: "admin@example.com",
		Admin: true,
	}

	targetUser := &types.User{
		ID:    "user-456",
		Email: "otheruser@example.com",
		Admin: false,
	}

	_, apiClient := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/status":
			json.NewEncoder(w).Encode(currentUser)
		case "/api/v1/users":
			// Return list of users matching email filter
			assert.Equal(t, "otheruser@example.com", r.URL.Query().Get("email"))
			json.NewEncoder(w).Encode(&types.PaginatedUsersList{
				Users:      []*types.User{targetUser},
				TotalCount: 1,
			})
		case "/api/v1/admin/users/user-456/password":
			assert.Equal(t, http.MethodPut, r.Method)

			var req map[string]string
			err := json.NewDecoder(r.Body).Decode(&req)
			require.NoError(t, err)
			assert.Equal(t, "newuserpassword", req["new_password"])

			// Return updated user
			json.NewEncoder(w).Encode(targetUser)
		default:
			t.Errorf("Unexpected request to %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	cmd := newTestCmd()
	err := resetPassword(context.Background(), apiClient, "otheruser@example.com", "newuserpassword", cmd)
	assert.NoError(t, err)
}

func TestResetPassword_NonAdminResetsOtherUserPassword_Fails(t *testing.T) {
	currentUser := &types.User{
		ID:    "user-123",
		Email: "user@example.com",
		Admin: false,
	}

	_, apiClient := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/status":
			json.NewEncoder(w).Encode(currentUser)
		default:
			t.Errorf("Unexpected request to %s - non-admin should not make further requests", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	cmd := newTestCmd()
	err := resetPassword(context.Background(), apiClient, "otheruser@example.com", "newpassword", cmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "only admins can reset other users' passwords")
}

func TestResetPassword_InvalidAPIKey_Fails(t *testing.T) {
	_, apiClient := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Check for authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer valid-api-key" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error": "invalid API key"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	cmd := newTestCmd()
	err := resetPassword(context.Background(), apiClient, "user@example.com", "newpassword", cmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}

func TestResetPassword_UserNotFound_Fails(t *testing.T) {
	currentUser := &types.User{
		ID:    "admin-123",
		Email: "admin@example.com",
		Admin: true,
	}

	_, apiClient := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/status":
			json.NewEncoder(w).Encode(currentUser)
		case "/api/v1/users":
			// Return empty list - user not found
			json.NewEncoder(w).Encode(&types.PaginatedUsersList{
				Users:      []*types.User{},
				TotalCount: 0,
			})
		default:
			t.Errorf("Unexpected request to %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	cmd := newTestCmd()
	err := resetPassword(context.Background(), apiClient, "nonexistent@example.com", "newpassword", cmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "user not found")
}

func TestResetPassword_AdminEndpointReturnsError_Fails(t *testing.T) {
	currentUser := &types.User{
		ID:    "admin-123",
		Email: "admin@example.com",
		Admin: true,
	}

	targetUser := &types.User{
		ID:    "user-456",
		Email: "otheruser@example.com",
		Admin: false,
	}

	_, apiClient := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/status":
			json.NewEncoder(w).Encode(currentUser)
		case "/api/v1/users":
			json.NewEncoder(w).Encode(&types.PaginatedUsersList{
				Users:      []*types.User{targetUser},
				TotalCount: 1,
			})
		case "/api/v1/admin/users/user-456/password":
			// Return error from admin endpoint
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error": "password too short"}`))
		default:
			t.Errorf("Unexpected request to %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	cmd := newTestCmd()
	err := resetPassword(context.Background(), apiClient, "otheruser@example.com", "short", cmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "400")
}
