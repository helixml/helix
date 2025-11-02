package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// ProjectInternalRepoService manages internal Git repositories for projects
type ProjectInternalRepoService struct {
	basePath string // Base path for all project repos (e.g., /opt/helix/filestore/projects)
}

// NewProjectInternalRepoService creates a new project internal repo service
func NewProjectInternalRepoService(basePath string) *ProjectInternalRepoService {
	return &ProjectInternalRepoService{
		basePath: basePath,
	}
}

// ProjectConfig represents the .helix/project.json configuration file
type ProjectConfig struct {
	ProjectID     string            `json:"project_id"`
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	Technologies  []string          `json:"technologies,omitempty"`
	DefaultRepoID string            `json:"default_repo_id,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

// InitializeProjectRepo creates a new internal Git repository for a project
func (s *ProjectInternalRepoService) InitializeProjectRepo(ctx context.Context, project *types.Project) (string, error) {
	// Create project directory
	projectDir := filepath.Join(s.basePath, project.ID)
	repoPath := filepath.Join(projectDir, "repo")

	log.Info().
		Str("project_id", project.ID).
		Str("repo_path", repoPath).
		Msg("Initializing internal project Git repository")

	// Create directory
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create project directory: %w", err)
	}

	// Initialize Git repository
	repo, err := git.PlainInit(repoPath, false)
	if err != nil {
		return "", fmt.Errorf("failed to initialize git repository: %w", err)
	}

	// Create .helix directory structure
	helixDir := filepath.Join(repoPath, ".helix")
	if err := os.MkdirAll(filepath.Join(helixDir, "tasks"), 0755); err != nil {
		return "", fmt.Errorf("failed to create .helix/tasks directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(helixDir, "design-docs"), 0755); err != nil {
		return "", fmt.Errorf("failed to create .helix/design-docs directory: %w", err)
	}

	// Create project.json
	projectConfig := ProjectConfig{
		ProjectID:    project.ID,
		Name:         project.Name,
		Description:  project.Description,
		Technologies: project.Technologies,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	projectConfigData, err := json.MarshalIndent(projectConfig, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal project config: %w", err)
	}

	projectConfigPath := filepath.Join(helixDir, "project.json")
	if err := os.WriteFile(projectConfigPath, projectConfigData, 0644); err != nil {
		return "", fmt.Errorf("failed to write project.json: %w", err)
	}

	// Create startup.sh
	startupScriptPath := filepath.Join(helixDir, "startup.sh")
	startupScriptContent := project.StartupScript
	if startupScriptContent == "" {
		startupScriptContent = `#!/bin/bash
# Project startup script
# This runs when agents start working on this project

echo "🚀 Starting project: ` + project.Name + `"

# Add your setup commands here:
# - Install dependencies (npm install, pip install, etc.)
# - Start dev servers
# - Run migrations
# - Set up environment

echo "✅ Project startup complete"
`
	}

	if err := os.WriteFile(startupScriptPath, []byte(startupScriptContent), 0755); err != nil {
		return "", fmt.Errorf("failed to write startup.sh: %w", err)
	}

	// Create README.md
	readmePath := filepath.Join(helixDir, "README.md")
	readmeContent := fmt.Sprintf("# %s\n\n%s\n\n## Project Structure\n\n"+
		"- .helix/project.json - Project metadata and configuration\n"+
		"- .helix/startup.sh - Startup script executed when agents start\n"+
		"- .helix/tasks/ - Task definitions (markdown files)\n"+
		"- .helix/design-docs/ - Design documents from spec generation\n\n"+
		"## Getting Started\n\n"+
		"This is a Helix project with automated agent support. The startup script runs\n"+
		"automatically when agents begin work on this project.\n\n"+
		"## Tasks\n\n"+
		"Tasks are managed through the Helix UI and stored in the .helix/tasks/ directory.\n",
		project.Name, project.Description)

	if err := os.WriteFile(readmePath, []byte(readmeContent), 0644); err != nil {
		return "", fmt.Errorf("failed to write README.md: %w", err)
	}

	// Create tasks/.gitkeep
	tasksGitkeepPath := filepath.Join(helixDir, "tasks", ".gitkeep")
	if err := os.WriteFile(tasksGitkeepPath, []byte(""), 0644); err != nil {
		return "", fmt.Errorf("failed to write tasks/.gitkeep: %w", err)
	}

	// Create design-docs/.gitkeep
	designDocsGitkeepPath := filepath.Join(helixDir, "design-docs", ".gitkeep")
	if err := os.WriteFile(designDocsGitkeepPath, []byte(""), 0644); err != nil {
		return "", fmt.Errorf("failed to write design-docs/.gitkeep: %w", err)
	}

	// Commit initial structure
	worktree, err := repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("failed to get worktree: %w", err)
	}

	// Add all files
	if _, err := worktree.Add(".helix"); err != nil {
		return "", fmt.Errorf("failed to add .helix to git: %w", err)
	}

	// Commit
	commitMsg := fmt.Sprintf("Initialize Helix project: %s\n\nAuto-generated project structure with config, startup script, and templates.", project.Name)
	_, err = worktree.Commit(commitMsg, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Helix System",
			Email: "system@helix.ml",
			When:  time.Now(),
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to commit initial structure: %w", err)
	}

	log.Info().
		Str("project_id", project.ID).
		Str("repo_path", repoPath).
		Msg("Successfully initialized project internal repository")

	return repoPath, nil
}

// LoadStartupScript loads the startup script from the internal repo
func (s *ProjectInternalRepoService) LoadStartupScript(projectID string, internalRepoPath string) (string, error) {
	if internalRepoPath == "" {
		return "", fmt.Errorf("internal repo path not set for project")
	}

	scriptPath := filepath.Join(internalRepoPath, ".helix", "startup.sh")
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		return "", fmt.Errorf("failed to read startup script: %w", err)
	}

	return string(data), nil
}

// SaveStartupScript saves the startup script to the internal repo and commits it
func (s *ProjectInternalRepoService) SaveStartupScript(projectID string, internalRepoPath string, script string) error {
	if internalRepoPath == "" {
		return fmt.Errorf("internal repo path not set for project")
	}

	scriptPath := filepath.Join(internalRepoPath, ".helix", "startup.sh")

	// Write the script
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return fmt.Errorf("failed to write startup script: %w", err)
	}

	// Commit the change
	repo, err := git.PlainOpen(internalRepoPath)
	if err != nil {
		return fmt.Errorf("failed to open git repository: %w", err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Add the file
	if _, err := worktree.Add(".helix/startup.sh"); err != nil {
		return fmt.Errorf("failed to add startup script to git: %w", err)
	}

	// Commit
	commitMsg := fmt.Sprintf("Update startup script\n\nModified via Helix UI at %s", time.Now().Format(time.RFC3339))
	_, err = worktree.Commit(commitMsg, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Helix User",
			Email: "user@helix.ml",
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to commit startup script: %w", err)
	}

	log.Info().
		Str("project_id", projectID).
		Msg("Startup script saved and committed to internal repo")

	return nil
}

// UpdateProjectConfig updates the project.json file in the internal repo
func (s *ProjectInternalRepoService) UpdateProjectConfig(project *types.Project) error {
	if project.InternalRepoPath == "" {
		return fmt.Errorf("internal repo path not set for project")
	}

	configPath := filepath.Join(project.InternalRepoPath, ".helix", "project.json")

	projectConfig := ProjectConfig{
		ProjectID:     project.ID,
		Name:          project.Name,
		Description:   project.Description,
		Technologies:  project.Technologies,
		DefaultRepoID: project.DefaultRepoID,
		CreatedAt:     project.CreatedAt,
		UpdatedAt:     time.Now(),
	}

	projectConfigData, err := json.MarshalIndent(projectConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal project config: %w", err)
	}

	if err := os.WriteFile(configPath, projectConfigData, 0644); err != nil {
		return fmt.Errorf("failed to write project.json: %w", err)
	}

	// Commit the change
	repo, err := git.PlainOpen(project.InternalRepoPath)
	if err != nil {
		return fmt.Errorf("failed to open git repository: %w", err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Add the file
	if _, err := worktree.Add(".helix/project.json"); err != nil {
		return fmt.Errorf("failed to add project.json to git: %w", err)
	}

	// Commit
	commitMsg := fmt.Sprintf("Update project configuration\n\nModified via Helix UI at %s", time.Now().Format(time.RFC3339))
	_, err = worktree.Commit(commitMsg, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Helix System",
			Email: "system@helix.ml",
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to commit project config: %w", err)
	}

	log.Info().
		Str("project_id", project.ID).
		Msg("Project config updated and committed to internal repo")

	return nil
}

// CloneSampleProject clones a sample project repository into the project's internal repo
func (s *ProjectInternalRepoService) CloneSampleProject(ctx context.Context, project *types.Project, sampleRepoURL string) (string, error) {
	projectDir := filepath.Join(s.basePath, project.ID)
	repoPath := filepath.Join(projectDir, "repo")

	log.Info().
		Str("project_id", project.ID).
		Str("sample_url", sampleRepoURL).
		Str("repo_path", repoPath).
		Msg("Cloning sample project into internal repository")

	// Create directory
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create project directory: %w", err)
	}

	// Clone the sample repository
	_, err := git.PlainClone(repoPath, false, &git.CloneOptions{
		URL:      sampleRepoURL,
		Progress: os.Stdout,
		Depth:    1, // Shallow clone for speed
	})
	if err != nil {
		return "", fmt.Errorf("failed to clone sample repository: %w", err)
	}

	// Ensure .helix directory exists
	helixDir := filepath.Join(repoPath, ".helix")
	if err := os.MkdirAll(filepath.Join(helixDir, "tasks"), 0755); err != nil {
		return "", fmt.Errorf("failed to create .helix/tasks directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(helixDir, "design-docs"), 0755); err != nil {
		return "", fmt.Errorf("failed to create .helix/design-docs directory: %w", err)
	}

	// Create/update project.json
	projectConfig := ProjectConfig{
		ProjectID:    project.ID,
		Name:         project.Name,
		Description:  project.Description,
		Technologies: project.Technologies,
		CreatedAt:    project.CreatedAt,
		UpdatedAt:    time.Now(),
	}

	projectConfigData, err := json.MarshalIndent(projectConfig, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal project config: %w", err)
	}

	projectConfigPath := filepath.Join(helixDir, "project.json")
	if err := os.WriteFile(projectConfigPath, projectConfigData, 0644); err != nil {
		return "", fmt.Errorf("failed to write project.json: %w", err)
	}

	// Commit .helix structure
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return "", fmt.Errorf("failed to open git repository: %w", err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("failed to get worktree: %w", err)
	}

	// Add .helix directory
	if _, err := worktree.Add(".helix"); err != nil {
		return "", fmt.Errorf("failed to add .helix to git: %w", err)
	}

	// Commit
	commitMsg := fmt.Sprintf("Add Helix project structure\n\nInitialized from sample project")
	_, err = worktree.Commit(commitMsg, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Helix System",
			Email: "system@helix.ml",
			When:  time.Now(),
		},
	})
	if err != nil {
		// Ignore error if nothing to commit
		log.Debug().Err(err).Msg("No changes to commit (may already exist)")
	}

	log.Info().
		Str("project_id", project.ID).
		Str("repo_path", repoPath).
		Msg("Successfully cloned sample project and initialized Helix structure")

	return repoPath, nil
}

// GetInternalRepoPath returns the expected path for a project's internal repo
func (s *ProjectInternalRepoService) GetInternalRepoPath(projectID string) string {
	return filepath.Join(s.basePath, projectID, "repo")
}
