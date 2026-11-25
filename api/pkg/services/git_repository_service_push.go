package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"code.gitea.io/gitea/modules/git/gitcmd"
	"github.com/helixml/helix/api/pkg/types"
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

	pushStartTime := time.Now()
	err = s.trackUpstreamSync(ctx, gitRepo.ID, func() error {
		remote, err := s.upstreamRemoteForRepo(ctx, gitRepo, actingUserID)
		if err != nil {
			return err
		}
		authType := "none"
		if remote.Password != "" {
			authType = fmt.Sprintf("basic:%s", remote.Username)
			log.Trace().
				Str("repo_id", repoID).
				Str("auth_type", authType).
				Int("token_length", len(remote.Password)).
				Msg("[GitPush] Auth configured")
		} else {
			log.Warn().
				Str("repo_id", repoID).
				Msg("[GitPush] WARNING: No auth configured for push!")
		}

		log.Info().
			Str("repo_id", gitRepo.ID).
			Str("branch", branchName).
			Str("external_url", RedactURLCredentials(remote.URL)).
			Str("auth_type", authType).
			Bool("force", force).
			Dur("elapsed", time.Since(startTime)).
			Msg("[GitPush] Starting native git push to external repository")

		if err := s.seedEmptyUpstream(ctx, gitRepo, remote, branchName); err != nil {
			return err
		}

		err = pushBranchNative(ctx, gitRepo.LocalPath, remote, branchName, force)
		if err != nil {
			log.Error().
				Err(err).
				Str("repo_id", gitRepo.ID).
				Str("branch", branchName).
				Str("external_url", RedactURLCredentials(remote.URL)).
				Str("auth_type", authType).
				Dur("push_duration", time.Since(pushStartTime)).
				Dur("total_elapsed", time.Since(startTime)).
				Msg("[GitPush] FAILED to push to external repository")
			return fmt.Errorf("failed to push to external repository: %w", err)
		}
		return nil
	})
	pushDuration := time.Since(pushStartTime)

	if err != nil {
		return err
	}

	log.Info().
		Str("repo_id", gitRepo.ID).
		Str("branch", branchName).
		Dur("push_duration", pushDuration).
		Dur("total_elapsed", time.Since(startTime)).
		Msg("[GitPush] Successfully pushed local branch to external repository")

	return nil
}

// seedEmptyUpstream makes the default branch the first branch an empty
// upstream receives. Providers such as GitHub make the first branch pushed to
// an empty repository its default branch, so pushing helix-specs or a feature
// branch first would leave the repository defaulting to that branch. When the
// default branch does not exist locally yet it is created with a README, as
// the desktop workspace setup does for an empty clone.
func (s *GitRepositoryService) seedEmptyUpstream(ctx context.Context, gitRepo *types.GitRepository, remote upstreamRemote, branchName string) error {
	defaultBranch := gitRepo.DefaultBranch
	if defaultBranch == "" {
		defaultBranch = "main"
	}
	if branchName == defaultBranch {
		return nil
	}
	if _, known := s.upstreamHasBranches.Load(gitRepo.ID); known {
		return nil
	}

	stdout, stderr, err := runUpstreamGit(ctx, "", time.Minute, remote.env(), "ls-remote", "--heads", "--", remote.URL)
	if err != nil {
		return upstreamGitError("list upstream branches", err, stderr)
	}
	if len(parseLsRemoteBranches(stdout)) > 0 {
		s.upstreamHasBranches.Store(gitRepo.ID, true)
		return nil
	}

	head, err := branchHead(ctx, gitRepo.LocalPath, defaultBranch)
	if err != nil {
		return err
	}
	if head == "" {
		name, email, err := branchTipAuthor(ctx, gitRepo.LocalPath, branchName)
		if err != nil {
			return err
		}
		readme := fmt.Sprintf("# %s\n\nThis repository was initialized by Helix.\n", gitRepo.Name)
		if _, _, err := CommitFileToBareBranch(ctx, gitRepo.LocalPath, defaultBranch, "README.md", []byte(readme), name, email, "Initial commit"); err != nil {
			return fmt.Errorf("failed to create initial %s commit: %w", defaultBranch, err)
		}
	}

	log.Info().
		Str("repo_id", gitRepo.ID).
		Str("default_branch", defaultBranch).
		Str("branch", branchName).
		Msg("[GitPush] Upstream is empty; pushing the default branch first")
	if err := pushBranchNative(ctx, gitRepo.LocalPath, remote, defaultBranch, false); err != nil {
		return fmt.Errorf("failed to push %s to the empty external repository: %w", defaultBranch, err)
	}
	s.upstreamHasBranches.Store(gitRepo.ID, true)
	return nil
}

// branchTipAuthor returns the author of the commit branch points at.
func branchTipAuthor(ctx context.Context, repoPath, branch string) (string, string, error) {
	stdout, stderr, err := gitcmd.NewCommand("log", "-1", "--format=%an%x00%ae").
		AddDynamicArguments("refs/heads/"+branch).
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
	if err != nil {
		return "", "", fmt.Errorf("failed to read author of %s: %w - %s", branch, err, stderr)
	}
	name, email, ok := strings.Cut(strings.TrimSpace(stdout), "\x00")
	if !ok || name == "" || email == "" {
		return "", "", fmt.Errorf("branch %s has no commit author", branch)
	}
	return name, email, nil
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
