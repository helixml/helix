package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/transport/http"
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

func (g *GitManager) CloneRepository(_ context.Context, repoPath string) (*git.Repository, error) {
	repo, err := git.PlainClone(repoPath, &git.CloneOptions{
		URL:      g.remoteURL,
		Auth:     &http.BasicAuth{Username: "helixml", Password: g.token},
		Progress: os.Stdout,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to clone repository: %w", err)
	}

	return repo, err
}

func (g *GitManager) CheckoutCommit(_ context.Context, repo *git.Repository, commitID string) error {
	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	err = worktree.Checkout(&git.CheckoutOptions{
		Hash:  plumbing.NewHash(commitID),
		Force: true,
	})
	if err != nil {
		return fmt.Errorf("failed to checkout commit '%s': %w", commitID, err)
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

// parseGitDiffOutput parses the unified diff format output
func (g *GitManager) parseGitDiffOutput(diffOutput string) ([]*DiffHunk, int, int, error) { //nolint:unused
	var hunks []*DiffHunk
	var linesAdded, linesDeleted int

	lines := strings.Split(diffOutput, "\n")
	var currentHunk *DiffHunk

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Parse hunk header: @@ -oldStart,oldLines +newStart,newLines @@
		if strings.HasPrefix(line, "@@") && strings.HasSuffix(line, "@@") {
			// Save previous hunk if exists
			if currentHunk != nil {
				hunks = append(hunks, currentHunk)
			}

			// Parse hunk header
			hunk, err := g.parseHunkHeader(line)
			if err != nil {
				continue // Skip malformed hunk headers
			}
			currentHunk = hunk
			continue
		}

		// Parse diff lines
		if currentHunk != nil && line != "" {
			diffLine := g.parseDiffLine(line, currentHunk)
			if diffLine != nil {
				currentHunk.Lines = append(currentHunk.Lines, diffLine)

				// Count added/deleted lines
				switch diffLine.Type {
				case "added":
					linesAdded++
				case "deleted":
					linesDeleted++
				}
			}
		}
	}

	// Add the last hunk
	if currentHunk != nil {
		hunks = append(hunks, currentHunk)
	}

	return hunks, linesAdded, linesDeleted, nil
}

// parseHunkHeader parses a hunk header line like "@@ -1,3 +1,4 @@"
func (g *GitManager) parseHunkHeader(header string) (*DiffHunk, error) { //nolint:unused
	// Remove @@ markers
	header = strings.TrimPrefix(header, "@@ ")
	header = strings.TrimSuffix(header, " @@")

	// Split into old and new parts
	parts := strings.Split(header, " ")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid hunk header format: %s", header)
	}

	oldPart := parts[0]
	newPart := parts[1]

	// Parse old part: -start,lines
	oldStart, oldLines, err := g.parseRangePart(oldPart)
	if err != nil {
		return nil, fmt.Errorf("failed to parse old range: %w", err)
	}

	// Parse new part: +start,lines
	newStart, newLines, err := g.parseRangePart(newPart)
	if err != nil {
		return nil, fmt.Errorf("failed to parse new range: %w", err)
	}

	return &DiffHunk{
		Header:   header,
		OldStart: oldStart,
		OldLines: oldLines,
		NewStart: newStart,
		NewLines: newLines,
		Lines:    []*DiffLine{},
	}, nil
}

// parseRangePart parses a range part like "-1,3" or "+1,4"
func (g *GitManager) parseRangePart(part string) (start, lines int, err error) { //nolint:unused
	// Remove + or - prefix
	sign := part[0]
	part = part[1:]

	// Split by comma
	rangeParts := strings.Split(part, ",")
	if len(rangeParts) != 2 {
		return 0, 0, fmt.Errorf("invalid range format: %s", part)
	}

	start, err = strconv.Atoi(rangeParts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid start number: %w", err)
	}

	lines, err = strconv.Atoi(rangeParts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid lines number: %w", err)
	}

	// Adjust start for deleted lines (git uses 0 for deleted lines)
	if sign == '-' && lines == 0 {
		start = 0
	}

	return start, lines, nil
}

// parseDiffLine parses a single diff line and determines its type
func (g *GitManager) parseDiffLine(line string, hunk *DiffHunk) *DiffLine { //nolint:unused
	if len(line) == 0 {
		return nil
	}

	var diffLine DiffLine
	var lineNum int

	switch line[0] {
	case '+':
		diffLine.Type = "added"
		diffLine.Content = line[1:] // Remove + prefix
		lineNum = hunk.NewStart + len(hunk.Lines) + 1
		diffLine.NewNum = lineNum
		diffLine.OldNum = 0
	case '-':
		diffLine.Type = "deleted"
		diffLine.Content = line[1:] // Remove - prefix
		lineNum = hunk.OldStart + len(hunk.Lines) + 1
		diffLine.OldNum = lineNum
		diffLine.NewNum = 0
	case ' ':
		diffLine.Type = "context"
		diffLine.Content = line[1:] // Remove space prefix
		lineNum = hunk.OldStart + len(hunk.Lines) + 1
		diffLine.OldNum = lineNum
		diffLine.NewNum = lineNum
	default:
		return nil // Skip other lines
	}

	return &diffLine
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
