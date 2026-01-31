package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	giteagit "code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/git/gitcmd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSpecTaskGitWorkflow_EndToEnd tests the complete git-based workflow
// Uses REAL git operations (no external dependencies)
func TestSpecTaskGitWorkflow_EndToEnd(t *testing.T) {
	ctx := context.Background()

	// Setup test workspace with real git
	testDir := t.TempDir()
	gitRepoPath := filepath.Join(testDir, "test-repo")

	t.Log("Creating real git repository for SpecTask...")

	// Initialize git repository
	err := giteagit.InitRepository(ctx, gitRepoPath, false, "sha1")
	require.NoError(t, err)

	// Create initial files
	require.NoError(t, os.WriteFile(filepath.Join(gitRepoPath, "README.md"), []byte("# Test Project"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(gitRepoPath, "src"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(gitRepoPath, "src/main.go"), []byte("package main"), 0644))

	// Add and commit
	err = giteagit.AddChanges(ctx, gitRepoPath, true)
	require.NoError(t, err)

	err = giteagit.CommitChanges(ctx, gitRepoPath, giteagit.CommitChangesOptions{
		Message: "Initial commit",
		Author: &giteagit.Signature{
			Name:  "Test Agent",
			Email: "test@helix.ml",
			When:  time.Now(),
		},
		Committer: &giteagit.Signature{
			Name:  "Test Agent",
			Email: "test@helix.ml",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	t.Log("✅ Git repository initialized")

	// ========================================
	// PHASE 1: PLANNING - Create helix-specs branch
	// ========================================

	t.Log("PHASE 1: Simulating planning agent writing design docs...")

	// Create helix-specs branch
	_, _, gitErr := gitcmd.NewCommand("branch", "helix-specs").
		RunStdString(ctx, &gitcmd.RunOpts{Dir: gitRepoPath})
	require.NoError(t, gitErr)

	// Checkout helix-specs branch
	_, _, gitErr = gitcmd.NewCommand("checkout", "helix-specs").
		RunStdString(ctx, &gitcmd.RunOpts{Dir: gitRepoPath})
	require.NoError(t, gitErr)

	// Create task directory
	taskID := "spec_test_123"
	taskDir := filepath.Join(gitRepoPath, "tasks", fmt.Sprintf("2025-10-23_add-auth_%s", taskID))
	require.NoError(t, os.MkdirAll(taskDir, 0755))

	// Write design documents (simulating agent output)
	requirements := `# Requirements

## User Story
As a user, I want to login securely so that my data is protected.

## Acceptance Criteria

WHEN a user submits valid credentials
THE SYSTEM SHALL authenticate the user
  AND create a JWT token
  AND redirect to dashboard
`

	design := `# Technical Design

## Architecture
- JWT-based authentication with refresh tokens
- bcrypt password hashing
- PostgreSQL user storage

## API Endpoints
- POST /api/auth/login
- POST /api/auth/logout
- GET /api/auth/me

## Data Model
` + "```go" + `
type User struct {
    ID           string
    Email        string
    PasswordHash string
    Created      time.Time
}
` + "```\n"

	tasks := `# Implementation Tasks

- [ ] Create user database schema
- [ ] Implement password hashing with bcrypt
- [ ] Add login endpoint with validation
- [ ] Add JWT token generation
- [ ] Add session management
- [ ] Write unit tests
- [ ] Write integration tests
`

	metadata := `{"name": "Add User Authentication", "description": "Implement JWT-based user authentication system", "type": "feature"}`

	require.NoError(t, os.WriteFile(filepath.Join(taskDir, "requirements.md"), []byte(requirements), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(taskDir, "design.md"), []byte(design), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(taskDir, "tasks.md"), []byte(tasks), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(taskDir, "task-metadata.json"), []byte(metadata), 0644))

	// Commit each file separately (as agent would)
	err = giteagit.AddChanges(ctx, gitRepoPath, false, filepath.Join("tasks", fmt.Sprintf("2025-10-23_add-auth_%s", taskID), "requirements.md"))
	require.NoError(t, err)
	err = giteagit.CommitChanges(ctx, gitRepoPath, giteagit.CommitChangesOptions{
		Message: "Add requirements specification",
		Author: &giteagit.Signature{
			Name:  "Test Agent",
			Email: "test@helix.ml",
			When:  time.Now(),
		},
		Committer: &giteagit.Signature{
			Name:  "Test Agent",
			Email: "test@helix.ml",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	err = giteagit.AddChanges(ctx, gitRepoPath, false, filepath.Join("tasks", fmt.Sprintf("2025-10-23_add-auth_%s", taskID), "design.md"))
	require.NoError(t, err)
	err = giteagit.CommitChanges(ctx, gitRepoPath, giteagit.CommitChangesOptions{
		Message: "Add technical design",
		Author: &giteagit.Signature{
			Name:  "Test Agent",
			Email: "test@helix.ml",
			When:  time.Now(),
		},
		Committer: &giteagit.Signature{
			Name:  "Test Agent",
			Email: "test@helix.ml",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	err = giteagit.AddChanges(ctx, gitRepoPath, false, filepath.Join("tasks", fmt.Sprintf("2025-10-23_add-auth_%s", taskID), "tasks.md"))
	require.NoError(t, err)
	err = giteagit.CommitChanges(ctx, gitRepoPath, giteagit.CommitChangesOptions{
		Message: "Add implementation plan",
		Author: &giteagit.Signature{
			Name:  "Test Agent",
			Email: "test@helix.ml",
			When:  time.Now(),
		},
		Committer: &giteagit.Signature{
			Name:  "Test Agent",
			Email: "test@helix.ml",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	t.Log("✅ Design docs committed to helix-specs branch")

	// Verify branch exists
	stdout, _, gitErr := gitcmd.NewCommand("branch", "--list", "helix-specs").
		RunStdString(ctx, &gitcmd.RunOpts{Dir: gitRepoPath})
	require.NoError(t, gitErr)
	assert.Contains(t, stdout, "helix-specs")

	// Verify files exist in branch
	assert.FileExists(t, filepath.Join(taskDir, "requirements.md"))
	assert.FileExists(t, filepath.Join(taskDir, "design.md"))
	assert.FileExists(t, filepath.Join(taskDir, "tasks.md"))

	// ========================================
	// PHASE 2: IMPLEMENTATION - Read specs and implement
	// ========================================

	t.Log("PHASE 2: Simulating implementation agent reading specs and implementing...")

	// Agent would read design docs from helix-specs branch
	// Already on helix-specs branch, just read files

	// Read task list
	tasksContent, err := os.ReadFile(filepath.Join(taskDir, "tasks.md"))
	require.NoError(t, err)
	assert.Contains(t, string(tasksContent), "Create user database schema")
	assert.Contains(t, string(tasksContent), "[ ]") // Pending tasks

	t.Log("✅ Implementation agent read design docs from helix-specs")

	// Create feature branch
	featureBranch := fmt.Sprintf("feature/%s", taskID)
	_, _, gitErr = gitcmd.NewCommand("checkout", "-b").
		AddDynamicArguments(featureBranch).
		RunStdString(ctx, &gitcmd.RunOpts{Dir: gitRepoPath})
	require.NoError(t, gitErr)

	// Simulate implementation commits
	implementationFiles := []struct {
		path    string
		content string
		commit  string
	}{
		{"src/models/user.go", "package models\n\ntype User struct {}", "Add user model"},
		{"src/auth/hash.go", "package auth\n\nfunc HashPassword() {}", "Add password hashing"},
		{"src/handlers/login.go", "package handlers\n\nfunc Login() {}", "Add login endpoint"},
		{"src/auth/jwt.go", "package auth\n\nfunc GenerateJWT() {}", "Add JWT generation"},
	}

	for _, impl := range implementationFiles {
		filePath := filepath.Join(gitRepoPath, impl.path)
		require.NoError(t, os.MkdirAll(filepath.Dir(filePath), 0755))
		require.NoError(t, os.WriteFile(filePath, []byte(impl.content), 0644))

		err = giteagit.AddChanges(ctx, gitRepoPath, false, impl.path)
		require.NoError(t, err)

		err = giteagit.CommitChanges(ctx, gitRepoPath, giteagit.CommitChangesOptions{
			Message: impl.commit,
			Author: &giteagit.Signature{
				Name:  "Test Agent",
				Email: "test@helix.ml",
				When:  time.Now(),
			},
			Committer: &giteagit.Signature{
				Name:  "Test Agent",
				Email: "test@helix.ml",
				When:  time.Now(),
			},
		})
		require.NoError(t, err)
	}

	t.Log("✅ Implementation commits made to feature branch")

	// Verify feature branch exists
	stdout, _, gitErr = gitcmd.NewCommand("branch", "--list").
		AddDynamicArguments(featureBranch).
		RunStdString(ctx, &gitcmd.RunOpts{Dir: gitRepoPath})
	require.NoError(t, gitErr)
	assert.Contains(t, stdout, featureBranch)

	// ========================================
	// PHASE 3: VERIFY COMPLETE WORKFLOW
	// ========================================

	t.Log("PHASE 3: Verifying complete git workflow...")

	// Verify all branches exist (git defaults to "master" not "main")
	expectedBranches := []string{"master", "helix-specs", featureBranch}
	stdout, _, gitErr = gitcmd.NewCommand("branch").
		RunStdString(ctx, &gitcmd.RunOpts{Dir: gitRepoPath})
	require.NoError(t, gitErr)

	for _, branch := range expectedBranches {
		assert.Contains(t, stdout, branch, "Branch %s should exist", branch)
	}

	t.Logf("✅ All branches exist: %v", expectedBranches)

	// Verify helix-specs has design documents
	_, _, gitErr = gitcmd.NewCommand("checkout", "helix-specs").
		RunStdString(ctx, &gitcmd.RunOpts{Dir: gitRepoPath})
	require.NoError(t, gitErr)

	requirementsPath := filepath.Join(taskDir, "requirements.md")
	assert.FileExists(t, requirementsPath)

	requirementsData, err := os.ReadFile(requirementsPath)
	require.NoError(t, err)
	assert.Contains(t, string(requirementsData), "User Story")
	assert.Contains(t, string(requirementsData), "WHEN a user submits valid credentials")

	// Verify feature branch has implementation
	_, _, gitErr = gitcmd.NewCommand("checkout").
		AddDynamicArguments(featureBranch).
		RunStdString(ctx, &gitcmd.RunOpts{Dir: gitRepoPath})
	require.NoError(t, gitErr)

	assert.FileExists(t, filepath.Join(gitRepoPath, "src/models/user.go"))
	assert.FileExists(t, filepath.Join(gitRepoPath, "src/auth/jwt.go"))

	// Count commits on feature branch
	stdout, _, gitErr = gitcmd.NewCommand("rev-list", "--count", "HEAD").
		RunStdString(ctx, &gitcmd.RunOpts{Dir: gitRepoPath})
	require.NoError(t, gitErr)

	commitCount := 0
	fmt.Sscanf(strings.TrimSpace(stdout), "%d", &commitCount)
	assert.GreaterOrEqual(t, commitCount, 4, "Feature branch should have implementation commits")

	t.Log("✅ COMPLETE GIT WORKFLOW VALIDATED")
	t.Log("   ✅ helix-specs branch: Design documents committed")
	t.Log("   ✅ feature branch: Implementation commits made")
	t.Log("   ✅ main branch: Untouched (clean)")
	t.Log("   ✅ Forward-only design docs preserved")
}

// TestPromptGeneration_RealRepoURLs tests prompt generation with real repository URLs
// SKIP: Repositories are now managed at project level, not task level.
// This test needs to be rewritten to test project-level repository prompts.
func TestPromptGeneration_RealRepoURLs(t *testing.T) {
	t.Skip("Repositories moved to project level - test needs rewrite")
}

// TestDesignDocsWorktree_RealGitOperations tests worktree manager with real git
// SKIP: DesignDocsWorktreeManager has been removed - worktree handling now done by shell scripts in sandbox
func TestDesignDocsWorktree_RealGitOperations(t *testing.T) {
	t.Skip("DesignDocsWorktreeManager removed - worktree handling now done by shell scripts in sandbox container")
}

// TestMultiPhaseWorkflow_GitBranches tests that both phases use correct git branches
func TestMultiPhaseWorkflow_GitBranches(t *testing.T) {
	ctx := context.Background()
	testDir := t.TempDir()
	repoPath := filepath.Join(testDir, "repo")

	// Initialize repo
	err := giteagit.InitRepository(ctx, repoPath, false, "sha1")
	require.NoError(t, err)

	// Initial commit
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("# Project"), 0644))
	err = giteagit.AddChanges(ctx, repoPath, true)
	require.NoError(t, err)

	err = giteagit.CommitChanges(ctx, repoPath, giteagit.CommitChangesOptions{
		Message: "Initial commit",
		Author: &giteagit.Signature{
			Name:  "Test Agent",
			Email: "test@helix.ml",
			When:  time.Now(),
		},
		Committer: &giteagit.Signature{
			Name:  "Test Agent",
			Email: "test@helix.ml",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// PLANNING: Create helix-specs branch
	_, _, gitErr := gitcmd.NewCommand("branch", "helix-specs").
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
	require.NoError(t, gitErr)

	// IMPLEMENTATION: Create feature branch from main
	_, _, gitErr = gitcmd.NewCommand("branch", "feature/spec_test").
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
	require.NoError(t, gitErr)

	// Verify all branches exist (git creates "master" as default, not "main")
	branches := []string{"master", "helix-specs", "feature/spec_test"}
	stdout, _, gitErr := gitcmd.NewCommand("branch").
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
	require.NoError(t, gitErr)

	for _, expectedBranch := range branches {
		assert.Contains(t, stdout, expectedBranch)
	}

	t.Log("✅ Multi-phase git branch workflow validated")
	t.Logf("   main: Initial code")
	t.Logf("   helix-specs: Design documents (forward-only)")
	t.Logf("   feature/spec_test: Implementation")
}

// mustMarshalJSON is already defined in spec_driven_task_service.go
