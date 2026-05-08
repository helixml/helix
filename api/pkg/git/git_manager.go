package git

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"

	giteagit "code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/git/gitcmd"
	"github.com/jfrog/froggit-go/vcsclient"
	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/go-diff/diff"
)

type GitManager struct {
	remoteURL string
	token     string
}

func NewGitManager(remoteURL, token string) *GitManager {
	return &GitManager{
		remoteURL: remoteURL,
		token:     token,
	}
}

// CloneRepository clones a repository to the given path.
// Returns the path to the cloned repository (same as repoPath).
func (g *GitManager) CloneRepository(ctx context.Context, repoPath string) (string, error) {
	// Build authenticated URL
	authURL := g.buildAuthenticatedURL()

	err := giteagit.Clone(ctx, authURL, repoPath, giteagit.CloneRepoOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to clone repository: %w", err)
	}

	return repoPath, nil
}

// buildAuthenticatedURL embeds credentials into the remote URL
func (g *GitManager) buildAuthenticatedURL() string {
	if g.token == "" {
		return g.remoteURL
	}

	u, err := url.Parse(g.remoteURL)
	if err != nil {
		return g.remoteURL
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return g.remoteURL
	}

	u.User = url.UserPassword("helixml", g.token)
	return u.String()
}

// CheckoutCommit checks out a specific commit in the repository at repoPath.
func (g *GitManager) CheckoutCommit(ctx context.Context, repoPath string, commitID string) error {
	_, stderr, err := gitcmd.NewCommand("checkout", "--force").
		AddDynamicArguments(commitID).
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
	if err != nil {
		return fmt.Errorf("failed to checkout commit '%s': %w - %s", commitID, err, stderr)
	}

	return nil
}

func (g *GitManager) Diff(ctx context.Context, repoPath string, targetCommit, sourceCommit string) ([]*PullRequestChange, error) {
	logger := log.Ctx(ctx)

	// Change to the repository directory
	originalDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get current working directory: %w", err)
	}

	if err := os.Chdir(repoPath); err != nil {
		return nil, fmt.Errorf("failed to change to repository directory: %w", err)
	}
	defer func() {
		if chdirErr := os.Chdir(originalDir); chdirErr != nil {
			logger.Warn().Err(chdirErr).Msg("failed to restore original working directory")
		}
	}()

	// First, get the list of changed files with their status
	statusCmd := exec.CommandContext(ctx, "git", "diff", "--name-status", "--no-renames", targetCommit, sourceCommit)
	statusOutput, err := statusCmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git diff status command failed: %w", err)
	}

	logger.Info().
		Str("target_commit", targetCommit).
		Str("source_commit", sourceCommit).
		Str("status_output", string(statusOutput)).
		Msg("git diff status output")

	// Parse the status output to get changed files
	changedFiles, err := g.parseGitStatusOutput(string(statusOutput))
	if err != nil {
		return nil, fmt.Errorf("failed to parse git status output: %w", err)
	}

	// For each changed file, get the detailed diff
	var changes []*PullRequestChange
	for _, file := range changedFiles {
		change, err := g.getFileDiff(ctx, file.path, file.changeType, targetCommit, sourceCommit)
		if err != nil {
			logger.Warn().Err(err).Str("file_path", file.path).Msg("failed to get file diff, skipping")
			continue
		}

		changes = append(changes, change)
	}

	return changes, nil
}

// fileChange represents a single file change from git status
type fileChange struct {
	path       string
	changeType string
}

// parseGitStatusOutput parses the output of `git diff --name-status`
func (g *GitManager) parseGitStatusOutput(output string) ([]fileChange, error) {
	var changes []fileChange
	lines := strings.Split(strings.TrimSpace(output), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse format: STATUS\tPATH
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue // Skip malformed lines
		}

		status := parts[0]
		path := parts[1]

		changes = append(changes, fileChange{
			path:       path,
			changeType: status,
		})
	}

	return changes, nil
}

// getFileDiff gets the detailed diff for a single file
func (g *GitManager) getFileDiff(ctx context.Context, filePath, changeType, targetCommit, sourceCommit string) (*PullRequestChange, error) {
	// logger := log.Ctx(ctx)

	// Get the detailed diff for this file
	diffCmd := exec.CommandContext(ctx, "git", "diff", "--unified=0", "--no-renames", targetCommit, sourceCommit, "--", filePath)
	diffOutput, err := diffCmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git diff command failed for file %s: %w", filePath, err)
	}

	fileDiff, err := diff.ParseFileDiff(diffOutput)
	if err != nil {
		return &PullRequestChange{
			Path:          filePath,
			ChangeType:    changeType,
			Content:       string(diffOutput),
			ContentLength: len(diffOutput),
			IsBinary:      false,
		}, nil
	}
	// Note: This is more complicated but we can get line numbers. However with diff.ParseFileDiff
	// we can get the hunks with almost no effort

	// Parse the diff output to extract hunks and line changes
	// hunks, linesAdded, linesDeleted, err := g.parseGitDiffOutput(string(diffOutput))
	// if err != nil {
	// 	logger.Warn().Err(err).Str("file_path", filePath).Msg("failed to parse diff output, creating basic change")
	// 	// Return a basic change if parsing fails
	// 	return &PullRequestChange{
	// 		Path:          filePath,
	// 		ChangeType:    changeType,
	// 		Content:       string(diffOutput),
	// 		ContentLength: len(diffOutput),
	// 		IsBinary:      false,
	// 	}, nil
	// }

	return &PullRequestChange{
		Path:          filePath,
		ChangeType:    changeType,
		Content:       string(diffOutput),
		ContentLength: len(diffOutput),
		IsBinary:      false,
		Hunks:         fileDiff.Hunks,
	}, nil
}

// PullRequestDiffResult contains the diff information for a pull request
type PullRequestDiffResult struct {
	Changes     []*PullRequestChange      `json:"changes"`
	PullRequest vcsclient.PullRequestInfo `json:"pull_request"`
}

type PullRequestChange struct {
	Path          string       `json:"path"`
	ChangeType    string       `json:"change_type"`
	Content       string       `json:"content"`
	ContentLength int          `json:"content_length"`
	ContentType   string       `json:"content_type"`
	Encoding      string       `json:"encoding"`
	IsBinary      bool         `json:"is_binary"`
	Hunks         []*diff.Hunk `json:"hunks,omitempty"`
}

type DiffHunk struct {
	Header   string      `json:"header"`
	OldStart int         `json:"old_start"`
	OldLines int         `json:"old_lines"`
	NewStart int         `json:"new_start"`
	NewLines int         `json:"new_lines"`
	Lines    []*DiffLine `json:"lines"`
}

type DiffLine struct {
	Type    string `json:"type"`    // "added", "deleted", "context", "unchanged"
	Content string `json:"content"` // The actual line content
	OldNum  int    `json:"old_num"` // Line number in old file (0 if added)
	NewNum  int    `json:"new_num"` // Line number in new file (0 if deleted)
}
