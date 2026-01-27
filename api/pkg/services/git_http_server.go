package services

import (
	"compress/gzip"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	giteagit "code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/git/gitcmd"
	"code.gitea.io/gitea/modules/setting"
	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/trigger"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// AuthorizationFunc is the function signature for authorization checks
type AuthorizationFunc func(ctx context.Context, user *types.User, orgID string, resourceID string, resourceType types.Resource, action types.Action) error

// SpecTaskMessageSender is a function type for sending messages to spec task agents
type SpecTaskMessageSender func(ctx context.Context, task *types.SpecTask, message string, docPath string) (string, error)

// BranchRestriction holds the result of checking branch permissions for an API key
type BranchRestriction struct {
	IsAgentKey      bool     // True if this is a session-scoped agent key
	AllowedBranches []string // The branches the agent can push to
	ErrorMessage    string   // Set if the agent is not allowed to push at all
}

// setNoCacheHeaders sets HTTP headers to prevent caching of git protocol responses.
func setNoCacheHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-cache, max-age=0, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "Fri, 01 Jan 1980 00:00:00 GMT")
}

// flushingWriter wraps an http.ResponseWriter to flush after each write.
type flushingWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

func newFlushingWriter(w http.ResponseWriter) *flushingWriter {
	fw := &flushingWriter{w: w}
	if f, ok := w.(http.Flusher); ok {
		fw.flusher = f
	}
	return fw
}

func (fw *flushingWriter) Write(p []byte) (n int, err error) {
	n, err = fw.w.Write(p)
	if fw.flusher != nil && n > 0 {
		fw.flusher.Flush()
	}
	return
}

// GitHTTPServer provides HTTP access to git repositories using native git.
// This replaces the go-git based implementation for better performance and reliability.
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
}

// GitHTTPServerConfig holds configuration for the git HTTP server
type GitHTTPServerConfig struct {
	ServerBaseURL   string        `json:"server_base_url"`
	AuthTokenHeader string        `json:"auth_token_header"`
	EnablePush      bool          `json:"enable_push"`
	EnablePull      bool          `json:"enable_pull"`
	MaxRepoSize     int64         `json:"max_repo_size"`
	RequestTimeout  time.Duration `json:"request_timeout"`
}

var gitCmdInitialized bool
var gitCmdInitMu sync.Mutex

// initGitCmd initializes gitea's gitcmd module once.
// The gitHomePath is used as HOME for git commands (where .gitconfig is stored).
// This is separate from where the actual git repositories are stored.
func initGitCmd(gitHomePath string) error {
	gitCmdInitMu.Lock()
	defer gitCmdInitMu.Unlock()

	if gitCmdInitialized {
		return nil
	}

	// Set the git home path for gitea's setting module.
	// This is where git will store its global config (not where repos are stored).
	// Must be set BEFORE calling any gitcmd functions that use HomeDir().
	setting.Git.HomePath = gitHomePath
	log.Info().Str("home_path", gitHomePath).Msg("Set gitea git home path")

	// Find and set the git executable path
	if err := gitcmd.SetExecutablePath(""); err != nil {
		return fmt.Errorf("failed to find git executable: %w", err)
	}

	gitCmdInitialized = true
	log.Info().Msg("Initialized gitea gitcmd module")
	return nil
}

// NewGitHTTPServer creates a new native git HTTP server
func NewGitHTTPServer(
	store store.Store,
	gitRepoService *GitRepositoryService,
	config GitHTTPServerConfig,
	authorizeFn AuthorizationFunc,
	triggerManager *trigger.Manager,
) *GitHTTPServer {
	// Initialize gitcmd with the git home path from the git repo service.
	// This sets up gitea's setting module with the HOME path for git config.
	if err := initGitCmd(gitRepoService.GetGitHomePath()); err != nil {
		log.Error().Err(err).Msg("Failed to initialize gitcmd")
	}
	return &GitHTTPServer{
		store:           store,
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
}

func (s *GitHTTPServer) SetTestMode(enabled bool) {
	s.testMode = enabled
}

func (s *GitHTTPServer) SetMessageSender(sender SpecTaskMessageSender) {
	s.sendMessageToAgentFunc = sender
}

// RegisterRoutes registers the git HTTP routes
func (s *GitHTTPServer) RegisterRoutes(router *mux.Router) {
	// Git smart HTTP protocol endpoints
	gitRouter := router.PathPrefix("/git").Subrouter()
	gitRouter.Use(s.gzipDecompressMiddleware)
	gitRouter.Use(s.authMiddleware)

	// info/refs - ref advertisement
	gitRouter.HandleFunc("/{repo_id}/info/refs", s.handleInfoRefs).Methods("GET")

	// git-upload-pack - clone/fetch
	gitRouter.HandleFunc("/{repo_id}/git-upload-pack", s.handleUploadPack).Methods("POST")

	// git-receive-pack - push
	gitRouter.HandleFunc("/{repo_id}/git-receive-pack", s.handleReceivePack).Methods("POST")

	// Clone info endpoint (for UI)
	gitRouter.HandleFunc("/{repo_id}/clone-info", s.handleCloneInfo).Methods("GET")

	// Repository status endpoint
	gitRouter.HandleFunc("/{repo_id}/status", s.handleRepositoryStatus).Methods("GET")
}

func (s *GitHTTPServer) gzipDecompressMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Encoding") == "gzip" {
			gz, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, "Failed to decompress request", http.StatusBadRequest)
				return
			}
			defer gz.Close()
			r.Body = gz
			r.Header.Del("Content-Encoding")
		}
		next.ServeHTTP(w, r)
	})
}

func (s *GitHTTPServer) GetCloneURL(repositoryID string) string {
	baseURL := strings.TrimSuffix(s.serverBaseURL, "/")
	return fmt.Sprintf("%s/git/%s", baseURL, repositoryID)
}

func (s *GitHTTPServer) GetCloneInstructions(repositoryID string, apiKey string) string {
	cloneURL := s.GetCloneURL(repositoryID)
	return fmt.Sprintf("git clone %s\n\n# Use your API key as the password when prompted", cloneURL)
}

// authMiddleware handles authentication for git requests
func (s *GitHTTPServer) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Info().Str("method", r.Method).Str("path", r.URL.Path).Str("remote_addr", r.RemoteAddr).Msg("Git HTTP request received")

		apiKey := s.extractAPIKey(r)
		if apiKey == "" {
			log.Warn().Str("method", r.Method).Str("path", r.URL.Path).Msg("Git request missing API key")
			w.Header().Set("WWW-Authenticate", `Basic realm="Helix Git"`)
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}

		user, err := s.validateAPIKeyAndGetUser(r.Context(), apiKey)
		if err != nil {
			log.Warn().Err(err).Msg("Invalid API key")
			http.Error(w, "Invalid API key", http.StatusUnauthorized)
			return
		}

		log.Info().Str("path", r.URL.Path).Str("user_id", user.ID).Msg("Git request authenticated")

		ctx := context.WithValue(r.Context(), "user", user)
		ctx = context.WithValue(ctx, "api_key", apiKey)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *GitHTTPServer) extractAPIKey(r *http.Request) string {
	// Check Authorization header (Basic auth)
	if auth := r.Header.Get("Authorization"); auth != "" {
		if strings.HasPrefix(auth, "Basic ") {
			payload, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(auth, "Basic "))
			if err == nil {
				parts := strings.SplitN(string(payload), ":", 2)
				if len(parts) == 2 {
					return s.extractRawAPIKey(parts[1])
				}
			}
		}
	}

	// Check custom header
	if s.authTokenHeader != "" {
		if token := r.Header.Get(s.authTokenHeader); token != "" {
			return s.extractRawAPIKey(token)
		}
	}

	return ""
}

func (s *GitHTTPServer) validateAPIKeyAndGetUser(ctx context.Context, apiKey string) (*types.User, error) {
	rawKey := s.extractRawAPIKey(apiKey)

	// Use the correct query type for GetAPIKey
	apiKeyRecord, err := s.store.GetAPIKey(ctx, &types.ApiKey{Key: rawKey})
	if err != nil {
		return nil, fmt.Errorf("invalid API key: %w", err)
	}

	// Use the correct query type for GetUser
	user, err := s.store.GetUser(ctx, &store.GetUserQuery{ID: apiKeyRecord.Owner})
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	return user, nil
}

func (s *GitHTTPServer) extractRawAPIKey(apiKey string) string {
	if strings.HasPrefix(apiKey, "Bearer ") {
		return strings.TrimPrefix(apiKey, "Bearer ")
	}
	return apiKey
}

// getRepoPath gets the local filesystem path for a repository
func (s *GitHTTPServer) getRepoPath(ctx context.Context, repoID string) (string, *types.GitRepository, error) {
	repo, err := s.gitRepoService.GetRepository(ctx, repoID)
	if err != nil {
		return "", nil, fmt.Errorf("repository not found: %w", err)
	}

	if repo.LocalPath == "" {
		return "", nil, fmt.Errorf("repository has no local path")
	}

	if _, err := os.Stat(repo.LocalPath); os.IsNotExist(err) {
		return "", nil, fmt.Errorf("repository path does not exist: %s", repo.LocalPath)
	}

	return repo.LocalPath, repo, nil
}

// prepareGitCmd creates a git command for the given service using string literals
// This follows gitea's pattern to ensure type safety with internal.CmdArg
func (s *GitHTTPServer) prepareGitCmd(service string, advertiseRefs bool) *gitcmd.Command {
	switch service {
	case "git-upload-pack":
		if advertiseRefs {
			return gitcmd.NewCommand("upload-pack").AddArguments("--stateless-rpc", "--advertise-refs")
		}
		return gitcmd.NewCommand("upload-pack").AddArguments("--stateless-rpc")
	case "git-receive-pack":
		if advertiseRefs {
			return gitcmd.NewCommand("receive-pack").AddArguments("--stateless-rpc", "--advertise-refs")
		}
		return gitcmd.NewCommand("receive-pack").AddArguments("--stateless-rpc")
	default:
		return nil
	}
}

// handleInfoRefs handles GET /info/refs (ref advertisement)
func (s *GitHTTPServer) handleInfoRefs(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["repo_id"]
	service := r.URL.Query().Get("service")
	user := s.getUser(r)

	log.Info().Str("repo_id", repoID).Str("service", service).Str("user_id", user.ID).Msg("Handling info/refs request")

	if service != "git-upload-pack" && service != "git-receive-pack" {
		http.Error(w, "Invalid service", http.StatusBadRequest)
		return
	}

	// Check access based on service type
	if service == "git-upload-pack" {
		if !s.enablePull || !s.hasReadAccess(r.Context(), user, repoID) {
			http.Error(w, "Access denied", http.StatusForbidden)
			return
		}
	} else if service == "git-receive-pack" {
		if !s.enablePush || !s.hasWriteAccess(r.Context(), user, repoID) {
			http.Error(w, "Access denied", http.StatusForbidden)
			return
		}
	}

	repoPath, _, err := s.getRepoPath(r.Context(), repoID)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to get repository path")
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	// Build git command using the helper that uses string literals
	cmd := s.prepareGitCmd(service, true)
	if cmd == nil {
		http.Error(w, "Invalid service", http.StatusBadRequest)
		return
	}

	// Set up environment
	environ := append(os.Environ(), gitcmd.CommonGitCmdEnvs()...)
	if protocol := r.Header.Get("Git-Protocol"); protocol != "" {
		environ = append(environ, "GIT_PROTOCOL="+protocol)
	}

	// Run git command and capture output using RunOpts
	stdout, _, err := cmd.AddDynamicArguments(".").RunStdBytes(r.Context(), &gitcmd.RunOpts{
		Dir: repoPath,
		Env: environ,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to advertise references")
		http.Error(w, "Failed to get references", http.StatusInternalServerError)
		return
	}

	// Write response with proper headers
	w.Header().Set("Content-Type", fmt.Sprintf("application/x-%s-advertisement", service))
	setNoCacheHeaders(w)

	// Write packet header
	pktLine := fmt.Sprintf("# service=%s\n", service)
	pktLen := fmt.Sprintf("%04x", len(pktLine)+4)
	w.Write([]byte(pktLen + pktLine))
	w.Write([]byte("0000")) // flush packet

	// Write refs
	w.Write(stdout)

	log.Info().Str("repo_id", repoID).Str("service", service).Msg("Sent advertised references")
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

	// Validate Content-Type (following gitea's pattern)
	expectedContentType := "application/x-git-upload-pack-request"
	if r.Header.Get("Content-Type") != expectedContentType {
		log.Warn().Str("expected", expectedContentType).Str("got", r.Header.Get("Content-Type")).Msg("Invalid Content-Type for upload-pack")
		http.Error(w, "Invalid Content-Type", http.StatusBadRequest)
		return
	}

	repoPath, repo, err := s.getRepoPath(r.Context(), repoID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get repository path")
		http.Error(w, "Failed to access repository", http.StatusInternalServerError)
		return
	}

	// If this is an external repository, sync from upstream BEFORE serving the pull.
	if repo != nil && repo.ExternalURL != "" {
		log.Info().Str("repo_id", repoID).Str("external_url", repo.ExternalURL).Msg("Syncing from upstream before serving pull")
		if err := s.gitRepoService.SyncAllBranches(r.Context(), repoID, false); err != nil {
			log.Warn().Err(err).Str("repo_id", repoID).Msg("Failed to sync from upstream before pull - serving cached data")
		} else {
			log.Info().Str("repo_id", repoID).Msg("Successfully synced from upstream before pull")
		}
	}

	// Handle GZIP-encoded request body (following gitea's pattern)
	reqBody := r.Body
	if r.Header.Get("Content-Encoding") == "gzip" {
		var gzErr error
		reqBody, gzErr = gzip.NewReader(reqBody)
		if gzErr != nil {
			log.Error().Err(gzErr).Msg("Failed to create gzip reader")
			http.Error(w, "Failed to decompress request", http.StatusBadRequest)
			return
		}
	}

	// Set response headers
	w.Header().Set("Content-Type", "application/x-git-upload-pack-result")
	w.Header().Set("Connection", "Keep-Alive")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	setNoCacheHeaders(w)

	// Build git command: git upload-pack --stateless-rpc .
	// Using string literals for gitcmd.NewCommand to satisfy internal.CmdArg type
	cmd := gitcmd.NewCommand("upload-pack").
		AddArguments("--stateless-rpc").
		AddDynamicArguments(".")

	// Set up environment
	environ := append(os.Environ(), gitcmd.CommonGitCmdEnvs()...)
	if protocol := r.Header.Get("Git-Protocol"); protocol != "" {
		environ = append(environ, "GIT_PROTOCOL="+protocol)
	}

	// Use flushing writer for streaming
	fw := newFlushingWriter(w)

	// Run git upload-pack with stdin/stdout piped to HTTP
	err = cmd.Run(r.Context(), &gitcmd.RunOpts{
		Dir:    repoPath,
		Env:    environ,
		Stdin:  reqBody,
		Stdout: fw,
	})
	if err != nil {
		if !isContextCanceledOrKilled(err) {
			log.Error().Err(err).Msg("Upload-pack failed")
		}
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

	// Validate Content-Type (following gitea's pattern)
	expectedContentType := "application/x-git-receive-pack-request"
	if r.Header.Get("Content-Type") != expectedContentType {
		log.Warn().Str("expected", expectedContentType).Str("got", r.Header.Get("Content-Type")).Msg("Invalid Content-Type for receive-pack")
		http.Error(w, "Invalid Content-Type", http.StatusBadRequest)
		return
	}

	repoPath, repo, err := s.getRepoPath(r.Context(), repoID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get repository path")
		http.Error(w, "Failed to access repository", http.StatusInternalServerError)
		return
	}

	// If this is an external repository, sync from upstream BEFORE accepting the push.
	if repo != nil && repo.ExternalURL != "" {
		log.Info().Str("repo_id", repoID).Str("external_url", repo.ExternalURL).Msg("Syncing from upstream before accepting push")
		if err := s.gitRepoService.SyncAllBranches(r.Context(), repoID, false); err != nil {
			log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to sync from upstream - rejecting push")
			http.Error(w, fmt.Sprintf("Failed to sync from upstream repository: %v", err), http.StatusConflict)
			return
		}
		log.Info().Str("repo_id", repoID).Msg("Successfully synced from upstream before push")
	}

	// Handle GZIP-encoded request body (following gitea's pattern)
	reqBody := r.Body
	if r.Header.Get("Content-Encoding") == "gzip" {
		var gzErr error
		reqBody, gzErr = gzip.NewReader(reqBody)
		if gzErr != nil {
			log.Error().Err(gzErr).Msg("Failed to create gzip reader")
			http.Error(w, "Failed to decompress request", http.StatusBadRequest)
			return
		}
	}

	// Get branches before push to detect what changed
	branchesBefore := s.getBranchHashes(repoPath)

	// Set response headers
	w.Header().Set("Content-Type", "application/x-git-receive-pack-result")
	w.Header().Set("Connection", "Keep-Alive")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	setNoCacheHeaders(w)

	// Build git command: git receive-pack --stateless-rpc .
	cmd := gitcmd.NewCommand("receive-pack").
		AddArguments("--stateless-rpc").
		AddDynamicArguments(".")

	// Set up environment
	environ := append(os.Environ(), gitcmd.CommonGitCmdEnvs()...)
	if protocol := r.Header.Get("Git-Protocol"); protocol != "" {
		environ = append(environ, "GIT_PROTOCOL="+protocol)
	}

	// Use flushing writer for streaming
	fw := newFlushingWriter(w)

	// Run git receive-pack with stdin/stdout piped to HTTP
	err = cmd.Run(r.Context(), &gitcmd.RunOpts{
		Dir:    repoPath,
		Env:    environ,
		Stdin:  reqBody,
		Stdout: fw,
	})
	if err != nil {
		if !isContextCanceledOrKilled(err) {
			log.Error().Err(err).Msg("Receive-pack failed")
		}
		return
	}

	// Detect pushed branches by comparing before/after
	branchesAfter := s.getBranchHashes(repoPath)
	pushedBranches := s.detectChangedBranches(branchesBefore, branchesAfter)

	log.Info().Str("repo_id", repoID).Strs("pushed_branches", pushedBranches).Msg("Receive-pack completed")

	// Check branch restrictions for agent API keys
	if len(pushedBranches) > 0 {
		apiKey := s.extractAPIKey(r)
		restriction, err := s.getBranchRestrictionForAPIKey(r.Context(), apiKey)
		if err != nil {
			log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to get branch restriction for API key")
		}
		if restriction != nil && restriction.IsAgentKey {
			if restriction.ErrorMessage != "" {
				log.Error().Str("repo_id", repoID).Str("error", restriction.ErrorMessage).Msg("Agent push denied - rolling back")
				s.rollbackBranchRefs(repoPath, branchesBefore, pushedBranches)
				return
			}
			for _, branch := range pushedBranches {
				allowed := false
				for _, ab := range restriction.AllowedBranches {
					if branch == ab {
						allowed = true
						break
					}
				}
				if !allowed {
					log.Error().Str("repo_id", repoID).Str("pushed_branch", branch).Strs("allowed_branches", restriction.AllowedBranches).Msg("Agent attempted to push to unauthorized branch - rolling back")
					s.rollbackBranchRefs(repoPath, branchesBefore, pushedBranches)
					return
				}
			}
			log.Info().Str("repo_id", repoID).Strs("allowed_branches", restriction.AllowedBranches).Msg("Agent branch restriction verified")
		}
	}

	// For external repos, SYNCHRONOUSLY push to upstream
	if len(pushedBranches) > 0 && repo != nil && repo.ExternalURL != "" {
		upstreamPushFailed := false
		for _, branch := range pushedBranches {
			log.Info().Str("repo_id", repoID).Str("branch", branch).Msg("Pushing branch to upstream (synchronous)")
			if err := s.gitRepoService.PushBranchToRemote(r.Context(), repoID, branch, false); err != nil {
				log.Error().Err(err).Str("repo_id", repoID).Str("branch", branch).Msg("Failed to push branch to upstream - rolling back")
				upstreamPushFailed = true
				break
			}
			log.Info().Str("repo_id", repoID).Str("branch", branch).Msg("Successfully pushed branch to upstream")
		}

		if upstreamPushFailed {
			log.Warn().Str("repo_id", repoID).Msg("Rolling back refs due to upstream push failure")
			s.rollbackBranchRefs(repoPath, branchesBefore, pushedBranches)
			return
		}
	}

	// Trigger post-push hooks asynchronously
	if len(pushedBranches) > 0 && repo != nil {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handlePostPushHook(context.Background(), repoID, repo.LocalPath, pushedBranches)
		}()
	}
}

// isContextCanceledOrKilled checks if the error is due to context cancellation or process being killed
func isContextCanceledOrKilled(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	errStr := err.Error()
	return strings.Contains(errStr, "signal: killed") || strings.Contains(errStr, "context canceled")
}

// getBranchHashes returns a map of branch names to their current commit hashes
func (s *GitHTTPServer) getBranchHashes(repoPath string) map[string]string {
	result := make(map[string]string)

	stdout, _, err := gitcmd.NewCommand("for-each-ref").
		AddArguments("--format=%(refname:short) %(objectname)").
		AddDynamicArguments("refs/heads/").
		RunStdString(context.Background(), &gitcmd.RunOpts{
			Dir: repoPath,
		})
	if err != nil {
		return result
	}

	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}

	return result
}

// detectChangedBranches compares before/after branch hashes to find changed branches
func (s *GitHTTPServer) detectChangedBranches(before, after map[string]string) []string {
	var changed []string
	for branch, hash := range after {
		if beforeHash, exists := before[branch]; !exists || beforeHash != hash {
			changed = append(changed, branch)
		}
	}
	return changed
}

// rollbackBranchRefs restores branch refs to their previous state using native git
func (s *GitHTTPServer) rollbackBranchRefs(repoPath string, previousHashes map[string]string, branches []string) {
	for _, branch := range branches {
		refName := "refs/heads/" + branch
		if prevHash, existed := previousHashes[branch]; existed {
			// Branch existed before - restore to previous hash
			_, _, err := gitcmd.NewCommand("update-ref").
				AddDynamicArguments(refName, prevHash).
				RunStdString(context.Background(), &gitcmd.RunOpts{
					Dir: repoPath,
				})
			if err != nil {
				log.Error().Err(err).Str("branch", branch).Str("hash", prevHash).Msg("Failed to rollback branch ref")
			} else {
				log.Info().Str("branch", branch).Str("hash", prevHash).Msg("Rolled back branch ref")
			}
		} else {
			// Branch was newly created - delete it
			_, _, err := gitcmd.NewCommand("update-ref").
				AddArguments("-d").
				AddDynamicArguments(refName).
				RunStdString(context.Background(), &gitcmd.RunOpts{
					Dir: repoPath,
				})
			if err != nil {
				log.Error().Err(err).Str("branch", branch).Msg("Failed to remove newly created branch ref")
			} else {
				log.Info().Str("branch", branch).Msg("Removed newly created branch ref")
			}
		}
	}
}

// getBranchRestrictionForAPIKey checks if an API key has branch restrictions
func (s *GitHTTPServer) getBranchRestrictionForAPIKey(ctx context.Context, apiKey string) (*BranchRestriction, error) {
	rawKey := s.extractRawAPIKey(apiKey)

	apiKeyRecord, err := s.store.GetAPIKey(ctx, &types.ApiKey{Key: rawKey})
	if err != nil {
		return nil, err
	}

	// Check if this is an agent API key (has spec_task_id)
	if apiKeyRecord.SpecTaskID == "" {
		return nil, nil // Not an agent key, no restrictions
	}

	// Get the spec task to find allowed branches
	task, err := s.store.GetSpecTask(ctx, apiKeyRecord.SpecTaskID)
	if err != nil {
		return &BranchRestriction{
			IsAgentKey:   true,
			ErrorMessage: "Agent's spec task not found",
		}, nil
	}

	// Agent can push to their feature branch
	allowedBranches := []string{}
	if task.BranchName != "" {
		allowedBranches = append(allowedBranches, task.BranchName)
	}

	return &BranchRestriction{
		IsAgentKey:      true,
		AllowedBranches: allowedBranches,
	}, nil
}

func (s *GitHTTPServer) hasReadAccess(ctx context.Context, user *types.User, repoID string) bool {
	if s.authorizeFn == nil {
		log.Debug().Str("repo_id", repoID).Msg("hasReadAccess: no authorizeFn, allowing access")
		return true
	}
	// Use proper authorization check
	repo, err := s.gitRepoService.GetRepository(ctx, repoID)
	if err != nil {
		log.Warn().Err(err).Str("repo_id", repoID).Msg("hasReadAccess: failed to get repository")
		return false
	}

	// Repository owner can always read the repository
	if user.ID == repo.OwnerID {
		log.Debug().Str("repo_id", repoID).Str("user_id", user.ID).Msg("hasReadAccess: user is repo owner, allowing")
		return true
	}

	// Use ActionGet for read access on projects (requires org membership)
	err = s.authorizeFn(ctx, user, repo.OrganizationID, repo.ProjectID, types.ResourceProject, types.ActionGet)
	if err != nil {
		log.Warn().Err(err).Str("repo_id", repoID).Str("project_id", repo.ProjectID).Str("org_id", repo.OrganizationID).Str("user_id", user.ID).Msg("hasReadAccess: authorization failed")
		return false
	}
	log.Debug().Str("repo_id", repoID).Str("project_id", repo.ProjectID).Str("user_id", user.ID).Msg("hasReadAccess: authorized")
	return true
}

func (s *GitHTTPServer) hasWriteAccess(ctx context.Context, user *types.User, repoID string) bool {
	if s.authorizeFn == nil {
		log.Debug().Str("repo_id", repoID).Msg("hasWriteAccess: no authorizeFn, allowing access")
		return true
	}
	// Use proper authorization check
	repo, err := s.gitRepoService.GetRepository(ctx, repoID)
	if err != nil {
		log.Warn().Err(err).Str("repo_id", repoID).Msg("hasWriteAccess: failed to get repository")
		return false
	}

	// Repository owner can always write to the repository
	if user.ID == repo.OwnerID {
		log.Debug().Str("repo_id", repoID).Str("user_id", user.ID).Msg("hasWriteAccess: user is repo owner, allowing")
		return true
	}

	// Use ActionUpdate for write access on projects (requires org membership)
	err = s.authorizeFn(ctx, user, repo.OrganizationID, repo.ProjectID, types.ResourceProject, types.ActionUpdate)
	if err != nil {
		log.Warn().Err(err).Str("repo_id", repoID).Str("project_id", repo.ProjectID).Str("org_id", repo.OrganizationID).Str("user_id", user.ID).Msg("hasWriteAccess: authorization failed")
		return false
	}
	log.Debug().Str("repo_id", repoID).Str("project_id", repo.ProjectID).Str("user_id", user.ID).Msg("hasWriteAccess: authorized")
	return true
}

func (s *GitHTTPServer) getUser(r *http.Request) *types.User {
	if user, ok := r.Context().Value("user").(*types.User); ok {
		return user
	}
	return &types.User{ID: "anonymous"}
}

func (s *GitHTTPServer) handleCloneInfo(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["repo_id"]

	cloneURL := s.GetCloneURL(repoID)

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"clone_url": "%s", "instructions": "Use your API key as the password"}`, cloneURL)
}

func (s *GitHTTPServer) handleRepositoryStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["repo_id"]

	repoPath, repo, err := s.getRepoPath(r.Context(), repoID)
	if err != nil {
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	// Get branch count using gitea's high-level API
	gitRepo, err := giteagit.OpenRepository(r.Context(), repoPath)
	branchCount := 0
	if err == nil {
		defer gitRepo.Close()
		_, branchCount, _ = gitRepo.GetBranchNames(0, 0)
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"id": "%s", "name": "%s", "branch_count": %d, "is_external": %t}`,
		repo.ID, repo.Name, branchCount, repo.IsExternal)
}

// handlePostPushHook processes commits after a successful push
// This delegates to the existing post-push logic
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

// Placeholder methods that will be filled in from the existing implementation
// These handle the business logic for spec tasks, design docs, etc.

func (s *GitHTTPServer) handleFeatureBranchPush(ctx context.Context, repo *types.GitRepository, branchName, commitHash, repoPath string, gitRepo *GitRepo) {
	// TODO: Copy from existing implementation
	log.Info().Str("repo_id", repo.ID).Str("branch", branchName).Msg("Feature branch push detected")
}

func (s *GitHTTPServer) handleMainBranchPush(ctx context.Context, repo *types.GitRepository, commitHash, repoPath string, gitRepo *GitRepo) {
	// TODO: Copy from existing implementation
	log.Info().Str("repo_id", repo.ID).Str("commit", commitHash).Msg("Main branch push detected")
}

func (s *GitHTTPServer) processDesignDocsForBranch(ctx context.Context, repo *types.GitRepository, repoPath, pushedBranch, commitHash string, gitRepo *GitRepo) {
	// TODO: Copy from existing implementation
	log.Info().Str("repo_id", repo.ID).Str("branch", pushedBranch).Msg("Processing design docs")
}
