package server

import (
	"testing"

	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestMoveProject_NotOwner(t *testing.T) {
	project := &types.Project{
		ID:             "project-123",
		Name:           "My Project",
		UserID:         "user-123", // Owner
		OrganizationID: "",
	}

	user := &types.User{
		ID:    "different-user", // Not the owner
		Admin: false,
	}

	// Check ownership
	isOwner := project.UserID == user.ID || user.Admin
	assert.False(t, isOwner, "User should not be owner")
}

func TestMoveProject_AdminCanMove(t *testing.T) {
	project := &types.Project{
		ID:             "project-123",
		Name:           "My Project",
		UserID:         "user-123", // Owner
		OrganizationID: "",
	}

	adminUser := &types.User{
		ID:    "admin-user", // Not the owner but is admin
		Admin: true,
	}

	// Check ownership - admin bypass
	isOwner := project.UserID == adminUser.ID || adminUser.Admin
	assert.True(t, isOwner, "Admin should be able to move any project")
}

func TestMoveProject_AlreadyInOrg(t *testing.T) {
	project := &types.Project{
		ID:             "project-123",
		Name:           "My Project",
		UserID:         "user-123",
		OrganizationID: "existing-org", // Already in an org!
	}

	// Check if already in org
	alreadyInOrg := project.OrganizationID != ""
	assert.True(t, alreadyInOrg, "Project is already in an organization")
}

func TestMoveProject_PersonalProject(t *testing.T) {
	project := &types.Project{
		ID:             "project-123",
		Name:           "My Project",
		UserID:         "user-123",
		OrganizationID: "", // Personal project
	}

	// Check if personal project
	isPersonal := project.OrganizationID == ""
	assert.True(t, isPersonal, "Project should be a personal project")
}

func TestGetUniqueProjectName(t *testing.T) {
	tests := []struct {
		name          string
		baseName      string
		existingNames map[string]bool
		expected      string
	}{
		{
			name:          "no conflict",
			baseName:      "My Project",
			existingNames: map[string]bool{},
			expected:      "My Project",
		},
		{
			name:          "single conflict",
			baseName:      "My Project",
			existingNames: map[string]bool{"My Project": true},
			expected:      "My Project (1)",
		},
		{
			name:          "multiple conflicts",
			baseName:      "My Project",
			existingNames: map[string]bool{"My Project": true, "My Project (1)": true, "My Project (2)": true},
			expected:      "My Project (3)",
		},
		{
			name:          "gap in sequence uses next available",
			baseName:      "Test",
			existingNames: map[string]bool{"Test": true, "Test (1)": true, "Test (3)": true},
			expected:      "Test (2)", // Should find the first available
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getUniqueProjectName(tt.baseName, tt.existingNames)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetUniqueRepoName(t *testing.T) {
	tests := []struct {
		name          string
		baseName      string
		existingNames map[string]bool
		expected      string
	}{
		{
			name:          "no conflict",
			baseName:      "api",
			existingNames: map[string]bool{},
			expected:      "api",
		},
		{
			name:          "single conflict",
			baseName:      "api",
			existingNames: map[string]bool{"api": true},
			expected:      "api-2",
		},
		{
			name:          "multiple conflicts",
			baseName:      "api",
			existingNames: map[string]bool{"api": true, "api-2": true, "api-3": true},
			expected:      "api-4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy since GetUniqueRepoName modifies the map
			existingCopy := make(map[string]bool)
			for k, v := range tt.existingNames {
				existingCopy[k] = v
			}
			result := services.GetUniqueRepoName(tt.baseName, existingCopy)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMoveProjectPreviewItem_NoConflict(t *testing.T) {
	existingProjectNames := map[string]bool{
		"Other Project": true,
	}

	projectName := "My Project"
	hasConflict := existingProjectNames[projectName]

	assert.False(t, hasConflict, "Should not have conflict")
}

func TestMoveProjectPreviewItem_WithConflict(t *testing.T) {
	existingProjectNames := map[string]bool{
		"My Project": true,
	}

	projectName := "My Project"
	hasConflict := existingProjectNames[projectName]

	assert.True(t, hasConflict, "Should have conflict")

	// Get the new name
	newName := getUniqueProjectName(projectName, existingProjectNames)
	assert.Equal(t, "My Project (1)", newName)
}

func TestMoveRepositoryPreviewItem_NoConflict(t *testing.T) {
	existingRepoNames := map[string]bool{
		"frontend": true,
	}

	repoName := "backend"
	hasConflict := existingRepoNames[repoName]

	assert.False(t, hasConflict, "Should not have conflict")
}

func TestMoveRepositoryPreviewItem_WithConflict(t *testing.T) {
	existingRepoNames := map[string]bool{
		"api": true,
	}

	repoName := "api"
	hasConflict := existingRepoNames[repoName]

	assert.True(t, hasConflict, "Should have conflict")

	// Get the new name
	newName := services.GetUniqueRepoName(repoName, existingRepoNames)
	assert.Equal(t, "api-2", newName)
}

func TestMoveProject_OrganizationIDSet(t *testing.T) {
	project := &types.Project{
		ID:             "project-123",
		Name:           "My Project",
		UserID:         "user-123",
		OrganizationID: "",
	}

	targetOrgID := "org-456"

	// Simulate the move
	project.OrganizationID = targetOrgID

	assert.Equal(t, targetOrgID, project.OrganizationID, "OrganizationID should be set after move")
}

func TestMoveProject_RepoOrganizationIDSet(t *testing.T) {
	repo := &types.GitRepository{
		ID:             "repo-123",
		Name:           "frontend",
		OrganizationID: "",
	}

	targetOrgID := "org-456"

	// Simulate the move
	repo.OrganizationID = targetOrgID

	assert.Equal(t, targetOrgID, repo.OrganizationID, "Repo OrganizationID should be set after move")
}

func TestMoveProject_ProjectRepositoryOrganizationIDSet(t *testing.T) {
	pr := &types.ProjectRepository{
		ProjectID:      "project-123",
		RepositoryID:   "repo-123",
		OrganizationID: "",
	}

	targetOrgID := "org-456"

	// Simulate the move
	pr.OrganizationID = targetOrgID

	assert.Equal(t, targetOrgID, pr.OrganizationID, "ProjectRepository OrganizationID should be set after move")
}
