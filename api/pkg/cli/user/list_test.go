package user

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func newTestCmdWithOutput() (*cobra.Command, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	cmd := &cobra.Command{}
	cmd.SetOut(buf)
	return cmd, buf
}

func TestListUsers_NonAdmin_ShowsOnlyOwnAccount(t *testing.T) {
	userStatus := &types.UserStatus{
		Admin: false,
		User:  "user-123",
	}

	currentUser := &types.User{
		ID:       "user-123",
		Email:    "user@example.com",
		Username: "testuser",
		FullName: "Test User",
		Admin:    false,
	}

	_, apiClient := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/status":
			json.NewEncoder(w).Encode(userStatus)
		case "/api/v1/users/user-123":
			json.NewEncoder(w).Encode(currentUser)
		default:
			t.Errorf("Unexpected request to %s - non-admin should only call /status and /users/{id}", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	cmd, buf := newTestCmdWithOutput()
	err := listUsers(context.Background(), apiClient, cmd)
	assert.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "user-123")
	assert.Contains(t, output, "user@example.com")
	assert.Contains(t, output, "testuser")
	assert.Contains(t, output, "Test User")
	assert.Contains(t, output, "no") // Admin status
	// Should NOT contain pagination info (only shown for admin)
	assert.NotContains(t, output, "Page")
}

func TestListUsers_Admin_ShowsAllUsers(t *testing.T) {
	adminStatus := &types.UserStatus{
		Admin: true,
		User:  "admin-123",
	}

	allUsers := &types.PaginatedUsersList{
		Users: []*types.User{
			{
				ID:       "admin-123",
				Email:    "admin@example.com",
				Username: "admin",
				FullName: "Admin User",
				Admin:    true,
			},
			{
				ID:       "user-456",
				Email:    "user@example.com",
				Username: "regularuser",
				FullName: "Regular User",
				Admin:    false,
			},
		},
		Page:       1,
		PageSize:   50,
		TotalCount: 2,
		TotalPages: 1,
	}

	_, apiClient := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/status":
			json.NewEncoder(w).Encode(adminStatus)
		case "/api/v1/users":
			assert.Equal(t, http.MethodGet, r.Method)
			json.NewEncoder(w).Encode(allUsers)
		default:
			t.Errorf("Unexpected request to %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	cmd, buf := newTestCmdWithOutput()
	err := listUsers(context.Background(), apiClient, cmd)
	assert.NoError(t, err)

	output := buf.String()
	// Check both users are in output
	assert.Contains(t, output, "admin-123")
	assert.Contains(t, output, "admin@example.com")
	assert.Contains(t, output, "user-456")
	assert.Contains(t, output, "user@example.com")
	assert.Contains(t, output, "regularuser")
	// Check pagination info
	assert.Contains(t, output, "Page 1 of 1")
	assert.Contains(t, output, "total: 2 users")
}

func TestListUsers_Admin_WithEmailFilter(t *testing.T) {
	// Reset global flags
	listEmail = "user@example.com"
	listUsername = ""
	listPage = 1
	listPerPage = 50
	defer func() {
		listEmail = ""
	}()

	adminStatus := &types.UserStatus{
		Admin: true,
		User:  "admin-123",
	}

	filteredUsers := &types.PaginatedUsersList{
		Users: []*types.User{
			{
				ID:       "user-456",
				Email:    "user@example.com",
				Username: "filtereduser",
				FullName: "Filtered User",
				Admin:    false,
			},
		},
		Page:       1,
		PageSize:   50,
		TotalCount: 1,
		TotalPages: 1,
	}

	_, apiClient := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/status":
			json.NewEncoder(w).Encode(adminStatus)
		case "/api/v1/users":
			// Verify email filter is passed
			assert.Equal(t, "user@example.com", r.URL.Query().Get("email"))
			json.NewEncoder(w).Encode(filteredUsers)
		default:
			t.Errorf("Unexpected request to %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	cmd, buf := newTestCmdWithOutput()
	err := listUsers(context.Background(), apiClient, cmd)
	assert.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "user@example.com")
	assert.Contains(t, output, "filtereduser")
}

func TestListUsers_Admin_WithUsernameFilter(t *testing.T) {
	// Reset global flags
	listEmail = ""
	listUsername = "john"
	listPage = 1
	listPerPage = 50
	defer func() {
		listUsername = ""
	}()

	adminStatus := &types.UserStatus{
		Admin: true,
		User:  "admin-123",
	}

	filteredUsers := &types.PaginatedUsersList{
		Users: []*types.User{
			{
				ID:       "user-789",
				Email:    "john@example.com",
				Username: "johndoe",
				FullName: "John Doe",
				Admin:    false,
			},
		},
		Page:       1,
		PageSize:   50,
		TotalCount: 1,
		TotalPages: 1,
	}

	_, apiClient := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/status":
			json.NewEncoder(w).Encode(adminStatus)
		case "/api/v1/users":
			// Verify username filter is passed
			assert.Equal(t, "john", r.URL.Query().Get("username"))
			json.NewEncoder(w).Encode(filteredUsers)
		default:
			t.Errorf("Unexpected request to %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	cmd, buf := newTestCmdWithOutput()
	err := listUsers(context.Background(), apiClient, cmd)
	assert.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "johndoe")
	assert.Contains(t, output, "John Doe")
}

func TestListUsers_Admin_NoUsersFound(t *testing.T) {
	// Reset global flags
	listEmail = "nonexistent@example.com"
	listUsername = ""
	listPage = 1
	listPerPage = 50
	defer func() {
		listEmail = ""
	}()

	adminStatus := &types.UserStatus{
		Admin: true,
		User:  "admin-123",
	}

	_, apiClient := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/status":
			json.NewEncoder(w).Encode(adminStatus)
		case "/api/v1/users":
			json.NewEncoder(w).Encode(&types.PaginatedUsersList{
				Users:      []*types.User{},
				Page:       1,
				PageSize:   50,
				TotalCount: 0,
				TotalPages: 0,
			})
		default:
			t.Errorf("Unexpected request to %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	cmd, buf := newTestCmdWithOutput()
	err := listUsers(context.Background(), apiClient, cmd)
	assert.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "No users found")
}

func TestListUsers_Admin_ShowsAdminStatus(t *testing.T) {
	adminStatus := &types.UserStatus{
		Admin: true,
		User:  "admin-123",
	}

	allUsers := &types.PaginatedUsersList{
		Users: []*types.User{
			{
				ID:    "admin-123",
				Email: "admin@example.com",
				Admin: true,
			},
			{
				ID:    "user-456",
				Email: "user@example.com",
				Admin: false,
			},
		},
		Page:       1,
		PageSize:   50,
		TotalCount: 2,
		TotalPages: 1,
	}

	_, apiClient := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/status":
			json.NewEncoder(w).Encode(adminStatus)
		case "/api/v1/users":
			json.NewEncoder(w).Encode(allUsers)
		default:
			t.Errorf("Unexpected request to %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	cmd, buf := newTestCmdWithOutput()
	err := listUsers(context.Background(), apiClient, cmd)
	assert.NoError(t, err)

	output := buf.String()
	// Check that output contains both "yes" and "no" for admin status
	lines := strings.Split(output, "\n")
	var foundYes, foundNo bool
	for _, line := range lines {
		if strings.Contains(line, "admin@example.com") && strings.Contains(line, "yes") {
			foundYes = true
		}
		if strings.Contains(line, "user@example.com") && strings.Contains(line, "no") {
			foundNo = true
		}
	}
	assert.True(t, foundYes, "Admin user should show 'yes' for admin status")
	assert.True(t, foundNo, "Non-admin user should show 'no' for admin status")
}

func TestListUsers_Admin_ShowsDeactivatedStatus(t *testing.T) {
	adminStatus := &types.UserStatus{
		Admin: true,
		User:  "admin-123",
	}

	allUsers := &types.PaginatedUsersList{
		Users: []*types.User{
			{
				ID:          "user-456",
				Email:       "active@example.com",
				Deactivated: false,
			},
			{
				ID:          "user-789",
				Email:       "deactivated@example.com",
				Deactivated: true,
			},
		},
		Page:       1,
		PageSize:   50,
		TotalCount: 2,
		TotalPages: 1,
	}

	_, apiClient := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/status":
			json.NewEncoder(w).Encode(adminStatus)
		case "/api/v1/users":
			json.NewEncoder(w).Encode(allUsers)
		default:
			t.Errorf("Unexpected request to %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	cmd, buf := newTestCmdWithOutput()
	err := listUsers(context.Background(), apiClient, cmd)
	assert.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "deactivated@example.com")
}

func TestListUsers_APIError_Fails(t *testing.T) {
	_, apiClient := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "invalid API key"}`))
	})

	cmd, _ := newTestCmdWithOutput()
	err := listUsers(context.Background(), apiClient, cmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get current user status")
}

func TestListUsers_Admin_ListUsersAPIError_Fails(t *testing.T) {
	adminStatus := &types.UserStatus{
		Admin: true,
		User:  "admin-123",
	}

	_, apiClient := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/status":
			json.NewEncoder(w).Encode(adminStatus)
		case "/api/v1/users":
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": "database error"}`))
		default:
			t.Errorf("Unexpected request to %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	cmd, _ := newTestCmdWithOutput()
	err := listUsers(context.Background(), apiClient, cmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list users")
}

func TestListUsers_Admin_Pagination(t *testing.T) {
	// Reset global flags
	listEmail = ""
	listUsername = ""
	listPage = 2
	listPerPage = 10
	defer func() {
		listPage = 1
		listPerPage = 50
	}()

	adminStatus := &types.UserStatus{
		Admin: true,
		User:  "admin-123",
	}

	paginatedUsers := &types.PaginatedUsersList{
		Users: []*types.User{
			{
				ID:    "user-11",
				Email: "user11@example.com",
			},
		},
		Page:       2,
		PageSize:   10,
		TotalCount: 15,
		TotalPages: 2,
	}

	_, apiClient := setupTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/status":
			json.NewEncoder(w).Encode(adminStatus)
		case "/api/v1/users":
			// Verify pagination params
			assert.Equal(t, "2", r.URL.Query().Get("page"))
			assert.Equal(t, "10", r.URL.Query().Get("per_page"))
			json.NewEncoder(w).Encode(paginatedUsers)
		default:
			t.Errorf("Unexpected request to %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	cmd, buf := newTestCmdWithOutput()
	err := listUsers(context.Background(), apiClient, cmd)
	assert.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Page 2 of 2")
	assert.Contains(t, output, "total: 15 users")
}
