package services

import (
	"context"
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

// ProjectRepoService manages Git repositories for projects
// Startup scripts are stored in the primary CODE repository at .helix/startup.sh
type ProjectRepoService struct {
	basePath          string // Base path for all project repos (e.g., /opt/helix/filestore/projects)
	sampleCodeService *SampleProjectCodeService
}

// NewProjectRepoService creates a new project repo service
func NewProjectRepoService(basePath string) *ProjectRepoService {
	return &ProjectRepoService{
		basePath:          basePath,
		sampleCodeService: NewSampleProjectCodeService(),
	}
}

// StartupScriptVersion represents a version of the startup script from git history
type StartupScriptVersion struct {
	CommitHash string    `json:"commit_hash"`
	Content    string    `json:"content"`
	Timestamp  time.Time `json:"timestamp"`
	Author     string    `json:"author"`
	Message    string    `json:"message"`
}

// LoadStartupScriptFromCodeRepo loads the startup script from a code repository
// Startup script lives in the primary CODE repo at .helix/startup.sh
func (s *ProjectRepoService) LoadStartupScriptFromCodeRepo(codeRepoPath string) (string, error) {
	if codeRepoPath == "" {
		return "", fmt.Errorf("code repo path not set")
	}

	// Open bare repository
	repo, err := git.PlainOpen(codeRepoPath)
	if err != nil {
		return "", fmt.Errorf("failed to open git repository: %w", err)
	}

	// Get HEAD reference
	ref, err := repo.Head()
	if err != nil {
		// Empty repo or no commits yet - return empty script
		return "", nil
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
		// File doesn't exist yet - return empty script
		return "", nil
	}

	// Get file contents
	content, err := file.Contents()
	if err != nil {
		return "", fmt.Errorf("failed to read file contents: %w", err)
	}

	return content, nil
}

// SaveStartupScriptToCodeRepo saves the startup script to a code repository
// Commits to the default branch (main/master)
// userName and userEmail are required - must be the actual user's credentials for enterprise deployments
func (s *ProjectRepoService) SaveStartupScriptToCodeRepo(codeRepoPath string, script string, userName string, userEmail string) error {
	if userName == "" || userEmail == "" {
		return fmt.Errorf("userName and userEmail are required for commits")
	}
	if codeRepoPath == "" {
		return fmt.Errorf("code repo path not set")
	}

	// Create temporary clone of bare repo
	tempClone, err := os.MkdirTemp("", "helix-startup-script-code-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempClone) // Cleanup

	repo, err := git.PlainClone(tempClone, false, &git.CloneOptions{
		URL: codeRepoPath, // Clone from bare repo
	})
	if err != nil {
		return fmt.Errorf("failed to clone code repo: %w", err)
	}

	// Ensure .helix directory exists
	helixDir := filepath.Join(tempClone, ".helix")
	if err := os.MkdirAll(helixDir, 0755); err != nil {
		return fmt.Errorf("failed to create .helix directory: %w", err)
	}

	// Write the script
	scriptPath := filepath.Join(helixDir, "startup.sh")
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
		log.Debug().Msg("Startup script unchanged, skipping commit")
		return nil
	}

	// Commit the changes
	commitMsg := fmt.Sprintf("Update startup script\n\nModified via Helix UI at %s", time.Now().Format(time.RFC3339))
	_, err = worktree.Commit(commitMsg, &git.CommitOptions{
		Author: &object.Signature{
			Name:  userName,
			Email: userEmail,
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
		Str("code_repo_path", codeRepoPath).
		Msg("Startup script saved to code repo")

	return nil
}

// GetStartupScriptHistoryFromCodeRepo returns git commit history for the startup script in a code repo
func (s *ProjectRepoService) GetStartupScriptHistoryFromCodeRepo(codeRepoPath string) ([]StartupScriptVersion, error) {
	if codeRepoPath == "" {
		return nil, fmt.Errorf("code repo path not set")
	}

	repo, err := git.PlainOpen(codeRepoPath)
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

// InitializeStartupScriptInCodeRepo creates the initial .helix/startup.sh file in helix-specs branch
// This is used when a project is created and a primary repo is selected
// userName and userEmail are required - must be the actual user's credentials for enterprise deployments
func (s *ProjectRepoService) InitializeStartupScriptInCodeRepo(codeRepoPath string, projectName string, startupScript string, userName string, userEmail string) error {
	if userName == "" || userEmail == "" {
		return fmt.Errorf("userName and userEmail are required for commits")
	}
	if codeRepoPath == "" {
		return fmt.Errorf("code repo path not set")
	}

	// Check if startup script already exists in helix-specs branch
	existingScript, err := s.LoadStartupScriptFromHelixSpecs(codeRepoPath)
	if err != nil {
		return fmt.Errorf("failed to check existing startup script: %w", err)
	}

	// If startup script already exists and is not empty, don't overwrite
	if existingScript != "" {
		log.Info().
			Str("code_repo_path", codeRepoPath).
			Msg("Startup script already exists in helix-specs, not overwriting")
		return nil
	}

	// Use default startup script if none provided
	if startupScript == "" {
		startupScript = `#!/bin/bash
set -euo pipefail

# Project startup script
# This runs when agents start working on this project

echo "🚀 Starting project: ` + projectName + `"

# Add your setup commands here:
# Example: sudo apt-get install -y package-name
# Example: npm install
# Example: pip install -r requirements.txt

echo "✅ Project startup complete"
`
	}

	// Save to helix-specs branch (not main) to avoid protected branch issues
	return s.SaveStartupScriptToHelixSpecs(codeRepoPath, startupScript, userName, userEmail)
}

// InitializeCodeRepoFromSample creates a code repository with sample code
// Returns the repo ID and path for the caller to create a store.GitRepository entry
// userName and userEmail are required - must be the actual user's credentials for enterprise deployments
func (s *ProjectRepoService) InitializeCodeRepoFromSample(ctx context.Context, project *types.Project, sampleID string, userName string, userEmail string) (repoID string, repoPath string, err error) {
	if userName == "" || userEmail == "" {
		return "", "", fmt.Errorf("userName and userEmail are required for commits")
	}
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
	commitHash, err := worktree.Commit(commitMsg, &git.CommitOptions{
		Author: &object.Signature{
			Name:  userName,
			Email: userEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to commit initial structure: %w", err)
	}

	// Rename master to main before pushing
	headRef, err := repo.Head()
	if err == nil && headRef.Name().Short() == "master" {
		// Create main branch pointing to same commit
		mainRef := plumbing.NewBranchReferenceName("main")
		if err := repo.Storer.SetReference(plumbing.NewHashReference(mainRef, commitHash)); err != nil {
			log.Warn().Err(err).Msg("Failed to create main branch")
		} else {
			// Set HEAD to main
			newHead := plumbing.NewSymbolicReference(plumbing.HEAD, mainRef)
			if err := repo.Storer.SetReference(newHead); err != nil {
				log.Warn().Err(err).Msg("Failed to set HEAD to main")
			} else {
				// Delete master branch
				if err := repo.Storer.RemoveReference(plumbing.NewBranchReferenceName("master")); err != nil {
					log.Warn().Err(err).Msg("Failed to remove master branch")
				}
				log.Info().Msg("Renamed default branch from master to main")
			}
		}
	}

	// Push main branch to bare repo
	err = repo.Push(&git.PushOptions{
		RefSpecs: []config.RefSpec{"refs/heads/main:refs/heads/main"},
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to push to bare repo: %w", err)
	}

	// Update bare repo: set HEAD to main and delete master branch
	bareRepo, err := git.PlainOpen(repoPath)
	if err == nil {
		// Set HEAD to point to main (not master)
		mainHeadRef := plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("main"))
		if err := bareRepo.Storer.SetReference(mainHeadRef); err != nil {
			log.Warn().Err(err).Msg("Failed to set HEAD to main in bare repo")
		}

		// Delete master branch
		masterRef := plumbing.NewBranchReferenceName("master")
		if err := bareRepo.Storer.RemoveReference(masterRef); err != nil && err != plumbing.ErrReferenceNotFound {
			log.Warn().Err(err).Msg("Failed to remove master branch from bare repo")
		}

		log.Info().Msg("Set bare repo HEAD to main and removed master branch")
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
					Name:  userName,
					Email: userEmail,
					When:  time.Now(),
				},
				Committer: object.Signature{
					Name:  userName,
					Email: userEmail,
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

// CloneSampleProject clones a sample project repository (e.g., from GitHub)
// Returns the path to the cloned bare repository
// userName and userEmail are required - must be the actual user's credentials for enterprise deployments
func (s *ProjectRepoService) CloneSampleProject(ctx context.Context, project *types.Project, sampleRepoURL string, userName string, userEmail string) (string, error) {
	if userName == "" || userEmail == "" {
		return "", fmt.Errorf("userName and userEmail are required for commits")
	}
	// Create code repo in /filestore/git-repositories/ (standard location)
	repoPath := filepath.Join(s.basePath, "..", "git-repositories", fmt.Sprintf("%s-code", project.ID))

	log.Info().
		Str("project_id", project.ID).
		Str("sample_url", sampleRepoURL).
		Str("repo_path", repoPath).
		Msg("Cloning sample project repository")

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
			Name:  userName,
			Email: userEmail,
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

// SaveStartupScriptToHelixSpecs saves the startup script to the helix-specs branch
// This avoids modifying main branch which may be protected on external repos
// userName and userEmail are required - must be the actual user's credentials for enterprise deployments
func (s *ProjectRepoService) SaveStartupScriptToHelixSpecs(codeRepoPath string, script string, userName string, userEmail string) error {
	if userName == "" || userEmail == "" {
		return fmt.Errorf("userName and userEmail are required for commits")
	}
	if codeRepoPath == "" {
		return fmt.Errorf("code repo path not set")
	}

	// Open bare repository
	bareRepo, err := git.PlainOpen(codeRepoPath)
	if err != nil {
		return fmt.Errorf("failed to open bare repository: %w", err)
	}

	// Check if helix-specs branch exists
	helixSpecsRef := plumbing.NewBranchReferenceName("helix-specs")
	_, err = bareRepo.Reference(helixSpecsRef, true)
	if err != nil {
		// Branch doesn't exist - we need to create it as orphan
		log.Info().Str("repo_path", codeRepoPath).Msg("helix-specs branch doesn't exist, creating orphan branch")
		if err := s.createHelixSpecsBranch(bareRepo, userName, userEmail); err != nil {
			return fmt.Errorf("failed to create helix-specs branch: %w", err)
		}
	}

	// Create temporary clone of bare repo, checking out helix-specs branch
	tempClone, err := os.MkdirTemp("", "helix-startup-script-specs-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempClone) // Cleanup

	repo, err := git.PlainClone(tempClone, false, &git.CloneOptions{
		URL:           codeRepoPath, // Clone from bare repo
		ReferenceName: helixSpecsRef,
		SingleBranch:  true,
	})
	if err != nil {
		return fmt.Errorf("failed to clone helix-specs branch: %w", err)
	}

	// Ensure .helix directory exists
	helixDir := filepath.Join(tempClone, ".helix")
	if err := os.MkdirAll(helixDir, 0755); err != nil {
		return fmt.Errorf("failed to create .helix directory: %w", err)
	}

	// Write the script
	scriptPath := filepath.Join(helixDir, "startup.sh")
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
		log.Debug().Msg("Startup script unchanged in helix-specs, skipping commit")
		return nil
	}

	// Commit the changes
	commitMsg := fmt.Sprintf("Update startup script\n\nModified via Helix UI at %s", time.Now().Format(time.RFC3339))
	_, err = worktree.Commit(commitMsg, &git.CommitOptions{
		Author: &object.Signature{
			Name:  userName,
			Email: userEmail,
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
		Str("code_repo_path", codeRepoPath).
		Msg("Startup script saved to helix-specs branch")

	return nil
}

// createHelixSpecsBranch creates an orphan helix-specs branch in the bare repo
func (s *ProjectRepoService) createHelixSpecsBranch(bareRepo *git.Repository, userName string, userEmail string) error {
	// Create an empty tree (no files)
	emptyTree := object.Tree{}
	emptyTreeObj := bareRepo.Storer.NewEncodedObject()
	if err := emptyTree.Encode(emptyTreeObj); err != nil {
		return fmt.Errorf("failed to create empty tree: %w", err)
	}

	emptyTreeHash, err := bareRepo.Storer.SetEncodedObject(emptyTreeObj)
	if err != nil {
		return fmt.Errorf("failed to store empty tree: %w", err)
	}

	// Create initial commit with empty tree (orphan branch start)
	designDocsCommit := &object.Commit{
		Author: object.Signature{
			Name:  userName,
			Email: userEmail,
			When:  time.Now(),
		},
		Committer: object.Signature{
			Name:  userName,
			Email: userEmail,
			When:  time.Now(),
		},
		Message:  "Initialize helix-specs branch\n\nOrphan branch for Helix configuration and SpecTask design documents.",
		TreeHash: emptyTreeHash,
	}

	commitObj := bareRepo.Storer.NewEncodedObject()
	if err := designDocsCommit.Encode(commitObj); err != nil {
		return fmt.Errorf("failed to encode commit: %w", err)
	}

	commitHash, err := bareRepo.Storer.SetEncodedObject(commitObj)
	if err != nil {
		return fmt.Errorf("failed to store commit: %w", err)
	}

	// Set branch to point to the orphan commit
	helixSpecsRef := plumbing.NewBranchReferenceName("helix-specs")
	err = bareRepo.Storer.SetReference(plumbing.NewHashReference(helixSpecsRef, commitHash))
	if err != nil {
		return fmt.Errorf("failed to create helix-specs branch reference: %w", err)
	}

	log.Info().Msg("Created orphan helix-specs branch")
	return nil
}

// LoadStartupScriptFromHelixSpecs loads the startup script from the helix-specs branch
func (s *ProjectRepoService) LoadStartupScriptFromHelixSpecs(codeRepoPath string) (string, error) {
	if codeRepoPath == "" {
		return "", fmt.Errorf("code repo path not set")
	}

	// Open bare repository
	repo, err := git.PlainOpen(codeRepoPath)
	if err != nil {
		return "", fmt.Errorf("failed to open git repository: %w", err)
	}

	// Get helix-specs branch reference
	helixSpecsRef := plumbing.NewBranchReferenceName("helix-specs")
	ref, err := repo.Reference(helixSpecsRef, true)
	if err != nil {
		// helix-specs branch doesn't exist - fall back to main branch (legacy support)
		log.Debug().Str("repo_path", codeRepoPath).Msg("helix-specs branch not found, falling back to main branch")
		return s.LoadStartupScriptFromCodeRepo(codeRepoPath)
	}

	// Get commit from helix-specs
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
		// File doesn't exist yet - try legacy location on main branch
		log.Debug().Str("repo_path", codeRepoPath).Msg("startup.sh not in helix-specs, falling back to main branch")
		return s.LoadStartupScriptFromCodeRepo(codeRepoPath)
	}

	// Get file contents
	content, err := file.Contents()
	if err != nil {
		return "", fmt.Errorf("failed to read file contents: %w", err)
	}

	return content, nil
}

// GetStartupScriptHistoryFromHelixSpecs returns git commit history for the startup script from helix-specs branch
func (s *ProjectRepoService) GetStartupScriptHistoryFromHelixSpecs(codeRepoPath string) ([]StartupScriptVersion, error) {
	if codeRepoPath == "" {
		return nil, fmt.Errorf("code repo path not set")
	}

	repo, err := git.PlainOpen(codeRepoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open git repository: %w", err)
	}

	// Get helix-specs branch reference
	helixSpecsRef := plumbing.NewBranchReferenceName("helix-specs")
	ref, err := repo.Reference(helixSpecsRef, true)
	if err != nil {
		// helix-specs branch doesn't exist - fall back to main branch (legacy support)
		log.Debug().Str("repo_path", codeRepoPath).Msg("helix-specs branch not found for history, falling back to main branch")
		return s.GetStartupScriptHistoryFromCodeRepo(codeRepoPath)
	}

	// Get commit history for .helix/startup.sh starting from helix-specs
	filePath := ".helix/startup.sh"
	commitIter, err := repo.Log(&git.LogOptions{
		From:     ref.Hash(),
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

	// If no versions found in helix-specs, check main branch for legacy scripts
	if len(versions) == 0 {
		log.Debug().Str("repo_path", codeRepoPath).Msg("No startup script history in helix-specs, checking main branch")
		return s.GetStartupScriptHistoryFromCodeRepo(codeRepoPath)
	}

	return versions, nil
}

// Backward compatibility aliases - these will be removed in a future release
// They are kept temporarily for any code that hasn't been updated yet

// ProjectInternalRepoService is an alias for ProjectRepoService (backward compatibility)
type ProjectInternalRepoService = ProjectRepoService

// NewProjectInternalRepoService is an alias for NewProjectRepoService (backward compatibility)
func NewProjectInternalRepoService(basePath string) *ProjectInternalRepoService {
	return NewProjectRepoService(basePath)
}
