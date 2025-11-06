package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// AuthorizationFunc is a callback to check if a user can perform an action on a resource
type AuthorizationFunc func(ctx context.Context, user *types.User, orgID, resourceID string, resourceType types.Resource, action types.Action) error

// GitHTTPServer provides HTTP access to git repositories with API key authentication
// Enables Zed agents to clone/push to repositories over the network
type GitHTTPServer struct {
	store             store.Store
	gitRepoService    *GitRepositoryService
	serverBaseURL     string
	gitExecutablePath string
	authTokenHeader   string
	enablePush        bool
	enablePull        bool
	maxRepoSize       int64 // Maximum repository size in bytes
	requestTimeout    time.Duration
	testMode          bool
	authorizeFn       AuthorizationFunc // Callback to server's RBAC system
}

// GitHTTPServerConfig represents configuration for the git HTTP server
type GitHTTPServerConfig struct {
	ServerBaseURL     string        `json:"server_base_url"`
	GitExecutablePath string        `json:"git_executable_path"`
	AuthTokenHeader   string        `json:"auth_token_header"`
	EnablePush        bool          `json:"enable_push"`
	EnablePull        bool          `json:"enable_pull"`
	MaxRepoSize       int64         `json:"max_repo_size"`
	RequestTimeout    time.Duration `json:"request_timeout"`
}

// GitCloneInfo represents information needed for git clone operations
type GitCloneInfo struct {
	RepositoryID string `json:"repository_id"`
	CloneURL     string `json:"clone_url"`
	AuthToken    string `json:"auth_token"`
	Username     string `json:"username"`
	ProjectPath  string `json:"project_path"`
	Instructions string `json:"instructions"`
}

// NewGitHTTPServer creates a new git HTTP server
func NewGitHTTPServer(
	store store.Store,
	gitRepoService *GitRepositoryService,
	config *GitHTTPServerConfig,
	authorizeFn AuthorizationFunc,
) *GitHTTPServer {
	// Set defaults
	if config.GitExecutablePath == "" {
		config.GitExecutablePath = "git"
	}
	if config.AuthTokenHeader == "" {
		config.AuthTokenHeader = "Authorization"
	}
	if config.RequestTimeout == 0 {
		config.RequestTimeout = 5 * time.Minute
	}
	if config.MaxRepoSize == 0 {
		config.MaxRepoSize = 1024 * 1024 * 1024 // 1GB default
	}

	return &GitHTTPServer{
		store:             store,
		gitRepoService:    gitRepoService,
		serverBaseURL:     config.ServerBaseURL,
		gitExecutablePath: config.GitExecutablePath,
		authTokenHeader:   config.AuthTokenHeader,
		enablePush:        config.EnablePush,
		enablePull:        config.EnablePull,
		maxRepoSize:       config.MaxRepoSize,
		requestTimeout:    config.RequestTimeout,
		testMode:          false,
		authorizeFn:       authorizeFn,
	}
}

// SetTestMode enables or disables test mode
func (s *GitHTTPServer) SetTestMode(enabled bool) {
	s.testMode = enabled
}

// RegisterRoutes registers HTTP git server routes
func (s *GitHTTPServer) RegisterRoutes(router *mux.Router) {
	// Git HTTP protocol routes
	gitRouter := router.PathPrefix("/git").Subrouter()

	// Add authentication middleware
	gitRouter.Use(s.authMiddleware)

	// Git smart HTTP protocol routes
	gitRouter.HandleFunc("/{repo_id}/info/refs", s.handleInfoRefs).Methods("GET")
	gitRouter.HandleFunc("/{repo_id}/git-upload-pack", s.handleUploadPack).Methods("POST")
	gitRouter.HandleFunc("/{repo_id}/git-receive-pack", s.handleReceivePack).Methods("POST")

	// Repository information routes
	gitRouter.HandleFunc("/{repo_id}/clone-info", s.handleCloneInfo).Methods("GET")
	gitRouter.HandleFunc("/{repo_id}/status", s.handleRepositoryStatus).Methods("GET")

	log.Info().Msg("Git HTTP server routes registered")
}

// GetCloneURL returns the HTTP clone URL for a repository
func (s *GitHTTPServer) GetCloneURL(repositoryID string) string {
	return fmt.Sprintf("%s/git/%s", s.serverBaseURL, repositoryID)
}

// GetCloneCommand returns the complete git clone command with authentication
func (s *GitHTTPServer) GetCloneCommand(repositoryID string, apiKey string, targetDir string) string {
	cloneURL := s.GetCloneURL(repositoryID)

	// Add authentication to URL
	authenticatedURL := strings.Replace(cloneURL, "://", fmt.Sprintf("://api:%s@", apiKey), 1)

	if targetDir == "" {
		return fmt.Sprintf("git clone %s", authenticatedURL)
	}
	return fmt.Sprintf("git clone %s %s", authenticatedURL, targetDir)
}

// GetCloneInstructions returns comprehensive instructions for Zed agents
func (s *GitHTTPServer) GetCloneInstructions(repositoryID string, apiKey string) string {
	cloneURL := s.GetCloneURL(repositoryID)
	authenticatedURL := strings.Replace(cloneURL, "://", fmt.Sprintf("://api:%s@", apiKey), 1)

	return fmt.Sprintf(`# Git Repository Access Instructions

## Repository Information
- Repository ID: %s
- Clone URL: %s

## For Zed Agents - Clone Repository

### Method 1: Using API Key in URL
`+"```bash\n"+`git clone %s %s
cd %s
`+"```\n"+`

### Method 2: Using Environment Variables
`+"```bash\n"+`export GIT_USERNAME=api
export GIT_PASSWORD=%s
git clone %s %s
cd %s
`+"```\n"+`

### Getting Your API Key
Your Helix API key can be found in:
- Account Settings → API Keys
- Use any existing API key (no special git keys needed)
- Create new API key if needed: POST /api/v1/api_keys

## Working with Specifications

After cloning, you can find planning specifications in:
- docs/specs/requirements.md - User requirements (EARS notation)
- docs/specs/design.md - Technical design with codebase context
- docs/specs/tasks.md - Implementation plan
- docs/specs/coordination.md - Multi-session coordination strategy

## Committing Changes

When you make changes, commit them with descriptive messages:
`+"```bash\n"+`git add .
git commit -m "[SessionID] Description of changes"
git push origin your-branch-name
`+"```\n"+`

## Branching Strategy

- Use feature branches for implementation: feature/your-feature-name
- Use planning branches for specs: planning/spec-task-id
- Main branch contains approved specifications and completed features
`,
		repositoryID,
		cloneURL,
		authenticatedURL,
		repositoryID,
		repositoryID,
		apiKey,
		cloneURL,
		repositoryID,
		repositoryID,
	)
}

// authMiddleware provides API key authentication for git operations
func (s *GitHTTPServer) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract API key from various sources
		apiKey := s.extractAPIKey(r)

		if apiKey == "" {
			log.Warn().
				Str("path", r.URL.Path).
				Str("method", r.Method).
				Msg("Git request missing API key")
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}

		// Validate API key and get user object
		user, err := s.validateAPIKeyAndGetUser(r.Context(), apiKey)
		if err != nil {
			log.Error().Err(err).Msg("Failed to validate API key for git request")
			http.Error(w, "Authentication failed", http.StatusUnauthorized)
			return
		}

		if user == nil {
			log.Warn().
				Str("api_key_prefix", apiKey[:min(len(apiKey), 8)]).
				Msg("Invalid API key for git request")
			http.Error(w, "Invalid API key", http.StatusUnauthorized)
			return
		}

		// Add user object to request context (not just user ID)
		ctx := context.WithValue(r.Context(), "git_user", user)
		r = r.WithContext(ctx)

		log.Debug().
			Str("user_id", user.ID).
			Str("path", r.URL.Path).
			Msg("Git request authenticated")

		next.ServeHTTP(w, r)
	})
}

// extractAPIKey extracts API key from request
func (s *GitHTTPServer) extractAPIKey(r *http.Request) string {
	// Try Authorization header first
	if auth := r.Header.Get(s.authTokenHeader); auth != "" {
		// Handle "Bearer token" format
		if strings.HasPrefix(auth, "Bearer ") {
			return strings.TrimPrefix(auth, "Bearer ")
		}
		// Handle "Basic api:token" format for git
		if strings.HasPrefix(auth, "Basic ") {
			// For git HTTP auth, the format is usually "Basic base64(username:password)"
			// We'll expect "api:api_key" format
			return auth // Pass through for git to handle
		}
		return auth
	}

	// Try query parameter
	if token := r.URL.Query().Get("token"); token != "" {
		return token
	}

	// Try from URL userinfo (git clone https://api:token@server/repo)
	if r.URL.User != nil {
		if password, hasPassword := r.URL.User.Password(); hasPassword {
			return password
		}
	}

	return ""
}

// validateAPIKeyAndGetUser validates an API key and returns the full user object
func (s *GitHTTPServer) validateAPIKeyAndGetUser(ctx context.Context, apiKey string) (*types.User, error) {
	// In test mode, return a test user
	if s.testMode {
		return &types.User{
			ID:    "test_user",
			Email: "test@example.com",
			Admin: false,
		}, nil
	}

	// Handle Basic auth format from git
	if strings.HasPrefix(apiKey, "Basic ") {
		// Decode base64 and extract password (which is the API token)
		// Format: "Basic base64(username:password)" where username is "api" and password is the token
		encodedCreds := strings.TrimPrefix(apiKey, "Basic ")
		decodedBytes, err := base64.StdEncoding.DecodeString(encodedCreds)
		if err != nil {
			log.Debug().Err(err).Msg("Failed to decode Basic auth")
			return nil, fmt.Errorf("invalid Basic auth encoding")
		}

		// Split username:password
		credentials := string(decodedBytes)
		parts := strings.SplitN(credentials, ":", 2)
		if len(parts) != 2 {
			log.Debug().Str("credentials", credentials).Msg("Invalid Basic auth format")
			return nil, fmt.Errorf("invalid Basic auth format")
		}

		// Extract the password part (API token)
		apiKey = parts[1]

		log.Debug().
			Str("username", parts[0]).
			Str("api_key_prefix", apiKey[:min(len(apiKey), 8)]).
			Msg("Extracted API token from Basic auth")

		// Fall through to validate the extracted token below
	}

	// Use Helix's existing API key validation
	apiKeyRecord, err := s.store.GetAPIKey(ctx, apiKey)
	if err != nil {
		log.Debug().Err(err).Msg("Failed to get API key from store")
		return nil, err
	}

	if apiKeyRecord == nil {
		log.Debug().Str("api_key_prefix", apiKey[:min(len(apiKey), 8)]).Msg("API key not found")
		return nil, nil
	}

	// Check if API key is active and not expired
	if apiKeyRecord.Created.IsZero() {
		log.Debug().Str("api_key", apiKeyRecord.Key).Msg("API key is inactive")
		return nil, nil
	}

	// Get the user object
	user, err := s.store.GetUser(ctx, &store.GetUserQuery{ID: apiKeyRecord.Owner})
	if err != nil {
		log.Error().Err(err).Str("user_id", apiKeyRecord.Owner).Msg("Failed to get user for API key")
		return nil, err
	}

	log.Debug().
		Str("api_key", apiKeyRecord.Key).
		Str("user_id", user.ID).
		Msg("API key validated successfully for git access")

	return user, nil
}

// handleInfoRefs handles the git info/refs request
func (s *GitHTTPServer) handleInfoRefs(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["repo_id"]

	if repoID == "" {
		http.Error(w, "Repository ID required", http.StatusBadRequest)
		return
	}

	// Get repository
	repo, err := s.gitRepoService.GetRepository(r.Context(), repoID)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Repository not found for info/refs")
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	// Determine service type
	service := r.URL.Query().Get("service")
	if service == "" {
		http.Error(w, "Service parameter required", http.StatusBadRequest)
		return
	}

	// Execute git command
	cmd := exec.CommandContext(r.Context(), s.gitExecutablePath, service, "--stateless-rpc", "--advertise-refs", repo.LocalPath)
	cmd.Dir = repo.LocalPath

	// Set up response
	w.Header().Set("Content-Type", fmt.Sprintf("application/x-%s-advertisement", service))
	w.Header().Set("Cache-Control", "no-cache")

	// Write service header
	serviceHeader := fmt.Sprintf("# service=%s\n", service)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(serviceHeader)))
	w.Write([]byte(serviceHeader))

	// Execute git command and stream output
	output, err := cmd.Output()
	if err != nil {
		log.Error().Err(err).Str("service", service).Str("repo_id", repoID).Msg("Git command failed")
		http.Error(w, "Git operation failed", http.StatusInternalServerError)
		return
	}

	w.Write(output)

	log.Debug().
		Str("repo_id", repoID).
		Str("service", service).
		Int("response_size", len(output)).
		Msg("Git info/refs request completed")
}

// handleUploadPack handles git upload-pack requests (for git clone/pull)
func (s *GitHTTPServer) handleUploadPack(w http.ResponseWriter, r *http.Request) {
	if !s.enablePull {
		http.Error(w, "Pull operations disabled", http.StatusForbidden)
		return
	}

	vars := mux.Vars(r)
	repoID := vars["repo_id"]

	if repoID == "" {
		http.Error(w, "Repository ID required", http.StatusBadRequest)
		return
	}

	// Check if user has read access to repository
	user := s.getUser(r)
	if !s.hasReadAccess(r.Context(), user, repoID) {
		log.Warn().
			Str("user_id", user.ID).
			Str("repo_id", repoID).
			Msg("User does not have read access to repository")
		http.Error(w, "Read access denied", http.StatusForbidden)
		return
	}

	// Get repository
	repo, err := s.gitRepoService.GetRepository(r.Context(), repoID)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Repository not found for upload-pack")
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), s.requestTimeout)
	defer cancel()

	// Execute git upload-pack
	cmd := exec.CommandContext(ctx, s.gitExecutablePath, "upload-pack", "--stateless-rpc", repo.LocalPath)
	cmd.Dir = repo.LocalPath
	cmd.Stdin = r.Body

	// Set up response
	w.Header().Set("Content-Type", "application/x-git-upload-pack-result")
	w.Header().Set("Cache-Control", "no-cache")

	// Execute and stream output
	output, err := cmd.Output()
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Git upload-pack failed")
		http.Error(w, "Git upload-pack failed", http.StatusInternalServerError)
		return
	}

	w.Write(output)

	log.Info().
		Str("repo_id", repoID).
		Str("user_id", user.ID).
		Int("response_size", len(output)).
		Msg("Git upload-pack completed")
}

// handleReceivePack handles git receive-pack requests (for git push)
func (s *GitHTTPServer) handleReceivePack(w http.ResponseWriter, r *http.Request) {
	if !s.enablePush {
		http.Error(w, "Push operations disabled", http.StatusForbidden)
		return
	}

	vars := mux.Vars(r)
	repoID := vars["repo_id"]

	if repoID == "" {
		http.Error(w, "Repository ID required", http.StatusBadRequest)
		return
	}

	// Check if user has push permissions
	user := s.getUser(r)
	if !s.hasWriteAccess(r.Context(), user, repoID) {
		log.Warn().
			Str("user_id", user.ID).
			Str("repo_id", repoID).
			Msg("User does not have push access to repository")
		http.Error(w, "Push access denied", http.StatusForbidden)
		return
	}

	// Get repository
	repo, err := s.gitRepoService.GetRepository(r.Context(), repoID)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Repository not found for receive-pack")
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), s.requestTimeout)
	defer cancel()

	// Execute git receive-pack
	cmd := exec.CommandContext(ctx, s.gitExecutablePath, "receive-pack", "--stateless-rpc", repo.LocalPath)
	cmd.Dir = repo.LocalPath
	cmd.Stdin = r.Body

	// Set up response
	w.Header().Set("Content-Type", "application/x-git-receive-pack-result")
	w.Header().Set("Cache-Control", "no-cache")

	// Execute and stream output
	output, err := cmd.Output()
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Git receive-pack failed")
		http.Error(w, "Git receive-pack failed", http.StatusInternalServerError)
		return
	}

	w.Write(output)

	log.Info().
		Str("repo_id", repoID).
		Str("user_id", user.ID).
		Int("response_size", len(output)).
		Msg("Git receive-pack completed")
}

// handleCloneInfo provides clone information for a repository
func (s *GitHTTPServer) handleCloneInfo(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["repo_id"]

	if repoID == "" {
		http.Error(w, "Repository ID required", http.StatusBadRequest)
		return
	}

	// Get repository
	repo, err := s.gitRepoService.GetRepository(r.Context(), repoID)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Repository not found for clone info")
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	// Extract API key for instructions
	apiKey := s.extractAPIKey(r)

	cloneInfo := GitCloneInfo{
		RepositoryID: repoID,
		CloneURL:     s.GetCloneURL(repoID),
		AuthToken:    apiKey,
		Username:     "api",
		ProjectPath:  repo.LocalPath,
		Instructions: s.GetCloneInstructions(repoID, apiKey),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(cloneInfo); err != nil {
		log.Error().Err(err).Msg("Failed to encode clone info response")
	}
}

// handleRepositoryStatus provides repository status information
func (s *GitHTTPServer) handleRepositoryStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["repo_id"]

	if repoID == "" {
		http.Error(w, "Repository ID required", http.StatusBadRequest)
		return
	}

	// Get repository
	repo, err := s.gitRepoService.GetRepository(r.Context(), repoID)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Repository not found for status")
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	// Get repository stats
	stats, err := s.getRepositoryStats(repo.LocalPath)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to get repository stats")
		http.Error(w, "Failed to get repository stats", http.StatusInternalServerError)
		return
	}

	status := map[string]interface{}{
		"repository_id":  repoID,
		"name":           repo.Name,
		"status":         repo.Status,
		"default_branch": repo.DefaultBranch,
		"branches":       repo.Branches,
		"last_activity":  repo.LastActivity,
		"stats":          stats,
		"clone_url":      s.GetCloneURL(repoID),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// getRepositoryStats gets basic repository statistics
func (s *GitHTTPServer) getRepositoryStats(repoPath string) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Get repository size
	size, err := s.getDirectorySize(repoPath)
	if err != nil {
		log.Warn().Err(err).Str("repo_path", repoPath).Msg("Failed to get repository size")
		size = 0
	}
	stats["size_bytes"] = size

	// Get commit count (if git is available)
	if s.gitExecutablePath != "" {
		cmd := exec.Command(s.gitExecutablePath, "rev-list", "--count", "HEAD")
		cmd.Dir = repoPath

		if output, err := cmd.Output(); err == nil {
			stats["commit_count"] = strings.TrimSpace(string(output))
		}
	}

	// Get last commit info
	if s.gitExecutablePath != "" {
		cmd := exec.Command(s.gitExecutablePath, "log", "-1", "--format=%H,%an,%ae,%at", "HEAD")
		cmd.Dir = repoPath

		if output, err := cmd.Output(); err == nil {
			parts := strings.Split(strings.TrimSpace(string(output)), ",")
			if len(parts) == 4 {
				stats["last_commit"] = map[string]string{
					"hash":         parts[0],
					"author_name":  parts[1],
					"author_email": parts[2],
					"timestamp":    parts[3],
				}
			}
		}
	}

	return stats, nil
}

// getDirectorySize calculates the size of a directory
func (s *GitHTTPServer) getDirectorySize(path string) (int64, error) {
	var size int64

	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})

	return size, err
}

// hasReadAccess checks if a user has read access to a repository using existing RBAC system
func (s *GitHTTPServer) hasReadAccess(ctx context.Context, user *types.User, repoID string) bool {
	if user == nil {
		return false
	}

	// Get repository to check organization
	repo, err := s.gitRepoService.GetRepository(ctx, repoID)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to get repository for access check")
		return false
	}

	// Repository owner always has access
	if repo.OwnerID == user.ID {
		return true
	}

	// Admin users have access to all repositories
	if user.Admin {
		return true
	}

	// If repository has no organization, only owner and admin can access
	if repo.OrganizationID == "" {
		return false
	}

	// Use existing RBAC system from server
	err = s.authorizeFn(ctx, user, repo.OrganizationID, repoID, types.ResourceGitRepository, types.ActionGet)
	return err == nil
}

// hasWriteAccess checks if a user has write access to a repository using existing RBAC system
func (s *GitHTTPServer) hasWriteAccess(ctx context.Context, user *types.User, repoID string) bool {
	if user == nil {
		return false
	}

	// Get repository to check organization
	repo, err := s.gitRepoService.GetRepository(ctx, repoID)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to get repository for access check")
		return false
	}

	// Repository owner always has write access
	if repo.OwnerID == user.ID {
		return true
	}

	// Admin users have write access to all repositories
	if user.Admin {
		return true
	}

	// If repository has no organization, only owner and admin can access
	if repo.OrganizationID == "" {
		return false
	}

	// Use existing RBAC system from server
	err = s.authorizeFn(ctx, user, repo.OrganizationID, repoID, types.ResourceGitRepository, types.ActionUpdate)
	return err == nil
}

// getUser extracts user object from request context
func (s *GitHTTPServer) getUser(r *http.Request) *types.User {
	if user := r.Context().Value("git_user"); user != nil {
		if u, ok := user.(*types.User); ok {
			return u
		}
	}
	return nil
}

// CreateAPIKeyForRepository creates an API key specifically for repository access
func (s *GitHTTPServer) CreateAPIKeyForRepository(ctx context.Context, userID string, repoID string) (string, error) {
	// For git access, users can use their existing Helix API keys
	// No need to create repository-specific keys

	// In test mode, return a mock API key
	if s.testMode {
		return fmt.Sprintf("git_key_%s_%s_%d", userID, repoID, time.Now().Unix()), nil
	}

	// In production, users should use their existing API keys from:
	// - Account settings page
	// - API key management interface
	// - Or create new general-purpose API keys

	return "", fmt.Errorf("use existing Helix API keys for git access - no repository-specific keys needed")
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
