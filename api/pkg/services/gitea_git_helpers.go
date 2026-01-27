package services

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	giteagit "code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/git/gitcmd"
)

// FetchOptions contains options for git fetch operations
type FetchOptions struct {
	Remote    string        // Remote name (e.g., "origin")
	Branch    string        // Specific branch to fetch (empty for all)
	Force     bool          // Force fetch (overwrite local refs)
	Prune     bool          // Remove remote-tracking refs that no longer exist
	Depth     int           // Shallow fetch with depth limit (0 for full)
	Env       []string      // Environment variables (for auth)
	Timeout   time.Duration // Command timeout
	RefSpecs  []string      // Explicit refspecs (optional)
}

// Fetch fetches from a remote repository using native git
func Fetch(ctx context.Context, repoPath string, opts FetchOptions) error {
	cmd := gitcmd.NewCommand("fetch")

	if opts.Force {
		cmd.AddArguments("-f")
	}
	if opts.Prune {
		cmd.AddArguments("--prune")
	}
	if opts.Depth > 0 {
		cmd.AddArguments("--depth").AddDynamicArguments(fmt.Sprintf("%d", opts.Depth))
	}

	// Add remote name
	if opts.Remote == "" {
		opts.Remote = "origin"
	}
	cmd.AddDynamicArguments(opts.Remote)

	// Add branch or refspecs
	if len(opts.RefSpecs) > 0 {
		for _, refspec := range opts.RefSpecs {
			cmd.AddDynamicArguments(refspec)
		}
	} else if opts.Branch != "" {
		cmd.AddDynamicArguments(opts.Branch)
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}

	_, stderr, err := cmd.RunStdString(ctx, &gitcmd.RunOpts{
		Dir:     repoPath,
		Env:     opts.Env,
		Timeout: timeout,
	})
	if err != nil {
		// Check for "already up to date" which isn't an error
		if strings.Contains(stderr, "Already up to date") || strings.Contains(stderr, "already up-to-date") {
			return nil
		}
		return fmt.Errorf("fetch failed: %w - %s", err, stderr)
	}
	return nil
}

// BuildAuthenticatedURL embeds credentials into a git URL for HTTP(S) remotes
// For GitHub: uses x-access-token as username
// For GitLab/Azure DevOps: uses oauth2 as username
// Returns the original URL unchanged if it's not HTTP(S) or if no password provided
func BuildAuthenticatedURL(remoteURL, username, password string) (string, error) {
	if password == "" {
		return remoteURL, nil
	}

	u, err := url.Parse(remoteURL)
	if err != nil {
		return remoteURL, err
	}

	// Only modify HTTP(S) URLs - if scheme is empty or not http(s), return unchanged
	// This prevents malformed URLs like "//user:pass@hostname" when scheme is missing
	if u.Scheme != "http" && u.Scheme != "https" {
		return remoteURL, nil
	}

	// Verify we have a valid host (url.Parse succeeds but sets Host="" for schemeless URLs)
	if u.Host == "" {
		return remoteURL, fmt.Errorf("URL has no host: %s", remoteURL)
	}

	// Set user info (username:password)
	if username == "" {
		username = "git" // Default username
	}
	u.User = url.UserPassword(username, password)

	return u.String(), nil
}

// SetRemoteURL updates the URL for a remote in a git repository
func SetRemoteURL(ctx context.Context, repoPath, remoteName, remoteURL string) error {
	_, stderr, err := gitcmd.NewCommand("remote", "set-url").
		AddDynamicArguments(remoteName, remoteURL).
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
	if err != nil {
		return fmt.Errorf("failed to set remote URL: %w - %s", err, stderr)
	}
	return nil
}

// AddRemote adds a new remote to a git repository
func AddRemote(ctx context.Context, repoPath, remoteName, remoteURL string) error {
	_, stderr, err := gitcmd.NewCommand("remote", "add").
		AddDynamicArguments(remoteName, remoteURL).
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
	if err != nil {
		if strings.Contains(stderr, "already exists") {
			// Remote already exists, update URL instead
			return SetRemoteURL(ctx, repoPath, remoteName, remoteURL)
		}
		return fmt.Errorf("failed to add remote: %w - %s", err, stderr)
	}
	return nil
}

// SetHEAD sets the HEAD reference to point to a branch
func SetHEAD(ctx context.Context, repoPath, branchName string) error {
	_, stderr, err := gitcmd.NewCommand("symbolic-ref", "HEAD").
		AddDynamicArguments("refs/heads/"+branchName).
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
	if err != nil {
		return fmt.Errorf("failed to set HEAD: %w - %s", err, stderr)
	}
	return nil
}

// DeleteBranch deletes a branch from a repository
func DeleteBranch(ctx context.Context, repoPath, branchName string) error {
	// Use gitea's high-level API
	repo, err := giteagit.OpenRepository(ctx, repoPath)
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}
	defer repo.Close()

	err = repo.DeleteBranch(branchName, giteagit.DeleteBranchOptions{Force: true})
	if err != nil {
		// Ignore "not found" errors
		if strings.Contains(err.Error(), "not found") {
			return nil
		}
		return fmt.Errorf("failed to delete branch: %w", err)
	}
	return nil
}

// GetHEADBranch returns the branch name that HEAD points to
func GetHEADBranch(ctx context.Context, repoPath string) (string, error) {
	stdout, stderr, err := gitcmd.NewCommand("symbolic-ref", "--short", "HEAD").
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD branch: %w - %s", err, stderr)
	}
	return strings.TrimSpace(stdout), nil
}

// GetBranchCommitID returns the commit ID for a branch
func GetBranchCommitID(ctx context.Context, repoPath, branchName string) (string, error) {
	// Use gitea's high-level API
	repo, err := giteagit.OpenRepository(ctx, repoPath)
	if err != nil {
		return "", fmt.Errorf("failed to open repository: %w", err)
	}
	defer repo.Close()

	commitID, err := repo.GetBranchCommitID(branchName)
	if err != nil {
		return "", fmt.Errorf("failed to get branch commit: %w", err)
	}
	return commitID, nil
}

// ListBranches returns all branch names in a repository
func ListBranches(ctx context.Context, repoPath string) ([]string, error) {
	// Use gitea's high-level API
	repo, err := giteagit.OpenRepository(ctx, repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open repository: %w", err)
	}
	defer repo.Close()

	branches, _, err := repo.GetBranchNames(0, 0) // 0, 0 = no skip, no limit
	if err != nil {
		return nil, fmt.Errorf("failed to list branches: %w", err)
	}
	return branches, nil
}

// CreateBranchFromRef creates a new branch pointing to the same commit as the source ref
func CreateBranchFromRef(ctx context.Context, repoPath, newBranch, sourceRef string) error {
	// Use gitea's high-level API
	repo, err := giteagit.OpenRepository(ctx, repoPath)
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}
	defer repo.Close()

	err = repo.CreateBranch(newBranch, sourceRef)
	if err != nil {
		return fmt.Errorf("failed to create branch: %w", err)
	}
	return nil
}

// GetDivergence returns how many commits ahead/behind a branch is compared to another
func GetDivergence(ctx context.Context, repoPath, localBranch, remoteBranch string) (ahead, behind int, err error) {
	diverge, err := giteagit.GetDivergingCommits(ctx, repoPath, remoteBranch, localBranch)
	if err != nil {
		return 0, 0, err
	}
	return diverge.Ahead, diverge.Behind, nil
}

// CommitInfo represents basic commit information
type CommitInfo struct {
	Hash      string
	Author    string
	Email     string
	Timestamp time.Time
	Message   string
}

// GetCommitInfo returns information about a specific commit
func GetCommitInfo(ctx context.Context, repoPath, commitHash string) (*CommitInfo, error) {
	// Use gitea's high-level API to get commit info
	repo, err := giteagit.OpenRepository(ctx, repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open repository: %w", err)
	}
	defer repo.Close()

	commit, err := repo.GetCommit(commitHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit: %w", err)
	}

	return &CommitInfo{
		Hash:      commit.ID.String(),
		Author:    commit.Author.Name,
		Email:     commit.Author.Email,
		Timestamp: commit.Author.When,
		Message:   commit.Summary(),
	}, nil
}

// Note: For git operations, prefer using gitea's built-in wrappers:
// - giteagit.InitRepository() for git init
// - giteagit.AddChanges() for git add
// - giteagit.CommitChanges() for git commit
// - giteagit.Clone() for git clone
// - giteagit.Push() for git push

// GitCheckout checks out a branch (gitea doesn't provide this wrapper)
func GitCheckout(ctx context.Context, repoPath, branchName string, create bool) error {
	cmd := gitcmd.NewCommand("checkout")
	if create {
		cmd.AddArguments("-b")
	}
	cmd.AddDynamicArguments(branchName)

	_, stderr, err := cmd.RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
	if err != nil {
		return fmt.Errorf("failed to checkout: %w - %s", err, stderr)
	}
	return nil
}

// GitRenameBranch renames a branch (e.g., master -> main)
func GitRenameBranch(ctx context.Context, repoPath, oldName, newName string) error {
	// Use gitea's high-level API
	repo, err := giteagit.OpenRepository(ctx, repoPath)
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}
	defer repo.Close()

	err = repo.RenameBranch(oldName, newName)
	if err != nil {
		return fmt.Errorf("failed to rename branch: %w", err)
	}
	return nil
}

// ShortHash safely truncates a commit hash to 8 characters for display.
// Returns the full string if shorter than 8 characters, or "(empty)" if empty.
func ShortHash(hash string) string {
	if hash == "" {
		return "(empty)"
	}
	if len(hash) <= 8 {
		return hash
	}
	return hash[:8]
}
