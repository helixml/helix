package services

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	giteagit "code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/git/gitcmd"
	"github.com/rs/zerolog/log"
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
		// Handle empty repositories gracefully - if remote has no refs, that's not an error
		if strings.Contains(stderr, "couldn't find remote ref") {
			log.Debug().
				Str("repo_path", repoPath).
				Str("stderr", stderr).
				Msg("Remote repository appears to be empty (no refs found)")
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

// PreReceiveHookVersion is incremented when the hook logic changes.
// The hook script contains this version and will be updated if it differs.
const PreReceiveHookVersion = "4"

// preReceiveHookScript is the shell script that:
// 1. Protects helix-specs branch from force pushes
// 2. Enforces branch restrictions for agent API keys via HELIX_ALLOWED_BRANCHES env var
// It reads ref updates from stdin and rejects unauthorized pushes with GitHub-style error messages.
const preReceiveHookScript = `#!/bin/sh
# Helix pre-receive hook v` + PreReceiveHookVersion + `
# Protects helix-specs from force pushes and enforces agent branch restrictions.

ZERO="0000000000000000000000000000000000000000"

# HELIX_ALLOWED_BRANCHES is set by the git HTTP server for agent API keys.
# Format: comma-separated list of branch names, e.g., "helix-specs,feature/001062-my-task"
# If unset or empty, all branches are allowed (normal user behavior).
ALLOWED_BRANCHES="$HELIX_ALLOWED_BRANCHES"

# Helper function to check if a branch is in the allowed list
branch_allowed() {
    local branch="$1"
    local allowed="$2"

    # If no restrictions, allow all
    if [ -z "$allowed" ]; then
        return 0
    fi

    # Use case statement for exact matching (avoids subshell issues with pipes)
    # Add commas to both ends so we match whole branch names only
    case ",$allowed," in
        *",$branch,"*) return 0 ;;
    esac
    return 1
}

while read oldrev newrev refname; do
    # Extract branch name from ref (refs/heads/branch-name -> branch-name)
    branch="${refname#refs/heads/}"

    # Skip if not a branch ref (e.g., tags)
    case "$refname" in
        refs/heads/*) ;;
        *) continue ;;
    esac

    # Check 1: Force-push and deletion protection for helix-specs
    if [ "$branch" = "helix-specs" ]; then
        # Block deletion of helix-specs
        if [ "$newrev" = "$ZERO" ]; then
            echo "error: refusing to delete protected branch 'helix-specs'" >&2
            echo "hint: helix-specs contains design documents and cannot be deleted." >&2
            exit 1
        fi
        # Skip force-push check if this is a new branch (old is all zeros)
        if [ "$oldrev" != "$ZERO" ]; then
            # Check if old is ancestor of new (fast-forward)
            if ! git merge-base --is-ancestor "$oldrev" "$newrev" 2>/dev/null; then
                echo "error: refusing to force-push to protected branch 'helix-specs'" >&2
                echo "hint: helix-specs is a forward-only branch to protect design documents." >&2
                echo "hint: If you need to revert changes, create a new commit instead." >&2
                exit 1
            fi
        fi
    fi

    # Check 2: Branch restriction for agent API keys
    if [ -n "$ALLOWED_BRANCHES" ]; then
        if ! branch_allowed "$branch" "$ALLOWED_BRANCHES"; then
            echo "error: refusing to update refs/heads/$branch" >&2
            echo "hint: This push is restricted to: $ALLOWED_BRANCHES" >&2
            echo "hint: Push to your assigned feature branch instead." >&2
            exit 1
        fi
    fi
done
exit 0
`

// InstallPreReceiveHook installs the force-push protection hook in a bare repository.
// The hook prevents force pushes to the helix-specs branch.
// If the hook already exists with the current version, this is a no-op.
func InstallPreReceiveHook(repoPath string) error {
	hooksDir := filepath.Join(repoPath, "hooks")
	hookPath := filepath.Join(hooksDir, "pre-receive")

	// Check if hook exists and has current version
	if content, err := os.ReadFile(hookPath); err == nil {
		if strings.Contains(string(content), "Helix pre-receive hook v"+PreReceiveHookVersion) {
			return nil // Already up to date
		}
	}

	// Create hooks directory if needed
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		return fmt.Errorf("failed to create hooks directory: %w", err)
	}

	// Write hook script
	if err := os.WriteFile(hookPath, []byte(preReceiveHookScript), 0755); err != nil {
		return fmt.Errorf("failed to write pre-receive hook: %w", err)
	}

	return nil
}

// InstallPreReceiveHooksForAllRepos installs or updates pre-receive hooks in all
// bare repositories in the given directory. This should be called on API startup
// to ensure existing repos have the latest hook version.
func InstallPreReceiveHooksForAllRepos(reposDir string) error {
	entries, err := os.ReadDir(reposDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No repos dir yet
		}
		return fmt.Errorf("failed to read repos directory: %w", err)
	}

	var installedCount, updatedCount, errorCount int
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Bare repos typically end in .git or contain a HEAD file
		repoPath := filepath.Join(reposDir, entry.Name())
		headPath := filepath.Join(repoPath, "HEAD")
		if _, err := os.Stat(headPath); err != nil {
			continue // Not a git repo
		}

		hookPath := filepath.Join(repoPath, "hooks", "pre-receive")
		hadHook := false
		if content, err := os.ReadFile(hookPath); err == nil {
			hadHook = true
			if strings.Contains(string(content), "Helix pre-receive hook v"+PreReceiveHookVersion) {
				continue // Already up to date
			}
		}

		if err := InstallPreReceiveHook(repoPath); err != nil {
			errorCount++
			continue
		}

		if hadHook {
			updatedCount++
		} else {
			installedCount++
		}
	}

	if installedCount > 0 || updatedCount > 0 {
		// Log is imported in the file that calls this, we return counts instead
	}

	return nil
}
