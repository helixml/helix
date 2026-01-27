package services

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	giteagit "code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/git/gitcmd"
	"github.com/rs/zerolog/log"
)

// GitRepo wraps a gitea repository for common operations.
// This replaces exec.Command("git", ...) calls with pure Go equivalents.
type GitRepo struct {
	repo *giteagit.Repository
	path string
	ctx  context.Context
}

// OpenGitRepo opens a git repository at the given path.
// Works with both bare and non-bare repositories.
func OpenGitRepo(repoPath string) (*GitRepo, error) {
	ctx := context.Background()
	repo, err := giteagit.OpenRepository(ctx, repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open git repository at %s: %w", repoPath, err)
	}
	return &GitRepo{repo: repo, path: repoPath, ctx: ctx}, nil
}

// Close releases resources held by the repository
func (g *GitRepo) Close() {
	if g.repo != nil {
		g.repo.Close()
	}
}

// GetBranchCommitHash returns the commit hash for a branch.
// Equivalent to: git rev-parse <branch>
func (g *GitRepo) GetBranchCommitHash(branchName string) (string, error) {
	commit, err := g.repo.GetBranchCommit(branchName)
	if err != nil {
		return "", fmt.Errorf("branch %s not found: %w", branchName, err)
	}
	return commit.ID.String(), nil
}

// ListFilesInBranch returns all file paths in a branch.
// Equivalent to: git ls-tree --name-only -r <branch>
func (g *GitRepo) ListFilesInBranch(branchName string) ([]string, error) {
	commit, err := g.repo.GetBranchCommit(branchName)
	if err != nil {
		return nil, fmt.Errorf("branch %s not found: %w", branchName, err)
	}

	// Get the tree - commit embeds Tree
	tree := &commit.Tree

	// List all entries recursively by walking the tree
	var files []string
	err = g.walkTree(tree, "", &files)
	if err != nil {
		return nil, fmt.Errorf("failed to walk tree: %w", err)
	}

	return files, nil
}

// walkTree recursively walks a git tree and collects file paths
func (g *GitRepo) walkTree(tree *giteagit.Tree, prefix string, files *[]string) error {
	entries, err := tree.ListEntries()
	if err != nil {
		return err
	}

	for _, entry := range entries {
		path := entry.Name()
		if prefix != "" {
			path = filepath.Join(prefix, entry.Name())
		}

		if entry.IsDir() {
			// Recurse into subdirectory
			subtree, err := tree.SubTree(entry.Name())
			if err != nil {
				continue // Skip if we can't access subtree
			}
			if err := g.walkTree(subtree, path, files); err != nil {
				continue // Skip on error
			}
		} else {
			*files = append(*files, path)
		}
	}
	return nil
}

// ReadFileFromBranch reads the content of a file from a specific branch.
// Equivalent to: git show <branch>:<filepath>
func (g *GitRepo) ReadFileFromBranch(branchName, filePath string) ([]byte, error) {
	commit, err := g.repo.GetBranchCommit(branchName)
	if err != nil {
		return nil, fmt.Errorf("branch %s not found: %w", branchName, err)
	}

	// Use GetFileContent from commit which returns string
	content, err := commit.GetFileContent(filePath, 0) // 0 = no limit
	if err != nil {
		return nil, fmt.Errorf("file %s not found in branch %s: %w", filePath, branchName, err)
	}

	return []byte(content), nil
}

// IsBranchMergedInto checks if sourceBranch is merged into targetBranch.
// Equivalent to: git branch --merged <target> --list <source>
func (g *GitRepo) IsBranchMergedInto(sourceBranch, targetBranch string) (bool, error) {
	sourceCommit, err := g.repo.GetBranchCommit(sourceBranch)
	if err != nil {
		return false, fmt.Errorf("source branch %s not found: %w", sourceBranch, err)
	}

	targetCommit, err := g.repo.GetBranchCommit(targetBranch)
	if err != nil {
		return false, fmt.Errorf("target branch %s not found: %w", targetBranch, err)
	}

	// Check if source commit is an ancestor of target commit using git merge-base
	_, _, err = gitcmd.NewCommand("merge-base", "--is-ancestor").
		AddDynamicArguments(sourceCommit.ID.String(), targetCommit.ID.String()).
		RunStdString(g.ctx, &gitcmd.RunOpts{Dir: g.path})
	if err != nil {
		// Exit code 1 means not an ancestor, exit code 0 means it is
		return false, nil // Not an ancestor
	}
	return true, nil // Is an ancestor
}

// GetChangedFilesInCommit returns files changed in a specific commit.
// Uses gitea's high-level GetCommitFileStatus API.
func (g *GitRepo) GetChangedFilesInCommit(commitHash string) ([]string, error) {
	// Use gitea's high-level API to get file status for the commit
	status, err := giteagit.GetCommitFileStatus(g.ctx, g.path, commitHash)
	if err != nil {
		return nil, fmt.Errorf("failed to get changed files: %w", err)
	}

	// Combine all changed files (added, modified, removed)
	var files []string
	files = append(files, status.Added...)
	files = append(files, status.Modified...)
	files = append(files, status.Removed...)
	return files, nil
}

// GetChangedFilesInBranch returns files changed in the latest commit of a branch.
// Equivalent to: git diff-tree -m --no-commit-id --name-only -r <branch>
func (g *GitRepo) GetChangedFilesInBranch(branchName string) ([]string, error) {
	commit, err := g.repo.GetBranchCommit(branchName)
	if err != nil {
		return nil, fmt.Errorf("branch %s not found: %w", branchName, err)
	}
	return g.GetChangedFilesInCommit(commit.ID.String())
}

// ListBranches returns all branch names in the repository.
func (g *GitRepo) ListBranches() ([]string, error) {
	branches, _, err := g.repo.GetBranchNames(0, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to list branches: %w", err)
	}
	return branches, nil
}

// FindTaskDirInBranch finds the directory for a specific task in the helix-specs branch.
// Searches for either DesignDocPath or task ID in the directory structure.
func (g *GitRepo) FindTaskDirInBranch(branchName, designDocPath, taskID string) (string, error) {
	files, err := g.ListFilesInBranch(branchName)
	if err != nil {
		return "", err
	}

	// First try DesignDocPath (new human-readable format)
	if designDocPath != "" {
		for _, file := range files {
			if strings.Contains(file, designDocPath) {
				parts := strings.Split(file, "/")
				if len(parts) >= 2 {
					return strings.Join(parts[:len(parts)-1], "/"), nil
				}
			}
		}
	}

	// Fall back to taskID for backwards compatibility
	for _, file := range files {
		if strings.Contains(file, taskID) {
			parts := strings.Split(file, "/")
			if len(parts) >= 2 {
				return strings.Join(parts[:len(parts)-1], "/"), nil
			}
		}
	}

	return "", fmt.Errorf("task directory not found for %s", taskID)
}

// ReadDesignDocs reads the standard design documents from a task directory.
// Returns a map of filename -> content for requirements.md, design.md, tasks.md
func (g *GitRepo) ReadDesignDocs(branchName, taskDir string) (map[string]string, error) {
	docs := make(map[string]string)
	docFilenames := []string{"requirements.md", "design.md", "tasks.md"}

	for _, filename := range docFilenames {
		filePath := taskDir + "/" + filename
		content, err := g.ReadFileFromBranch(branchName, filePath)
		if err != nil {
			log.Debug().
				Err(err).
				Str("filename", filename).
				Str("path", filePath).
				Msg("Design doc file not found (may not exist yet)")
			continue
		}
		docs[filename] = string(content)
	}

	return docs, nil
}

// ParseDesignDocTaskIDs extracts task IDs from design doc file paths.
// Supports both old format (task ID in directory name) and new format (task number).
// Returns taskIDs found directly and dirNames that need DB lookup.
func ParseDesignDocTaskIDs(files []string) (taskIDs []string, dirNamesNeedingLookup []string) {
	taskIDSet := make(map[string]bool)
	dirNameSet := make(map[string]bool)

	for _, file := range files {
		if !strings.Contains(file, "design/tasks/") && !strings.Contains(file, "tasks/") {
			continue
		}

		parts := strings.Split(file, "/")
		if len(parts) < 3 {
			continue
		}

		// The directory name is the second-to-last part (before the filename)
		dirName := parts[len(parts)-2]

		// Task ID is after the last underscore
		lastUnderscore := strings.LastIndex(dirName, "_")
		if lastUnderscore == -1 {
			continue
		}

		lastPart := dirName[lastUnderscore+1:]

		// Check for UUID format (old format)
		isValidUUID := len(lastPart) == 36 && strings.Count(lastPart, "-") == 4

		taskID := lastPart
		foundOldFormat := false

		// For spt_ prefixed IDs
		if strings.Contains(dirName, "_spt_") {
			sptIdx := strings.LastIndex(dirName, "_spt_")
			if sptIdx != -1 {
				taskID = dirName[sptIdx+1:]
				foundOldFormat = true
			}
		}

		// For legacy task_ prefix format
		if !foundOldFormat && strings.Contains(dirName, "task_") {
			taskPrefixIdx := strings.LastIndex(dirName, "task_")
			if taskPrefixIdx != -1 {
				taskID = dirName[taskPrefixIdx:]
				foundOldFormat = true
			}
		}

		if foundOldFormat || isValidUUID {
			taskIDSet[taskID] = true
		} else {
			// New format: needs DB lookup
			dirNameSet[dirName] = true
		}
	}

	for id := range taskIDSet {
		taskIDs = append(taskIDs, id)
	}
	for name := range dirNameSet {
		dirNamesNeedingLookup = append(dirNamesNeedingLookup, name)
	}

	return taskIDs, dirNamesNeedingLookup
}

// WriteBlob writes content to the git object store and returns the blob SHA.
// Equivalent to: echo "content" | git hash-object -w --stdin
func WriteBlob(ctx context.Context, repoPath string, content []byte) (string, error) {
	cmd := gitcmd.NewCommand("hash-object", "-w", "--stdin")
	stdout, stderr, err := cmd.RunStdBytes(ctx, &gitcmd.RunOpts{
		Dir:   repoPath,
		Stdin: strings.NewReader(string(content)),
	})
	if err != nil {
		return "", fmt.Errorf("hash-object failed: %w - %s", err, stderr)
	}
	return strings.TrimSpace(string(stdout)), nil
}

// GetTreeSHA returns the tree SHA for a given commit/branch.
// Equivalent to: git rev-parse <ref>^{tree}
func GetTreeSHA(ctx context.Context, repoPath, ref string) (string, error) {
	stdout, _, err := gitcmd.NewCommand("rev-parse").
		AddDynamicArguments(ref + "^{tree}").
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout), nil
}

// TreeEntry represents an entry in a git tree
type TreeEntry struct {
	Mode string // "100644" for file, "040000" for directory
	Type string // "blob" or "tree"
	SHA  string
	Name string
}

// ListTreeEntries lists entries in a tree.
// Equivalent to: git ls-tree <tree-sha>
func ListTreeEntries(ctx context.Context, repoPath, treeSHA string) ([]TreeEntry, error) {
	stdout, _, err := gitcmd.NewCommand("ls-tree").
		AddDynamicArguments(treeSHA).
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
	if err != nil {
		return nil, err
	}

	var entries []TreeEntry
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		if line == "" {
			continue
		}
		// Format: <mode> <type> <sha>\t<name>
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		meta := strings.Fields(parts[0])
		if len(meta) != 3 {
			continue
		}
		entries = append(entries, TreeEntry{
			Mode: meta[0],
			Type: meta[1],
			SHA:  meta[2],
			Name: parts[1],
		})
	}
	return entries, nil
}

// WriteTree creates a tree object from entries.
// Equivalent to: git mktree
func WriteTree(ctx context.Context, repoPath string, entries []TreeEntry) (string, error) {
	var input strings.Builder
	for _, e := range entries {
		// Format: <mode> <type> <sha>\t<name>
		fmt.Fprintf(&input, "%s %s %s\t%s\n", e.Mode, e.Type, e.SHA, e.Name)
	}

	stdout, stderr, err := gitcmd.NewCommand("mktree").
		RunStdBytes(ctx, &gitcmd.RunOpts{
			Dir:   repoPath,
			Stdin: strings.NewReader(input.String()),
		})
	if err != nil {
		return "", fmt.Errorf("mktree failed: %w - %s", err, stderr)
	}
	return strings.TrimSpace(string(stdout)), nil
}

// CreateCommit creates a commit object.
// Equivalent to: git commit-tree <tree> -p <parent> -m "message"
func CreateCommit(ctx context.Context, repoPath, treeSHA, parentSHA, authorName, authorEmail, message string) (string, error) {
	cmd := gitcmd.NewCommand("commit-tree").AddDynamicArguments(treeSHA)
	if parentSHA != "" {
		cmd.AddArguments("-p").AddDynamicArguments(parentSHA)
	}
	cmd.AddArguments("-m").AddDynamicArguments(message)

	// Set author/committer via environment
	env := []string{
		fmt.Sprintf("GIT_AUTHOR_NAME=%s", authorName),
		fmt.Sprintf("GIT_AUTHOR_EMAIL=%s", authorEmail),
		fmt.Sprintf("GIT_COMMITTER_NAME=%s", authorName),
		fmt.Sprintf("GIT_COMMITTER_EMAIL=%s", authorEmail),
	}

	stdout, stderr, err := cmd.RunStdBytes(ctx, &gitcmd.RunOpts{
		Dir: repoPath,
		Env: env,
	})
	if err != nil {
		return "", fmt.Errorf("commit-tree failed: %w - %s", err, stderr)
	}
	return strings.TrimSpace(string(stdout)), nil
}

// UpdateBranchRef updates a branch to point to a commit.
// Equivalent to: git update-ref refs/heads/<branch> <commit>
func UpdateBranchRef(ctx context.Context, repoPath, branch, commitSHA string) error {
	_, _, err := gitcmd.NewCommand("update-ref").
		AddDynamicArguments("refs/heads/"+branch, commitSHA).
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
	return err
}

// CommitFileToBareBranch commits a single file to a branch in a bare repo directly.
// This avoids creating a working copy - much faster for single file updates.
// Returns (commitSHA, changed, error) where changed indicates if a commit was made.
func CommitFileToBareBranch(ctx context.Context, repoPath, branch, filePath string, content []byte, authorName, authorEmail, message string) (string, bool, error) {
	// 1. Write the blob
	blobSHA, err := WriteBlob(ctx, repoPath, content)
	if err != nil {
		return "", false, fmt.Errorf("failed to write blob: %w", err)
	}

	// 2. Get current branch state (may not exist)
	var parentSHA, currentTreeSHA string
	parentSHA, _ = GetBranchCommitID(ctx, repoPath, branch)
	if parentSHA != "" {
		currentTreeSHA, _ = GetTreeSHA(ctx, repoPath, branch)
	}

	// 3. Build the new tree with the file at the correct path
	// Split path into components (e.g., ".helix/startup.sh" -> [".helix", "startup.sh"])
	pathParts := strings.Split(filePath, "/")
	newRootTreeSHA, err := buildTreeWithFile(ctx, repoPath, currentTreeSHA, pathParts, blobSHA)
	if err != nil {
		return "", false, fmt.Errorf("failed to build tree: %w", err)
	}

	// 4. Check if tree changed (no-op if identical)
	if newRootTreeSHA == currentTreeSHA {
		log.Debug().Str("branch", branch).Str("file", filePath).Msg("File unchanged, skipping commit")
		return parentSHA, false, nil
	}

	// 5. Create commit
	commitSHA, err := CreateCommit(ctx, repoPath, newRootTreeSHA, parentSHA, authorName, authorEmail, message)
	if err != nil {
		return "", false, fmt.Errorf("failed to create commit: %w", err)
	}

	// 6. Update branch ref
	if err := UpdateBranchRef(ctx, repoPath, branch, commitSHA); err != nil {
		return "", false, fmt.Errorf("failed to update branch ref: %w", err)
	}

	log.Info().
		Str("branch", branch).
		Str("file", filePath).
		Str("commit", ShortHash(commitSHA)).
		Msg("Committed file directly to bare repo")

	return commitSHA, true, nil
}

// buildTreeWithFile recursively builds a tree with a file at the given path.
// pathParts is the remaining path components, blobSHA is the file content.
// Returns the SHA of the new/modified tree.
func buildTreeWithFile(ctx context.Context, repoPath, currentTreeSHA string, pathParts []string, blobSHA string) (string, error) {
	if len(pathParts) == 0 {
		return "", fmt.Errorf("empty path")
	}

	// Get current entries (if tree exists)
	var entries []TreeEntry
	if currentTreeSHA != "" {
		var err error
		entries, err = ListTreeEntries(ctx, repoPath, currentTreeSHA)
		if err != nil {
			// Tree might not exist yet, start fresh
			entries = nil
		}
	}

	targetName := pathParts[0]

	if len(pathParts) == 1 {
		// This is the file - add/update it
		found := false
		for i, e := range entries {
			if e.Name == targetName {
				entries[i].SHA = blobSHA
				entries[i].Mode = "100644"
				entries[i].Type = "blob"
				found = true
				break
			}
		}
		if !found {
			entries = append(entries, TreeEntry{
				Mode: "100644",
				Type: "blob",
				SHA:  blobSHA,
				Name: targetName,
			})
		}
	} else {
		// This is a directory - recurse
		var subtreeSHA string
		for _, e := range entries {
			if e.Name == targetName && e.Type == "tree" {
				subtreeSHA = e.SHA
				break
			}
		}

		// Build the subtree
		newSubtreeSHA, err := buildTreeWithFile(ctx, repoPath, subtreeSHA, pathParts[1:], blobSHA)
		if err != nil {
			return "", err
		}

		// Update or add directory entry
		found := false
		for i, e := range entries {
			if e.Name == targetName {
				entries[i].SHA = newSubtreeSHA
				entries[i].Mode = "040000"
				entries[i].Type = "tree"
				found = true
				break
			}
		}
		if !found {
			entries = append(entries, TreeEntry{
				Mode: "040000",
				Type: "tree",
				SHA:  newSubtreeSHA,
				Name: targetName,
			})
		}
	}

	// Write the tree
	return WriteTree(ctx, repoPath, entries)
}
