package services

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/transport"
	"github.com/go-git/go-git/v6/plumbing/transport/http"
	"github.com/rs/zerolog/log"
)

// PushBranchToRemote pushes a local branch from the bare repository to an external remote.
// This is used when you have local commits in the Helix bare repo that need to be pushed
// to an external repository (GitHub, Azure DevOps, etc.).
//
// The method:
// 1. Creates a temporary working copy from the local bare repo
// 2. Adds the external URL as a remote called "external"
// 3. Pushes the specified branch to the external remote
// 4. Cleans up the temporary working copy
func (s *GitRepositoryService) PushBranchToRemote(ctx context.Context, repoID, branchName string, force bool) error {
	startTime := time.Now()

	log.Debug().
		Str("repo_id", repoID).
		Str("branch", branchName).
		Bool("force", force).
		Msg("[GitPush] Starting push operation")

	gitRepo, err := s.GetRepository(ctx, repoID)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("[GitPush] Failed to get repository")
		return fmt.Errorf("repository not found: %w", err)
	}

	log.Debug().
		Str("repo_id", repoID).
		Str("external_url", gitRepo.ExternalURL).
		Str("external_type", string(gitRepo.ExternalType)).
		Bool("is_external", gitRepo.IsExternal).
		Str("oauth_connection_id", gitRepo.OAuthConnectionID).
		Dur("elapsed", time.Since(startTime)).
		Msg("[GitPush] Repository loaded")

	if !gitRepo.IsExternal {
		return fmt.Errorf("repository is not external, cannot push to remote")
	}

	if gitRepo.ExternalURL == "" {
		return fmt.Errorf("external URL is not configured")
	}

	if branchName == "" {
		return fmt.Errorf("branch name is required")
	}

	// Get auth config - use context-aware version to support OAuth refresh
	log.Debug().
		Str("repo_id", repoID).
		Str("oauth_connection_id", gitRepo.OAuthConnectionID).
		Msg("[GitPush] Getting auth config")

	auth := s.GetAuthConfigWithContext(ctx, gitRepo)

	// Log auth type for debugging (without exposing token)
	authType := "none"
	if auth != nil {
		if basicAuth, ok := auth.(*http.BasicAuth); ok {
			authType = fmt.Sprintf("basic:%s", basicAuth.Username)
			// Check if token looks valid (not empty, not obviously expired)
			if basicAuth.Password == "" {
				log.Warn().
					Str("repo_id", repoID).
					Str("auth_type", authType).
					Msg("[GitPush] WARNING: Auth password/token is empty!")
			} else {
				log.Debug().
					Str("repo_id", repoID).
					Str("auth_type", authType).
					Int("token_length", len(basicAuth.Password)).
					Msg("[GitPush] Auth configured")
			}
		} else {
			authType = fmt.Sprintf("%T", auth)
			log.Debug().
				Str("repo_id", repoID).
				Str("auth_type", authType).
				Msg("[GitPush] Auth configured (non-basic)")
		}
	} else {
		log.Warn().
			Str("repo_id", repoID).
			Msg("[GitPush] WARNING: No auth configured for push!")
	}

	log.Debug().
		Str("repo_id", repoID).
		Str("local_path", gitRepo.LocalPath).
		Str("branch", branchName).
		Dur("elapsed", time.Since(startTime)).
		Msg("[GitPush] Creating working copy for push")

	wc, err := s.getWorkingCopyForExternalPush(gitRepo.LocalPath, gitRepo.ExternalURL, branchName)
	if err != nil {
		log.Error().
			Err(err).
			Str("repo_id", repoID).
			Str("local_path", gitRepo.LocalPath).
			Dur("elapsed", time.Since(startTime)).
			Msg("[GitPush] Failed to create working copy")
		return fmt.Errorf("failed to create working copy: %w", err)
	}
	defer wc.Cleanup()

	log.Info().
		Str("repo_id", gitRepo.ID).
		Str("branch", branchName).
		Str("external_url", gitRepo.ExternalURL).
		Str("auth_type", authType).
		Bool("force", force).
		Dur("elapsed", time.Since(startTime)).
		Msg("[GitPush] Starting git push to external repository")

	pushStartTime := time.Now()
	err = wc.pushToExternal(ctx, branchName, force, auth)
	pushDuration := time.Since(pushStartTime)

	if err != nil {
		log.Error().
			Err(err).
			Str("repo_id", gitRepo.ID).
			Str("branch", branchName).
			Str("external_url", gitRepo.ExternalURL).
			Str("auth_type", authType).
			Dur("push_duration", pushDuration).
			Dur("total_elapsed", time.Since(startTime)).
			Msg("[GitPush] FAILED to push to external repository")
		return fmt.Errorf("failed to push to external repository: %w", err)
	}

	log.Info().
		Str("repo_id", gitRepo.ID).
		Str("branch", branchName).
		Dur("push_duration", pushDuration).
		Dur("total_elapsed", time.Since(startTime)).
		Msg("[GitPush] Successfully pushed local branch to external repository")

	return nil
}

// pushToExternal pushes changes to the "external" remote with optional force and auth
func (wc *WorkingCopy) pushToExternal(ctx context.Context, branch string, force bool, auth transport.AuthMethod) error {
	refSpec := config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", branch, branch))
	if force {
		refSpec = config.RefSpec(fmt.Sprintf("+refs/heads/%s:refs/heads/%s", branch, branch))
	}

	log.Debug().
		Str("branch", branch).
		Str("refspec", string(refSpec)).
		Bool("force", force).
		Bool("has_auth", auth != nil).
		Msg("[GitPush] Preparing push options")

	opts := &git.PushOptions{
		RemoteName: "external",
		RefSpecs:   []config.RefSpec{refSpec},
	}

	if auth != nil {
		opts.Auth = auth
	}

	// Check if context is already cancelled before starting push
	select {
	case <-ctx.Done():
		log.Warn().
			Err(ctx.Err()).
			Str("branch", branch).
			Msg("[GitPush] Context cancelled before push started")
		return fmt.Errorf("context cancelled before push: %w", ctx.Err())
	default:
	}

	log.Debug().
		Str("branch", branch).
		Msg("[GitPush] Calling go-git Push() - this may block on network operations")

	pushStart := time.Now()
	err := wc.Repo.Push(opts)
	pushDuration := time.Since(pushStart)

	if err == git.NoErrAlreadyUpToDate {
		log.Debug().
			Str("branch", branch).
			Dur("duration", pushDuration).
			Msg("[GitPush] Push completed - already up to date")
		return nil
	}

	if err != nil {
		log.Error().
			Err(err).
			Str("branch", branch).
			Dur("duration", pushDuration).
			Str("error_type", fmt.Sprintf("%T", err)).
			Msg("[GitPush] Push failed")
		return fmt.Errorf("failed to push to external: %w", err)
	}

	log.Debug().
		Str("branch", branch).
		Dur("duration", pushDuration).
		Msg("[GitPush] Push completed successfully")

	return nil
}

// getWorkingCopyForExternalPush creates a temporary working copy from the local bare repo
// with the external URL added as a remote called "external".
// This is used for pushing local changes to an external repository.
// The caller MUST call Cleanup() when done to remove the temporary directory.
func (s *GitRepositoryService) getWorkingCopyForExternalPush(
	bareRepoPath string,
	externalURL string,
	branch string,
) (*WorkingCopy, error) {
	if branch == "" {
		return nil, fmt.Errorf("branch is required")
	}

	tempDir, err := os.MkdirTemp("", "helix-git-push-workdir-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	branchRef := plumbing.NewBranchReferenceName(branch)

	cloneOpts := &git.CloneOptions{
		URL:           bareRepoPath,
		ReferenceName: branchRef,
		SingleBranch:  true,
	}

	repo, err := git.PlainClone(tempDir, cloneOpts)
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to clone bare repo branch %s: %w", branch, err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to get worktree: %w", err)
	}

	_, err = repo.CreateRemote(&config.RemoteConfig{
		Name: "external",
		URLs: []string{externalURL},
	})
	if err != nil {
		os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to add external remote: %w", err)
	}

	return &WorkingCopy{
		TempDir:  tempDir,
		Repo:     repo,
		Worktree: worktree,
	}, nil
}
