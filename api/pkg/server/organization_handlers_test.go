package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestValidateAndNormalizeDomain(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    string
		expectError bool
		errorMsg    string
	}{
		// Valid cases
		{
			name:        "simple domain",
			input:       "example.com",
			expected:    "example.com",
			expectError: false,
		},
		{
			name:        "subdomain",
			input:       "sub.example.com",
			expected:    "sub.example.com",
			expectError: false,
		},
		{
			name:        "uppercase normalized to lowercase",
			input:       "EXAMPLE.COM",
			expected:    "example.com",
			expectError: false,
		},
		{
			name:        "mixed case normalized",
			input:       "Example.Com",
			expected:    "example.com",
			expectError: false,
		},
		{
			name:        "domain with whitespace trimmed",
			input:       "  example.com  ",
			expected:    "example.com",
			expectError: false,
		},
		{
			name:        "empty string allowed (clears domain)",
			input:       "",
			expected:    "",
			expectError: false,
		},
		{
			name:        "co.uk domain",
			input:       "company.co.uk",
			expected:    "company.co.uk",
			expectError: false,
		},
		{
			name:        "domain with hyphen",
			input:       "my-company.com",
			expected:    "my-company.com",
			expectError: false,
		},
		{
			name:        "domain with numbers",
			input:       "company123.com",
			expected:    "company123.com",
			expectError: false,
		},

		// Invalid cases
		{
			name:        "starts with @",
			input:       "@example.com",
			expectError: true,
			errorMsg:    "should not start with @",
		},
		{
			name:        "email address",
			input:       "user@example.com",
			expectError: true,
			errorMsg:    "should not contain @",
		},
		{
			name:        "no TLD",
			input:       "example",
			expectError: true,
			errorMsg:    "invalid domain format",
		},
		{
			name:        "starts with dot",
			input:       ".example.com",
			expectError: true,
			errorMsg:    "invalid domain format",
		},
		{
			name:        "ends with dot",
			input:       "example.com.",
			expectError: true,
			errorMsg:    "invalid domain format",
		},
		{
			name:        "starts with hyphen",
			input:       "-example.com",
			expectError: true,
			errorMsg:    "invalid domain format",
		},
		{
			name:        "double dot",
			input:       "example..com",
			expectError: true,
			errorMsg:    "invalid domain format",
		},
		{
			name:        "special characters",
			input:       "example!.com",
			expectError: true,
			errorMsg:    "invalid domain format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := validateAndNormalizeDomain(tt.input)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorMsg)
					return
				}
				if tt.errorMsg != "" && !containsString(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errorMsg, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// containsString checks if s contains substr (case-insensitive)
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestDeleteOrganization_OrganizationNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{
		Store: mockStore,
	}

	orgID := "org_123"
	mockStore.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{
		ID: orgID,
	}).Return(nil, store.ErrNotFound)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/organizations/"+orgID, nil)
	req = mux.SetURLVars(req, map[string]string{"id": orgID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{
		ID: "user_1",
	}))

	rr := httptest.NewRecorder()
	server.deleteOrganization(rr, req)

	require.Equal(t, http.StatusNotFound, rr.Code)
	require.Contains(t, rr.Body.String(), "Organization not found")
}

func TestDeleteOrganization_OnlyOwnerCanDelete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{
		Store: mockStore,
	}

	orgID := "org_123"
	userID := "user_1"

	gomock.InOrder(
		mockStore.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{
			ID: orgID,
		}).Return(&types.Organization{
			ID: orgID,
		}, nil),
		mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
			OrganizationID: orgID,
			UserID:         userID,
		}).Return(&types.OrganizationMembership{
			OrganizationID: orgID,
			UserID:         userID,
			Role:           types.OrganizationRoleMember,
		}, nil),
	)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/organizations/"+orgID, nil)
	req = mux.SetURLVars(req, map[string]string{"id": orgID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{
		ID: userID,
	}))

	rr := httptest.NewRecorder()
	server.deleteOrganization(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	require.Contains(t, rr.Body.String(), "Could not authorize org owner")
}

func TestDeleteOrganization_DeletesRepositoriesBeforeOrganization(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	gitRepositoryService := services.NewGitRepositoryService(
		mockStore,
		t.TempDir(),
		"http://localhost:8080",
		"test",
		"test@example.com",
	)

	server := &HelixAPIServer{
		Store:                mockStore,
		gitRepositoryService: gitRepositoryService,
	}

	orgID := "org_123"
	userID := "user_1"
	repo1 := &types.GitRepository{ID: "repo_1", OrganizationID: orgID}
	repo2 := &types.GitRepository{ID: "repo_2", OrganizationID: orgID}

	gomock.InOrder(
		mockStore.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{
			ID: orgID,
		}).Return(&types.Organization{ID: orgID}, nil),
		mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
			OrganizationID: orgID,
			UserID:         userID,
		}).Return(&types.OrganizationMembership{
			OrganizationID: orgID,
			UserID:         userID,
			Role:           types.OrganizationRoleOwner,
		}, nil),
		mockStore.EXPECT().ListGitRepositories(gomock.Any(), &types.ListGitRepositoriesRequest{
			OrganizationID: orgID,
		}).Return([]*types.GitRepository{repo1, repo2}, nil),
		mockStore.EXPECT().GetGitRepository(gomock.Any(), repo1.ID).Return(repo1, nil),
		mockStore.EXPECT().DeleteGitRepository(gomock.Any(), repo1.ID).Return(nil),
		mockStore.EXPECT().GetGitRepository(gomock.Any(), repo2.ID).Return(repo2, nil),
		mockStore.EXPECT().DeleteGitRepository(gomock.Any(), repo2.ID).Return(nil),
		mockStore.EXPECT().DeleteOrganization(gomock.Any(), orgID).Return(nil),
	)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/organizations/"+orgID, nil)
	req = mux.SetURLVars(req, map[string]string{"id": orgID})
	req = req.WithContext(setRequestUser(req.Context(), types.User{
		ID: userID,
	}))

	rr := httptest.NewRecorder()
	server.deleteOrganization(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
}
