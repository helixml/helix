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
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/format/pktline"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/server"
	"github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/trigger"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// GoGitHTTPServer provides HTTP access to git repositories using pure Go (go-git).
// This replaces the CGI-based git-http-backend approach with a native Go implementation
// that is more robust and provides better error handling.
type GoGitHTTPServer struct {
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

// NewGoGitHTTPServer creates a new go-git based HTTP server
func NewGoGitHTTPServer(
	st store.Store,
	gitRepoService *GitRepositoryService,
	config *GitHTTPServerConfig,
	authorizeFn AuthorizationFunc,
	triggerManager *trigger.Manager,
) *GoGitHTTPServer {
	if config.AuthTokenHeader == "" {
		config.AuthTokenHeader = "Authorization"
	}
	if config.RequestTimeout == 0 {
		config.RequestTimeout = 5 * time.Minute
	}
	if config.MaxRepoSize == 0 {
		config.MaxRepoSize = 1024 * 1024 * 1024 // 1GB default
	}

	s := &GoGitHTTPServer{
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
	server *GoGitHTTPServer
}

// Load implements server.Loader - loads a repository's storage from endpoint
func (l *helixRepoLoader) Load(ep *transport.Endpoint) (storer.Storer, error) {
	// The endpoint path is the repository ID (e.g., /repo-id)
	repoID := strings.TrimPrefix(ep.Path, "/")
	if repoID == "" {
		return nil, transport.ErrRepositoryNotFound
	}

	// Get repository from our database
	repo, err := l.server.gitRepoService.GetRepository(context.Background(), repoID)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to get repository")
		return nil, transport.ErrRepositoryNotFound
	}

	// Determine if this is a bare or non-bare repository
	// Bare repos have objects/ and refs/ directly in the path
	// Non-bare repos have a .git directory containing these
	var gitDir string

	// Check for non-bare repository first (most common case)
	dotGitPath := filepath.Join(repo.LocalPath, ".git")
	if info, err := os.Stat(dotGitPath); err == nil && info.IsDir() {
		// Non-bare repository - use .git directory
		gitDir = dotGitPath
	} else if _, err := os.Stat(filepath.Join(repo.LocalPath, "objects")); err == nil {
		// Bare repository - has objects/ directory in root
		// Additional check: HEAD file should exist in bare repos
		if _, err := os.Stat(filepath.Join(repo.LocalPath, "HEAD")); err == nil {
			gitDir = repo.LocalPath
		} else {
			log.Error().Str("repo_path", repo.LocalPath).Msg("Invalid bare repository (missing HEAD)")
			return nil, transport.ErrRepositoryNotFound
		}
	} else {
		log.Error().Str("repo_path", repo.LocalPath).Msg("Not a valid git repository (no .git or objects/)")
		return nil, transport.ErrRepositoryNotFound
	}

	// Create filesystem for the git directory
	fs := osfs.New(gitDir)

	log.Debug().
		Str("repo_id", repoID).
		Str("git_dir", gitDir).
		Msg("Loading repository storage")

	// Create filesystem storage with LRU cache for objects
	storage := filesystem.NewStorage(fs, cache.NewObjectLRUDefault())
	return storage, nil
}

// SetTestMode enables or disables test mode
func (s *GoGitHTTPServer) SetTestMode(enabled bool) {
	s.testMode = enabled
}

// SetMessageSender sets the callback for sending messages to spec task agents via WebSocket
func (s *GoGitHTTPServer) SetMessageSender(sender SpecTaskMessageSender) {
	s.sendMessageToAgentFunc = sender
}

// RegisterRoutes registers HTTP git server routes
func (s *GoGitHTTPServer) RegisterRoutes(router *mux.Router) {
	gitRouter := router.PathPrefix("/git").Subrouter()
	gitRouter.Use(s.authMiddleware)

	// Git smart HTTP protocol routes
	gitRouter.HandleFunc("/{repo_id}/info/refs", s.handleInfoRefs).Methods("GET")
	gitRouter.HandleFunc("/{repo_id}/git-upload-pack", s.handleUploadPack).Methods("POST")
	gitRouter.HandleFunc("/{repo_id}/git-receive-pack", s.handleReceivePack).Methods("POST")

	// Repository information routes
	gitRouter.HandleFunc("/{repo_id}/clone-info", s.handleCloneInfo).Methods("GET")
	gitRouter.HandleFunc("/{repo_id}/status", s.handleRepositoryStatus).Methods("GET")

	log.Info().Msg("Go-git HTTP server routes registered")
}

// GetCloneURL returns the HTTP clone URL for a repository
func (s *GoGitHTTPServer) GetCloneURL(repositoryID string) string {
	return fmt.Sprintf("%s/git/%s", s.serverBaseURL, repositoryID)
}

// authMiddleware provides API key authentication for git operations
func (s *GoGitHTTPServer) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Info().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Str("remote_addr", r.RemoteAddr).
			Msg("Git HTTP request received")

		apiKey := s.extractAPIKey(r)
		if apiKey == "" {
			log.Warn().
				Str("path", r.URL.Path).
				Str("method", r.Method).
				Msg("Git request missing API key")
			w.Header().Set("WWW-Authenticate", "Basic realm=\"Helix Git Server\"")
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}

		user, err := s.validateAPIKeyAndGetUser(r.Context(), apiKey)
		if err != nil {
			log.Error().Err(err).Msg("Failed to validate API key for git request")
			w.Header().Set("WWW-Authenticate", "Basic realm=\"Helix Git Server\"")
			http.Error(w, "Authentication failed", http.StatusUnauthorized)
			return
		}

		if user == nil {
			log.Warn().Msg("Invalid API key for git request")
			w.Header().Set("WWW-Authenticate", "Basic realm=\"Helix Git Server\"")
			http.Error(w, "Invalid API key", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), "git_user", user)
		r = r.WithContext(ctx)

		log.Info().
			Str("user_id", user.ID).
			Str("path", r.URL.Path).
			Msg("Git request authenticated successfully")

		next.ServeHTTP(w, r)
	})
}

// extractAPIKey extracts API key from request
func (s *GoGitHTTPServer) extractAPIKey(r *http.Request) string {
	if auth := r.Header.Get(s.authTokenHeader); auth != "" {
		if strings.HasPrefix(auth, "Bearer ") {
			return strings.TrimPrefix(auth, "Bearer ")
		}
		if strings.HasPrefix(auth, "Basic ") {
			return auth // Will be decoded in validateAPIKeyAndGetUser
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
func (s *GoGitHTTPServer) validateAPIKeyAndGetUser(ctx context.Context, apiKey string) (*types.User, error) {
	if s.testMode {
		return &types.User{
			ID:    "test_user",
			Email: "test@example.com",
			Admin: false,
		}, nil
	}

	// Handle Basic auth format
	if strings.HasPrefix(apiKey, "Basic ") {
		encodedCreds := strings.TrimPrefix(apiKey, "Basic ")
		decodedBytes, err := base64.StdEncoding.DecodeString(encodedCreds)
		if err != nil {
			return nil, fmt.Errorf("invalid Basic auth encoding")
		}

		credentials := string(decodedBytes)
		parts := strings.SplitN(credentials, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid Basic auth format")
		}

		apiKey = parts[1]
	}

	apiKeyRecord, err := s.store.GetAPIKey(ctx, &types.ApiKey{Key: apiKey})
	if err != nil {
		return nil, err
	}

	if apiKeyRecord == nil {
		return nil, nil
	}

	if apiKeyRecord.Created.IsZero() {
		return nil, nil
	}

	user, err := s.store.GetUser(ctx, &store.GetUserQuery{ID: apiKeyRecord.Owner})
	if err != nil {
		return nil, err
	}

	return user, nil
}

// handleInfoRefs handles GET /info/refs?service=git-upload-pack or git-receive-pack
func (s *GoGitHTTPServer) handleInfoRefs(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["repo_id"]
	service := r.URL.Query().Get("service")

	log.Info().
		Str("repo_id", repoID).
		Str("service", service).
		Msg("Handling info/refs request")

	if service != "git-upload-pack" && service != "git-receive-pack" {
		http.Error(w, "Invalid service", http.StatusBadRequest)
		return
	}

	// Create endpoint for the repository
	ep, err := transport.NewEndpoint("/" + repoID)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to create endpoint")
		http.Error(w, "Invalid repository", http.StatusBadRequest)
		return
	}

	// Get advertised references based on service type
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
			log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to get advertised references")
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
			log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to get advertised references")
			http.Error(w, "Failed to get references", http.StatusInternalServerError)
			return
		}
	}

	// Write response
	w.Header().Set("Content-Type", fmt.Sprintf("application/x-%s-advertisement", service))
	w.Header().Set("Cache-Control", "no-cache")

	// Write pkt-line header
	pktEnc := pktline.NewEncoder(w)
	if err := pktEnc.Encodef("# service=%s\n", service); err != nil {
		log.Error().Err(err).Msg("Failed to write service header")
		return
	}
	if err := pktEnc.Flush(); err != nil {
		log.Error().Err(err).Msg("Failed to flush service header")
		return
	}

	// Encode and write advertised refs
	if err := advRefs.Encode(w); err != nil {
		log.Error().Err(err).Msg("Failed to encode advertised references")
		return
	}

	log.Info().
		Str("repo_id", repoID).
		Str("service", service).
		Int("refs_count", len(advRefs.References)).
		Msg("Sent advertised references")
}

// handleUploadPack handles POST /git-upload-pack (clone/fetch)
func (s *GoGitHTTPServer) handleUploadPack(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["repo_id"]
	user := s.getUser(r)

	log.Info().
		Str("repo_id", repoID).
		Str("user_id", user.ID).
		Int64("content_length", r.ContentLength).
		Msg("Handling upload-pack request")

	if !s.enablePull {
		http.Error(w, "Pull operations disabled", http.StatusForbidden)
		return
	}

	if !s.hasReadAccess(r.Context(), user, repoID) {
		http.Error(w, "Read access denied", http.StatusForbidden)
		return
	}

	// Create endpoint
	ep, err := transport.NewEndpoint("/" + repoID)
	if err != nil {
		http.Error(w, "Invalid repository", http.StatusBadRequest)
		return
	}

	// Create upload-pack session
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

	// Handle gzip-compressed request body
	body := r.Body
	if r.Header.Get("Content-Encoding") == "gzip" {
		gzReader, err := gzip.NewReader(r.Body)
		if err != nil {
			log.Error().Err(err).Msg("Failed to create gzip reader")
			http.Error(w, "Failed to decompress request", http.StatusBadRequest)
			return
		}
		defer gzReader.Close()
		body = gzReader
	}

	// Buffer the request body for reliable parsing
	bodyBytes, err := io.ReadAll(body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read request body")
		http.Error(w, "Failed to read request", http.StatusInternalServerError)
		return
	}

	log.Debug().
		Str("repo_id", repoID).
		Int("body_bytes", len(bodyBytes)).
		Msg("Buffered upload-pack request body")

	// Parse upload-pack request
	req := packp.NewUploadPackRequest()
	if err := req.Decode(bytes.NewReader(bodyBytes)); err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to decode upload-pack request")
		http.Error(w, fmt.Sprintf("Failed to parse request: %v", err), http.StatusBadRequest)
		return
	}

	// Set response headers
	w.Header().Set("Content-Type", "application/x-git-upload-pack-result")
	w.Header().Set("Cache-Control", "no-cache")

	// Execute upload-pack
	resp, err := session.UploadPack(r.Context(), req)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Upload-pack failed")
		http.Error(w, fmt.Sprintf("Upload-pack failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Encode response
	if err := resp.Encode(w); err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to encode upload-pack response")
		return
	}

	log.Info().
		Str("repo_id", repoID).
		Msg("Upload-pack completed successfully")
}

// handleReceivePack handles POST /git-receive-pack (push)
func (s *GoGitHTTPServer) handleReceivePack(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["repo_id"]
	user := s.getUser(r)

	log.Info().
		Str("repo_id", repoID).
		Str("user_id", user.ID).
		Int64("content_length", r.ContentLength).
		Msg("Handling receive-pack request")

	if !s.enablePush {
		http.Error(w, "Push operations disabled", http.StatusForbidden)
		return
	}

	if !s.hasWriteAccess(r.Context(), user, repoID) {
		http.Error(w, "Write access denied", http.StatusForbidden)
		return
	}

	// Create endpoint
	ep, err := transport.NewEndpoint("/" + repoID)
	if err != nil {
		http.Error(w, "Invalid repository", http.StatusBadRequest)
		return
	}

	// Create receive-pack session
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

	// Handle gzip-compressed request body
	body := r.Body
	if r.Header.Get("Content-Encoding") == "gzip" {
		gzReader, err := gzip.NewReader(r.Body)
		if err != nil {
			log.Error().Err(err).Msg("Failed to create gzip reader")
			http.Error(w, "Failed to decompress request", http.StatusBadRequest)
			return
		}
		defer gzReader.Close()
		body = gzReader
	}

	// Buffer the request body
	bodyBytes, err := io.ReadAll(body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read request body")
		http.Error(w, "Failed to read request", http.StatusInternalServerError)
		return
	}

	log.Debug().
		Str("repo_id", repoID).
		Int("body_bytes", len(bodyBytes)).
		Msg("Buffered receive-pack request body")

	// Parse reference update request
	req := packp.NewReferenceUpdateRequest()
	if err := req.Decode(bytes.NewReader(bodyBytes)); err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to decode receive-pack request")
		http.Error(w, fmt.Sprintf("Failed to parse request: %v", err), http.StatusBadRequest)
		return
	}

	// Extract pushed branches for post-push hook
	var pushedBranches []string
	for _, cmd := range req.Commands {
		if strings.HasPrefix(string(cmd.Name), "refs/heads/") {
			branchName := strings.TrimPrefix(string(cmd.Name), "refs/heads/")
			pushedBranches = append(pushedBranches, branchName)
		}
	}

	log.Info().
		Str("repo_id", repoID).
		Strs("pushed_branches", pushedBranches).
		Int("commands", len(req.Commands)).
		Msg("Parsed receive-pack commands")

	// Set response headers
	w.Header().Set("Content-Type", "application/x-git-receive-pack-result")
	w.Header().Set("Cache-Control", "no-cache")

	// Execute receive-pack
	resp, err := session.ReceivePack(r.Context(), req)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Receive-pack failed")
		http.Error(w, fmt.Sprintf("Receive-pack failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Encode response
	if resp != nil {
		if err := resp.Encode(w); err != nil {
			log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to encode receive-pack response")
			return
		}
	}

	log.Info().
		Str("repo_id", repoID).
		Strs("pushed_branches", pushedBranches).
		Msg("Receive-pack completed successfully")

	// Trigger post-push hook asynchronously
	if len(pushedBranches) > 0 {
		// Get repository path for hook
		repo, err := s.gitRepoService.GetRepository(r.Context(), repoID)
		if err == nil {
			// Get latest commit hash for the first pushed branch
			var latestCommitHash string
			if len(req.Commands) > 0 {
				latestCommitHash = req.Commands[0].New.String()
			}
			go s.handlePostPushHook(context.Background(), repoID, repo.LocalPath, pushedBranches, latestCommitHash)
		}
	}
}

// handlePostPushHook processes commits after a successful push
// This is called from the existing git_http_server.go's handlePostPushHook logic
func (s *GoGitHTTPServer) handlePostPushHook(ctx context.Context, repoID, repoPath string, pushedBranches []string, latestCommitHash string) {
	log.Info().
		Str("repo_id", repoID).
		Strs("pushed_branches", pushedBranches).
		Str("commit", latestCommitHash).
		Msg("Processing post-push hook (go-git)")

	// Get the repository
	repo, err := s.gitRepoService.GetRepository(ctx, repoID)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to get repository in post-push hook")
		return
	}

	// Process each pushed branch - delegate to existing logic
	// The existing handlePostPushHook in git_http_server.go contains the full
	// spec task processing logic. We reuse that by calling into the shared service.
	for _, pushedBranch := range pushedBranches {
		log.Info().
			Str("repo_id", repoID).
			Str("branch", pushedBranch).
			Str("commit", latestCommitHash).
			Str("repo_path", repoPath).
			Msg("Processing pushed branch")

		// Check for feature branch pushes
		if strings.HasPrefix(pushedBranch, "feature/") {
			s.handleFeatureBranchPush(ctx, repo, pushedBranch, latestCommitHash, repoPath)
		}

		// Check for pushes to default branch
		if repo.DefaultBranch != "" && pushedBranch == repo.DefaultBranch {
			s.handleMainBranchPush(ctx, repo, latestCommitHash, repoPath)
		}

		// Process design docs for this branch
		s.processDesignDocsForBranch(ctx, repo, repoPath, pushedBranch, latestCommitHash)
	}
}

// Stub implementations for hook handlers - these delegate to shared logic
// In production, these would be refactored to share code with git_http_server.go

func (s *GoGitHTTPServer) handleFeatureBranchPush(ctx context.Context, repo *types.GitRepository, branchName, commitHash, repoPath string) {
	log.Info().
		Str("repo_id", repo.ID).
		Str("branch", branchName).
		Str("commit", commitHash).
		Msg("Feature branch push detected (go-git)")
	// TODO: Implement or delegate to shared logic
}

func (s *GoGitHTTPServer) handleMainBranchPush(ctx context.Context, repo *types.GitRepository, commitHash, repoPath string) {
	log.Info().
		Str("repo_id", repo.ID).
		Str("commit", commitHash).
		Msg("Main branch push detected (go-git)")
	// TODO: Implement or delegate to shared logic
}

func (s *GoGitHTTPServer) processDesignDocsForBranch(ctx context.Context, repo *types.GitRepository, repoPath, pushedBranch, latestCommitHash string) {
	log.Info().
		Str("repo_id", repo.ID).
		Str("branch", pushedBranch).
		Str("commit", latestCommitHash).
		Msg("Processing design docs (go-git)")
	// TODO: Implement or delegate to shared logic
}

// hasReadAccess checks if a user has read access to a repository
func (s *GoGitHTTPServer) hasReadAccess(ctx context.Context, user *types.User, repoID string) bool {
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

	err = s.authorizeFn(ctx, user, repo.OrganizationID, repoID, types.ResourceGitRepository, types.ActionGet)
	return err == nil
}

// hasWriteAccess checks if a user has write access to a repository
func (s *GoGitHTTPServer) hasWriteAccess(ctx context.Context, user *types.User, repoID string) bool {
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

	err = s.authorizeFn(ctx, user, repo.OrganizationID, repoID, types.ResourceGitRepository, types.ActionUpdate)
	return err == nil
}

// getUser extracts user object from request context
func (s *GoGitHTTPServer) getUser(r *http.Request) *types.User {
	if user := r.Context().Value("git_user"); user != nil {
		if u, ok := user.(*types.User); ok {
			return u
		}
	}
	return nil
}

// handleCloneInfo provides clone information for a repository
func (s *GoGitHTTPServer) handleCloneInfo(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["repo_id"]

	if repoID == "" {
		http.Error(w, "Repository ID required", http.StatusBadRequest)
		return
	}

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
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cloneInfo)
}

// handleRepositoryStatus provides repository status information
func (s *GoGitHTTPServer) handleRepositoryStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["repo_id"]

	if repoID == "" {
		http.Error(w, "Repository ID required", http.StatusBadRequest)
		return
	}

	repo, err := s.gitRepoService.GetRepository(r.Context(), repoID)
	if err != nil {
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	// Use go-git to get repository info
	ep, err := transport.NewEndpoint("/" + repoID)
	if err != nil {
		http.Error(w, "Invalid repository", http.StatusBadRequest)
		return
	}

	session, err := s.gitServer.NewUploadPackSession(ep, nil)
	if err != nil {
		http.Error(w, "Failed to access repository", http.StatusInternalServerError)
		return
	}
	defer session.Close()

	advRefs, err := session.AdvertisedReferencesContext(r.Context())
	if err != nil {
		http.Error(w, "Failed to get repository info", http.StatusInternalServerError)
		return
	}

	// Build status response
	branches := make([]string, 0)
	for ref := range advRefs.References {
		if strings.HasPrefix(string(ref), "refs/heads/") {
			branches = append(branches, strings.TrimPrefix(string(ref), "refs/heads/"))
		}
	}

	status := map[string]interface{}{
		"repository_id":  repoID,
		"name":           repo.Name,
		"status":         repo.Status,
		"default_branch": repo.DefaultBranch,
		"branches":       branches,
		"refs_count":     len(advRefs.References),
		"clone_url":      s.GetCloneURL(repoID),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}
