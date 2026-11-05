package services

import (
	"context"
	"fmt"
	"time"

	"code.gitea.io/gitea/modules/git/gitcmd"
	"github.com/rs/zerolog/log"
)

// PushBranchToRemote pushes a local branch from the bare repository to an external remote.
// This is used when you have local commits in the Helix bare repo that need to be pushed
// to an external repository (GitHub, Azure DevOps, etc.).
//
// Uses gitea/git module for native git operations.
func (s *GitRepositoryService) PushBranchToRemote(ctx context.Context, repoID, branchName string, force bool, userID ...string) error {
	var actingUserID string
	if len(userID) > 0 {
		actingUserID = userID[0]
	}
	startTime := time.Now()

	log.Trace().
		Str("repo_id", repoID).
		Str("branch", branchName).
		Bool("force", force).
		Msg("[GitPush] Starting push operation using native git")

	gitRepo, err := s.GetRepository(ctx, repoID)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("[GitPush] Failed to get repository")
		return fmt.Errorf("repository not found: %w", err)
	}

	log.Trace().
		Str("repo_id", repoID).
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
	if actingUserID != "" {
		if err := s.ValidateUserOAuth(ctx, gitRepo, actingUserID); err != nil {
			return err
		}
	}

	// Get credentials for the external URL using the acting user's OAuth.
	username, password := s.getCredentialsForRepo(ctx, gitRepo, actingUserID)
	authType := "none"
	if password != "" {
		authType = fmt.Sprintf("basic:%s", username)
		log.Trace().
			Str("repo_id", repoID).
			Str("auth_type", authType).
			Int("token_length", len(password)).
			Msg("[GitPush] Auth configured")
	} else {
		log.Warn().
			Str("repo_id", repoID).
			Msg("[GitPush] WARNING: No auth configured for push!")
	}

	remote := newUpstreamRemote(gitRepo.ExternalURL, username, password)

	log.Info().
		Str("repo_id", gitRepo.ID).
		Str("branch", branchName).
		Str("external_url", remote.URL).
		Str("auth_type", authType).
		Bool("force", force).
		Dur("elapsed", time.Since(startTime)).
		Msg("[GitPush] Starting native git push to external repository")

	pushStartTime := time.Now()
	err = s.trackUpstreamSync(ctx, gitRepo.ID, func() error {
		return pushBranchNative(ctx, gitRepo.LocalPath, remote, branchName, force)
	})
	pushDuration := time.Since(pushStartTime)

	if err != nil {
		log.Error().
			Err(err).
			Str("repo_id", gitRepo.ID).
			Str("branch", branchName).
			Str("external_url", remote.URL).
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

// pushBranchNative pushes a branch from the bare repo to the upstream remote.
func pushBranchNative(ctx context.Context, repoPath string, remote upstreamRemote, branch string, force bool) error {
	refspec := fmt.Sprintf("refs/heads/%s:refs/heads/%s", branch, branch)
	args := []string{"push", "--no-progress"}
	if force {
		refspec = "+" + refspec
		args = append(args, "-f")
	}
	args = append(args, "--", remote.URL, refspec)

	_, stderr, err := runUpstreamGit(ctx, repoPath, upstreamGitDefaultTimeout, remote.env(), args...)
	if err != nil {
		return upstreamGitError("push", err, stderr)
	}
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
