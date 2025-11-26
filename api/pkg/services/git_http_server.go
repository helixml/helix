package services

import (
	"bytes"
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
	"github.com/helixml/helix/api/pkg/system"
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
		// Log incoming request for debugging
		log.Info().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Str("auth_header", r.Header.Get("Authorization")).
			Str("remote_addr", r.RemoteAddr).
			Msg("Git HTTP request received")

		// Extract API key from various sources
		apiKey := s.extractAPIKey(r)

		if apiKey == "" {
			log.Warn().
				Str("path", r.URL.Path).
				Str("method", r.Method).
				Msg("Git request missing API key")
			// Send WWW-Authenticate header for HTTP Basic auth flow
			// This tells git to retry with credentials from the URL
			w.Header().Set("WWW-Authenticate", "Basic realm=\"Helix Git Server\"")
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}

		log.Debug().
			Str("api_key_prefix", apiKey[:min(len(apiKey), 15)]).
			Bool("is_basic_auth", strings.HasPrefix(apiKey, "Basic ")).
			Msg("Extracted API key from request")

		// Validate API key and get user object
		user, err := s.validateAPIKeyAndGetUser(r.Context(), apiKey)
		if err != nil {
			log.Error().
				Err(err).
				Str("api_key_prefix", apiKey[:min(len(apiKey), 15)]).
				Msg("Failed to validate API key for git request")
			w.Header().Set("WWW-Authenticate", "Basic realm=\"Helix Git Server\"")
			http.Error(w, "Authentication failed", http.StatusUnauthorized)
			return
		}

		if user == nil {
			log.Warn().
				Str("api_key_prefix", apiKey[:min(len(apiKey), 8)]).
				Msg("Invalid API key for git request")
			w.Header().Set("WWW-Authenticate", "Basic realm=\"Helix Git Server\"")
			http.Error(w, "Invalid API key", http.StatusUnauthorized)
			return
		}

		// Add user object to request context (not just user ID)
		ctx := context.WithValue(r.Context(), "git_user", user)
		r = r.WithContext(ctx)

		log.Info().
			Str("user_id", user.ID).
			Str("path", r.URL.Path).
			Msg("✅ Git request authenticated successfully")

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

// handleInfoRefs handles the git info/refs request using git http-backend CGI
func (s *GitHTTPServer) handleInfoRefs(w http.ResponseWriter, r *http.Request) {
	s.handleGitHTTPBackend(w, r)
}

// handleUploadPack handles git upload-pack requests (for git clone/pull) using git http-backend
func (s *GitHTTPServer) handleUploadPack(w http.ResponseWriter, r *http.Request) {
	if !s.enablePull {
		http.Error(w, "Pull operations disabled", http.StatusForbidden)
		return
	}

	// Check if user has read access to repository
	vars := mux.Vars(r)
	repoID := vars["repo_id"]
	user := s.getUser(r)
	if !s.hasReadAccess(r.Context(), user, repoID) {
		log.Warn().
			Str("user_id", user.ID).
			Str("repo_id", repoID).
			Msg("User does not have read access to repository")
		http.Error(w, "Read access denied", http.StatusForbidden)
		return
	}

	s.handleGitHTTPBackend(w, r)
}

// handleReceivePack handles git receive-pack requests (for git push) using git http-backend
func (s *GitHTTPServer) handleReceivePack(w http.ResponseWriter, r *http.Request) {
	if !s.enablePush {
		http.Error(w, "Push operations disabled", http.StatusForbidden)
		return
	}

	// Check if user has push permissions
	vars := mux.Vars(r)
	repoID := vars["repo_id"]
	user := s.getUser(r)
	if !s.hasWriteAccess(r.Context(), user, repoID) {
		log.Warn().
			Str("user_id", user.ID).
			Str("repo_id", repoID).
			Msg("User does not have push access to repository")
		http.Error(w, "Push access denied", http.StatusForbidden)
		return
	}

	s.handleGitHTTPBackend(w, r)
}

// handleGitHTTPBackend delegates to git's official http-backend CGI
// This handles the complete git smart HTTP protocol correctly
func (s *GitHTTPServer) handleGitHTTPBackend(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["repo_id"]

	// Get repository
	repo, err := s.gitRepoService.GetRepository(r.Context(), repoID)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Repository not found")
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	// Use git http-backend CGI (official git implementation)
	cmd := exec.CommandContext(r.Context(), "/usr/libexec/git-core/git-http-backend")
	cmd.Dir = repo.LocalPath

	// Extract the git service path (e.g., /info/refs or /git-upload-pack)
	// From URL like /git/{repo-id}/info/refs, extract /info/refs
	gitPath := strings.TrimPrefix(r.URL.Path, "/git/"+repoID)

	// Set CGI environment variables for git-http-backend
	// GIT_PROJECT_ROOT points to the repo itself (bare repo)
	// PATH_INFO is the git service path (without repo name)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("GIT_PROJECT_ROOT=%s", repo.LocalPath),
		fmt.Sprintf("PATH_INFO=%s", gitPath),
		fmt.Sprintf("QUERY_STRING=%s", r.URL.RawQuery),
		"REQUEST_METHOD="+r.Method,
		"GIT_HTTP_EXPORT_ALL=1", // Allow serving without git-daemon-export-ok file
		fmt.Sprintf("CONTENT_TYPE=%s", r.Header.Get("Content-Type")),
		fmt.Sprintf("REMOTE_USER=%s", s.getUser(r).ID), // Pass authenticated user for logging
	)

	// Pipe request body to git http-backend
	cmd.Stdin = r.Body

	// Capture combined output (CGI format: headers\r\n\r\nbody)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Error().
			Err(err).
			Str("repo_id", repoID).
			Str("output", string(output)).
			Msg("Git http-backend failed")
		http.Error(w, "Git operation failed", http.StatusInternalServerError)
		return
	}

	// Parse CGI response (headers + body separated by \r\n\r\n or \n\n)
	headerEnd := bytes.Index(output, []byte("\r\n\r\n"))
	if headerEnd == -1 {
		headerEnd = bytes.Index(output, []byte("\n\n"))
	}

	if headerEnd == -1 {
		// No CGI headers found, write raw output
		w.Write(output)
		return
	}

	// Parse and set response headers
	headers := string(output[:headerEnd])
	for _, line := range strings.Split(headers, "\n") {
		line = strings.TrimSpace(line)
		if idx := strings.Index(line, ": "); idx > 0 {
			key := line[:idx]
			value := line[idx+2:]
			w.Header().Set(key, value)
		}
	}

	// Write body
	bodyStart := headerEnd + 4
	if bytes.HasPrefix(output[headerEnd:], []byte("\n\n")) {
		bodyStart = headerEnd + 2
	}
	w.Write(output[bodyStart:])

	// Post-push hook: Check for design doc commits (async, don't block response)
	if strings.HasSuffix(gitPath, "/git-receive-pack") {
		// Use background context - request context gets canceled after response
		go s.handlePostPushHook(context.Background(), repoID, repo.LocalPath)
	}

	log.Debug().
		Str("repo_id", repoID).
		Str("method", r.Method).
		Str("path", r.URL.Path).
		Int("response_size", len(output)).
		Msg("Git HTTP request completed via http-backend")
}

// handlePostPushHook processes commits after a successful push
// Checks for design doc commits and auto-transitions SpecTasks
// Also detects feature branch pushes and merges to main
func (s *GitHTTPServer) handlePostPushHook(ctx context.Context, repoID, repoPath string) {
	log.Info().
		Str("repo_id", repoID).
		Msg("Processing post-push hook")

	// Get the repository to find associated project/spec tasks
	repo, err := s.gitRepoService.GetRepository(ctx, repoID)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to get repository in post-push hook")
		return
	}

	// Get the current branch that was pushed
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = repoPath
	branchOutput, err := cmd.Output()
	var pushedBranch string
	if err == nil {
		pushedBranch = strings.TrimSpace(string(branchOutput))
	}

	// Get the latest commit hash
	cmd = exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoPath
	hashOutput, err := cmd.Output()
	var latestCommitHash string
	if err == nil {
		latestCommitHash = strings.TrimSpace(string(hashOutput))
	}

	log.Info().
		Str("repo_id", repoID).
		Str("branch", pushedBranch).
		Str("commit", latestCommitHash).
		Msg("Detected push to repository")

	// Check for feature branch pushes (implementation workflow)
	if strings.HasPrefix(pushedBranch, "feature/") {
		s.handleFeatureBranchPush(ctx, repo, pushedBranch, latestCommitHash, repoPath)
	}

	// Check for pushes to main/master (merge detection)
	if pushedBranch == repo.DefaultBranch || pushedBranch == "main" || pushedBranch == "master" {
		s.handleMainBranchPush(ctx, repo, latestCommitHash, repoPath)
	}

	// Check if design docs were pushed and extract which task IDs
	pushedTaskIDs, err := s.getTaskIDsFromPushedDesignDocs(repoPath, latestCommitHash)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to check for design docs")
		return
	}

	if len(pushedTaskIDs) == 0 {
		log.Debug().Str("repo_id", repoID).Msg("No design docs found in push, skipping spec task transition")
		return
	}

	log.Info().
		Str("repo_id", repoID).
		Strs("task_ids", pushedTaskIDs).
		Msg("Design docs detected in push for specific tasks")

	// Process only the specific tasks that pushed design docs
	for _, taskID := range pushedTaskIDs {
		task, err := s.store.GetSpecTask(ctx, taskID)
		if err != nil {
			log.Error().
				Err(err).
				Str("task_id", taskID).
				Msg("Failed to get spec task")
			continue
		}

		if task.Status != types.TaskStatusSpecGeneration {
			log.Debug().
				Str("task_id", taskID).
				Str("current_status", task.Status).
				Msg("Task not in spec_generation status, skipping")
			continue
		}

		log.Info().
			Str("task_id", task.ID).
			Str("task_name", task.Name).
			Bool("yolo_mode", task.YoloMode).
			Msg("Processing SpecTask for design doc push")

		now := time.Now()
		task.DesignDocsPushedAt = &now // Track when design docs were actually pushed

		if task.YoloMode {
			// YOLO mode: Auto-approve specs and start implementation
			task.Status = types.TaskStatusSpecApproved
			task.SpecApprovedBy = "system"
			task.SpecApprovedAt = &now
			log.Info().
				Str("task_id", task.ID).
				Msg("YOLO mode enabled: Auto-approving specs")
		} else {
			// Normal mode: Move to spec review
			task.Status = types.TaskStatusSpecReview
			log.Info().
				Str("task_id", task.ID).
				Msg("Moving task to spec review")

			// Auto-create a design review record so the floating window viewer can open
			go s.createDesignReviewForPush(context.Background(), task.ID, pushedBranch, latestCommitHash, repoPath)
		}

		task.UpdatedAt = now
		err = s.store.UpdateSpecTask(ctx, task)
		if err != nil {
			log.Error().
				Err(err).
				Str("task_id", task.ID).
				Msg("Failed to update spec task status after design doc push")
		}

		// Check for design review comments that need auto-resolution
		go s.checkCommentResolution(context.Background(), task.ID, repoPath)
	}
}

// createDesignReviewForPush auto-creates a design review record when design docs are pushed
func (s *GitHTTPServer) createDesignReviewForPush(ctx context.Context, specTaskID, branch, commitHash, repoPath string) {
	log.Info().
		Str("spec_task_id", specTaskID).
		Str("branch", branch).
		Str("commit", commitHash).
		Msg("Auto-creating design review for pushed design docs")

	// List all files in helix-specs branch to find task directory
	cmd := exec.Command("git", "ls-tree", "--name-only", "-r", "helix-specs")
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Error().
			Err(err).
			Str("spec_task_id", specTaskID).
			Msg("Failed to list files in helix-specs branch")
		return
	}

	// Find task directory by searching for task ID in file paths
	files := strings.Split(strings.TrimSpace(string(output)), "\n")
	var taskDir string
	for _, file := range files {
		if strings.Contains(file, specTaskID) {
			// Extract directory path (e.g., design/tasks/2025-11-11_..._taskid/)
			parts := strings.Split(file, "/")
			if len(parts) >= 3 {
				taskDir = strings.Join(parts[:len(parts)-1], "/")
				break
			}
		}
	}

	if taskDir == "" {
		log.Warn().
			Str("spec_task_id", specTaskID).
			Msg("No task directory found in helix-specs branch")
		return
	}

	log.Info().
		Str("spec_task_id", specTaskID).
		Str("task_dir", taskDir).
		Msg("Found task directory in helix-specs")

	// Read design documents from task directory
	docs := make(map[string]string)
	docFilenames := []string{"requirements.md", "design.md", "tasks.md"}

	for _, filename := range docFilenames {
		filePath := fmt.Sprintf("%s/%s", taskDir, filename)
		cmd := exec.Command("git", "show", fmt.Sprintf("helix-specs:%s", filePath))
		cmd.Dir = repoPath
		output, err := cmd.Output()
		if err != nil {
			log.Debug().
				Err(err).
				Str("filename", filename).
				Str("path", filePath).
				Msg("Design doc file not found (may not exist yet)")
			continue
		}
		docs[filename] = string(output)
	}

	// Create design review record
	review := &types.SpecTaskDesignReview{
		ID:                 system.GenerateUUID(),
		SpecTaskID:         specTaskID,
		Status:             types.SpecTaskDesignReviewStatusPending,
		RequirementsSpec:   docs["requirements.md"],
		TechnicalDesign:    docs["design.md"],
		ImplementationPlan: docs["tasks.md"],
		GitBranch:          branch,
		GitCommitHash:      commitHash,
		GitPushedAt:        time.Now(),
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	if err := s.store.CreateSpecTaskDesignReview(ctx, review); err != nil {
		log.Error().
			Err(err).
			Str("spec_task_id", specTaskID).
			Msg("Failed to auto-create design review")
		return
	}

	log.Info().
		Str("review_id", review.ID).
		Str("spec_task_id", specTaskID).
		Msg("✅ Design review auto-created successfully")
}

// checkCommentResolution checks if any unresolved design review comments need auto-resolution
// because the quoted text was removed/updated in the design documents
func (s *GitHTTPServer) checkCommentResolution(ctx context.Context, specTaskID, repoPath string) {
	comments, err := s.store.GetUnresolvedCommentsForTask(ctx, specTaskID)
	if err != nil {
		log.Error().
			Err(err).
			Str("spec_task_id", specTaskID).
			Msg("Failed to get unresolved comments for auto-resolution check")
		return
	}

	if len(comments) == 0 {
		return
	}

	log.Info().
		Str("spec_task_id", specTaskID).
		Int("comment_count", len(comments)).
		Msg("Checking design review comments for auto-resolution")

	// Read current content of each document from helix-specs branch
	docContents := make(map[string]string)
	documentTypes := []string{"requirements", "technical_design", "implementation_plan"}

	for _, docType := range documentTypes {
		// Map document types to actual filenames
		filename := ""
		switch docType {
		case "requirements":
			filename = "requirements.md"
		case "technical_design":
			filename = "design.md"
		case "implementation_plan":
			filename = "tasks.md"
		}

		// Read file from helix-specs branch
		cmd := exec.Command("git", "show", "helix-specs:"+filename)
		cmd.Dir = repoPath
		output, err := cmd.CombinedOutput()
		if err != nil {
			log.Debug().
				Err(err).
				Str("spec_task_id", specTaskID).
				Str("filename", filename).
				Msg("Could not read design document from helix-specs (may not exist yet)")
			docContents[docType] = ""
		} else {
			docContents[docType] = string(output)
		}
	}

	// Check each comment to see if quoted text still exists
	resolvedCount := 0
	for _, comment := range comments {
		if comment.QuotedText == "" {
			continue // Skip comments without quoted text
		}

		// Get document content for this comment's document type
		docContent, exists := docContents[comment.DocumentType]
		if !exists {
			continue
		}

		// Check if quoted text still exists in document
		if !strings.Contains(docContent, comment.QuotedText) {
			// Quoted text was removed - auto-resolve the comment
			comment.Resolved = true
			comment.ResolvedBy = "system"
			comment.ResolutionReason = "auto_text_removed"
			now := time.Now()
			comment.ResolvedAt = &now

			if err := s.store.UpdateSpecTaskDesignReviewComment(ctx, &comment); err != nil {
				log.Error().
					Err(err).
					Str("comment_id", comment.ID).
					Msg("Failed to auto-resolve comment")
				continue
			}

			resolvedCount++
			log.Info().
				Str("comment_id", comment.ID).
				Str("document_type", comment.DocumentType).
				Str("quoted_text", comment.QuotedText[:min(50, len(comment.QuotedText))]).
				Msg("Auto-resolved design review comment (quoted text no longer exists)")
		}
	}

	if resolvedCount > 0 {
		log.Info().
			Str("spec_task_id", specTaskID).
			Int("resolved_count", resolvedCount).
			Msg("Auto-resolved design review comments")
	}
}

// handleFeatureBranchPush detects when an agent pushes to a feature branch
// Transitions task from implementation → implementation_review
func (s *GitHTTPServer) handleFeatureBranchPush(ctx context.Context, repo *types.GitRepository, branchName string, commitHash string, repoPath string) {
	log.Info().
		Str("repo_id", repo.ID).
		Str("branch", branchName).
		Str("commit", commitHash).
		Msg("Detected feature branch push")

	// Find spec tasks in implementation status with this branch name
	tasks, err := s.store.ListSpecTasks(ctx, &types.SpecTaskFilters{
		ProjectID: repo.ProjectID,
	})
	if err != nil {
		log.Error().Err(err).Str("project_id", repo.ProjectID).Msg("Failed to get spec tasks")
		return
	}

	for _, task := range tasks {
		if task == nil {
			continue
		}

		// Check if this task matches the pushed branch
		if task.BranchName != branchName {
			continue
		}

		// Only process tasks in implementation status
		if task.Status != types.TaskStatusImplementation {
			log.Debug().
				Str("task_id", task.ID).
				Str("status", task.Status).
				Str("branch", branchName).
				Msg("Skipping task - not in implementation status")
			continue
		}

		log.Info().
			Str("task_id", task.ID).
			Str("task_name", task.Name).
			Str("branch", branchName).
			Str("commit", commitHash).
			Msg("Feature branch push detected - transitioning to implementation review")

		// Update task status
		now := time.Now()
		task.Status = types.TaskStatusImplementationReview
		task.LastPushCommitHash = commitHash
		task.LastPushAt = &now
		task.UpdatedAt = now

		if err := s.store.UpdateSpecTask(ctx, task); err != nil {
			log.Error().
				Err(err).
				Str("task_id", task.ID).
				Msg("Failed to update task status after feature branch push")
			continue
		}

		// Send notification to agent that push was detected (reuse planning session)
		sessionID := task.PlanningSessionID

		if sessionID != "" {
			agentInstructionService := NewAgentInstructionService(s.store)
			go func() {
				err := agentInstructionService.SendImplementationReviewRequest(
					context.Background(),
					sessionID,
					task.CreatedBy, // User who created the task
					branchName,
				)
				if err != nil {
					log.Error().
						Err(err).
						Str("task_id", task.ID).
						Str("session_id", sessionID).
						Msg("Failed to send implementation review request")
				}
			}()
		}

		log.Info().
			Str("task_id", task.ID).
			Str("status", task.Status).
			Msg("Task transitioned to implementation review")
	}
}

// handleMainBranchPush detects when code is merged to main
// Transitions task from implementation_review → done
func (s *GitHTTPServer) handleMainBranchPush(ctx context.Context, repo *types.GitRepository, commitHash string, repoPath string) {
	log.Info().
		Str("repo_id", repo.ID).
		Str("commit", commitHash).
		Msg("Detected push to main branch")

	// Find spec tasks in implementation_review status
	tasks, err := s.store.ListSpecTasks(ctx, &types.SpecTaskFilters{
		ProjectID: repo.ProjectID,
	})
	if err != nil {
		log.Error().Err(err).Str("project_id", repo.ProjectID).Msg("Failed to get spec tasks")
		return
	}

	for _, task := range tasks {
		if task == nil || task.BranchName == "" {
			continue
		}

		// Only process tasks in implementation_review status
		if task.Status != types.TaskStatusImplementationReview {
			continue
		}

		// Check if this task's feature branch was merged
		// git branch --merged will show all branches merged into current branch
		cmd := exec.Command("git", "branch", "--merged", "HEAD", "--list", task.BranchName)
		cmd.Dir = repoPath
		output, err := cmd.Output()
		if err != nil {
			log.Debug().
				Err(err).
				Str("task_id", task.ID).
				Str("branch", task.BranchName).
				Msg("Could not check if branch is merged")
			continue
		}

		// If the branch appears in the merged list, it's been merged
		if strings.Contains(string(output), task.BranchName) {
			log.Info().
				Str("task_id", task.ID).
				Str("task_name", task.Name).
				Str("branch", task.BranchName).
				Str("commit", commitHash).
				Msg("Feature branch merged to main - transitioning to done")

			// Update task status
			now := time.Now()
			task.Status = types.TaskStatusDone
			task.MergedToMain = true
			task.MergedAt = &now
			task.MergeCommitHash = commitHash
			task.CompletedAt = &now
			task.UpdatedAt = now

			if err := s.store.UpdateSpecTask(ctx, task); err != nil {
				log.Error().
					Err(err).
					Str("task_id", task.ID).
					Msg("Failed to update task status after merge to main")
				continue
			}

			log.Info().
				Str("task_id", task.ID).
				Str("status", task.Status).
				Msg("Task transitioned to done")
		}
	}
}

// getTaskIDsFromPushedDesignDocs extracts task IDs from design docs that were pushed in this commit
// Returns only the task IDs that actually had design docs pushed, not all tasks in the project
func (s *GitHTTPServer) getTaskIDsFromPushedDesignDocs(repoPath, commitHash string) ([]string, error) {
	// Get the changed files in this commit on helix-specs branch
	// Use diff-tree to see what files changed in the latest commit to helix-specs
	cmd := exec.Command("git", "diff-tree", "--no-commit-id", "--name-only", "-r", "helix-specs")
	cmd.Dir = repoPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		// If helix-specs branch doesn't exist or has no commits, no design docs
		return nil, nil
	}

	files := strings.Split(strings.TrimSpace(string(output)), "\n")
	taskIDSet := make(map[string]bool)

	// Parse task IDs from file paths
	// Expected format: design/tasks/2025-11-12_task-name_<taskid>/requirements.md
	// Task ID is the last component before the slash, typically 8-4-4-4-12 UUID format
	for _, file := range files {
		if !strings.Contains(file, "design/tasks/") && !strings.Contains(file, "tasks/") {
			continue
		}

		// Extract task ID from path (last component of directory name)
		// Example: design/tasks/2025-11-12_add-feature_014930c9-3031-4bd0-b0cc-19741947665c/requirements.md
		parts := strings.Split(file, "/")
		if len(parts) < 3 {
			continue
		}

		// The directory name is the second-to-last part (before the filename)
		dirName := parts[len(parts)-2]

		// Task ID is after the last underscore
		lastUnderscore := strings.LastIndex(dirName, "_")
		if lastUnderscore == -1 {
			continue
		}

		taskID := dirName[lastUnderscore+1:]

		// Validate it looks like a valid task ID
		// Supported formats:
		// 1. Current format: "spt_<ulid>", e.g., "spt_01jdfg5h2k3m4n5p6q7r8s9t0u"
		// 2. Legacy UUID format: 8-4-4-4-12 (36 chars with dashes), e.g., "014930c9-3031-4bd0-b0cc-19741947665c"
		// 3. Legacy timestamp format: "task_<nanoseconds>", e.g., "task_1764170879478485974"
		isValidUUID := len(taskID) == 36 && strings.Count(taskID, "-") == 4
		isValidSpecTaskID := strings.HasPrefix(taskID, "spt_")

		// For legacy task_ prefix format, extract the full task ID from directory name
		if !isValidUUID && !isValidSpecTaskID && strings.Contains(dirName, "task_") {
			taskPrefixIdx := strings.LastIndex(dirName, "task_")
			if taskPrefixIdx != -1 {
				taskID = dirName[taskPrefixIdx:]
			}
		}

		// Accept current sptask_ format, legacy UUID format, or legacy task_ prefix format
		if isValidSpecTaskID || isValidUUID || strings.HasPrefix(taskID, "task_") {
			taskIDSet[taskID] = true
			log.Debug().
				Str("file", file).
				Str("task_id", taskID).
				Msg("Extracted task ID from design doc path")
		}
	}

	// Convert set to slice
	taskIDs := make([]string, 0, len(taskIDSet))
	for taskID := range taskIDSet {
		taskIDs = append(taskIDs, taskID)
	}

	return taskIDs, nil
}

// checkForDesignDocs checks if the helix-specs branch contains design documentation files
// DEPRECATED: Use getTaskIDsFromPushedDesignDocs instead
func (s *GitHTTPServer) checkForDesignDocs(repoPath string) (bool, error) {
	// Check for design doc files in helix-specs branch (not HEAD)
	// Design docs are committed to a separate branch, not the main code branch
	cmd := exec.Command("git", "ls-tree", "--name-only", "-r", "helix-specs")
	cmd.Dir = repoPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		// If helix-specs branch doesn't exist, no design docs
		return false, nil
	}

	files := strings.Split(string(output), "\n")
	designDocPatterns := []string{
		"tasks/", // SpecTask design docs are in tasks/ subdirectories
		"design/",
		"requirements.md",
		"design.md",
		"tasks.md",
	}

	for _, file := range files {
		for _, pattern := range designDocPatterns {
			if strings.Contains(file, pattern) {
				log.Debug().
					Str("file", file).
					Str("pattern", pattern).
					Msg("Design doc pattern matched")
				return true, nil
			}
		}
	}

	return false, nil
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
