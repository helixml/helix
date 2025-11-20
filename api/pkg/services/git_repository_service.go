package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/filemode"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/plumbing/transport/http"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// GitRepositoryService manages git repositories hosted on the Helix server
// Uses the filestore mount for persistent storage of git repositories
type GitRepositoryService struct {
	store           store.Store
	filestoreBase   string        // Base path for filestore (e.g., "/tmp/helix/filestore")
	gitRepoBase     string        // Base path for git repositories within filestore
	serverBaseURL   string        // Base URL for git server (e.g., "http://api:8080")
	gitUserName     string        // Default git user name
	gitUserEmail    string        // Default git user email
	enableGitServer bool          // Whether to enable git server functionality
	testMode        bool          // Test mode for unit tests
	koditService    *KoditService // Optional Kodit service for code intelligence
}

// NewGitRepositoryService creates a new git repository service
func NewGitRepositoryService(
	store store.Store,
	filestoreBase string,
	serverBaseURL string,
	gitUserName string,
	gitUserEmail string,
) *GitRepositoryService {
	gitRepoBase := filepath.Join(filestoreBase, "git-repositories")

	// Ensure the git-repositories directory exists
	if err := os.MkdirAll(gitRepoBase, 0755); err != nil {
		log.Error().Err(err).Str("path", gitRepoBase).Msg("Failed to create git-repositories directory")
	}

	return &GitRepositoryService{
		store:           store,
		filestoreBase:   filestoreBase,
		gitRepoBase:     gitRepoBase,
		serverBaseURL:   strings.TrimSuffix(serverBaseURL, "/"),
		gitUserName:     gitUserName,
		gitUserEmail:    gitUserEmail,
		enableGitServer: true,
		testMode:        false,
	}
}

// SetTestMode enables or disables test mode
func (s *GitRepositoryService) SetTestMode(enabled bool) {
	s.testMode = enabled
}

// SetKoditService sets the Kodit service for code intelligence
func (s *GitRepositoryService) SetKoditService(koditService *KoditService) {
	s.koditService = koditService
}

// Initialize creates the git repository base directory and sets up git server
func (s *GitRepositoryService) Initialize(ctx context.Context) error {
	// Create git repositories base directory
	err := os.MkdirAll(s.gitRepoBase, 0755)
	if err != nil {
		return fmt.Errorf("failed to create git repositories directory: %w", err)
	}

	log.Info().
		Str("git_repo_base", s.gitRepoBase).
		Str("server_base_url", s.serverBaseURL).
		Msg("Initialized git repository service")

	return nil
}

// CreateRepository creates a new git repository
func (s *GitRepositoryService) CreateRepository(ctx context.Context, request *types.GitRepositoryCreateRequest) (*types.GitRepository, error) {
	// Check for duplicate repository name for this owner
	existingRepos, err := s.store.ListGitRepositories(ctx, &types.ListGitRepositoriesRequest{
		OrganizationID: request.OrganizationID,
		OwnerID:        request.OwnerID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list repositories: %w", err)
	}
	// Check for duplicates
	for _, repo := range existingRepos {
		if repo.Name == request.Name && repo.OwnerID == request.OwnerID {
			return nil, fmt.Errorf("repository with name '%s' already exists for this owner", request.Name)
		}
	}

	// Generate repository ID
	repoID := s.generateRepositoryID(request.RepoType, request.Name)

	// Resolve organization ID
	orgID := request.OrganizationID
	if orgID == "" {
		// If attached to a project, use project's organization
		if request.ProjectID != "" {
			project, err := s.store.GetProject(ctx, request.ProjectID)
			if err == nil && project.OrganizationID != "" {
				orgID = project.OrganizationID
			}
		}

		// If still no org, get owner's first organization
		if orgID == "" {
			memberships, err := s.store.ListOrganizationMemberships(ctx, &store.ListOrganizationMembershipsQuery{
				UserID: request.OwnerID,
			})
			if err == nil && len(memberships) > 0 {
				orgID = memberships[0].OrganizationID
			}
		}
	}

	// Set default branch if not specified
	defaultBranch := request.DefaultBranch
	if defaultBranch == "" {
		defaultBranch = "main"
	}

	// Create repository path
	repoPath := filepath.Join(s.gitRepoBase, repoID)

	isExternal := request.ExternalURL != ""

	// Create repository object
	gitRepo := &types.GitRepository{
		ID:             repoID,
		Name:           request.Name,
		Description:    request.Description,
		OwnerID:        request.OwnerID,
		OrganizationID: orgID,
		ProjectID:      request.ProjectID,
		RepoType:       request.RepoType,
		Status:         types.GitRepositoryStatusActive,
		CloneURL:       s.generateCloneURL(repoID),
		IsExternal:     isExternal,
		ExternalURL:    request.ExternalURL,
		ExternalType:   request.ExternalType,
		Username:       request.Username,
		Password:       request.Password,
		LocalPath:      repoPath,
		DefaultBranch:  defaultBranch,
		LastActivity:   time.Now(),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Metadata:       request.Metadata,
	}

	if gitRepo.ExternalURL == "" {
		// Initialize git repository as bare
		// ALL filestore repos are bare - agents and API server push to them
		err = s.initializeGitRepository(repoPath, defaultBranch, request.Name, request.InitialFiles, true)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize git repository: %w", err)
		}
	}

	// Store repository metadata (if store supports it)
	err = s.store.CreateGitRepository(ctx, gitRepo)
	if err != nil {
		log.Warn().Err(err).Str("repo_id", gitRepo.ID).Msg("Failed to store repository metadata in database")
		return nil, err
	}

	log.Info().
		Str("repo_id", repoID).
		Str("repo_path", repoPath).
		Str("repo_type", string(gitRepo.RepoType)).
		Str("external_url", gitRepo.ExternalURL).
		Msg("Created git repository")

	// Register with Kodit if kodit_indexing is enabled
	if s.koditService != nil && request.KoditIndexing {
		// Register repository with Kodit (non-blocking - failures are logged but don't fail repo creation)
		go func() {
			koditResp, err := s.koditService.RegisterRepository(context.Background(), gitRepo.ExternalURL)
			if err != nil {
				log.Error().
					Err(err).
					Str("repo_id", repoID).
					Str("external_url", gitRepo.ExternalURL).
					Msg("Failed to register repository with Kodit")
				return
			}

			if koditResp != nil {
				// Store Kodit repository ID in metadata for future reference
				if gitRepo.Metadata == nil {
					gitRepo.Metadata = make(map[string]interface{})
				}
				gitRepo.Metadata["kodit_repo_id"] = koditResp.Data.ID

				// Update repository metadata with Kodit ID
				if err := s.store.UpdateGitRepository(context.Background(), gitRepo); err != nil {
					log.Warn().
						Err(err).
						Str("repo_id", repoID).
						Str("kodit_repo_id", koditResp.Data.ID).
						Msg("Failed to update repository with Kodit ID")
				} else {
					log.Info().
						Str("repo_id", repoID).
						Str("kodit_repo_id", koditResp.Data.ID).
						Str("external_url", gitRepo.ExternalURL).
						Msg("Registered repository with Kodit for code intelligence")
				}
			}
		}()
	}

	return gitRepo, nil
}

// GetRepository retrieves repository information by ID
func (s *GitRepositoryService) GetRepository(ctx context.Context, repoID string) (*types.GitRepository, error) {
	// Try to get metadata from store first (has correct LocalPath for all repo types)
	log.Info().Str("repo_id", repoID).Msg("Getting repository metadata from store")

	gitRepo, err := s.store.GetGitRepository(ctx, repoID)
	if err != nil {
		return nil, fmt.Errorf("repository %s not found: %w", repoID, err)
	}

	// Got from database - verify the LocalPath exists if this is not external
	if gitRepo.ExternalURL == "" {
		if gitRepo.LocalPath != "" {
			if _, err := os.Stat(gitRepo.LocalPath); os.IsNotExist(err) {
				return nil, fmt.Errorf("repository not found: %s", repoID)
			}
		} else {
			// No LocalPath in DB - try default path
			repoPath := filepath.Join(s.gitRepoBase, repoID)
			if _, err := os.Stat(repoPath); os.IsNotExist(err) {
				return nil, fmt.Errorf("repository not found: %s", repoID)
			}
			gitRepo.LocalPath = repoPath
		}
	}

	// Update with current git information
	err = s.updateRepositoryFromGit(gitRepo)
	if err != nil {
		return nil, fmt.Errorf("failed to update repository info from git: %w", err)
	}

	return gitRepo, nil
}

// UpdateRepository updates an existing repository's metadata
func (s *GitRepositoryService) UpdateRepository(
	ctx context.Context,
	repoID string,
	request *types.GitRepositoryUpdateRequest,
) (*types.GitRepository, error) {
	// Get existing repository
	existing, err := s.GetRepository(ctx, repoID)
	if err != nil {
		return nil, fmt.Errorf("repository not found: %w", err)
	}

	// Update fields if provided
	if request.Name != "" {
		existing.Name = request.Name
	}
	if request.Description != "" {
		existing.Description = request.Description
	}
	if request.DefaultBranch != "" {
		existing.DefaultBranch = request.DefaultBranch
	}
	if request.Metadata != nil {
		// Merge metadata
		if existing.Metadata == nil {
			existing.Metadata = make(map[string]interface{})
		}
		for k, v := range request.Metadata {
			existing.Metadata[k] = v
		}
	}

	existing.UpdatedAt = time.Now()

	// Update in store
	err = s.store.UpdateGitRepository(ctx, existing)
	if err != nil {
		return nil, fmt.Errorf("failed to update repository metadata: %w", err)
	}

	log.Info().
		Str("repo_id", repoID).
		Str("name", existing.Name).
		Msg("updated git repository")

	return existing, nil
}

// DeleteRepository deletes a repository
func (s *GitRepositoryService) DeleteRepository(ctx context.Context, repoID string) error {
	repoPath := filepath.Join(s.gitRepoBase, repoID)

	// Delete repository directory
	if err := os.RemoveAll(repoPath); err != nil {
		log.Warn().Err(err).Str("repo_id", repoID).Msg("failed to delete repository directory")
		// Continue to delete metadata even if filesystem deletion fails
	}

	err := s.store.DeleteGitRepository(ctx, repoID)
	if err != nil {
		log.Warn().Err(err).Str("repo_id", repoID).Msg("failed to delete repository metadata")
	}

	log.Info().
		Str("repo_id", repoID).
		Msg("deleted git repository")

	return nil
}

// incrementRepositoryName intelligently increments a repository name suffix
// Examples: "repo" -> "repo-2", "repo-2" -> "repo-3", "repo-5" -> "repo-6"
func incrementRepositoryName(name string) string {
	// Check if name ends with -<number>
	re := regexp.MustCompile(`^(.+)-(\d+)$`)
	matches := re.FindStringSubmatch(name)

	if len(matches) == 3 {
		// Name has a numeric suffix, increment it
		baseName := matches[1]
		currentNum, _ := strconv.Atoi(matches[2])
		return fmt.Sprintf("%s-%d", baseName, currentNum+1)
	}

	// No numeric suffix, add -2
	return fmt.Sprintf("%s-2", name)
}

// CreateSampleRepository creates a sample/demo repository
// Automatically handles name conflicts by appending -2, -3, -4, etc.
func (s *GitRepositoryService) CreateSampleRepository(
	ctx context.Context,
	request *types.CreateSampleRepositoryRequest,
) (*types.GitRepository, error) {
	// Get sample files based on type
	initialFiles := s.getSampleProjectFiles(request.SampleType)

	// Try creating with incremented names if there's a conflict
	maxRetries := 100
	currentName := request.Name

	for attempt := 0; attempt < maxRetries; attempt++ {
		request := &types.GitRepositoryCreateRequest{
			Name:           currentName,
			Description:    request.Description,
			RepoType:       types.GitRepositoryTypeCode,
			OwnerID:        request.OwnerID,
			OrganizationID: request.OrganizationID,
			InitialFiles:   initialFiles,
			DefaultBranch:  "main",
			Metadata: map[string]interface{}{
				"sample_type":    request.SampleType,
				"created_from":   "sample",
				"kodit_indexing": request.KoditIndexing,
			},
		}

		repo, err := s.CreateRepository(ctx, request)
		if err == nil {
			// Success!
			if currentName != request.Name {
				log.Info().
					Str("original_name", request.Name).
					Str("final_name", currentName).
					Msg("Created sample repository with auto-incremented name")
			}
			return repo, nil
		}

		// Check if error is due to name conflict
		if !strings.Contains(err.Error(), "already exists") {
			// Different error, return it
			return nil, err
		}

		// Name conflict, try next increment
		currentName = incrementRepositoryName(currentName)
		log.Debug().
			Str("next_name", currentName).
			Int("attempt", attempt+1).
			Msg("Repository name conflict, trying incremented name")
	}

	return nil, fmt.Errorf("failed to create repository after %d attempts (name conflicts)", maxRetries)
}

// GetCloneCommand returns the git clone command for a repository
func (s *GitRepositoryService) GetCloneCommand(repoID string, targetDir string) string {
	cloneURL := s.generateCloneURL(repoID)
	if targetDir == "" {
		return fmt.Sprintf("git clone %s", cloneURL)
	}
	return fmt.Sprintf("git clone %s %s", cloneURL, targetDir)
}

// generateRepositoryID generates a unique repository ID
func (s *GitRepositoryService) generateRepositoryID(repoType types.GitRepositoryType, name string) string {
	// Sanitize name for filesystem
	sanitizedName := strings.ReplaceAll(strings.ToLower(name), " ", "-")
	sanitizedName = strings.ReplaceAll(sanitizedName, "_", "-")

	timestamp := time.Now().Unix()
	return fmt.Sprintf("%s-%s-%d", repoType, sanitizedName, timestamp)
}

// generateCloneURL generates the clone URL for a repository
func (s *GitRepositoryService) generateCloneURL(repoID string) string {
	// Use HTTP URLs for network access by Zed agents
	// Format: http://api-server/git/{repo_id}
	return fmt.Sprintf("%s/git/%s", s.serverBaseURL, repoID)
}

// initializeGitRepository initializes a new git repository with initial files
func (s *GitRepositoryService) initializeGitRepository(
	repoPath string,
	defaultBranch string,
	repoName string,
	initialFiles map[string]string,
	isBare bool,
) error {
	// Create repository directory
	err := os.MkdirAll(repoPath, 0755)
	if err != nil {
		return fmt.Errorf("failed to create repository directory: %w", err)
	}

	// For bare repos with initial files, we need to create a temp clone, commit files, then push
	if isBare && len(initialFiles) > 0 {
		// Initialize bare repository
		_, err := git.PlainInit(repoPath, true)
		if err != nil {
			return fmt.Errorf("failed to initialize bare git repository: %w", err)
		}

		// Create temporary working directory to add initial files
		tempClone, err := os.MkdirTemp("", "helix-git-init-*")
		if err != nil {
			return fmt.Errorf("failed to create temp directory: %w", err)
		}
		defer os.RemoveAll(tempClone) // Cleanup temp clone

		// Initialize a new non-bare repo in the temp directory
		repo, err := git.PlainInit(tempClone, false)
		if err != nil {
			return fmt.Errorf("failed to initialize temp repository: %w", err)
		}

		// Ensure we have at least one file
		if len(initialFiles) == 0 {
			initialFiles = map[string]string{
				"README.md": fmt.Sprintf("# %s\n", repoName),
			}
		}

		// Write initial files to temp clone
		for filePath, content := range initialFiles {
			fullPath := filepath.Join(tempClone, filePath)
			dir := filepath.Dir(fullPath)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", dir, err)
			}
			if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
				return fmt.Errorf("failed to write file %s: %w", filePath, err)
			}
		}

		// Commit and push to bare repo
		worktree, err := repo.Worktree()
		if err != nil {
			return fmt.Errorf("failed to get worktree: %w", err)
		}

		if _, err := worktree.Add("."); err != nil {
			return fmt.Errorf("failed to add files: %w", err)
		}

		_, err = worktree.Commit("Initial commit", &git.CommitOptions{
			Author: &object.Signature{
				Name:  "Helix System",
				Email: "system@helix.ml",
				When:  time.Now(),
			},
		})
		if err != nil {
			return fmt.Errorf("failed to commit: %w", err)
		}

		// Rename master to main if needed (same fix for temp clone)
		headRef, err := repo.Head()
		if err == nil {
			currentBranch := headRef.Name().Short()
			if currentBranch == "master" && defaultBranch == "main" {
				// Checkout main branch
				mainRef := plumbing.NewBranchReferenceName("main")
				if err := repo.Storer.SetReference(plumbing.NewHashReference(mainRef, headRef.Hash())); err != nil {
					log.Warn().Err(err).Msg("Failed to create main branch in temp clone")
				} else {
					// Checkout main
					worktree, _ := repo.Worktree()
					if worktree != nil {
						worktree.Checkout(&git.CheckoutOptions{
							Branch: mainRef,
							Create: false,
						})
					}
					// Delete master
					repo.Storer.RemoveReference(plumbing.NewBranchReferenceName("master"))
					log.Info().Msg("Renamed temp clone branch from master to main")
				}
			}
		}

		// Add the bare repo as a remote
		_, err = repo.CreateRemote(&config.RemoteConfig{
			Name: "origin",
			URLs: []string{repoPath},
		})
		if err != nil {
			return fmt.Errorf("failed to add remote: %w", err)
		}

		// Push to bare repo
		err = repo.Push(&git.PushOptions{
			RemoteName: "origin",
			RefSpecs: []config.RefSpec{
				config.RefSpec(fmt.Sprintf("+refs/heads/%s:refs/heads/%s", defaultBranch, defaultBranch)),
			},
		})
		if err != nil {
			return fmt.Errorf("failed to push to bare repo: %w", err)
		}

		// Open the bare repo and delete master branch if it exists
		bareRepo, err := git.PlainOpen(repoPath)
		if err == nil {
			masterRef := plumbing.NewBranchReferenceName("master")
			if err := bareRepo.Storer.RemoveReference(masterRef); err != nil && err != plumbing.ErrReferenceNotFound {
				log.Warn().Err(err).Msg("Failed to remove master branch from bare repo")
			} else if err == nil {
				log.Info().Msg("Removed master branch from bare repository")
			}
		}

		log.Info().
			Str("repo_path", repoPath).
			Str("default_branch", defaultBranch).
			Msg("Created bare git repository with initial files")

		return nil
	}

	// For non-bare repos or bare repos without initial files
	repo, err := git.PlainInit(repoPath, isBare)
	if err != nil {
		return fmt.Errorf("failed to initialize git repository: %w", err)
	}

	// If bare repo with no initial files, we're done
	if isBare {
		// Set HEAD to main instead of master
		headRef := plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName(defaultBranch))
		if err := repo.Storer.SetReference(headRef); err != nil {
			log.Warn().Err(err).Msg("Failed to set HEAD to main branch")
		}

		// Delete master branch if it exists
		masterRef := plumbing.NewBranchReferenceName("master")
		if err := repo.Storer.RemoveReference(masterRef); err != nil && err != plumbing.ErrReferenceNotFound {
			log.Warn().Err(err).Msg("Failed to remove master branch")
		}

		log.Info().
			Str("repo_path", repoPath).
			Str("default_branch", defaultBranch).
			Msg("Created empty bare git repository (accepts pushes from agents)")
		return nil
	}

	// Ensure we have at least one file to commit (can't create empty commits)
	if len(initialFiles) == 0 {
		initialFiles = map[string]string{
			"README.md": fmt.Sprintf("# %s\n", repoName),
		}
	}

	// Write initial files
	for filePath, content := range initialFiles {
		fullPath := filepath.Join(repoPath, filePath)

		// Create directory if needed
		dir := filepath.Dir(fullPath)
		err = os.MkdirAll(dir, 0755)
		if err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}

		// Write file
		err = os.WriteFile(fullPath, []byte(content), 0644)
		if err != nil {
			return fmt.Errorf("failed to write file %s: %w", filePath, err)
		}
	}

	// Add files to git
	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Add all files
	_, err = worktree.Add(".")
	if err != nil {
		return fmt.Errorf("failed to add files to git: %w", err)
	}

	// Initial commit
	signature := &object.Signature{
		Name:  s.gitUserName,
		Email: s.gitUserEmail,
		When:  time.Now(),
	}

	_, err = worktree.Commit("Initial commit", &git.CommitOptions{
		Author:    signature,
		Committer: signature,
	})
	if err != nil {
		return fmt.Errorf("failed to create initial commit: %w", err)
	}

	// Rename master to main if needed (go-git defaults to master)
	// Get current branch name
	headRef, err := repo.Head()
	if err == nil {
		currentBranch := headRef.Name().Short()
		if currentBranch == "master" && defaultBranch == "main" {
			// Create main branch pointing to same commit
			mainRef := plumbing.NewBranchReferenceName("main")
			if err := repo.Storer.SetReference(plumbing.NewHashReference(mainRef, headRef.Hash())); err != nil {
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
	}

	return nil
}

// updateRepositoryFromGit updates repository info from git metadata
func (s *GitRepositoryService) updateRepositoryFromGit(gitRepo *types.GitRepository) error {
	var (
		repo *git.Repository
		err  error
	)

	if gitRepo.ExternalURL != "" {
		repo, err = git.PlainOpen(gitRepo.LocalPath)
		if err != nil {
			cloneOptions := &git.CloneOptions{
				URL:      gitRepo.ExternalURL,
				Progress: os.Stdout,
			}

			if gitRepo.Password != "" {
				username := gitRepo.Username
				if username == "" {
					username = "PAT"
				}
				cloneOptions.Auth = &http.BasicAuth{Username: username, Password: gitRepo.Password}
			}

			repo, err = git.PlainClone(gitRepo.LocalPath, cloneOptions)
			if err != nil {
				return fmt.Errorf("failed to clone git repository: %w", err)
			}
		}
	} else {
		repo, err = git.PlainOpen(gitRepo.LocalPath)
		if err != nil {
			return fmt.Errorf("failed to open git repository: %w", err)
		}
	}

	// Get branches
	refs, err := repo.References()
	if err == nil {
		branches := []string{}
		refs.ForEach(func(ref *plumbing.Reference) error {
			if ref.Name().IsBranch() {
				branchName := ref.Name().Short()
				branches = append(branches, branchName)
			}
			return nil
		})
		gitRepo.Branches = branches
	}

	// Update last activity (could be improved with actual git log)
	gitRepo.LastActivity = time.Now()
	gitRepo.UpdatedAt = time.Now()

	return nil
}

// getSampleProjectFiles returns sample files based on project type
func (s *GitRepositoryService) getSampleProjectFiles(sampleType string) map[string]string {
	files := make(map[string]string)

	switch sampleType {
	case "empty":
		files["README.md"] = fmt.Sprintf("# %s\n\nAn empty project repository ready for development.\n\n## Getting Started\n\nThis repository was created as an empty project. Add your code and documentation as needed.\n", sampleType)
		// Just basic structure - no framework-specific files
		return files

	case "nodejs-todo":
		files["README.md"] = "# Node.js Todo App\n\nA simple todo application built with Node.js and Express.\n"
		files["package.json"] = `{
  "name": "nodejs-todo-app",
  "version": "1.0.0",
  "description": "A simple todo application",
  "main": "src/index.js",
  "scripts": {
    "start": "node src/index.js",
    "dev": "nodemon src/index.js",
    "test": "jest"
  },
  "dependencies": {
    "express": "^4.18.0"
  },
  "devDependencies": {
    "nodemon": "^2.0.0",
    "jest": "^28.0.0"
  }
}`
		files["src/index.js"] = `const express = require('express');
const app = express();
const PORT = process.env.PORT || 3000;

app.use(express.json());

// Simple todos array
let todos = [];

// Routes
app.get('/api/todos', (req, res) => {
  res.json(todos);
});

app.post('/api/todos', (req, res) => {
  const todo = {
    id: Date.now(),
    text: req.body.text,
    completed: false
  };
  todos.push(todo);
  res.status(201).json(todo);
});

app.listen(PORT, () => {
  console.log('Server running on port ' + PORT);
});
`

	case "python-api":
		files["README.md"] = "# Python API Service\n\nA FastAPI microservice with PostgreSQL.\n"
		files["requirements.txt"] = `fastapi==0.104.1
uvicorn==0.24.0
sqlalchemy==2.0.23
psycopg2-binary==2.9.9
pydantic==2.5.0
`
		files["app/main.py"] = `from fastapi import FastAPI
from app.routers import items

app = FastAPI(title="Python API Service")

app.include_router(items.router, prefix="/api/v1")

@app.get("/")
async def root():
    return {"message": "Python API Service"}

@app.get("/health")
async def health():
    return {"status": "healthy"}
`
		files["app/__init__.py"] = ""
		files["app/routers/__init__.py"] = ""
		files["app/routers/items.py"] = `from fastapi import APIRouter

router = APIRouter()

@router.get("/items")
async def get_items():
    return {"items": []}

@router.post("/items")
async def create_item(item: dict):
    return {"message": "Item created", "item": item}
`

	case "react-dashboard":
		files["README.md"] = "# React Dashboard\n\nA modern admin dashboard built with React and Material-UI.\n"
		files["package.json"] = `{
  "name": "react-dashboard",
  "version": "1.0.0",
  "description": "A modern admin dashboard",
  "dependencies": {
    "react": "^18.2.0",
    "react-dom": "^18.2.0",
    "@mui/material": "^5.14.0",
    "@mui/icons-material": "^5.14.0"
  },
  "scripts": {
    "start": "react-scripts start",
    "build": "react-scripts build",
    "test": "react-scripts test"
  }
}`
		files["src/App.js"] = `import React from 'react';
import { AppBar, Toolbar, Typography, Container, Grid, Paper } from '@mui/material';

function App() {
  return (
    <div>
      <AppBar position="static">
        <Toolbar>
          <Typography variant="h6">
            React Dashboard
          </Typography>
        </Toolbar>
      </AppBar>
      <Container maxWidth="lg" sx={{ mt: 4, mb: 4 }}>
        <Grid container spacing={3}>
          <Grid item xs={12}>
            <Paper sx={{ p: 2 }}>
              <Typography variant="h4">Welcome to React Dashboard</Typography>
              <Typography variant="body1">
                This is a sample dashboard application.
              </Typography>
            </Paper>
          </Grid>
        </Grid>
      </Container>
    </div>
  );
}

export default App;
`
		files["public/index.html"] = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>React Dashboard</title>
</head>
<body>
    <div id="root"></div>
</body>
</html>
`

	case "linkedin-outreach":
		files["README.md"] = "# LinkedIn Outreach Campaign\n\nMulti-session campaign to reach out to 100 prospects using Helix LinkedIn integration.\n"
		files["campaign/README.md"] = `# LinkedIn Outreach Campaign

## Campaign Overview
Goal: Reach out to 100 qualified prospects using Helix LinkedIn skill integration

## Multi-Session Strategy
- **Session A**: Prospect research and list building
- **Session B**: Message personalization and outreach
- **Session C**: Follow-up tracking and relationship management
- **Session D**: Campaign analysis and optimization

## Campaign Structure
- Target: 100 prospects in AI/ML industry
- Personalized messaging based on profile analysis
- Multi-touch follow-up sequence
- Conversion tracking and analysis
`
		files["campaign/prospect-criteria.md"] = `# Prospect Criteria

## Ideal Customer Profile
- **Industry**: AI/ML, Software Development, Tech Startups
- **Role**: CTO, VP Engineering, AI Lead, Technical Founder
- **Company Size**: 10-500 employees
- **Location**: Global (English-speaking)
- **Activity**: Active on LinkedIn, posts about AI/ML

## Qualification Criteria
- Has engineering team (5+ developers)
- Shows interest in AI/automation tools
- Posts about development challenges
- Engages with technical content

## Exclusion Criteria
- Competitors or similar companies
- Non-technical decision makers only
- Inactive LinkedIn profiles (no posts in 6 months)
- Already contacted in previous campaigns
`
		files["campaign/message-templates.md"] = `# Message Templates

## Initial Outreach Message
Hi [First Name],

I noticed your recent post about [specific topic from their profile]. Your insights on [specific detail] really resonated with me.

I'm reaching out because we've built something that might interest you - Helix is an AI development platform that helps engineering teams like yours accelerate development with AI-powered coding assistants.

What caught my attention about your work is [personalized observation based on their posts/company].

Would you be open to a brief conversation about how teams like yours are using AI to enhance their development workflows?

Best regards,
[Your name]

## Follow-up Message 1 (1 week later)
Hi [First Name],

I hope you don't mind the follow-up. I shared some insights about AI-powered development workflows that might be relevant to [their company/challenges].

I noticed [specific recent activity or post], which aligns with what we're seeing across the industry.

Would a quick 15-minute call work to discuss how other teams are solving [specific challenge they've mentioned]?

## Follow-up Message 2 (2 weeks later)
[Personalized based on new activity/posts]
`
		files["campaign/tracking-template.md"] = `# Campaign Tracking

## Prospect Status Tracking
| Name | Company | Role | Outreach Date | Status | Response | Next Action |
|------|---------|------|---------------|--------|----------|-------------|
| | | | | pending | | |

## Response Categories
- **Interested**: Wants to learn more / schedule call
- **Not Now**: Interested but timing not right
- **Not Relevant**: Not a good fit
- **No Response**: No reply after 3 touches
- **Connected**: Accepted invitation, ready for outreach

## Campaign Metrics
- **Total Prospects**: 100
- **Messages Sent**: 0
- **Responses Received**: 0
- **Positive Responses**: 0
- **Meetings Scheduled**: 0
- **Response Rate**: 0%
- **Conversion Rate**: 0%
`

	case "helix-blog-posts":
		files["README.md"] = "# Helix Technical Blog Posts\n\nWrite 10 technical blog posts about the Helix system by analyzing the actual codebase.\n"
		files["blog-project/README.md"] = `# Helix Blog Post Project

## Project Overview
Write 10 comprehensive blog posts about different aspects of the Helix AI development platform by analyzing the actual codebase.

## Multi-Session Strategy
- **Session A**: Repository analysis and content planning
- **Session B**: Technical deep-dive posts (architecture, APIs)
- **Session C**: User-focused posts (tutorials, use cases)
- **Session D**: Advanced topics and future roadmap

## Blog Post Topics
1. "Getting Started with Helix: Your First AI Assistant"
2. "Understanding Helix Architecture: From API to Models"
3. "Building Custom Skills: Extending Helix with APIs"
4. "Multi-Model Orchestration in Helix"
5. "Helix Security: Authentication and Access Control"
6. "Scaling Helix: Deployment and Operations"
7. "Advanced Helix: Custom Agents and Workflows"
8. "Helix vs. Other AI Platforms: Technical Comparison"
9. "Contributing to Helix: Developer Guide"
10. "The Future of Helix: Roadmap and Vision"

## Content Strategy
- Technical accuracy through code analysis
- Practical examples and tutorials
- Screenshots and diagrams
- Code samples from actual implementation
- User journey and use case focus
`
		files["blog-project/helix-repo-analysis.md"] = `# Helix Repository Analysis Plan

## Repository Information
- **Source**: https://github.com/helixml/helix
- **Clone Strategy**: Use Zed agent git access for live code analysis
- **Update Frequency**: Re-clone periodically to capture latest changes

## Code Analysis Approach

### 1. Architecture Analysis
- Main entry points: cmd/, main.go
- API structure: api/pkg/
- Frontend: frontend/src/
- Documentation: docs/, README.md

### 2. Key Components to Analyze
- **Session Management**: api/pkg/controller/
- **Model Integration**: api/pkg/model/
- **Skills System**: api/pkg/agent/skill/
- **Authentication**: api/pkg/auth/
- **Storage**: api/pkg/store/
- **WebSocket**: api/pkg/pubsub/

### 3. Content Generation Strategy
- Extract code examples for blog posts
- Understand data flows and interactions
- Identify key features and capabilities
- Generate practical tutorials from actual usage patterns

## Git Commands for Zed Agent
` + "```bash" + `
# Clone Helix repository
git clone https://github.com/helixml/helix.git /workspace/helix-analysis
cd /workspace/helix-analysis

# Analyze codebase structure
find . -name "*.go" | head -20
find . -name "*.ts" -o -name "*.tsx" | head -20
cat README.md
cat docs/*.md

# Understand main components
ls -la api/pkg/
ls -la frontend/src/
cat main.go
` + "```" + `
`
		files["blog-project/post-templates/"] = ""
		files["blog-project/post-templates/technical-post-template.md"] = `# [Blog Post Title]

*Part [X] of the Helix Technical Series*

## Introduction
[Brief introduction explaining what this post covers and why it matters]

## Overview
[High-level explanation of the topic]

## Technical Deep Dive

### Architecture
[Explain the architecture with references to actual code]

### Implementation Details
[Code examples from the actual Helix repository]

` + "```go" + `
// Example from helix/api/pkg/[component]/[file].go
[actual code snippet with explanation]
` + "```" + `

### Key Features
[Highlight important features and capabilities]

## Practical Examples

### Basic Usage
[Step-by-step tutorial with real examples]

### Advanced Usage
[More complex scenarios and configurations]

## Best Practices
[Recommendations based on codebase analysis]

## Troubleshooting
[Common issues and solutions based on code understanding]

## Conclusion
[Summary and next steps]

## Related Resources
- [Link to relevant documentation]
- [Link to code examples]
- [Link to other blog posts in series]

---
*This post was generated by analyzing the Helix codebase at [commit hash] on [date]*
`
		files["blog-project/content-calendar.md"] = `# Blog Post Content Calendar

## Publishing Schedule
| Post # | Title | Focus Area | Target Date | Status | Session |
|--------|-------|------------|-------------|--------|---------|
| 1 | Getting Started with Helix | User Tutorial | Week 1 | Planned | Session C |
| 2 | Helix Architecture Deep Dive | Technical | Week 1 | Planned | Session B |
| 3 | Building Custom Skills | Developer Guide | Week 2 | Planned | Session B |
| 4 | Multi-Model Orchestration | Technical | Week 2 | Planned | Session B |
| 5 | Authentication & Security | Technical | Week 3 | Planned | Session B |
| 6 | Deployment & Operations | DevOps | Week 3 | Planned | Session D |
| 7 | Advanced Workflows | Advanced | Week 4 | Planned | Session D |
| 8 | Platform Comparison | Business | Week 4 | Planned | Session C |
| 9 | Contributing Guide | Community | Week 5 | Planned | Session C |
| 10 | Future Roadmap | Vision | Week 5 | Planned | Session D |

## Content Distribution
- **Technical Posts (40%)**: Architecture, APIs, advanced features
- **Tutorial Posts (30%)**: Getting started, how-to guides
- **Business Posts (20%)**: Use cases, comparisons, ROI
- **Community Posts (10%)**: Contributing, roadmap, vision

## Session Coordination
- **Session A**: Repository analysis, research, planning
- **Session B**: Technical deep-dive content creation
- **Session C**: User-focused tutorials and guides
- **Session D**: Advanced topics and strategic content
`

	case "jupyter-notebooks":
		files["README.md"] = `# Jupyter Financial Analysis - Notebooks

This repository contains Jupyter notebooks for financial data analysis.

## Quick Start

The agent can run notebooks using ipynb commands from the terminal:

` + "```bash" + `
# Execute notebook and save results in-place
jupyter nbconvert --execute --to notebook --inplace portfolio_analysis.ipynb

# Execute and export to HTML for viewing
jupyter nbconvert --execute --to html portfolio_analysis.ipynb

# View the HTML output in browser
# The HTML file will be at portfolio_analysis.html
` + "```" + `

## Notebooks

- **portfolio_analysis.ipynb**: Microsoft stock returns analysis with pyforest library

## Related Repositories

- **pyforest**: Python library for financial calculations (in separate repository)

## Setup

See AGENT_INSTRUCTIONS.md for setup and workflow details.
`
		files["AGENT_INSTRUCTIONS.md"] = `# Agent Instructions for Jupyter Workflow

## Running Notebooks from Command Line

You can execute Jupyter notebooks without opening the browser using these commands:

` + "```bash" + `
# Method 1: Execute and save results in the notebook
jupyter nbconvert --execute --to notebook --inplace portfolio_analysis.ipynb

# Method 2: Execute and generate HTML output
jupyter nbconvert --execute --to html portfolio_analysis.ipynb

# Method 3: Execute and show output in terminal
jupyter nbconvert --execute --to notebook --stdout portfolio_analysis.ipynb | grep -A 10 "output"
` + "```" + `

## Viewing Results

After running a notebook:
1. The .ipynb file will contain cell outputs if you used --inplace
2. The .html file can be viewed in a browser to see rendered results
3. You can use the Bash tool to open the HTML: ` + "`open portfolio_analysis.html`" + ` (macOS) or ` + "`xdg-open portfolio_analysis.html`" + ` (Linux)

## Iterative Development Workflow

1. **Read the notebook**: Use the Read tool on the .ipynb file to see the code
2. **Modify the notebook**: Use the Edit tool to change notebook cells
3. **Run the notebook**: Use jupyter nbconvert to execute
4. **Check results**: Read the updated .ipynb or generated .html
5. **Iterate**: Repeat based on results

## Working with PyForest Library

The pyforest library is in a separate repository. To make changes:
1. Navigate to the pyforest repository
2. Edit files in pyforest/pyforest/ directory
3. The library is installed with ` + "`pip install -e .`" + ` so changes are immediately available
4. Restart Python kernel or re-import to pick up changes

## Example Task Flow

For "Add multiple tickers" task:
1. Read portfolio_analysis.ipynb to understand current structure
2. Edit the notebook to download multiple tickers
3. Run: ` + "`jupyter nbconvert --execute --to html portfolio_analysis.ipynb`" + `
4. Check portfolio_analysis.html to verify the new ticker data appears
5. Commit changes to git
`
		files["requirements.txt"] = `jupyterlab==4.0.9
pandas==2.1.4
numpy==1.26.2
matplotlib==3.8.2
yfinance==0.2.33
scipy==1.11.4
seaborn==0.13.0
`
		// Notebook file
		files["portfolio_analysis.ipynb"] = `{
 "cells": [
  {
   "cell_type": "markdown",
   "metadata": {},
   "source": [
    "# Microsoft Portfolio Returns Analysis\n",
    "\n",
    "This notebook calculates portfolio returns for Microsoft (MSFT) over different date ranges.\n",
    "\n",
    "**Note**: This notebook uses the pyforest library from the separate pyforest repository."
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "metadata": {},
   "outputs": [],
   "source": [
    "import pandas as pd\n",
    "import numpy as np\n",
    "import yfinance as yf\n",
    "import matplotlib.pyplot as plt\n",
    "from datetime import datetime, timedelta\n",
    "\n",
    "# Import pyforest library (install from ../pyforest with: pip install -e ../pyforest)\n",
    "try:\n",
    "    from pyforest import calculate_returns, calculate_cumulative_returns, calculate_sharpe_ratio\n",
    "    print(\"PyForest library imported successfully\")\n",
    "except ImportError:\n",
    "    print(\"WARNING: PyForest library not found. Install with: pip install -e ../pyforest\")\n",
    "    print(\"Falling back to basic pandas calculations\")\n",
    "\n",
    "# Set display options\n",
    "pd.set_option('display.max_columns', None)\n",
    "pd.set_option('display.width', None)\n",
    "\n",
    "print(\"Libraries imported successfully\")"
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "metadata": {},
   "outputs": [],
   "source": [
    "# Download Microsoft stock data\n",
    "ticker = 'MSFT'\n",
    "print(f\"Downloading {ticker} data...\")\n",
    "\n",
    "# Get data for last 5 years\n",
    "end_date = datetime.now()\n",
    "start_date = end_date - timedelta(days=5*365)\n",
    "\n",
    "msft = yf.download(ticker, start=start_date, end=end_date, progress=False)\n",
    "print(f\"Downloaded {len(msft)} days of data\")\n",
    "msft.head()"
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "metadata": {},
   "outputs": [],
   "source": [
    "# Calculate returns using pyforest library if available, otherwise use pandas\n",
    "try:\n",
    "    msft['Daily_Return'] = calculate_returns(msft['Close'])\n",
    "    msft['Cumulative_Return'] = calculate_cumulative_returns(msft['Daily_Return'])\n",
    "    sharpe = calculate_sharpe_ratio(msft['Daily_Return'])\n",
    "except NameError:\n",
    "    # Fallback if pyforest not imported\n",
    "    msft['Daily_Return'] = msft['Close'].pct_change()\n",
    "    msft['Cumulative_Return'] = (1 + msft['Daily_Return']).cumprod() - 1\n",
    "    sharpe = (msft['Daily_Return'].mean() / msft['Daily_Return'].std()) * np.sqrt(252)\n",
    "\n",
    "print(\"\\nReturn Statistics:\")\n",
    "print(f\"Total Return: {msft['Cumulative_Return'].iloc[-1]:.2%}\")\n",
    "print(f\"Average Daily Return: {msft['Daily_Return'].mean():.4%}\")\n",
    "print(f\"Daily Return Std Dev: {msft['Daily_Return'].std():.4%}\")\n",
    "print(f\"Sharpe Ratio (annualized): {sharpe:.2f}\")"
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "metadata": {},
   "outputs": [],
   "source": [
    "# Analyze returns over different date ranges\n",
    "date_ranges = [\n",
    "    ('1 Month', 30),\n",
    "    ('3 Months', 90),\n",
    "    ('6 Months', 180),\n",
    "    ('1 Year', 365),\n",
    "    ('2 Years', 730),\n",
    "    ('5 Years', 1825),\n",
    "]\n",
    "\n",
    "results = []\n",
    "for label, days in date_ranges:\n",
    "    if len(msft) >= days:\n",
    "        period_data = msft.iloc[-days:]\n",
    "        total_return = (period_data['Close'].iloc[-1] / period_data['Close'].iloc[0] - 1)\n",
    "        results.append({\n",
    "            'Period': label,\n",
    "            'Days': days,\n",
    "            'Return': total_return,\n",
    "            'Start_Price': period_data['Close'].iloc[0],\n",
    "            'End_Price': period_data['Close'].iloc[-1],\n",
    "        })\n",
    "\n",
    "returns_df = pd.DataFrame(results)\n",
    "print(\"\\nReturns by Period:\")\n",
    "print(returns_df.to_string(index=False))"
   ]
  },
  {
   "cell_type": "code",
   "execution_count": null,
   "metadata": {},
   "outputs": [],
   "source": [
    "# Plot cumulative returns\n",
    "plt.figure(figsize=(14, 7))\n",
    "plt.plot(msft.index, msft['Cumulative_Return'] * 100, linewidth=2, color='#1f77b4')\n",
    "plt.title(f'{ticker} Cumulative Returns Over Time', fontsize=16, fontweight='bold')\n",
    "plt.xlabel('Date', fontsize=12)\n",
    "plt.ylabel('Cumulative Return (%)', fontsize=12)\n",
    "plt.grid(True, alpha=0.3)\n",
    "plt.tight_layout()\n",
    "plt.savefig('msft_returns.png', dpi=150, bbox_inches='tight')\n",
    "plt.show()\n",
    "\n",
    "print(f\"\\nCurrent Price: ${msft['Close'].iloc[-1]:.2f}\")\n",
    "print(f\"52-Week High: ${msft['Close'].iloc[-252:].max():.2f}\")\n",
    "print(f\"52-Week Low: ${msft['Close'].iloc[-252:].min():.2f}\")\n",
    "print(\"\\nChart saved to msft_returns.png\")"
   ]
  }
 ],
 "metadata": {
  "kernelspec": {
   "display_name": "Python 3",
   "language": "python",
   "name": "python3"
  },
  "language_info": {
   "codemirror_mode": {
    "name": "ipython",
    "version": 3
   },
   "file_extension": ".py",
   "mimetype": "text/x-python",
   "name": "python",
   "nbconvert_exporter": "python",
   "pygments_lexer": "ipython3",
   "version": "3.11.0"
  }
 },
 "nbformat": 4,
 "nbformat_minor": 4
}`

	case "pyforest-library":
		files["README.md"] = `# PyForest - Financial Analysis Library

A Python library for financial data analysis and portfolio management.

## Installation

` + "```bash" + `
pip install -e .
` + "```" + `

## Modules

- **returns.py**: Return calculations (simple, log, cumulative, Sharpe ratio)
- **portfolio.py**: Portfolio management and optimization
- **indicators.py**: Technical indicators (to be implemented)

## Usage

` + "```python" + `
from pyforest import calculate_returns, Portfolio

# Calculate returns
returns = calculate_returns(price_series)

# Create portfolio
portfolio = Portfolio(['MSFT', 'AAPL'], weights=[0.6, 0.4])
` + "```" + `

## Development

This library is designed to be extended by the agent with new financial analysis functions.
`
		files["setup.py"] = `from setuptools import setup, find_packages

setup(
    name="pyforest",
    version="0.1.0",
    description="Financial analysis library for portfolio returns calculation",
    packages=find_packages(),
    install_requires=[
        "pandas>=2.0.0",
        "numpy>=1.24.0",
    ],
)
`
		files["pyforest/__init__.py"] = `"""
PyForest - Financial Analysis Library
"""

from .returns import calculate_returns, calculate_cumulative_returns, calculate_sharpe_ratio
from .portfolio import Portfolio, PortfolioOptimizer

__version__ = "0.1.0"
__all__ = [
    "calculate_returns",
    "calculate_cumulative_returns",
    "calculate_sharpe_ratio",
    "Portfolio",
    "PortfolioOptimizer",
]
`
		files["pyforest/returns.py"] = `"""
Return calculation utilities for financial analysis
"""

import pandas as pd
import numpy as np


def calculate_returns(prices: pd.Series, method='simple') -> pd.Series:
    """
    Calculate returns from a price series.

    Parameters:
    -----------
    prices : pd.Series
        Time series of prices
    method : str
        'simple' for simple returns, 'log' for logarithmic returns

    Returns:
    --------
    pd.Series
        Series of returns
    """
    if method == 'simple':
        return prices.pct_change()
    elif method == 'log':
        return np.log(prices / prices.shift(1))
    else:
        raise ValueError(f"Unknown method: {method}")


def calculate_cumulative_returns(returns: pd.Series) -> pd.Series:
    """
    Calculate cumulative returns from a returns series.

    Parameters:
    -----------
    returns : pd.Series
        Series of period returns

    Returns:
    --------
    pd.Series
        Cumulative returns
    """
    return (1 + returns).cumprod() - 1


def calculate_sharpe_ratio(returns: pd.Series, risk_free_rate: float = 0.02, periods_per_year: int = 252) -> float:
    """
    Calculate annualized Sharpe ratio.

    Parameters:
    -----------
    returns : pd.Series
        Series of period returns
    risk_free_rate : float
        Annual risk-free rate (default 2%)
    periods_per_year : int
        Number of periods per year (252 for daily trading days)

    Returns:
    --------
    float
        Annualized Sharpe ratio
    """
    excess_returns = returns - (risk_free_rate / periods_per_year)
    return (excess_returns.mean() / excess_returns.std()) * np.sqrt(periods_per_year)


def calculate_period_returns(df: pd.DataFrame, price_column: str = 'Close') -> pd.DataFrame:
    """
    Calculate returns over multiple standard periods.

    Parameters:
    -----------
    df : pd.DataFrame
        DataFrame with price data
    price_column : str
        Name of the price column

    Returns:
    --------
    pd.DataFrame
        DataFrame with period returns
    """
    periods = [
        ('1M', 21),
        ('3M', 63),
        ('6M', 126),
        ('1Y', 252),
        ('2Y', 504),
        ('5Y', 1260),
    ]

    results = []
    for label, days in periods:
        if len(df) >= days:
            period_data = df.iloc[-days:]
            total_return = (period_data[price_column].iloc[-1] / period_data[price_column].iloc[0] - 1)
            results.append({
                'Period': label,
                'Days': days,
                'Return': total_return,
                'Start_Price': period_data[price_column].iloc[0],
                'End_Price': period_data[price_column].iloc[-1],
            })

    return pd.DataFrame(results)
`
		files["pyforest/portfolio.py"] = `"""
Portfolio management and optimization utilities
"""

import pandas as pd
import numpy as np
from typing import List, Dict


class Portfolio:
    """
    Portfolio class for managing multiple assets and calculating portfolio metrics.
    """

    def __init__(self, tickers: List[str], weights: List[float] = None):
        """
        Initialize portfolio with tickers and optional weights.

        Parameters:
        -----------
        tickers : List[str]
            List of ticker symbols
        weights : List[float], optional
            Portfolio weights (must sum to 1.0). If None, equal weights are used.
        """
        self.tickers = tickers

        if weights is None:
            self.weights = np.array([1.0 / len(tickers)] * len(tickers))
        else:
            self.weights = np.array(weights)
            if not np.isclose(self.weights.sum(), 1.0):
                raise ValueError("Weights must sum to 1.0")

    def calculate_portfolio_returns(self, returns_df: pd.DataFrame) -> pd.Series:
        """
        Calculate portfolio returns from individual asset returns.

        Parameters:
        -----------
        returns_df : pd.DataFrame
            DataFrame with returns for each ticker (columns = tickers)

        Returns:
        --------
        pd.Series
            Portfolio returns time series
        """
        return (returns_df[self.tickers] * self.weights).sum(axis=1)

    def calculate_metrics(self, returns: pd.Series, periods_per_year: int = 252) -> Dict:
        """
        Calculate portfolio performance metrics.

        Returns:
        --------
        Dict with metrics: mean_return, volatility, sharpe_ratio, max_drawdown
        """
        metrics = {
            'mean_return': returns.mean() * periods_per_year,
            'volatility': returns.std() * np.sqrt(periods_per_year),
            'sharpe_ratio': (returns.mean() / returns.std()) * np.sqrt(periods_per_year),
        }

        # Calculate maximum drawdown
        cumulative = (1 + returns).cumprod()
        running_max = cumulative.expanding().max()
        drawdown = (cumulative - running_max) / running_max
        metrics['max_drawdown'] = drawdown.min()

        return metrics


class PortfolioOptimizer:
    """
    Portfolio optimization using mean-variance optimization.
    """

    def __init__(self, returns_df: pd.DataFrame):
        """
        Initialize optimizer with historical returns data.

        Parameters:
        -----------
        returns_df : pd.DataFrame
            DataFrame with returns for each ticker
        """
        self.returns_df = returns_df
        self.mean_returns = returns_df.mean()
        self.cov_matrix = returns_df.cov()

    def optimize_sharpe(self) -> Dict:
        """
        Find portfolio weights that maximize Sharpe ratio.

        Returns:
        --------
        Dict with optimal weights and expected metrics
        """
        # Simple implementation - can be improved with scipy.optimize
        # For now, return equal weights
        n_assets = len(self.returns_df.columns)
        weights = np.array([1.0 / n_assets] * n_assets)

        return {
            'weights': dict(zip(self.returns_df.columns, weights)),
            'expected_return': np.dot(weights, self.mean_returns),
            'expected_volatility': np.sqrt(np.dot(weights.T, np.dot(self.cov_matrix, weights))),
        }
`

	default:
		// Generic project
		files["README.md"] = fmt.Sprintf("# %s\n\nA sample project.\n", sampleType)
		files["src/main.txt"] = "Main source file\n"
	}

	// Add common files
	files[".gitignore"] = `# Dependencies
node_modules/
__pycache__/
*.pyc

# Build outputs
build/
dist/

# Environment
.env
.env.local

# IDE
.vscode/
.idea/
`

	return files
}

// BrowseTree lists files and directories at a given path in a specific branch
func (s *GitRepositoryService) BrowseTree(ctx context.Context, repoID string, path string, branch string) ([]types.TreeEntry, error) {
	// Get repository to find local path
	repo, err := s.GetRepository(ctx, repoID)
	if err != nil {
		return nil, fmt.Errorf("repository not found: %w", err)
	}

	if repo.LocalPath == "" {
		return nil, fmt.Errorf("repository has no local path")
	}

	// Open the bare repository
	gitRepo, err := git.PlainOpen(repo.LocalPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open git repository: %w", err)
	}

	// Get reference for specified branch, default to HEAD
	var ref *plumbing.Reference
	if branch != "" {
		// Try to resolve the branch
		branchRef := plumbing.NewBranchReferenceName(branch)
		ref, err = gitRepo.Reference(branchRef, true)
		if err != nil {
			return nil, fmt.Errorf("failed to find branch %s: %w", branch, err)
		}
	} else {
		// Default to HEAD
		ref, err = gitRepo.Head()
		if err != nil {
			return nil, fmt.Errorf("failed to get HEAD: %w", err)
		}
	}

	// Get the commit
	commit, err := gitRepo.CommitObject(ref.Hash())
	if err != nil {
		return nil, fmt.Errorf("failed to get commit: %w", err)
	}

	// Get the tree
	tree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("failed to get tree: %w", err)
	}

	// Navigate to the requested path
	if path != "." && path != "" {
		tree, err = tree.Tree(path)
		if err != nil {
			return nil, fmt.Errorf("path not found in repository: %w", err)
		}
	}

	// Build tree entries
	result := make([]types.TreeEntry, 0, len(tree.Entries))
	for _, entry := range tree.Entries {
		entryPath := path
		if entryPath == "." || entryPath == "" {
			entryPath = entry.Name
		} else {
			entryPath = filepath.Join(path, entry.Name)
		}

		// Determine if entry is a directory
		isDir := entry.Mode == filemode.Dir

		// Get size (only available for files/blobs)
		var size int64
		if !isDir {
			// Get blob to read size
			blob, err := gitRepo.BlobObject(entry.Hash)
			if err == nil {
				size = blob.Size
			}
		}

		result = append(result, types.TreeEntry{
			Name:  entry.Name,
			Path:  entryPath,
			IsDir: isDir,
			Size:  size,
		})
	}

	return result, nil
}

// ListBranches returns all branches in the repository
func (s *GitRepositoryService) ListBranches(ctx context.Context, repoID string) ([]string, error) {
	// Get repository to find local path
	repo, err := s.GetRepository(ctx, repoID)
	if err != nil {
		return nil, fmt.Errorf("repository not found: %w", err)
	}

	if repo.LocalPath == "" {
		return nil, fmt.Errorf("repository has no local path")
	}

	// Open the bare repository
	gitRepo, err := git.PlainOpen(repo.LocalPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open git repository: %w", err)
	}

	// Get all references
	refs, err := gitRepo.References()
	if err != nil {
		return nil, fmt.Errorf("failed to get references: %w", err)
	}

	// Filter to branches only
	var branches []string
	err = refs.ForEach(func(ref *plumbing.Reference) error {
		if ref.Name().IsBranch() {
			branches = append(branches, ref.Name().Short())
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to iterate references: %w", err)
	}

	return branches, nil
}

// GetFileContents reads the contents of a file from a specific branch
func (s *GitRepositoryService) GetFileContents(ctx context.Context, repoID string, path string, branch string) (string, error) {
	// Get repository to find local path
	repo, err := s.GetRepository(ctx, repoID)
	if err != nil {
		return "", fmt.Errorf("repository not found: %w", err)
	}

	if repo.LocalPath == "" {
		return "", fmt.Errorf("repository has no local path")
	}

	// Open the bare repository
	gitRepo, err := git.PlainOpen(repo.LocalPath)
	if err != nil {
		return "", fmt.Errorf("failed to open git repository: %w", err)
	}

	// Get branch reference - use HEAD if no branch specified
	var ref *plumbing.Reference
	if branch == "" {
		ref, err = gitRepo.Head()
		if err != nil {
			return "", fmt.Errorf("failed to get HEAD: %w", err)
		}
	} else {
		ref, err = gitRepo.Reference(plumbing.NewBranchReferenceName(branch), true)
		if err != nil {
			return "", fmt.Errorf("failed to get branch reference: %w", err)
		}
	}

	// Get the commit
	commit, err := gitRepo.CommitObject(ref.Hash())
	if err != nil {
		return "", fmt.Errorf("failed to get commit: %w", err)
	}

	// Get the tree
	tree, err := commit.Tree()
	if err != nil {
		return "", fmt.Errorf("failed to get tree: %w", err)
	}

	// Get the file
	file, err := tree.File(path)
	if err != nil {
		return "", fmt.Errorf("file not found in repository: %w", err)
	}

	// Read file contents
	content, err := file.Contents()
	if err != nil {
		return "", fmt.Errorf("failed to read file contents: %w", err)
	}

	return content, nil
}

func (s *GitRepositoryService) pullChanges(gitRepo *types.GitRepository) error {
	log.Info().Str("repo_id", gitRepo.ID).Msg("Checking for new commits")

	repo, err := git.PlainOpen(gitRepo.LocalPath)
	if err != nil {
		return fmt.Errorf("failed to open git repository %s: %w", gitRepo.LocalPath, err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	pullOptions := &git.PullOptions{
		RemoteName: "origin",
	}
	if gitRepo.Password != "" {
		username := gitRepo.Username
		if username == "" {
			username = "PAT"
		}
		pullOptions.Auth = &http.BasicAuth{Username: username, Password: gitRepo.Password}
	}

	err = worktree.Pull(pullOptions)
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return fmt.Errorf("failed to pull changes: %w", err)
	}

	log.Info().Str("repo_id", gitRepo.ID).Msg("Pulled latest changes")

	return nil
}

func (s *GitRepositoryService) pushChangesToExternal(gitRepo *types.GitRepository) error {
	log.Info().Str("repo_id", gitRepo.ID).Msg("Pushing changes to external repository")

	repo, err := git.PlainOpen(gitRepo.LocalPath)
	if err != nil {
		return fmt.Errorf("failed to open git repository %s: %w", gitRepo.LocalPath, err)
	}

	pushOptions := &git.PushOptions{
		RemoteName: "origin",
		Progress:   os.Stdout,
	}
	if gitRepo.Password != "" {
		username := gitRepo.Username
		if username == "" {
			username = "PAT"
		}
		pushOptions.Auth = &http.BasicAuth{Username: username, Password: gitRepo.Password}
	}

	err = repo.Push(pushOptions)
	if err != nil {
		if errors.Is(err, git.NoErrAlreadyUpToDate) {
			log.Info().Str("repo_id", gitRepo.ID).Msg("External repository already up to date")
			return nil
		}
		return fmt.Errorf("failed to push changes: %w", err)
	}

	log.Info().Str("repo_id", gitRepo.ID).Msg("Pushed changes to external repository")

	return nil
}

// PushPullRequest pulls from external repository all commits and pushes any commits from the local repository to the external repository (upstream)
// This is used to update the external repository with the latest commits from the local repository that we have made changes to
func (s *GitRepositoryService) PushPullRequest(ctx context.Context, repoID, branchName string) error {
	gitRepo, err := s.GetRepository(ctx, repoID)
	if err != nil {
		return fmt.Errorf("repository not found: %w", err)
	}

	if !gitRepo.IsExternal {
		return fmt.Errorf("repository is not external, cannot push/pull")
	}

	// If branchName is specified, checkout that branch first
	if branchName != "" {
		repo, err := git.PlainOpen(gitRepo.LocalPath)
		if err != nil {
			return fmt.Errorf("failed to open git repository: %w", err)
		}

		w, err := repo.Worktree()
		if err != nil {
			return fmt.Errorf("failed to get worktree: %w", err)
		}

		head, err := repo.Head()
		if err != nil {
			return fmt.Errorf("failed to get HEAD: %w", err)
		}

		if head.Name().Short() != branchName {
			// Try to find the branch
			branchRef := plumbing.NewBranchReferenceName(branchName)
			err = w.Checkout(&git.CheckoutOptions{
				Branch: branchRef,
			})
			if err != nil {
				return fmt.Errorf("failed to checkout branch %s: %w", branchName, err)
			}
		}
	}

	// 1. call pullChanges
	err = s.pullChanges(gitRepo)
	if err != nil {
		// Return error on merge conflict or other pull errors
		return fmt.Errorf("failed to pull changes (possible merge conflict): %w", err)
	}

	// 2. call pushChangesToExternal
	err = s.pushChangesToExternal(gitRepo)
	if err != nil {
		return fmt.Errorf("failed to push changes: %w", err)
	}

	return nil
}

// CreateBranch creates a new branch in the repository
func (s *GitRepositoryService) CreateBranch(ctx context.Context, repoID, branchName, baseBranch string) error {
	repo, err := s.store.GetGitRepository(ctx, repoID)
	if err != nil {
		return fmt.Errorf("failed to get repository: %w", err)
	}

	// Open the repository
	gitRepo, err := git.PlainOpen(repo.LocalPath)
	if err != nil {
		return fmt.Errorf("failed to open git repository: %w", err)
	}

	// Get base branch reference
	var baseRef *plumbing.Reference
	if baseBranch == "" {
		baseBranch = repo.DefaultBranch
	}

	baseRef, err = gitRepo.Reference(plumbing.NewBranchReferenceName(baseBranch), true)
	if err != nil {
		// Try HEAD if base branch not found
		baseRef, err = gitRepo.Head()
		if err != nil {
			return fmt.Errorf("failed to get base branch reference: %w", err)
		}
	}

	// Create new branch reference
	newBranchRef := plumbing.NewBranchReferenceName(branchName)

	// Check if branch already exists
	_, err = gitRepo.Reference(newBranchRef, true)
	if err == nil {
		// Branch already exists
		log.Warn().
			Str("branch", branchName).
			Str("repo", repoID).
			Msg("[GitRepo] Branch already exists")
		return nil // Not an error - branch exists
	}

	// Create the new branch
	err = gitRepo.Storer.SetReference(plumbing.NewHashReference(newBranchRef, baseRef.Hash()))
	if err != nil {
		return fmt.Errorf("failed to create branch: %w", err)
	}

	log.Info().
		Str("branch", branchName).
		Str("base_branch", baseBranch).
		Str("repo", repoID).
		Msg("[GitRepo] Created branch")

	return nil
}

// CreateOrUpdateFileContents creates or updates a file in a repository and commits it
func (s *GitRepositoryService) CreateOrUpdateFileContents(
	ctx context.Context,
	repoID string,
	path string,
	branch string,
	content []byte,
	commitMessage string,
	authorName string,
	authorEmail string,
) (string, error) {
	repo, err := s.GetRepository(ctx, repoID)
	if err != nil {
		return "", fmt.Errorf("repository not found: %w", err)
	}

	if repo.LocalPath == "" {
		return "", fmt.Errorf("repository has no local path")
	}

	if branch == "" {
		branch = repo.DefaultBranch
		if branch == "" {
			branch = "main"
		}
	}

	if commitMessage == "" {
		commitMessage = fmt.Sprintf("Update %s", path)
	}

	tempClone, err := os.MkdirTemp("", "helix-git-update-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempClone)

	var gitRepo *git.Repository
	if repo.IsExternal && repo.ExternalURL != "" {
		cloneOptions := &git.CloneOptions{
			URL: repo.ExternalURL,
		}
		if repo.Password != "" {
			username := repo.Username
			if username == "" {
				username = "PAT"
			}
			cloneOptions.Auth = &http.BasicAuth{
				Username: username,
				Password: repo.Password,
			}
		}
		gitRepo, err = git.PlainClone(tempClone, cloneOptions)
		if err != nil {
			return "", fmt.Errorf("failed to clone external repository: %w", err)
		}
	} else {
		cloneURL := repo.LocalPath
		if !strings.HasPrefix(cloneURL, "file://") && !strings.HasPrefix(cloneURL, "http://") && !strings.HasPrefix(cloneURL, "https://") {
			cloneURL = "file://" + cloneURL
		}
		gitRepo, err = git.PlainClone(tempClone, &git.CloneOptions{
			URL: cloneURL,
		})
		if err != nil {
			return "", fmt.Errorf("failed to clone repository: %w", err)
		}
	}

	branchRef := plumbing.NewBranchReferenceName(branch)
	_, err = gitRepo.Reference(branchRef, true)
	if err != nil {
		headRef, headErr := gitRepo.Head()
		if headErr != nil {
			return "", fmt.Errorf("failed to get HEAD and branch %s does not exist: %w", branch, err)
		}
		branchRef = plumbing.NewBranchReferenceName(branch)
		err = gitRepo.Storer.SetReference(plumbing.NewHashReference(branchRef, headRef.Hash()))
		if err != nil {
			return "", fmt.Errorf("failed to create branch %s: %w", branch, err)
		}
	}

	worktree, err := gitRepo.Worktree()
	if err != nil {
		return "", fmt.Errorf("failed to get worktree: %w", err)
	}

	err = worktree.Checkout(&git.CheckoutOptions{
		Branch: branchRef,
		Create: false,
	})
	if err != nil {
		return "", fmt.Errorf("failed to checkout branch %s: %w", branch, err)
	}

	fullPath := filepath.Join(tempClone, path)
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(fullPath, content, 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	if _, err := worktree.Add(path); err != nil {
		return "", fmt.Errorf("failed to stage file: %w", err)
	}

	status, err := worktree.Status()
	if err != nil {
		return "", fmt.Errorf("failed to get git status: %w", err)
	}

	if status.IsClean() {
		existingContent, readErr := os.ReadFile(fullPath)
		if readErr == nil && string(existingContent) == string(content) {
			return string(content), nil
		}
	}

	if authorName == "" {
		authorName = s.gitUserName
	}
	if authorEmail == "" {
		authorEmail = s.gitUserEmail
	}

	_, err = worktree.Commit(commitMessage, &git.CommitOptions{
		Author: &object.Signature{
			Name:  authorName,
			Email: authorEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to commit: %w", err)
	}

	pushOptions := &git.PushOptions{
		RemoteName: "origin",
		RefSpecs: []config.RefSpec{
			config.RefSpec(fmt.Sprintf("+refs/heads/%s:refs/heads/%s", branch, branch)),
		},
	}
	if repo.IsExternal && repo.Password != "" {
		username := repo.Username
		if username == "" {
			username = "PAT"
		}
		pushOptions.Auth = &http.BasicAuth{
			Username: username,
			Password: repo.Password,
		}
	}
	err = gitRepo.Push(pushOptions)
	if err != nil {
		return "", fmt.Errorf("failed to push to repository: %w", err)
	}

	log.Info().
		Str("repo_id", repoID).
		Str("path", path).
		Str("branch", branch).
		Msg("Successfully created/updated file in repository")

	return string(content), nil
}
