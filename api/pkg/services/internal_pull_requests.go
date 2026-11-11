package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"code.gitea.io/gitea/modules/git/gitcmd"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/agent/skill/github"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// Pull requests on Helix-hosted repositories. Helix serves a GitHub REST v3
// subset for its own repos (server/github_compat.go), and GitRepositoryService
// drives it with the same go-github client it uses for GitHub, so internal PRs
// take the GitHub code path end to end.

// ErrInternalPRNotMergeable maps to GitHub's 405 "Pull Request is not mergeable".
var ErrInternalPRNotMergeable = errors.New("Pull Request is not mergeable")

// ErrInternalPRMergeMethod maps to GitHub's 405 for an unsupported merge method.
var ErrInternalPRMergeMethod = errors.New("Merge method not allowed: Helix-hosted repositories support merge only")

// ErrInternalPRClosed maps to GitHub's 405 for merging a closed or merged PR.
var ErrInternalPRClosed = errors.New("Pull Request is not open")

// ErrInternalPRBaseMoved maps to GitHub's 409 when the base branch moved
// while the merge was being prepared, e.g. by a concurrent git push.
var ErrInternalPRBaseMoved = errors.New("Base branch was modified. Review and try the merge again.")

// InternalCompatDispatch serves a request against the GitHub-compatible API
// in-process, acting as user.
type InternalCompatDispatch func(ctx context.Context, req *http.Request, user types.User) *http.Response

// SetInternalGitHubCompat wires the in-process bridge to the compat router.
// baseURL is this Helix's public URL; go-github appends api/v3/.
func (s *GitRepositoryService) SetInternalGitHubCompat(baseURL string, dispatch InternalCompatDispatch) {
	s.compatBaseURL = strings.TrimSuffix(baseURL, "/") + "/api/github/"
	s.compatDispatch = dispatch
}

// InternalCompatOwner is the {owner} segment addressing a Helix-hosted repo in
// the compat API: the organization name, or "u-<owner id>" for personal repos.
func InternalCompatOwner(ctx context.Context, st store.Store, repo *types.GitRepository) (string, error) {
	if repo.OrganizationID != "" {
		org, err := st.GetOrganization(ctx, &store.GetOrganizationQuery{ID: repo.OrganizationID})
		if err != nil {
			return "", fmt.Errorf("lookup organization %s: %w", repo.OrganizationID, err)
		}
		return org.Name, nil
	}
	if repo.OwnerID == "" {
		return "", fmt.Errorf("repository %s has neither organization nor owner", repo.ID)
	}
	return "u-" + repo.OwnerID, nil
}

// internalGitHubClient returns a go-github client for a Helix-hosted repo that
// talks to the compat API in-process as userID, or as the repository owner for
// automated callers (the PR poller).
func (s *GitRepositoryService) internalGitHubClient(ctx context.Context, repo *types.GitRepository, userID string) (*github.Client, string, string, error) {
	if s.compatDispatch == nil {
		return nil, "", "", fmt.Errorf("internal pull requests are not wired (SetInternalGitHubCompat not called)")
	}
	actingID := userID
	if actingID == "" {
		actingID = repo.OwnerID
	}
	if actingID == "" {
		return nil, "", "", fmt.Errorf("repository %s has no owner to act as", repo.ID)
	}
	user, err := s.store.GetUser(ctx, &store.GetUserQuery{ID: actingID})
	if err != nil {
		return nil, "", "", fmt.Errorf("lookup user %s: %w", actingID, err)
	}
	owner, err := InternalCompatOwner(ctx, s.store, repo)
	if err != nil {
		return nil, "", "", err
	}
	transport := &internalCompatRoundTripper{dispatch: s.compatDispatch, user: *user}
	client := github.NewClientWithPATBaseURLAndHTTPClient("helix-internal", s.compatBaseURL, &http.Client{Transport: transport})
	return client, owner, repo.Name, nil
}

type internalCompatRoundTripper struct {
	dispatch InternalCompatDispatch
	user     types.User
}

func (t *internalCompatRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp := t.dispatch(req.Context(), req, t.user)
	if resp == nil {
		return nil, fmt.Errorf("internal GitHub-compat dispatch returned no response")
	}
	resp.Request = req
	return resp, nil
}

// RefreshInternalPR brings an open PR up to date with the bare repository:
// live head/base SHAs, and the merged state when the head has already landed
// on the base by another route (task Accept, a direct push). Returns the
// mergeability verdict for open PRs when withMergeability is set.
func (s *GitHTTPServer) RefreshInternalPR(ctx context.Context, repo *types.GitRepository, pr *types.GitPullRequest, withMergeability bool) (*bool, error) {
	if pr.State != types.GitPullRequestStateOpen {
		return nil, nil
	}
	head, err := GetBranchCommitID(ctx, repo.LocalPath, pr.HeadBranch)
	if err != nil {
		// Head branch deleted: nothing live to show.
		return nil, nil
	}
	base, err := GetBranchCommitID(ctx, repo.LocalPath, pr.BaseBranch)
	if err != nil {
		// Base branch deleted: keep the stored SHAs, as for a deleted head.
		return nil, nil
	}
	pr.HeadSHA = head

	landed, err := isAncestor(ctx, repo.LocalPath, head, base)
	if err != nil {
		return nil, err
	}
	if landed {
		if err := s.recordLandedPR(ctx, repo, pr, head); err != nil {
			return nil, err
		}
		return nil, nil
	}
	pr.BaseSHA = base

	if !withMergeability {
		return nil, nil
	}
	clean, _, err := mergeTree(ctx, repo.LocalPath, base, head)
	if err != nil {
		return nil, err
	}
	return &clean, nil
}

// recordLandedPR marks an open PR merged because its head reached the base
// by another route. It runs under the repository lock and re-reads the row,
// so it cannot overwrite a merge or close that finished meanwhile.
func (s *GitHTTPServer) recordLandedPR(ctx context.Context, repo *types.GitRepository, pr *types.GitPullRequest, head string) error {
	return s.gitRepoService.WithRepoLock(repo.ID, func() error {
		current, err := s.store.GetGitPullRequest(ctx, repo.ID, pr.Number)
		if err != nil {
			return err
		}
		if current.State == types.GitPullRequestStateOpen {
			now := time.Now()
			current.State = types.GitPullRequestStateClosed
			current.Merged = true
			current.MergedAt = &now
			current.ClosedAt = &now
			current.MergeCommit = head
			current.HeadSHA = head
			if err := s.store.UpdateGitPullRequest(ctx, current); err != nil {
				return fmt.Errorf("record landed PR %s: %w", current, err)
			}
			log.Info().Str("repo_id", repo.ID).Str("pr", current.String()).Msg("internal pull request head landed on base; marked merged")
		}
		*pr = *current
		return nil
	})
}

// SettleInternalPRSHAs pins the live head/base SHAs on a PR about to be
// closed, so its diff stays viewable after the branches move on.
func (s *GitHTTPServer) SettleInternalPRSHAs(ctx context.Context, repo *types.GitRepository, pr *types.GitPullRequest) {
	if head, err := GetBranchCommitID(ctx, repo.LocalPath, pr.HeadBranch); err == nil {
		pr.HeadSHA = head
	}
	if base, err := GetBranchCommitID(ctx, repo.LocalPath, pr.BaseBranch); err == nil {
		pr.BaseSHA = base
	}
}

func isAncestor(ctx context.Context, repoPath, ancestor, descendant string) (bool, error) {
	_, stderr, err := gitcmd.NewCommand("merge-base", "--is-ancestor").
		AddDynamicArguments(ancestor, descendant).
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
	if err == nil {
		return true, nil
	}
	if gitcmd.IsErrorExitCode(err, 1) {
		return false, nil
	}
	return false, fmt.Errorf("merge-base --is-ancestor: %w - %s", err, stderr)
}

// mergeTree rehearses a three-way merge in the bare repo. Clean merges return
// the resulting tree; conflicting ones return clean=false and no error.
func mergeTree(ctx context.Context, repoPath, base, head string) (clean bool, tree string, err error) {
	stdout, stderr, err := gitcmd.NewCommand("merge-tree", "--write-tree", "--no-messages").
		AddDynamicArguments(base, head).
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
	if err == nil {
		return true, strings.TrimSpace(strings.SplitN(stdout, "\n", 2)[0]), nil
	}
	if gitcmd.IsErrorExitCode(err, 1) {
		return false, "", nil
	}
	return false, "", fmt.Errorf("merge-tree: %w - %s", err, stderr)
}

// InternalPullRequestDiff is the unified diff of base...head.
func (s *GitHTTPServer) InternalPullRequestDiff(ctx context.Context, repo *types.GitRepository, pr *types.GitPullRequest) (string, error) {
	stdout, stderr, err := gitcmd.NewCommand("-c", "core.quotePath=false", "diff", "--no-color", "--find-renames").
		AddDynamicArguments(pr.BaseSHA+"..."+pr.HeadSHA).
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repo.LocalPath})
	if err != nil {
		return "", fmt.Errorf("diff %s...%s: %w - %s", pr.BaseSHA, pr.HeadSHA, err, stderr)
	}
	return stdout, nil
}

// InternalPRFileChanges lists the changed files of base...head in the shape of
// GitHub's pull request files endpoint. numstat, name-status and the patch come
// from the same diff options, so their entries are in the same order.
func (s *GitHTTPServer) InternalPRFileChanges(ctx context.Context, repo *types.GitRepository, pr *types.GitPullRequest) ([]*types.InternalPRFile, error) {
	rng := pr.BaseSHA + "..." + pr.HeadSHA
	run := func(args ...string) (string, error) {
		stdout, stderr, err := gitcmd.NewCommand("-c", "core.quotePath=false", "diff", "--no-color", "--find-renames").
			AddArguments(gitcmd.ToTrustedCmdArgs(args)...).
			AddDynamicArguments(rng).
			RunStdString(ctx, &gitcmd.RunOpts{Dir: repo.LocalPath})
		if err != nil {
			return "", fmt.Errorf("diff %v %s: %w - %s", args, rng, err, stderr)
		}
		return stdout, nil
	}
	numstat, err := run("--numstat", "-z")
	if err != nil {
		return nil, err
	}
	nameStatus, err := run("--name-status", "-z")
	if err != nil {
		return nil, err
	}
	patch, err := run()
	if err != nil {
		return nil, err
	}
	return parseInternalPRFiles(numstat, nameStatus, patch)
}

func parseInternalPRFiles(numstat, nameStatus, patch string) ([]*types.InternalPRFile, error) {
	var files []*types.InternalPRFile

	// name-status -z: STATUS\0path\0, or Rnnn\0old\0new\0 for renames/copies.
	fields := strings.Split(strings.TrimSuffix(nameStatus, "\x00"), "\x00")
	for i := 0; i < len(fields) && fields[i] != ""; {
		status := fields[i]
		f := &types.InternalPRFile{}
		switch status[0] {
		case 'R', 'C':
			if i+2 >= len(fields) {
				return nil, fmt.Errorf("truncated name-status entry %q", status)
			}
			f.PreviousFilename, f.Filename = fields[i+1], fields[i+2]
			f.Status = "renamed"
			if status[0] == 'C' {
				f.Status = "copied"
			}
			i += 3
		default:
			if i+1 >= len(fields) {
				return nil, fmt.Errorf("truncated name-status entry %q", status)
			}
			f.Filename = fields[i+1]
			switch status[0] {
			case 'A':
				f.Status = "added"
			case 'D':
				f.Status = "removed"
			default:
				f.Status = "modified"
			}
			i += 2
		}
		files = append(files, f)
	}

	// numstat -z: add\tdel\tpath\0, or add\tdel\t\0old\0new\0 for renames.
	entries := strings.Split(strings.TrimSuffix(numstat, "\x00"), "\x00")
	idx := 0
	for i := 0; i < len(entries) && entries[i] != ""; i++ {
		parts := strings.SplitN(entries[i], "\t", 3)
		if len(parts) != 3 {
			return nil, fmt.Errorf("malformed numstat entry %q", entries[i])
		}
		if parts[2] == "" {
			i += 2 // rename: old and new paths follow
		}
		if idx >= len(files) {
			return nil, fmt.Errorf("numstat has more entries than name-status")
		}
		// Binary files report "-".
		files[idx].Additions, _ = strconv.Atoi(parts[0])
		files[idx].Deletions, _ = strconv.Atoi(parts[1])
		files[idx].Changes = files[idx].Additions + files[idx].Deletions
		idx++
	}

	sections := splitPatchByFile(patch)
	if len(sections) == len(files) {
		for i, section := range sections {
			files[i].Patch = section
		}
	}
	return files, nil
}

// splitPatchByFile returns each file's hunks (from the first "@@"), as in
// GitHub's "patch" field.
func splitPatchByFile(patch string) []string {
	if patch == "" {
		return nil
	}
	var out []string
	for _, chunk := range strings.Split("\n"+patch, "\ndiff --git ")[1:] {
		hunks := ""
		if at := strings.Index(chunk, "\n@@"); at >= 0 {
			hunks = strings.TrimSuffix(chunk[at+1:], "\n")
		}
		out = append(out, hunks)
	}
	return out
}

// MergeInternalPullRequest merges an open internal PR under the repository
// lock: fast-forward when the base is an ancestor of the head, otherwise a
// merge commit built in the bare repo. Merging into the default branch fires
// the deploy hook, since a server-side ref update has no receive-pack.
func (s *GitHTTPServer) MergeInternalPullRequest(ctx context.Context, repo *types.GitRepository, number int, mergeMethod, commitTitle, commitMessage, userID string) (*types.GitPullRequest, error) {
	switch mergeMethod {
	case "", "merge":
	default:
		return nil, ErrInternalPRMergeMethod
	}

	var merged *types.GitPullRequest
	err := s.gitRepoService.WithRepoLock(repo.ID, func() error {
		pr, err := s.store.GetGitPullRequest(ctx, repo.ID, number)
		if err != nil {
			return err
		}
		if pr.State != types.GitPullRequestStateOpen {
			return ErrInternalPRClosed
		}
		head, err := GetBranchCommitID(ctx, repo.LocalPath, pr.HeadBranch)
		if err != nil {
			return ErrInternalPRNotMergeable
		}
		base, err := GetBranchCommitID(ctx, repo.LocalPath, pr.BaseBranch)
		if err != nil {
			return fmt.Errorf("base branch %q: %w", pr.BaseBranch, err)
		}

		mergeSHA := head
		landed, err := isAncestor(ctx, repo.LocalPath, head, base)
		if err != nil {
			return err
		}
		fastForward, err := isAncestor(ctx, repo.LocalPath, base, head)
		if err != nil {
			return err
		}
		switch {
		case landed:
			mergeSHA = head
		case fastForward:
			if err := moveBaseBranch(ctx, repo.LocalPath, pr.BaseBranch, head, base); err != nil {
				return err
			}
		default:
			clean, tree, err := mergeTree(ctx, repo.LocalPath, base, head)
			if err != nil {
				return err
			}
			if !clean {
				return ErrInternalPRNotMergeable
			}
			title := strings.TrimSpace(commitTitle)
			if title == "" {
				title = fmt.Sprintf("Merge pull request #%d from %s", pr.Number, pr.HeadBranch)
			}
			body := strings.TrimSpace(commitMessage)
			if body == "" {
				body = strings.TrimSpace(pr.Title)
			}
			name, email := s.mergeCommitIdentity(ctx, userID)
			mergeSHA, err = commitTree(ctx, repo.LocalPath, tree, []string{base, head}, name, email, title+"\n\n"+body)
			if err != nil {
				return err
			}
			if err := moveBaseBranch(ctx, repo.LocalPath, pr.BaseBranch, mergeSHA, base); err != nil {
				return err
			}
		}

		now := time.Now()
		pr.State = types.GitPullRequestStateClosed
		pr.Merged = true
		pr.MergedAt = &now
		pr.ClosedAt = &now
		pr.MergedBy = userID
		pr.MergeCommit = mergeSHA
		pr.HeadSHA = head
		pr.BaseSHA = base // pre-merge base keeps base...head the PR's own diff
		if err := s.store.UpdateGitPullRequest(ctx, pr); err != nil {
			return fmt.Errorf("record merge of %s: %w", pr, err)
		}
		merged = pr
		return nil
	})
	if err != nil {
		return nil, err
	}

	s.NotifyDefaultBranchMerged(repo, merged.BaseBranch, userID)
	log.Info().Str("repo_id", repo.ID).Str("pr", merged.String()).Str("merge_commit", merged.MergeCommit).Msg("internal pull request merged")
	return merged, nil
}

// moveBaseBranch points the base branch at newSHA only if it still points at
// the base the merge was computed from. A push that landed in between (git
// receive-pack does not take the repository lock) fails the merge instead of
// being silently dropped from the branch.
func moveBaseBranch(ctx context.Context, repoPath, branch, newSHA, expectedSHA string) error {
	if err := UpdateBranchRefFrom(ctx, repoPath, branch, newSHA, expectedSHA); err != nil {
		if now, _ := GetBranchCommitID(ctx, repoPath, branch); now != expectedSHA {
			return ErrInternalPRBaseMoved
		}
		return fmt.Errorf("update %s: %w", branch, err)
	}
	return nil
}

func commitTree(ctx context.Context, repoPath, tree string, parents []string, name, email, message string) (string, error) {
	cmd := gitcmd.NewCommand("commit-tree").AddDynamicArguments(tree)
	for _, p := range parents {
		cmd.AddArguments("-p").AddDynamicArguments(p)
	}
	stdout, stderr, err := cmd.RunStdBytes(ctx, &gitcmd.RunOpts{
		Dir: repoPath,
		Env: []string{
			"GIT_AUTHOR_NAME=" + name, "GIT_AUTHOR_EMAIL=" + email,
			"GIT_COMMITTER_NAME=" + name, "GIT_COMMITTER_EMAIL=" + email,
		},
		Stdin: bytes.NewReader([]byte(message)),
	})
	if err != nil {
		return "", fmt.Errorf("commit-tree: %w - %s", err, stderr)
	}
	return strings.TrimSpace(string(stdout)), nil
}

func (s *GitHTTPServer) mergeCommitIdentity(ctx context.Context, userID string) (string, string) {
	if userID != "" {
		if u, err := s.store.GetUser(ctx, &store.GetUserQuery{ID: userID}); err == nil && u.Email != "" {
			name := strings.TrimSpace(u.FullName)
			if name == "" {
				name = u.Email
			}
			return name, u.Email
		}
	}
	return "Helix", "helix@helix.ml"
}
