package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	giteagit "code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/git/gitcmd"
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

	ctx := context.Background()

	// Open bare repository using gitea's wrapper
	repo, err := giteagit.OpenRepository(ctx, codeRepoPath)
	if err != nil {
		return "", fmt.Errorf("failed to open git repository: %w", err)
	}
	defer repo.Close()

	// Get HEAD branch
	headBranch, err := GetHEADBranch(ctx, codeRepoPath)
	if err != nil {
		// Empty repo or no commits yet - return empty script
		return "", nil
	}

	// Get commit for HEAD branch
	commit, err := repo.GetBranchCommit(headBranch)
	if err != nil {
		return "", fmt.Errorf("failed to get commit: %w", err)
	}

	// Get file content directly from commit
	content, err := commit.GetFileContent(".helix/startup.sh", 0)
	if err != nil {
		// File doesn't exist yet - return empty script
		return "", nil
	}

	return content, nil
}

// GetStartupScriptHistoryFromCodeRepo returns git commit history for the startup script in a code repo
func (s *ProjectRepoService) GetStartupScriptHistoryFromCodeRepo(codeRepoPath string) ([]StartupScriptVersion, error) {
	if codeRepoPath == "" {
		return nil, fmt.Errorf("code repo path not set")
	}

	ctx := context.Background()

	// Open repo to use high-level API
	repo, err := giteagit.OpenRepository(ctx, codeRepoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open git repository: %w", err)
	}
	defer repo.Close()

	// Get HEAD branch
	headBranch, err := GetHEADBranch(ctx, codeRepoPath)
	if err != nil {
		return nil, nil // Empty repo, no commits
	}

	// Use gitea's high-level CommitsByFileAndRange API
	// Page 0 with no limit returns all commits for the file
	commits, err := repo.CommitsByFileAndRange(giteagit.CommitsByFileAndRangeOptions{
		Revision: headBranch,
		File:     ".helix/startup.sh",
		Page:     0, // All commits
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get commits for startup.sh: %w", err)
	}

	var versions []StartupScriptVersion
	for _, commit := range commits {
		content, err := commit.GetFileContent(".helix/startup.sh", 0)
		if err != nil {
			log.Warn().Err(err).Str("commit", commit.ID.String()).Msg("Failed to get file from commit")
			continue
		}

		versions = append(versions, StartupScriptVersion{
			CommitHash: commit.ID.String(),
			Content:    content,
			Timestamp:  commit.Author.When,
			Author:     commit.Author.Name,
			Message:    commit.Summary(),
		})
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
	_, err = s.SaveStartupScriptToHelixSpecs(codeRepoPath, startupScript, userName, userEmail)
	return err
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

	// Initialize bare Git repository using gitea's wrapper
	err = giteagit.InitRepository(ctx, repoPath, true, "sha1")
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

	// Initialize temp repo as non-bare using gitea's wrapper
	err = giteagit.InitRepository(ctx, tempClone, false, "sha1")
	if err != nil {
		return "", "", fmt.Errorf("failed to initialize temp repository: %w", err)
	}

	// Add remote pointing to bare repo
	err = AddRemote(ctx, tempClone, "origin", repoPath)
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

	// Add all files using gitea's wrapper
	if err := giteagit.AddChanges(ctx, tempClone, true); err != nil {
		return "", "", fmt.Errorf("failed to add files to git: %w", err)
	}

	// Commit using gitea's wrapper
	commitMsg := fmt.Sprintf("Initial commit: %s\n\nSample: %s", project.Name, sampleCode.Name)
	if err := giteagit.CommitChanges(ctx, tempClone, giteagit.CommitChangesOptions{
		Committer: &giteagit.Signature{
			Name:  userName,
			Email: userEmail,
			When:  time.Now(),
		},
		Author: &giteagit.Signature{
			Name:  userName,
			Email: userEmail,
			When:  time.Now(),
		},
		Message: commitMsg,
	}); err != nil {
		return "", "", fmt.Errorf("failed to commit initial structure: %w", err)
	}

	// Check if default branch is master and rename to main
	headBranch, err := GetHEADBranch(ctx, tempClone)
	if err == nil && headBranch == "master" {
		if err := GitRenameBranch(ctx, tempClone, "master", "main"); err != nil {
			log.Warn().Err(err).Msg("Failed to rename master to main")
		} else {
			log.Info().Msg("Renamed default branch from master to main")
		}
	}

	// Push main branch to bare repo
	if err := giteagit.Push(ctx, tempClone, giteagit.PushOptions{
		Remote: "origin",
		Branch: "refs/heads/main:refs/heads/main",
	}); err != nil {
		return "", "", fmt.Errorf("failed to push to bare repo: %w", err)
	}

	// Update bare repo: set HEAD to main
	if err := SetHEAD(ctx, repoPath, "main"); err != nil {
		log.Warn().Err(err).Msg("Failed to set HEAD to main in bare repo")
	}

	// Create helix-specs as an ORPHAN branch (empty, no code files)
	// This branch is exclusively for design documents, separate from the code
	if err := s.createOrphanHelixSpecsBranch(ctx, tempClone, repoPath, userName, userEmail); err != nil {
		log.Warn().Err(err).Msg("Failed to create helix-specs branch (continuing)")
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

// createOrphanHelixSpecsBranch creates an orphan helix-specs branch
func (s *ProjectRepoService) createOrphanHelixSpecsBranch(ctx context.Context, tempClone, bareRepoPath, userName, userEmail string) error {
	// Create a new temp directory for the orphan branch
	orphanTemp, err := os.MkdirTemp("", "helix-orphan-*")
	if err != nil {
		return fmt.Errorf("failed to create orphan temp directory: %w", err)
	}
	defer os.RemoveAll(orphanTemp)

	// Initialize a fresh repo
	if err := giteagit.InitRepository(ctx, orphanTemp, false, "sha1"); err != nil {
		return fmt.Errorf("failed to init orphan repo: %w", err)
	}

	// Add remote pointing to bare repo
	if err := AddRemote(ctx, orphanTemp, "origin", bareRepoPath); err != nil {
		return fmt.Errorf("failed to add remote: %w", err)
	}

	// Create a placeholder file so we can make a commit
	placeholderPath := filepath.Join(orphanTemp, ".gitkeep")
	if err := os.WriteFile(placeholderPath, []byte("# Helix specs branch\n"), 0644); err != nil {
		return fmt.Errorf("failed to create placeholder: %w", err)
	}

	// Add and commit
	if err := giteagit.AddChanges(ctx, orphanTemp, true); err != nil {
		return fmt.Errorf("failed to add: %w", err)
	}

	if err := giteagit.CommitChanges(ctx, orphanTemp, giteagit.CommitChangesOptions{
		Committer: &giteagit.Signature{
			Name:  userName,
			Email: userEmail,
			When:  time.Now(),
		},
		Author: &giteagit.Signature{
			Name:  userName,
			Email: userEmail,
			When:  time.Now(),
		},
		Message: "Initialize helix-specs branch\n\nOrphan branch for SpecTask design documents only.",
	}); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	// Rename branch to helix-specs
	headBranch, err := GetHEADBranch(ctx, orphanTemp)
	if err == nil && headBranch != "helix-specs" {
		if err := GitRenameBranch(ctx, orphanTemp, headBranch, "helix-specs"); err != nil {
			return fmt.Errorf("failed to rename to helix-specs: %w", err)
		}
	}

	// Push helix-specs branch to bare repo
	if err := giteagit.Push(ctx, orphanTemp, giteagit.PushOptions{
		Remote: "origin",
		Branch: "refs/heads/helix-specs:refs/heads/helix-specs",
	}); err != nil {
		return fmt.Errorf("failed to push helix-specs: %w", err)
	}

	log.Info().Msg("Created orphan helix-specs branch (empty, no code files)")
	return nil
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

	// Initialize bare repository first using gitea's wrapper
	if err := giteagit.InitRepository(ctx, repoPath, true, "sha1"); err != nil {
		return "", fmt.Errorf("failed to initialize bare repository: %w", err)
	}

	// Clone the sample repository to temp location
	tempClone, err := os.MkdirTemp("", "helix-sample-clone-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempClone)

	// Clone using gitea's wrapper with shallow clone
	if err := giteagit.Clone(ctx, sampleRepoURL, tempClone, giteagit.CloneRepoOptions{
		Depth: 1,
	}); err != nil {
		return "", fmt.Errorf("failed to clone sample repository: %w", err)
	}

	// Ensure .helix directory exists in temp clone
	helixDir := filepath.Join(tempClone, ".helix")
	if err := os.MkdirAll(filepath.Join(helixDir, "tasks"), 0755); err != nil {
		return "", fmt.Errorf("failed to create .helix/tasks directory: %w", err)
	}

	// Remove origin remote and add new one pointing to our bare repo
	_, _, _ = gitcmd.NewCommand("remote", "remove", "origin").
		RunStdString(ctx, &gitcmd.RunOpts{Dir: tempClone})

	if err := AddRemote(ctx, tempClone, "origin", repoPath); err != nil {
		return "", fmt.Errorf("failed to create origin remote: %w", err)
	}

	// Add .helix directory
	if err := giteagit.AddChanges(ctx, tempClone, true); err != nil {
		return "", fmt.Errorf("failed to add .helix to git: %w", err)
	}

	// Commit
	commitMsg := "Add Helix project structure\n\nInitialized from sample project"
	if err := giteagit.CommitChanges(ctx, tempClone, giteagit.CommitChangesOptions{
		Author: &giteagit.Signature{
			Name:  userName,
			Email: userEmail,
			When:  time.Now(),
		},
		Message: commitMsg,
	}); err != nil {
		// Ignore error if nothing to commit
		log.Debug().Err(err).Msg("No changes to commit (may already exist)")
	}

	// Push all branches to bare repo
	if err := giteagit.Push(ctx, tempClone, giteagit.PushOptions{
		Remote: "origin",
		Branch: "refs/heads/*:refs/heads/*",
	}); err != nil {
		log.Warn().Err(err).Msg("Failed to push all branches (may be empty or up-to-date)")
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
// Returns (changed bool, err error) where changed indicates if a commit was made
//
// This uses a temporary working copy to make the commit.
func (s *ProjectRepoService) SaveStartupScriptToHelixSpecs(codeRepoPath string, script string, userName string, userEmail string) (bool, error) {
	if userName == "" || userEmail == "" {
		return false, fmt.Errorf("userName and userEmail are required for commits")
	}
	if codeRepoPath == "" {
		return false, fmt.Errorf("code repo path not set")
	}

	ctx := context.Background()

	// Check if helix-specs branch exists
	branches, err := ListBranches(ctx, codeRepoPath)
	if err != nil {
		return false, fmt.Errorf("failed to list branches: %w", err)
	}

	helixSpecsExists := false
	for _, b := range branches {
		if b == "helix-specs" {
			helixSpecsExists = true
			break
		}
	}

	if !helixSpecsExists {
		// Branch doesn't exist - we need to create it as orphan
		log.Info().Str("repo_path", codeRepoPath).Msg("helix-specs branch doesn't exist, creating orphan branch")
		if err := s.createHelixSpecsBranch(ctx, codeRepoPath, userName, userEmail); err != nil {
			return false, fmt.Errorf("failed to create helix-specs branch: %w", err)
		}
	}

	// Check if script already exists and is unchanged
	existingScript, _ := s.LoadStartupScriptFromHelixSpecs(codeRepoPath)
	if existingScript == script {
		log.Debug().Msg("Startup script unchanged in helix-specs, skipping commit")
		return false, nil
	}

	// Create temp working copy of helix-specs branch
	tempDir, err := os.MkdirTemp("", "helix-specs-save-*")
	if err != nil {
		return false, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Clone the bare repo
	if err := giteagit.Clone(ctx, codeRepoPath, tempDir, giteagit.CloneRepoOptions{
		Branch: "helix-specs",
	}); err != nil {
		return false, fmt.Errorf("failed to clone bare repo: %w", err)
	}

	// Checkout helix-specs branch
	if err := GitCheckout(ctx, tempDir, "helix-specs", false); err != nil {
		return false, fmt.Errorf("failed to checkout helix-specs: %w", err)
	}

	// Write the script file
	helixDir := filepath.Join(tempDir, ".helix")
	if err := os.MkdirAll(helixDir, 0755); err != nil {
		return false, fmt.Errorf("failed to create .helix directory: %w", err)
	}

	scriptPath := filepath.Join(helixDir, "startup.sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		return false, fmt.Errorf("failed to write startup script: %w", err)
	}

	// Add and commit
	if err := giteagit.AddChanges(ctx, tempDir, true); err != nil {
		return false, fmt.Errorf("failed to add changes: %w", err)
	}

	now := time.Now()
	commitMsg := fmt.Sprintf("Update startup script\n\nModified via Helix UI at %s", now.Format(time.RFC3339))
	if err := giteagit.CommitChanges(ctx, tempDir, giteagit.CommitChangesOptions{
		Author: &giteagit.Signature{
			Name:  userName,
			Email: userEmail,
			When:  now,
		},
		Committer: &giteagit.Signature{
			Name:  userName,
			Email: userEmail,
			When:  now,
		},
		Message: commitMsg,
	}); err != nil {
		return false, fmt.Errorf("failed to commit: %w", err)
	}

	// Push to bare repo
	if err := giteagit.Push(ctx, tempDir, giteagit.PushOptions{
		Remote: "origin",
		Branch: "refs/heads/helix-specs:refs/heads/helix-specs",
	}); err != nil {
		return false, fmt.Errorf("failed to push: %w", err)
	}

	// Get commit hash
	commitHash, _ := GetBranchCommitID(ctx, tempDir, "helix-specs")

	log.Info().
		Str("code_repo_path", codeRepoPath).
		Str("commit", commitHash[:8]).
		Msg("Startup script saved to helix-specs branch")

	return true, nil
}

// createHelixSpecsBranch creates an orphan helix-specs branch in the bare repo
func (s *ProjectRepoService) createHelixSpecsBranch(ctx context.Context, bareRepoPath, userName, userEmail string) error {
	// Create a temp working directory
	tempDir, err := os.MkdirTemp("", "helix-specs-create-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Initialize a fresh repo (creates an orphan branch)
	if err := giteagit.InitRepository(ctx, tempDir, false, "sha1"); err != nil {
		return fmt.Errorf("failed to init temp repo: %w", err)
	}

	// Add remote pointing to bare repo
	if err := AddRemote(ctx, tempDir, "origin", bareRepoPath); err != nil {
		return fmt.Errorf("failed to add remote: %w", err)
	}

	// Create a placeholder file
	helixDir := filepath.Join(tempDir, ".helix")
	if err := os.MkdirAll(helixDir, 0755); err != nil {
		return fmt.Errorf("failed to create .helix directory: %w", err)
	}

	gitkeep := filepath.Join(helixDir, ".gitkeep")
	if err := os.WriteFile(gitkeep, []byte("# Helix configuration branch\n"), 0644); err != nil {
		return fmt.Errorf("failed to create .gitkeep: %w", err)
	}

	// Add and commit
	if err := giteagit.AddChanges(ctx, tempDir, true); err != nil {
		return fmt.Errorf("failed to add: %w", err)
	}

	if err := giteagit.CommitChanges(ctx, tempDir, giteagit.CommitChangesOptions{
		Author: &giteagit.Signature{
			Name:  userName,
			Email: userEmail,
			When:  time.Now(),
		},
		Committer: &giteagit.Signature{
			Name:  userName,
			Email: userEmail,
			When:  time.Now(),
		},
		Message: "Initialize helix-specs branch\n\nOrphan branch for Helix configuration and SpecTask design documents.",
	}); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	// Rename branch to helix-specs
	headBranch, err := GetHEADBranch(ctx, tempDir)
	if err == nil && headBranch != "helix-specs" {
		if err := GitRenameBranch(ctx, tempDir, headBranch, "helix-specs"); err != nil {
			return fmt.Errorf("failed to rename to helix-specs: %w", err)
		}
	}

	// Push helix-specs branch to bare repo
	if err := giteagit.Push(ctx, tempDir, giteagit.PushOptions{
		Remote: "origin",
		Branch: "refs/heads/helix-specs:refs/heads/helix-specs",
	}); err != nil {
		return fmt.Errorf("failed to push: %w", err)
	}

	log.Info().Msg("Created orphan helix-specs branch")
	return nil
}

// LoadStartupScriptFromHelixSpecs loads the startup script from the helix-specs branch
func (s *ProjectRepoService) LoadStartupScriptFromHelixSpecs(codeRepoPath string) (string, error) {
	if codeRepoPath == "" {
		return "", fmt.Errorf("code repo path not set")
	}

	ctx := context.Background()

	// Open bare repository
	repo, err := giteagit.OpenRepository(ctx, codeRepoPath)
	if err != nil {
		return "", fmt.Errorf("failed to open git repository: %w", err)
	}
	defer repo.Close()

	// Get helix-specs branch commit
	commit, err := repo.GetBranchCommit("helix-specs")
	if err != nil {
		// helix-specs branch doesn't exist - fall back to main branch (legacy support)
		log.Debug().Str("repo_path", codeRepoPath).Msg("helix-specs branch not found, falling back to main branch")
		return s.LoadStartupScriptFromCodeRepo(codeRepoPath)
	}

	// Get file content
	content, err := commit.GetFileContent(".helix/startup.sh", 0)
	if err != nil {
		// File doesn't exist yet - try legacy location on main branch
		log.Debug().Str("repo_path", codeRepoPath).Msg("startup.sh not in helix-specs, falling back to main branch")
		return s.LoadStartupScriptFromCodeRepo(codeRepoPath)
	}

	return content, nil
}

// GetStartupScriptHistoryFromHelixSpecs returns git commit history for the startup script from helix-specs branch
func (s *ProjectRepoService) GetStartupScriptHistoryFromHelixSpecs(codeRepoPath string) ([]StartupScriptVersion, error) {
	if codeRepoPath == "" {
		return nil, fmt.Errorf("code repo path not set")
	}

	ctx := context.Background()

	// Open repo to use high-level API
	repo, err := giteagit.OpenRepository(ctx, codeRepoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open git repository: %w", err)
	}
	defer repo.Close()

	// Check if helix-specs branch exists
	_, err = repo.GetBranchCommit("helix-specs")
	if err != nil {
		log.Debug().Str("repo_path", codeRepoPath).Msg("helix-specs branch not found for history, falling back to main branch")
		return s.GetStartupScriptHistoryFromCodeRepo(codeRepoPath)
	}

	// Use gitea's high-level CommitsByFileAndRange API
	commits, err := repo.CommitsByFileAndRange(giteagit.CommitsByFileAndRangeOptions{
		Revision: "helix-specs",
		File:     ".helix/startup.sh",
		Page:     0, // All commits
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get commits for startup.sh: %w", err)
	}

	var versions []StartupScriptVersion
	for _, commit := range commits {
		content, err := commit.GetFileContent(".helix/startup.sh", 0)
		if err != nil {
			log.Warn().Err(err).Str("commit", commit.ID.String()).Msg("Failed to get file from commit")
			continue
		}

		versions = append(versions, StartupScriptVersion{
			CommitHash: commit.ID.String(),
			Content:    content,
			Timestamp:  commit.Author.When,
			Author:     commit.Author.Name,
			Message:    commit.Summary(),
		})
	}

	// If no versions found in helix-specs, check main branch for legacy scripts
	if len(versions) == 0 {
		log.Debug().Str("repo_path", codeRepoPath).Msg("No startup script history in helix-specs, checking main branch")
		return s.GetStartupScriptHistoryFromCodeRepo(codeRepoPath)
	}

	return versions, nil
}

// Helper functions

func splitLines(s string) []string {
	var lines []string
	for _, line := range splitByNewline(s) {
		if line = trimString(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func splitByNewline(s string) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		result = append(result, s[start:])
	}
	return result
}

func splitN(s, sep string, n int) []string {
	var result []string
	remaining := s
	for i := 0; i < n-1 && len(remaining) > 0; i++ {
		idx := indexOf(remaining, sep)
		if idx < 0 {
			break
		}
		result = append(result, remaining[:idx])
		remaining = remaining[idx+len(sep):]
	}
	if len(remaining) > 0 {
		result = append(result, remaining)
	}
	return result
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// Backward compatibility aliases - these will be removed in a future release
// They are kept temporarily for any code that hasn't been updated yet

// ProjectInternalRepoService is an alias for ProjectRepoService (backward compatibility)
type ProjectInternalRepoService = ProjectRepoService

// NewProjectInternalRepoService is an alias for NewProjectRepoService (backward compatibility)
func NewProjectInternalRepoService(basePath string) *ProjectInternalRepoService {
	return NewProjectRepoService(basePath)
}
