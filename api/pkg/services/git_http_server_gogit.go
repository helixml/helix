package services

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/format/pktline"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/server"
	"github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/trigger"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// GitHTTPServer provides HTTP access to git repositories using pure Go (go-git).
// This is the primary implementation - no CGI or external git processes.
type GitHTTPServer struct {
	store                  store.Store
	gitRepoService         *GitRepositoryService
	serverBaseURL          string
	authTokenHeader        string
	enablePush             bool
	enablePull             bool
	maxRepoSize            int64
	requestTimeout         time.Duration
	testMode               bool
	authorizeFn            AuthorizationFunc
	sendMessageToAgentFunc SpecTaskMessageSender
	triggerManager         *trigger.Manager
	wg                     sync.WaitGroup

	// go-git server for handling git protocol
	gitServer transport.Transport
}

// GitHTTPServerConfig holds configuration for the git HTTP server
type GitHTTPServerConfig struct {
	ServerBaseURL     string        `json:"server_base_url"`
	GitExecutablePath string        `json:"git_executable_path"` // Ignored - using go-git
	AuthTokenHeader   string        `json:"auth_token_header"`
	EnablePush        bool          `json:"enable_push"`
	EnablePull        bool          `json:"enable_pull"`
	MaxRepoSize       int64         `json:"max_repo_size"`
	RequestTimeout    time.Duration `json:"request_timeout"`
}

// AuthorizationFunc is the function signature for authorization checks
type AuthorizationFunc func(ctx context.Context, user *types.User, orgID string, resourceID string, resourceType types.Resource, action types.Action) error

// SpecTaskMessageSender is a function type for sending messages to spec task agents
type SpecTaskMessageSender func(ctx context.Context, task *types.SpecTask, message string, docPath string) (string, error)

// GitCloneInfo contains information for cloning a repository
type GitCloneInfo struct {
	RepositoryID string `json:"repository_id"`
	CloneURL     string `json:"clone_url"`
	AuthToken    string `json:"auth_token,omitempty"`
	Username     string `json:"username"`
	ProjectPath  string `json:"project_path,omitempty"`
	Instructions string `json:"instructions,omitempty"`
}

// NewGitHTTPServer creates a new go-git based HTTP server
func NewGitHTTPServer(
	st store.Store,
	gitRepoService *GitRepositoryService,
	config *GitHTTPServerConfig,
	authorizeFn AuthorizationFunc,
	triggerManager *trigger.Manager,
) *GitHTTPServer {
	if config.AuthTokenHeader == "" {
		config.AuthTokenHeader = "Authorization"
	}
	if config.RequestTimeout == 0 {
		config.RequestTimeout = 5 * time.Minute
	}
	if config.MaxRepoSize == 0 {
		config.MaxRepoSize = 1024 * 1024 * 1024 // 1GB default
	}

	s := &GitHTTPServer{
		store:           st,
		gitRepoService:  gitRepoService,
		serverBaseURL:   config.ServerBaseURL,
		authTokenHeader: config.AuthTokenHeader,
		enablePush:      config.EnablePush,
		enablePull:      config.EnablePull,
		maxRepoSize:     config.MaxRepoSize,
		requestTimeout:  config.RequestTimeout,
		authorizeFn:     authorizeFn,
		triggerManager:  triggerManager,
	}

	// Create a custom loader that resolves repository IDs to filesystem storage
	loader := &helixRepoLoader{server: s}
	s.gitServer = server.NewServer(loader)

	return s
}

// helixRepoLoader implements server.Loader to map repository IDs to storage
type helixRepoLoader struct {
	server *GitHTTPServer
}

// Load implements server.Loader - loads a repository's storage from endpoint
func (l *helixRepoLoader) Load(ep *transport.Endpoint) (storer.Storer, error) {
	repoID := strings.TrimPrefix(ep.Path, "/")
	if repoID == "" {
		return nil, transport.ErrRepositoryNotFound
	}

	repo, err := l.server.gitRepoService.GetRepository(context.Background(), repoID)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to get repository")
		return nil, transport.ErrRepositoryNotFound
	}

	// Verify this is a bare repository
	objectsPath := filepath.Join(repo.LocalPath, "objects")
	headPath := filepath.Join(repo.LocalPath, "HEAD")

	if _, err := os.Stat(objectsPath); os.IsNotExist(err) {
		log.Error().Str("repo_path", repo.LocalPath).Str("repo_id", repoID).Msg("Not a valid bare repository (missing objects/)")
		return nil, transport.ErrRepositoryNotFound
	}

	if _, err := os.Stat(headPath); os.IsNotExist(err) {
		log.Error().Str("repo_path", repo.LocalPath).Str("repo_id", repoID).Msg("Not a valid bare repository (missing HEAD)")
		return nil, transport.ErrRepositoryNotFound
	}

	// Reject non-bare repositories
	dotGitPath := filepath.Join(repo.LocalPath, ".git")
	if info, err := os.Stat(dotGitPath); err == nil && info.IsDir() {
		log.Error().Str("repo_path", repo.LocalPath).Str("repo_id", repoID).Msg("Non-bare repository not supported")
		return nil, fmt.Errorf("non-bare repository not supported: %s", repoID)
	}

	fs := osfs.New(repo.LocalPath)
	storage := filesystem.NewStorage(fs, cache.NewObjectLRUDefault())

	log.Debug().Str("repo_id", repoID).Str("repo_path", repo.LocalPath).Msg("Loaded repository storage")
	return storage, nil
}

// SetTestMode enables or disables test mode
func (s *GitHTTPServer) SetTestMode(enabled bool) {
	s.testMode = enabled
}

// SetMessageSender sets the callback for sending messages to spec task agents
func (s *GitHTTPServer) SetMessageSender(sender SpecTaskMessageSender) {
	s.sendMessageToAgentFunc = sender
}

// RegisterRoutes registers HTTP git server routes
func (s *GitHTTPServer) RegisterRoutes(router *mux.Router) {
	gitRouter := router.PathPrefix("/git").Subrouter()
	gitRouter.Use(s.gzipDecompressMiddleware) // Handle gzip-compressed request bodies
	gitRouter.Use(s.authMiddleware)

	gitRouter.HandleFunc("/{repo_id}/info/refs", s.handleInfoRefs).Methods("GET")
	gitRouter.HandleFunc("/{repo_id}/git-upload-pack", s.handleUploadPack).Methods("POST")
	gitRouter.HandleFunc("/{repo_id}/git-receive-pack", s.handleReceivePack).Methods("POST")
	gitRouter.HandleFunc("/{repo_id}/clone-info", s.handleCloneInfo).Methods("GET")
	gitRouter.HandleFunc("/{repo_id}/status", s.handleRepositoryStatus).Methods("GET")

	log.Info().Msg("Git HTTP server routes registered (go-git implementation)")
}

// gzipDecompressMiddleware transparently decompresses gzip-encoded request bodies.
// Git clients often send gzip-compressed POST bodies for efficiency.
func (s *GitHTTPServer) gzipDecompressMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Encoding") == "gzip" {
			gzReader, err := gzip.NewReader(r.Body)
			if err != nil {
				log.Error().Err(err).Msg("Failed to create gzip reader for request body")
				http.Error(w, "Failed to decompress request", http.StatusBadRequest)
				return
			}
			r.Body = &gzipReadCloser{gzReader: gzReader, original: r.Body}
			r.Header.Del("Content-Encoding") // Mark as decompressed
		}
		next.ServeHTTP(w, r)
	})
}

// gzipReadCloser wraps a gzip.Reader to properly close both the gzip reader and original body
type gzipReadCloser struct {
	gzReader *gzip.Reader
	original io.ReadCloser
}

func (g *gzipReadCloser) Read(p []byte) (n int, err error) {
	return g.gzReader.Read(p)
}

func (g *gzipReadCloser) Close() error {
	if err := g.gzReader.Close(); err != nil {
		g.original.Close()
		return err
	}
	return g.original.Close()
}

// GetCloneURL returns the HTTP clone URL for a repository
func (s *GitHTTPServer) GetCloneURL(repositoryID string) string {
	return fmt.Sprintf("%s/git/%s", s.serverBaseURL, repositoryID)
}

// GetCloneInstructions returns comprehensive instructions for cloning a repository
func (s *GitHTTPServer) GetCloneInstructions(repositoryID string, apiKey string) string {
	cloneURL := s.GetCloneURL(repositoryID)
	authenticatedURL := strings.Replace(cloneURL, "://", fmt.Sprintf("://api:%s@", apiKey), 1)

	return fmt.Sprintf(`# Git Repository Access Instructions

## Repository Information
- Repository ID: %s
- Clone URL: %s

## Clone Repository

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

## Working with Specifications

After cloning, you can find planning specifications in:
- .helix/tasks/<task-id>/requirements.md - User requirements
- .helix/tasks/<task-id>/design.md - Technical design
- .helix/tasks/<task-id>/tasks.md - Implementation plan

## Committing Changes

When you make changes, commit them with descriptive messages:
`+"```bash\n"+`git add .
git commit -m "[SessionID] Description of changes"
git push origin your-branch-name
`+"```\n"+`
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
		log.Info().Str("method", r.Method).Str("path", r.URL.Path).Str("remote_addr", r.RemoteAddr).Msg("Git HTTP request received")

		apiKey := s.extractAPIKey(r)
		if apiKey == "" {
			log.Warn().Str("path", r.URL.Path).Str("method", r.Method).Msg("Git request missing API key")
			w.Header().Set("WWW-Authenticate", "Basic realm=\"Helix Git Server\"")
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}

		user, err := s.validateAPIKeyAndGetUser(r.Context(), apiKey)
		if err != nil || user == nil {
			log.Warn().Err(err).Msg("Invalid API key for git request")
			w.Header().Set("WWW-Authenticate", "Basic realm=\"Helix Git Server\"")
			http.Error(w, "Invalid API key", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), "git_user", user)
		log.Info().Str("user_id", user.ID).Str("path", r.URL.Path).Msg("Git request authenticated")
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// extractAPIKey extracts API key from request
func (s *GitHTTPServer) extractAPIKey(r *http.Request) string {
	if auth := r.Header.Get(s.authTokenHeader); auth != "" {
		if strings.HasPrefix(auth, "Bearer ") {
			return strings.TrimPrefix(auth, "Bearer ")
		}
		if strings.HasPrefix(auth, "Basic ") {
			return auth
		}
		return auth
	}
	if token := r.URL.Query().Get("token"); token != "" {
		return token
	}
	if r.URL.User != nil {
		if password, hasPassword := r.URL.User.Password(); hasPassword {
			return password
		}
	}
	return ""
}

// validateAPIKeyAndGetUser validates an API key and returns the user
func (s *GitHTTPServer) validateAPIKeyAndGetUser(ctx context.Context, apiKey string) (*types.User, error) {
	if s.testMode {
		return &types.User{ID: "test_user", Email: "test@example.com", Admin: false}, nil
	}

	if strings.HasPrefix(apiKey, "Basic ") {
		encodedCreds := strings.TrimPrefix(apiKey, "Basic ")
		decodedBytes, err := base64.StdEncoding.DecodeString(encodedCreds)
		if err != nil {
			return nil, fmt.Errorf("invalid Basic auth encoding")
		}
		parts := strings.SplitN(string(decodedBytes), ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid Basic auth format")
		}
		apiKey = parts[1]
	}

	apiKeyRecord, err := s.store.GetAPIKey(ctx, &types.ApiKey{Key: apiKey})
	if err != nil || apiKeyRecord == nil || apiKeyRecord.Created.IsZero() {
		return nil, err
	}

	return s.store.GetUser(ctx, &store.GetUserQuery{ID: apiKeyRecord.Owner})
}

// handleInfoRefs handles GET /info/refs
func (s *GitHTTPServer) handleInfoRefs(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["repo_id"]
	service := r.URL.Query().Get("service")

	log.Info().Str("repo_id", repoID).Str("service", service).Msg("Handling info/refs request")

	if service != "git-upload-pack" && service != "git-receive-pack" {
		http.Error(w, "Invalid service", http.StatusBadRequest)
		return
	}

	ep, err := transport.NewEndpoint("/" + repoID)
	if err != nil {
		http.Error(w, "Invalid repository", http.StatusBadRequest)
		return
	}

	var advRefs *packp.AdvRefs
	if service == "git-upload-pack" {
		session, err := s.gitServer.NewUploadPackSession(ep, nil)
		if err != nil {
			log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to create upload-pack session")
			if err == transport.ErrRepositoryNotFound {
				http.Error(w, "Repository not found", http.StatusNotFound)
			} else {
				http.Error(w, "Failed to access repository", http.StatusInternalServerError)
			}
			return
		}
		defer session.Close()
		advRefs, err = session.AdvertisedReferencesContext(r.Context())
		if err != nil {
			log.Error().Err(err).Msg("Failed to get advertised references")
			http.Error(w, "Failed to get references", http.StatusInternalServerError)
			return
		}
	} else {
		session, err := s.gitServer.NewReceivePackSession(ep, nil)
		if err != nil {
			log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to create receive-pack session")
			if err == transport.ErrRepositoryNotFound {
				http.Error(w, "Repository not found", http.StatusNotFound)
			} else {
				http.Error(w, "Failed to access repository", http.StatusInternalServerError)
			}
			return
		}
		defer session.Close()
		advRefs, err = session.AdvertisedReferencesContext(r.Context())
		if err != nil {
			log.Error().Err(err).Msg("Failed to get advertised references")
			http.Error(w, "Failed to get references", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", fmt.Sprintf("application/x-%s-advertisement", service))
	w.Header().Set("Cache-Control", "no-cache")

	pktEnc := pktline.NewEncoder(w)
	if err := pktEnc.Encodef("# service=%s\n", service); err != nil {
		log.Error().Err(err).Msg("Failed to write service header")
		return
	}
	if err := pktEnc.Flush(); err != nil {
		log.Error().Err(err).Msg("Failed to flush")
		return
	}
	if err := advRefs.Encode(w); err != nil {
		log.Error().Err(err).Msg("Failed to encode refs")
		return
	}

	log.Info().Str("repo_id", repoID).Int("refs_count", len(advRefs.References)).Msg("Sent advertised references")
}

// handleUploadPack handles POST /git-upload-pack (clone/fetch)
func (s *GitHTTPServer) handleUploadPack(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["repo_id"]
	user := s.getUser(r)

	log.Info().Str("repo_id", repoID).Str("user_id", user.ID).Int64("content_length", r.ContentLength).Msg("Handling upload-pack request")

	if !s.enablePull || !s.hasReadAccess(r.Context(), user, repoID) {
		http.Error(w, "Access denied", http.StatusForbidden)
		return
	}

	ep, _ := transport.NewEndpoint("/" + repoID)
	session, err := s.gitServer.NewUploadPackSession(ep, nil)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create session")
		http.Error(w, "Failed to access repository", http.StatusInternalServerError)
		return
	}
	defer session.Close()

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request", http.StatusInternalServerError)
		return
	}

	req := packp.NewUploadPackRequest()
	if err := req.Decode(bytes.NewReader(bodyBytes)); err != nil {
		log.Error().Err(err).Msg("Failed to decode request")
		http.Error(w, fmt.Sprintf("Failed to parse request: %v", err), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/x-git-upload-pack-result")
	w.Header().Set("Cache-Control", "no-cache")

	resp, err := session.UploadPack(r.Context(), req)
	if err != nil {
		log.Error().Err(err).Msg("Upload-pack failed")
		http.Error(w, fmt.Sprintf("Upload-pack failed: %v", err), http.StatusInternalServerError)
		return
	}

	if err := resp.Encode(w); err != nil {
		log.Error().Err(err).Msg("Failed to encode response")
		return
	}

	log.Info().Str("repo_id", repoID).Msg("Upload-pack completed")
}

// handleReceivePack handles POST /git-receive-pack (push)
func (s *GitHTTPServer) handleReceivePack(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["repo_id"]
	user := s.getUser(r)

	log.Info().Str("repo_id", repoID).Str("user_id", user.ID).Int64("content_length", r.ContentLength).Msg("Handling receive-pack request")

	if !s.enablePush || !s.hasWriteAccess(r.Context(), user, repoID) {
		http.Error(w, "Access denied", http.StatusForbidden)
		return
	}

	ep, _ := transport.NewEndpoint("/" + repoID)
	session, err := s.gitServer.NewReceivePackSession(ep, nil)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create session")
		http.Error(w, "Failed to access repository", http.StatusInternalServerError)
		return
	}
	defer session.Close()

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request", http.StatusInternalServerError)
		return
	}

	req := packp.NewReferenceUpdateRequest()
	if err := req.Decode(bytes.NewReader(bodyBytes)); err != nil {
		log.Error().Err(err).Msg("Failed to decode request")
		http.Error(w, fmt.Sprintf("Failed to parse request: %v", err), http.StatusBadRequest)
		return
	}

	// Extract pushed branches from commands
	var pushedBranches []string
	for _, cmd := range req.Commands {
		if strings.HasPrefix(string(cmd.Name), "refs/heads/") {
			branchName := strings.TrimPrefix(string(cmd.Name), "refs/heads/")
			pushedBranches = append(pushedBranches, branchName)
		}
	}

	log.Info().Str("repo_id", repoID).Strs("pushed_branches", pushedBranches).Int("commands", len(req.Commands)).Msg("Parsed receive-pack commands")

	w.Header().Set("Content-Type", "application/x-git-receive-pack-result")
	w.Header().Set("Cache-Control", "no-cache")

	resp, err := session.ReceivePack(r.Context(), req)
	if err != nil {
		log.Error().Err(err).Msg("Receive-pack failed")
		http.Error(w, fmt.Sprintf("Receive-pack failed: %v", err), http.StatusInternalServerError)
		return
	}

	if resp != nil {
		if err := resp.Encode(w); err != nil {
			log.Error().Err(err).Msg("Failed to encode response")
			return
		}
	}

	log.Info().Str("repo_id", repoID).Strs("pushed_branches", pushedBranches).Msg("Receive-pack completed")

	// Trigger post-push hooks asynchronously
	if len(pushedBranches) > 0 {
		repo, err := s.gitRepoService.GetRepository(r.Context(), repoID)
		if err == nil {
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				s.handlePostPushHook(context.Background(), repoID, repo.LocalPath, pushedBranches)
			}()
		}
	}
}

// handlePostPushHook processes commits after a successful push
func (s *GitHTTPServer) handlePostPushHook(ctx context.Context, repoID, repoPath string, pushedBranches []string) {
	log.Info().Str("repo_id", repoID).Strs("pushed_branches", pushedBranches).Msg("Processing post-push hook")

	repo, err := s.gitRepoService.GetRepository(ctx, repoID)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to get repository in post-push hook")
		return
	}

	gitRepo, err := OpenGitRepo(repoPath)
	if err != nil {
		log.Error().Err(err).Str("repo_path", repoPath).Msg("Failed to open git repository")
		return
	}

	for _, pushedBranch := range pushedBranches {
		commitHash, err := gitRepo.GetBranchCommitHash(pushedBranch)
		if err != nil {
			log.Warn().Err(err).Str("branch", pushedBranch).Msg("Failed to get commit hash")
			continue
		}

		log.Info().Str("repo_id", repoID).Str("branch", pushedBranch).Str("commit", commitHash).Msg("Processing pushed branch")

		// Feature branch push detection
		if strings.HasPrefix(pushedBranch, "feature/") {
			s.handleFeatureBranchPush(ctx, repo, pushedBranch, commitHash, repoPath, gitRepo)
		}

		// Main branch push detection
		if repo.DefaultBranch != "" && pushedBranch == repo.DefaultBranch {
			s.handleMainBranchPush(ctx, repo, commitHash, repoPath, gitRepo)
		}

		// Process design docs
		s.processDesignDocsForBranch(ctx, repo, repoPath, pushedBranch, commitHash, gitRepo)
	}
}

// processDesignDocsForBranch handles design doc detection and spec task processing
func (s *GitHTTPServer) processDesignDocsForBranch(ctx context.Context, repo *types.GitRepository, repoPath, pushedBranch, commitHash string, gitRepo *GitRepo) {
	repoID := repo.ID

	// Get task IDs from pushed design docs
	pushedTaskIDs, dirNamesNeedingLookup, err := s.getTaskIDsFromPushedDesignDocs(ctx, gitRepo)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to check for design docs")
		return
	}

	// Look up tasks by DesignDocPath for new-format directories
	for _, dirName := range dirNamesNeedingLookup {
		tasks, err := s.store.ListSpecTasks(ctx, &types.SpecTaskFilters{
			DesignDocPath:   dirName,
			IncludeArchived: true,
		})
		if err != nil || len(tasks) == 0 {
			continue
		}
		pushedTaskIDs = append(pushedTaskIDs, tasks[0].ID)
	}

	if len(pushedTaskIDs) == 0 {
		log.Debug().Str("repo_id", repoID).Msg("No design docs found in push")
		return
	}

	log.Info().Str("repo_id", repoID).Strs("task_ids", pushedTaskIDs).Msg("Design docs detected in push")

	for _, taskID := range pushedTaskIDs {
		task, err := s.store.GetSpecTask(ctx, taskID)
		if err != nil {
			log.Error().Err(err).Str("task_id", taskID).Msg("Failed to get spec task")
			continue
		}

		log.Info().Str("task_id", task.ID).Str("status", task.Status.String()).Str("commit", commitHash).Msg("Processing SpecTask for design doc push")

		switch task.Status {
		case types.TaskStatusSpecGeneration:
			now := time.Now()
			task.DesignDocsPushedAt = &now
			task.Status = types.TaskStatusSpecReview
			task.UpdatedAt = now
			if err := s.store.UpdateSpecTask(ctx, task); err != nil {
				log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to update spec task status")
			}
			s.wg.Add(1)
			go func(t *types.SpecTask) {
				defer s.wg.Done()
				s.createDesignReviewForPush(context.Background(), t.ID, pushedBranch, commitHash, repoPath, gitRepo)
			}(task)
			s.wg.Add(1)
			go func(t *types.SpecTask) {
				defer s.wg.Done()
				s.checkCommentResolution(context.Background(), t.ID, repoPath, gitRepo)
			}(task)

		case types.TaskStatusSpecReview, types.TaskStatusSpecRevision:
			s.wg.Add(1)
			go func(t *types.SpecTask) {
				defer s.wg.Done()
				s.createDesignReviewForPush(context.Background(), t.ID, pushedBranch, commitHash, repoPath, gitRepo)
			}(task)
			s.wg.Add(1)
			go func(t *types.SpecTask) {
				defer s.wg.Done()
				s.checkCommentResolution(context.Background(), t.ID, repoPath, gitRepo)
			}(task)

		case types.TaskStatusImplementation:
			s.wg.Add(1)
			go func(t *types.SpecTask) {
				defer s.wg.Done()
				s.createDesignReviewForPush(context.Background(), t.ID, pushedBranch, commitHash, repoPath, gitRepo)
			}(task)
			if repo.ExternalURL != "" {
				s.wg.Add(1)
				go func(t *types.SpecTask) {
					defer s.wg.Done()
					log.Info().Str("spec_task_id", t.ID).Str("branch", pushedBranch).Msg("Pushing branch to external repository")
					if err := s.gitRepoService.PushBranchToRemote(context.Background(), repo.ID, t.BranchName, false); err != nil {
						log.Error().Err(err).Str("spec_task_id", t.ID).Msg("Failed to push branch")
					}
				}(task)
			}

		case types.TaskStatusPullRequest:
			s.wg.Add(1)
			go func(t *types.SpecTask) {
				defer s.wg.Done()
				s.createDesignReviewForPush(context.Background(), t.ID, pushedBranch, commitHash, repoPath, gitRepo)
			}(task)
			s.wg.Add(1)
			go func(t *types.SpecTask, r *types.GitRepository, commit string) {
				defer s.wg.Done()
				if err := s.ensurePullRequest(context.Background(), r, t, t.BranchName); err != nil {
					log.Error().Err(err).Str("spec_task_id", t.ID).Msg("Failed to ensure pull request")
					return
				}
				if s.triggerManager != nil {
					s.wg.Add(1)
					go func() {
						defer s.wg.Done()
						if err := s.triggerManager.ProcessGitPushEvent(context.Background(), t, r, commit); err != nil {
							log.Error().Err(err).Str("spec_task_id", t.ID).Msg("Failed to process code review")
						}
					}()
				}
			}(task, repo, commitHash)
		}
	}
}

// getTaskIDsFromPushedDesignDocs extracts task IDs from design docs using go-git
func (s *GitHTTPServer) getTaskIDsFromPushedDesignDocs(ctx context.Context, gitRepo *GitRepo) ([]string, []string, error) {
	files, err := gitRepo.GetChangedFilesInBranch("helix-specs")
	if err != nil {
		return nil, nil, nil // Branch doesn't exist
	}

	taskIDs, dirNamesNeedingLookup := ParseDesignDocTaskIDs(files)
	return taskIDs, dirNamesNeedingLookup, nil
}

// handleFeatureBranchPush transitions task from implementation → implementation_review
func (s *GitHTTPServer) handleFeatureBranchPush(ctx context.Context, repo *types.GitRepository, branchName, commitHash, repoPath string, gitRepo *GitRepo) {
	log.Info().Str("repo_id", repo.ID).Str("branch", branchName).Str("commit", commitHash).Msg("Detected feature branch push")

	projectIDs, err := s.store.GetProjectsForRepository(ctx, repo.ID)
	if err != nil || len(projectIDs) == 0 {
		return
	}

	var allTasks []*types.SpecTask
	for _, projectID := range projectIDs {
		tasks, _ := s.store.ListSpecTasks(ctx, &types.SpecTaskFilters{ProjectID: projectID})
		allTasks = append(allTasks, tasks...)
	}

	for _, task := range allTasks {
		if task == nil || task.BranchName != branchName || task.Status != types.TaskStatusImplementation {
			continue
		}

		log.Info().Str("task_id", task.ID).Str("branch", branchName).Msg("Transitioning to implementation review")

		now := time.Now()
		task.Status = types.TaskStatusImplementationReview
		task.LastPushCommitHash = commitHash
		task.LastPushAt = &now
		task.UpdatedAt = now
		if err := s.store.UpdateSpecTask(ctx, task); err != nil {
			log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to update task")
			continue
		}

		if s.sendMessageToAgentFunc != nil {
			s.wg.Add(1)
			go func(t *types.SpecTask, branch string) {
				defer s.wg.Done()
				message := BuildImplementationReviewPrompt(t, branch)
				if _, err := s.sendMessageToAgentFunc(context.Background(), t, message, ""); err != nil {
					log.Error().Err(err).Str("task_id", t.ID).Msg("Failed to send implementation review request")
				}
			}(task, branchName)
		}
	}
}

// handleMainBranchPush transitions task from implementation_review → done
func (s *GitHTTPServer) handleMainBranchPush(ctx context.Context, repo *types.GitRepository, commitHash, repoPath string, gitRepo *GitRepo) {
	log.Info().Str("repo_id", repo.ID).Str("commit", commitHash).Msg("Detected push to main branch")

	projectIDs, err := s.store.GetProjectsForRepository(ctx, repo.ID)
	if err != nil || len(projectIDs) == 0 {
		return
	}

	var allTasks []*types.SpecTask
	for _, projectID := range projectIDs {
		tasks, _ := s.store.ListSpecTasks(ctx, &types.SpecTaskFilters{ProjectID: projectID})
		allTasks = append(allTasks, tasks...)
	}

	for _, task := range allTasks {
		if task == nil || task.BranchName == "" || task.Status != types.TaskStatusImplementationReview {
			continue
		}

		// Check if branch is merged using go-git
		merged, err := gitRepo.IsBranchMergedInto(task.BranchName, repo.DefaultBranch)
		if err != nil {
			log.Debug().Err(err).Str("branch", task.BranchName).Msg("Could not check merge status")
			continue
		}

		if merged {
			log.Info().Str("task_id", task.ID).Str("branch", task.BranchName).Msg("Branch merged to main - transitioning to done")

			now := time.Now()
			task.Status = types.TaskStatusDone
			task.MergedToMain = true
			task.MergedAt = &now
			task.MergeCommitHash = commitHash
			task.CompletedAt = &now
			task.UpdatedAt = now
			if err := s.store.UpdateSpecTask(ctx, task); err != nil {
				log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to update task")
			}
		}
	}
}

// createDesignReviewForPush creates or updates design review records
func (s *GitHTTPServer) createDesignReviewForPush(ctx context.Context, specTaskID, branch, commitHash, repoPath string, gitRepo *GitRepo) {
	log.Info().Str("spec_task_id", specTaskID).Str("branch", branch).Str("commit", commitHash).Msg("Creating/updating design review")

	task, err := s.store.GetSpecTask(ctx, specTaskID)
	if err != nil {
		log.Error().Err(err).Str("spec_task_id", specTaskID).Msg("Failed to get task")
		return
	}

	taskDir, err := gitRepo.FindTaskDirInBranch("helix-specs", task.DesignDocPath, specTaskID)
	if err != nil {
		log.Warn().Err(err).Str("spec_task_id", specTaskID).Msg("No task directory found")
		return
	}

	docs, err := gitRepo.ReadDesignDocs("helix-specs", taskDir)
	if err != nil {
		log.Error().Err(err).Str("spec_task_id", specTaskID).Msg("Failed to read design docs")
		return
	}

	existingReviews, _ := s.store.ListSpecTaskDesignReviews(ctx, specTaskID)
	var activeReview *types.SpecTaskDesignReview
	for i := range existingReviews {
		if existingReviews[i].Status != types.SpecTaskDesignReviewStatusSuperseded {
			activeReview = &existingReviews[i]
			break
		}
	}

	now := time.Now()
	if activeReview != nil {
		activeReview.RequirementsSpec = docs["requirements.md"]
		activeReview.TechnicalDesign = docs["design.md"]
		activeReview.ImplementationPlan = docs["tasks.md"]
		activeReview.GitBranch = branch
		activeReview.GitCommitHash = commitHash
		activeReview.GitPushedAt = now
		activeReview.UpdatedAt = now
		if err := s.store.UpdateSpecTaskDesignReview(ctx, activeReview); err != nil {
			log.Error().Err(err).Str("review_id", activeReview.ID).Msg("Failed to update review")
		} else {
			log.Info().Str("review_id", activeReview.ID).Msg("Design review updated")
		}
	} else {
		review := &types.SpecTaskDesignReview{
			ID:                 system.GenerateUUID(),
			SpecTaskID:         specTaskID,
			Status:             types.SpecTaskDesignReviewStatusPending,
			RequirementsSpec:   docs["requirements.md"],
			TechnicalDesign:    docs["design.md"],
			ImplementationPlan: docs["tasks.md"],
			GitBranch:          branch,
			GitCommitHash:      commitHash,
			GitPushedAt:        now,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		if err := s.store.CreateSpecTaskDesignReview(ctx, review); err != nil {
			log.Error().Err(err).Str("spec_task_id", specTaskID).Msg("Failed to create review")
		} else {
			log.Info().Str("review_id", review.ID).Msg("Design review created")
		}
	}
}

// checkCommentResolution checks if design review comments should be auto-resolved
// because the quoted text was removed/updated in the design documents
func (s *GitHTTPServer) checkCommentResolution(ctx context.Context, specTaskID, repoPath string, gitRepo *GitRepo) {
	comments, err := s.store.GetUnresolvedCommentsForTask(ctx, specTaskID)
	if err != nil || len(comments) == 0 {
		return
	}

	log.Info().Str("spec_task_id", specTaskID).Int("count", len(comments)).Msg("Checking comments for auto-resolution")

	// Get task to find the design doc path
	task, err := s.store.GetSpecTask(ctx, specTaskID)
	if err != nil {
		log.Error().Err(err).Str("spec_task_id", specTaskID).Msg("Failed to get task for comment resolution")
		return
	}

	// Find task directory in helix-specs branch
	taskDir, err := gitRepo.FindTaskDirInBranch("helix-specs", task.DesignDocPath, specTaskID)
	if err != nil {
		log.Debug().Err(err).Str("spec_task_id", specTaskID).Msg("Task directory not found in helix-specs")
		return
	}

	// Read design docs from the task directory
	docContents := make(map[string]string)
	docTypes := map[string]string{
		"requirements":        "requirements.md",
		"technical_design":    "design.md",
		"implementation_plan": "tasks.md",
	}

	for docType, filename := range docTypes {
		filePath := taskDir + "/" + filename
		content, err := gitRepo.ReadFileFromBranch("helix-specs", filePath)
		if err == nil {
			docContents[docType] = string(content)
		}
	}

	now := time.Now()
	resolvedCount := 0
	for _, comment := range comments {
		if comment.QuotedText == "" {
			continue
		}
		content, exists := docContents[comment.DocumentType]
		if !exists {
			continue
		}
		if !strings.Contains(content, comment.QuotedText) {
			log.Info().Str("comment_id", comment.ID).Str("document_type", comment.DocumentType).Msg("Auto-resolving comment - quoted text removed")
			comment.Resolved = true
			comment.ResolvedAt = &now
			comment.ResolvedBy = "system"
			comment.ResolutionReason = "auto_text_removed"
			s.store.UpdateSpecTaskDesignReviewComment(ctx, &comment)
			resolvedCount++
		}
	}

	if resolvedCount > 0 {
		log.Info().Str("spec_task_id", specTaskID).Int("resolved_count", resolvedCount).Msg("Auto-resolved design review comments")
	}
}

// ensurePullRequest creates a PR if one doesn't exist
func (s *GitHTTPServer) ensurePullRequest(ctx context.Context, repo *types.GitRepository, task *types.SpecTask, branch string) error {
	if repo.ExternalURL == "" {
		return nil
	}

	log.Info().Str("repo_id", repo.ID).Str("branch", branch).Msg("Ensuring pull request")

	if err := s.gitRepoService.PushBranchToRemote(ctx, repo.ID, branch, false); err != nil {
		return fmt.Errorf("failed to push branch: %w", err)
	}

	prs, err := s.gitRepoService.ListPullRequests(ctx, repo.ID)
	if err != nil {
		return fmt.Errorf("failed to list PRs: %w", err)
	}

	sourceBranchRef := "refs/heads/" + branch
	for _, pr := range prs {
		if pr.SourceBranch == sourceBranchRef && (pr.State == "active" || pr.State == "open") {
			if task.PullRequestID != pr.ID {
				task.PullRequestID = pr.ID
				task.UpdatedAt = time.Now()
				s.store.UpdateSpecTask(ctx, task)
			}
			return nil
		}
	}

	description := fmt.Sprintf("> **Helix**: %s\n", task.Description)
	prID, err := s.gitRepoService.CreatePullRequest(ctx, repo.ID, task.Name, description, branch, repo.DefaultBranch)
	if err != nil {
		return fmt.Errorf("failed to create PR: %w", err)
	}

	task.PullRequestID = prID
	task.UpdatedAt = time.Now()
	s.store.UpdateSpecTask(ctx, task)
	log.Info().Str("pr_id", prID).Str("branch", branch).Msg("Created pull request")
	return nil
}

// hasReadAccess checks read access
func (s *GitHTTPServer) hasReadAccess(ctx context.Context, user *types.User, repoID string) bool {
	if user == nil {
		return false
	}
	repo, err := s.gitRepoService.GetRepository(ctx, repoID)
	if err != nil {
		return false
	}
	if repo.OwnerID == user.ID || user.Admin {
		return true
	}
	if repo.OrganizationID == "" {
		return false
	}
	return s.authorizeFn(ctx, user, repo.OrganizationID, repoID, types.ResourceGitRepository, types.ActionGet) == nil
}

// hasWriteAccess checks write access
func (s *GitHTTPServer) hasWriteAccess(ctx context.Context, user *types.User, repoID string) bool {
	if user == nil {
		return false
	}
	repo, err := s.gitRepoService.GetRepository(ctx, repoID)
	if err != nil {
		return false
	}
	if repo.OwnerID == user.ID || user.Admin {
		return true
	}
	if repo.OrganizationID == "" {
		return false
	}
	return s.authorizeFn(ctx, user, repo.OrganizationID, repoID, types.ResourceGitRepository, types.ActionUpdate) == nil
}

// getUser extracts user from context
func (s *GitHTTPServer) getUser(r *http.Request) *types.User {
	if user := r.Context().Value("git_user"); user != nil {
		if u, ok := user.(*types.User); ok {
			return u
		}
	}
	return nil
}

// handleCloneInfo provides clone information
func (s *GitHTTPServer) handleCloneInfo(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["repo_id"]

	repo, err := s.gitRepoService.GetRepository(r.Context(), repoID)
	if err != nil {
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

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
	json.NewEncoder(w).Encode(cloneInfo)
}

// handleRepositoryStatus provides repository status
func (s *GitHTTPServer) handleRepositoryStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["repo_id"]

	repo, err := s.gitRepoService.GetRepository(r.Context(), repoID)
	if err != nil {
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	gitRepo, err := OpenGitRepo(repo.LocalPath)
	if err != nil {
		http.Error(w, "Failed to open repository", http.StatusInternalServerError)
		return
	}

	branches, _ := gitRepo.ListBranches()

	// Get repository stats
	stats := s.getRepositoryStats(repo.LocalPath, gitRepo)

	status := map[string]interface{}{
		"repository_id":  repoID,
		"name":           repo.Name,
		"status":         repo.Status,
		"default_branch": repo.DefaultBranch,
		"branches":       branches,
		"last_activity":  repo.LastActivity,
		"stats":          stats,
		"clone_url":      s.GetCloneURL(repoID),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// getRepositoryStats gets basic repository statistics using go-git
func (s *GitHTTPServer) getRepositoryStats(repoPath string, gitRepo *GitRepo) map[string]interface{} {
	stats := make(map[string]interface{})

	// Get repository size
	size, err := s.getDirectorySize(repoPath)
	if err != nil {
		log.Warn().Err(err).Str("repo_path", repoPath).Msg("Failed to get repository size")
		size = 0
	}
	stats["size_bytes"] = size

	// Get commit count and last commit using go-git
	if gitRepo != nil {
		// Get HEAD reference directly (not via branch name)
		headRef, err := gitRepo.repo.Head()
		if err == nil {
			commit, err := gitRepo.repo.CommitObject(headRef.Hash())
			if err == nil {
				// Count commits by walking history
				count := 0
				iter, err := gitRepo.repo.Log(&git.LogOptions{From: commit.Hash})
				if err == nil {
					iter.ForEach(func(c *object.Commit) error {
						count++
						return nil
					})
				}
				stats["commit_count"] = count

				// Get last commit info
				stats["last_commit"] = map[string]interface{}{
					"hash":          commit.Hash.String(),
					"author_name":   commit.Author.Name,
					"author_email":  commit.Author.Email,
					"timestamp":     commit.Author.When.Unix(),
					"message":       strings.Split(commit.Message, "\n")[0], // First line only
				}
			}
		}
	}

	return stats
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
