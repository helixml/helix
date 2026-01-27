package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	giteagit "code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/git/gitcmd"
	"github.com/rs/zerolog/log"
)

// PushBranchToRemote pushes a local branch from the bare repository to an external remote.
// This is used when you have local commits in the Helix bare repo that need to be pushed
// to an external repository (GitHub, Azure DevOps, etc.).
//
// Uses native git for reliable network operations (avoids go-git deadlock issues).
func (s *GitRepositoryService) PushBranchToRemote(ctx context.Context, repoID, branchName string, force bool) error {
	startTime := time.Now()

	log.Debug().
		Str("repo_id", repoID).
		Str("branch", branchName).
		Bool("force", force).
		Msg("[GitPush] Starting push operation using native git")

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

	// Get credentials for the external URL
	username, password := s.getCredentialsForRepo(ctx, gitRepo)
	authType := "none"
	if password != "" {
		authType = fmt.Sprintf("basic:%s", username)
		log.Debug().
			Str("repo_id", repoID).
			Str("auth_type", authType).
			Int("token_length", len(password)).
			Msg("[GitPush] Auth configured")
	} else {
		log.Warn().
			Str("repo_id", repoID).
			Msg("[GitPush] WARNING: No auth configured for push!")
	}

	// Build authenticated URL for push
	pushURL := s.buildAuthenticatedCloneURLForRepo(ctx, gitRepo)

	log.Info().
		Str("repo_id", gitRepo.ID).
		Str("branch", branchName).
		Str("external_url", gitRepo.ExternalURL).
		Str("auth_type", authType).
		Bool("force", force).
		Dur("elapsed", time.Since(startTime)).
		Msg("[GitPush] Starting native git push to external repository")

	// Use native git push directly from bare repo
	// This avoids creating temp working copies and is more reliable
	pushStartTime := time.Now()
	err = pushBranchNative(ctx, gitRepo.LocalPath, pushURL, branchName, force)
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

// pushBranchNative pushes a branch to a remote URL using native git
func pushBranchNative(ctx context.Context, repoPath, remoteURL, branch string, force bool) error {
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
		Str("repo_path", repoPath).
		Bool("force", force).
		Msg("[GitPush] Calling native git push")

	// Build refspec - use + prefix for force push
	refspec := fmt.Sprintf("refs/heads/%s:refs/heads/%s", branch, branch)
	if force {
		refspec = "+" + refspec
	}

	// Use gitea's Push function which handles native git push
	err := giteagit.Push(ctx, repoPath, giteagit.PushOptions{
		Remote:  remoteURL, // Push directly to URL with embedded credentials
		Branch:  refspec,
		Force:   force,
		Timeout: 5 * time.Minute,
	})

	if err != nil {
		// Check for "already up to date" which isn't an error
		errStr := err.Error()
		if strings.Contains(errStr, "Everything up-to-date") ||
			strings.Contains(errStr, "up to date") {
			log.Debug().
				Str("branch", branch).
				Msg("[GitPush] Push completed - already up to date")
			return nil
		}
		return err
	}

	log.Debug().
		Str("branch", branch).
		Msg("[GitPush] Push completed successfully")

	return nil
}

// PushToLocalBareRepo pushes changes from a working copy to the local bare repository
// This is used after making changes in a temp working copy
func pushToLocalBareRepo(ctx context.Context, workingDir, bareRepoPath, branch string) error {
	// Use native git push from the working directory to the bare repo
	cmd := gitcmd.NewCommand("push")
	cmd.AddDynamicArguments(bareRepoPath, fmt.Sprintf("refs/heads/%s:refs/heads/%s", branch, branch))

	_, stderr, err := cmd.RunStdString(ctx, &gitcmd.RunOpts{
		Dir:     workingDir,
		Timeout: 2 * time.Minute,
	})
	if err != nil {
		return fmt.Errorf("failed to push to bare repo: %w - %s", err, stderr)
	}
	return nil
}
