package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// ProjectInternalRepoService manages internal Git repositories for projects
type ProjectInternalRepoService struct {
	basePath           string // Base path for all project repos (e.g., /opt/helix/filestore/projects)
	sampleCodeService  *SampleProjectCodeService
}

// NewProjectInternalRepoService creates a new project internal repo service
func NewProjectInternalRepoService(basePath string) *ProjectInternalRepoService {
	return &ProjectInternalRepoService{
		basePath:          basePath,
		sampleCodeService: NewSampleProjectCodeService(),
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

	// Create directory for bare repo
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create project directory: %w", err)
	}

	// Initialize bare Git repository (empty for now)
	// All filestore repos are bare so agents can push to them
	_, err := git.PlainInit(repoPath, true)
	if err != nil {
		return "", fmt.Errorf("failed to initialize bare git repository: %w", err)
	}

	// Create temp working repo to add initial files
	// We can't clone the bare repo yet - it's empty!
	tempClone, err := os.MkdirTemp("", "helix-repo-init-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempClone)

	// Initialize temp repo as non-bare
	tempRepo, err := git.PlainInit(tempClone, false)
	if err != nil {
		return "", fmt.Errorf("failed to initialize temp repository: %w", err)
	}

	// Create .helix directory structure in temp clone
	helixDir := filepath.Join(tempClone, ".helix")
	if err := os.MkdirAll(helixDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create .helix directory: %w", err)
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
set -euo pipefail

# Project startup script
# This runs when agents start working on this project

echo "🚀 Starting project: ` + project.Name + `"

# Add your setup commands here:
# Example: sudo apt-get install -y package-name
# Example: npm install
# Example: pip install -r requirements.txt

echo "✅ Project startup complete"
`
	}

	if err := os.WriteFile(startupScriptPath, []byte(startupScriptContent), 0755); err != nil {
		return "", fmt.Errorf("failed to write startup.sh: %w", err)
	}

	// Create README.md (generic - explains internal repo structure, not project-specific)
	readmePath := filepath.Join(helixDir, "README.md")
	readmeContent := `# Helix Internal Project Repository

This is the internal configuration repository for a Helix project.
It is mounted read-only at ` + "`/home/retro/work/.helix-project/`" + ` in agent workspaces.

## Structure

- ` + "`project.json`" + ` - Project metadata (ID, name, description, technologies)
- ` + "`startup.sh`" + ` - Startup script that runs when agents begin work

## Notes

- Tasks are stored in the database, not in files
- Design documents are stored in the code repository's ` + "`helix-specs`" + ` branch
- This repo is read-only in agent workspaces to prevent accidental modifications
- Project settings are managed through the Helix web UI
`

	if err := os.WriteFile(readmePath, []byte(readmeContent), 0644); err != nil {
		return "", fmt.Errorf("failed to write README.md: %w", err)
	}

	// Add remote pointing to bare repo
	_, err = tempRepo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{repoPath}, // Point to bare repo
	})
	if err != nil {
		return "", fmt.Errorf("failed to create remote: %w", err)
	}

	// Commit initial structure in temp clone
	worktree, err := tempRepo.Worktree()
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

	// Push to bare repo
	err = tempRepo.Push(&git.PushOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to push to bare repo: %w", err)
	}

	log.Info().
		Str("project_id", project.ID).
		Str("repo_path", repoPath).
		Msg("Successfully initialized project internal repository")

	return repoPath, nil
}

// LoadStartupScript loads the startup script from the internal repo (bare)
// Reads directly from git object database without needing a working tree
func (s *ProjectInternalRepoService) LoadStartupScript(projectID string, internalRepoPath string) (string, error) {
	if internalRepoPath == "" {
		return "", fmt.Errorf("internal repo path not set for project")
	}

	// Open bare repository
	repo, err := git.PlainOpen(internalRepoPath)
	if err != nil {
		return "", fmt.Errorf("failed to open git repository: %w", err)
	}

	// Get HEAD reference
	ref, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD: %w", err)
	}

	// Get commit from HEAD
	commit, err := repo.CommitObject(ref.Hash())
	if err != nil {
		return "", fmt.Errorf("failed to get commit: %w", err)
	}

	// Get tree from commit
	tree, err := commit.Tree()
	if err != nil {
		return "", fmt.Errorf("failed to get tree: %w", err)
	}

	// Get file from tree
	file, err := tree.File(".helix/startup.sh")
	if err != nil {
		return "", fmt.Errorf("failed to get startup script from git tree: %w", err)
	}

	// Get file contents
	content, err := file.Contents()
	if err != nil {
		return "", fmt.Errorf("failed to read file contents: %w", err)
	}

	return content, nil
}

// SaveStartupScript saves the startup script to the internal repo and commits it
func (s *ProjectInternalRepoService) SaveStartupScript(projectID string, internalRepoPath string, script string) error {
	if internalRepoPath == "" {
		return fmt.Errorf("internal repo path not set for project")
	}

	// Create temporary clone of bare repo
	tempClone, err := os.MkdirTemp("", "helix-startup-script-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempClone) // Cleanup

	repo, err := git.PlainClone(tempClone, false, &git.CloneOptions{
		URL: internalRepoPath, // Clone from bare repo
	})
	if err != nil {
		return fmt.Errorf("failed to clone internal repo: %w", err)
	}

	// Write the script
	scriptPath := filepath.Join(tempClone, ".helix", "startup.sh")
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0755); err != nil {
		return fmt.Errorf("failed to create .helix directory: %w", err)
	}
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return fmt.Errorf("failed to write startup script: %w", err)
	}

	// Commit the change
	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Add the file
	if _, err := worktree.Add(".helix/startup.sh"); err != nil {
		return fmt.Errorf("failed to add startup script to git: %w", err)
	}

	// Check if there are changes to commit
	status, err := worktree.Status()
	if err != nil {
		return fmt.Errorf("failed to get git status: %w", err)
	}

	if status.IsClean() {
		// No changes - script is identical to what's already committed
		log.Debug().
			Str("project_id", projectID).
			Msg("Startup script unchanged, skipping commit")
		return nil
	}

	// Commit the changes
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

	// Push to bare repo
	err = repo.Push(&git.PushOptions{})
	if err != nil {
		return fmt.Errorf("failed to push to bare repo: %w", err)
	}

	log.Info().
		Str("project_id", projectID).
		Msg("Startup script saved and pushed to bare internal repo")

	return nil
}

// UpdateProjectConfig updates the project.json file in the internal repo
func (s *ProjectInternalRepoService) UpdateProjectConfig(project *types.Project) error {
	if project.InternalRepoPath == "" {
		return fmt.Errorf("internal repo path not set for project")
	}

	// Create temporary clone of bare repo
	tempClone, err := os.MkdirTemp("", "helix-project-config-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempClone) // Cleanup

	repo, err := git.PlainClone(tempClone, false, &git.CloneOptions{
		URL: project.InternalRepoPath, // Clone from bare repo
	})
	if err != nil {
		return fmt.Errorf("failed to clone internal repo: %w", err)
	}

	// Prepare project config
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

	// Write project.json
	configPath := filepath.Join(tempClone, ".helix", "project.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("failed to create .helix directory: %w", err)
	}
	if err := os.WriteFile(configPath, projectConfigData, 0644); err != nil {
		return fmt.Errorf("failed to write project.json: %w", err)
	}

	// Commit the change
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

	// Push to bare repo
	err = repo.Push(&git.PushOptions{})
	if err != nil {
		return fmt.Errorf("failed to push to bare repo: %w", err)
	}

	log.Info().
		Str("project_id", project.ID).
		Msg("Project config updated and pushed to bare internal repo")

	return nil
}

// InitializeCodeRepoFromSample creates a separate code repository with sample code
// Returns the repo ID and path for the caller to create a store.GitRepository entry
func (s *ProjectInternalRepoService) InitializeCodeRepoFromSample(ctx context.Context, project *types.Project, sampleID string) (repoID string, repoPath string, err error) {
	// Get sample code
	sampleCode, err := s.sampleCodeService.GetProjectCode(ctx, sampleID)
	if err != nil {
		return "", "", fmt.Errorf("failed to get sample code: %w", err)
	}

	// Create code repo in /filestore/git-repositories/ (standard location)
	// Include sample ID to ensure uniqueness when multiple repos are created for one project
	repoID = fmt.Sprintf("%s-%s", project.ID, sampleID)
	repoPath = filepath.Join(s.basePath, "..", "git-repositories", repoID)

	log.Info().
		Str("project_id", project.ID).
		Str("sample_id", sampleID).
		Str("repo_path", repoPath).
		Msg("Creating code repository from hardcoded sample")

	// Create directory for bare repo
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		return "", "", fmt.Errorf("failed to create code repo directory: %w", err)
	}

	// Initialize bare Git repository (empty for now)
	_, err = git.PlainInit(repoPath, true)
	if err != nil {
		return "", "", fmt.Errorf("failed to initialize bare git repository: %w", err)
	}

	// Create temp working repo to add sample files
	// We can't clone the bare repo yet - it's empty!
	tempClone, err := os.MkdirTemp("", "helix-sample-code-*")
	if err != nil {
		return "", "", fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempClone)

	// Initialize temp repo as non-bare
	repo, err := git.PlainInit(tempClone, false)
	if err != nil {
		return "", "", fmt.Errorf("failed to initialize temp repository: %w", err)
	}

	// Add remote pointing to bare repo
	_, err = repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{repoPath}, // Point to bare repo
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to create remote: %w", err)
	}

	// Write all sample files to temp clone
	allFiles, err := s.sampleCodeService.GetProjectCodeArchive(ctx, sampleID)
	if err != nil {
		return "", "", fmt.Errorf("failed to get sample files: %w", err)
	}

	for filePath, content := range allFiles {
		fullPath := filepath.Join(tempClone, filePath)

		// Create parent directories
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return "", "", fmt.Errorf("failed to create directory for %s: %w", filePath, err)
		}

		// Write file
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			return "", "", fmt.Errorf("failed to write file %s: %w", filePath, err)
		}
	}

	// Commit all files
	worktree, err := repo.Worktree()
	if err != nil {
		return "", "", fmt.Errorf("failed to get worktree: %w", err)
	}

	// Add all files
	if _, err := worktree.Add("."); err != nil {
		return "", "", fmt.Errorf("failed to add files to git: %w", err)
	}

	// Commit
	commitMsg := fmt.Sprintf("Initial commit: %s\n\nSample: %s", project.Name, sampleCode.Name)
	_, err = worktree.Commit(commitMsg, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Helix System",
			Email: "system@helix.ml",
			When:  time.Now(),
		},
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to commit initial structure: %w", err)
	}

	// Push main branch to bare repo
	err = repo.Push(&git.PushOptions{})
	if err != nil {
		return "", "", fmt.Errorf("failed to push to bare repo: %w", err)
	}

	// Create helix-specs as an ORPHAN branch (empty, no code files)
	// This branch is exclusively for design documents, separate from the code
	designDocsBranchRef := plumbing.NewBranchReferenceName("helix-specs")

	// Create an empty tree (no files)
	emptyTree := object.Tree{}
	emptyTreeObj := repo.Storer.NewEncodedObject()
	if err := emptyTree.Encode(emptyTreeObj); err != nil {
		log.Warn().Err(err).Msg("Failed to create empty tree for helix-specs (continuing)")
	} else {
		emptyTreeHash, err := repo.Storer.SetEncodedObject(emptyTreeObj)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to store empty tree (continuing)")
		} else {
			// Create initial commit with empty tree (orphan branch start)
			designDocsCommit := &object.Commit{
				Author: object.Signature{
					Name:  "Helix System",
					Email: "system@helix.ml",
					When:  time.Now(),
				},
				Committer: object.Signature{
					Name:  "Helix System",
					Email: "system@helix.ml",
					When:  time.Now(),
				},
				Message:  "Initialize helix-specs branch\n\nOrphan branch for SpecTask design documents only.",
				TreeHash: emptyTreeHash,
			}

			commitObj := repo.Storer.NewEncodedObject()
			if err := designDocsCommit.Encode(commitObj); err != nil {
				log.Warn().Err(err).Msg("Failed to encode design docs commit (continuing)")
			} else {
				commitHash, err := repo.Storer.SetEncodedObject(commitObj)
				if err != nil {
					log.Warn().Err(err).Msg("Failed to store design docs commit (continuing)")
				} else {
					// Set branch to point to the orphan commit
					err = repo.Storer.SetReference(plumbing.NewHashReference(designDocsBranchRef, commitHash))
					if err != nil {
						log.Warn().Err(err).Msg("Failed to create helix-specs branch reference (continuing)")
					} else {
						// Push helix-specs branch to bare repo
						err = repo.Push(&git.PushOptions{
							RefSpecs: []config.RefSpec{"refs/heads/helix-specs:refs/heads/helix-specs"},
						})
						if err != nil && err != git.NoErrAlreadyUpToDate {
							log.Warn().Err(err).Msg("Failed to push helix-specs branch (continuing)")
						} else {
							log.Info().Msg("Created orphan helix-specs branch (empty, no code files)")
						}
					}
				}
			}
		}
	}

	log.Info().
		Str("project_id", project.ID).
		Str("sample_id", sampleID).
		Str("repo_id", repoID).
		Str("repo_path", repoPath).
		Int("files_created", len(allFiles)).
		Msg("Successfully created code repository from hardcoded sample with helix-specs branch")

	return repoID, repoPath, nil
}

// CloneSampleProject clones a sample project repository into the project's internal repo
// For helix-blog-posts, this clones the real HelixML/helix GitHub repo
func (s *ProjectInternalRepoService) CloneSampleProject(ctx context.Context, project *types.Project, sampleRepoURL string) (string, error) {
	projectDir := filepath.Join(s.basePath, project.ID)
	repoPath := filepath.Join(projectDir, "repo")

	log.Info().
		Str("project_id", project.ID).
		Str("sample_url", sampleRepoURL).
		Str("repo_path", repoPath).
		Msg("Cloning sample project repository into internal repository")

	// Create directory for bare repo
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create project directory: %w", err)
	}

	// Initialize bare repository first
	_, err := git.PlainInit(repoPath, true)
	if err != nil {
		return "", fmt.Errorf("failed to initialize bare repository: %w", err)
	}

	// Clone the sample repository to temp location
	tempClone, err := os.MkdirTemp("", "helix-sample-clone-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempClone)

	repo, err := git.PlainClone(tempClone, false, &git.CloneOptions{
		URL:      sampleRepoURL,
		Progress: os.Stdout,
		Depth:    1, // Shallow clone for speed
	})
	if err != nil {
		return "", fmt.Errorf("failed to clone sample repository: %w", err)
	}

	// Ensure .helix directory exists in temp clone
	helixDir := filepath.Join(tempClone, ".helix")
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

	// Change origin remote to point to our bare repo instead of GitHub
	err = repo.DeleteRemote("origin")
	if err != nil {
		log.Warn().Err(err).Msg("Failed to delete origin remote (continuing)")
	}

	_, err = repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{repoPath}, // Point to our bare repo
	})
	if err != nil {
		return "", fmt.Errorf("failed to create origin remote: %w", err)
	}

	// Commit .helix structure in temp clone
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

	// Push everything to bare repo
	err = repo.Push(&git.PushOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{"refs/heads/*:refs/heads/*"}, // Push all branches
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return "", fmt.Errorf("failed to push to bare repo: %w", err)
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

// StartupScriptVersion represents a version of the startup script from git history
type StartupScriptVersion struct {
	CommitHash string    `json:"commit_hash"`
	Content    string    `json:"content"`
	Timestamp  time.Time `json:"timestamp"`
	Author     string    `json:"author"`
	Message    string    `json:"message"`
}

// GetStartupScriptHistory returns git commit history for the startup script
func (s *ProjectInternalRepoService) GetStartupScriptHistory(internalRepoPath string) ([]StartupScriptVersion, error) {
	if internalRepoPath == "" {
		return nil, fmt.Errorf("internal repo path not set")
	}

	repo, err := git.PlainOpen(internalRepoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open git repository: %w", err)
	}

	// Get commit history for .helix/startup.sh
	filePath := ".helix/startup.sh"
	commitIter, err := repo.Log(&git.LogOptions{
		FileName: &filePath,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get git log: %w", err)
	}

	var versions []StartupScriptVersion
	err = commitIter.ForEach(func(commit *object.Commit) error {
		// Get file content at this commit
		tree, err := commit.Tree()
		if err != nil {
			log.Warn().Err(err).Str("commit", commit.Hash.String()).Msg("Failed to get tree")
			return nil // Skip this commit
		}

		file, err := tree.File(".helix/startup.sh")
		if err != nil {
			log.Warn().Err(err).Str("commit", commit.Hash.String()).Msg("Failed to get file from tree")
			return nil // Skip this commit
		}

		content, err := file.Contents()
		if err != nil {
			log.Warn().Err(err).Str("commit", commit.Hash.String()).Msg("Failed to get file contents")
			return nil // Skip this commit
		}

		versions = append(versions, StartupScriptVersion{
			CommitHash: commit.Hash.String(),
			Content:    content,
			Timestamp:  commit.Author.When,
			Author:     commit.Author.Name,
			Message:    commit.Message,
		})

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to iterate commits: %w", err)
	}

	return versions, nil
}
