package services

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	giteagit "code.gitea.io/gitea/modules/git"
	"code.gitea.io/gitea/modules/git/gitcmd"
	"github.com/helixml/helix/api/pkg/types"
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

// GetMergeParentSHAs returns the non-first-parent commit SHAs of merge commits
// reachable from targetBranch. If a branch's current tip exactly matches one of
// these SHAs, Git records affirmative evidence that tip was merged. Squash,
// rebase, fast-forward, and otherwise ambiguous histories intentionally produce
// no evidence.
func GetMergeParentSHAs(ctx context.Context, repoPath, targetBranch string) (map[string]struct{}, error) {
	stdout, _, err := gitcmd.NewCommand("rev-list", "--merges", "--parents").
		AddDynamicArguments(targetBranch).
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
	if err != nil {
		return nil, fmt.Errorf("failed to inspect merges into %s: %w", targetBranch, err)
	}

	return parseMergeParentSHAs(stdout), nil
}

func parseMergeParentSHAs(stdout string) map[string]struct{} {
	mergeParents := make(map[string]struct{})
	for _, line := range strings.Split(stdout, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		for _, parentSHA := range fields[2:] {
			mergeParents[parentSHA] = struct{}{}
		}
	}
	return mergeParents
}

// FindTaskDirInBranch returns the helix-specs path for a spec task's
// design-doc directory.
//
// When designDocPath is set (the modern path, populated for every task
// created since the human-readable directory format landed), the answer
// is structural: tasks live at `design/tasks/<designDocPath>` and
// nothing else needs to be true. Computing it directly is faster and —
// critically — independent of which files happen to be in the branch
// at the moment we look.
//
// The previous implementation walked the tree and returned the parent
// directory of the first filename containing designDocPath. That broke
// the moment https://github.com/helixml/helix/pull/2449 added attachment
// uploads: an attachment landing at
// `design/tasks/<dir>/attachments/image.jpg` *before* the agent's first
// docs push made the walk return `design/tasks/<dir>/attachments` as
// the task root. ReadDesignDocs then looked for `requirements.md`
// inside `attachments/`, found nothing, and left
// spec_task_design_reviews.requirements_spec / .technical_design /
// .implementation_plan empty — the UI showed "No requirements
// specification available" even though the .md files were already
// pushed at the correct path.
//
// The taskID-based fallback is kept for legacy rows whose
// designDocPath is empty (pre-dating the format switch). Those rows
// never had an attachments/ subdir to trip the walk.
func (g *GitRepo) FindTaskDirInBranch(branchName, designDocPath, taskID string) (string, error) {
	if designDocPath != "" {
		return "design/tasks/" + designDocPath, nil
	}

	files, err := g.ListFilesInBranch(branchName)
	if err != nil {
		return "", err
	}
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

		// For spt_ prefixed IDs (either "something_spt_xxx" or starts with "spt_xxx")
		if strings.Contains(dirName, "_spt_") {
			sptIdx := strings.LastIndex(dirName, "_spt_")
			if sptIdx != -1 {
				taskID = dirName[sptIdx+1:]
				foundOldFormat = true
			}
		} else if strings.HasPrefix(dirName, "spt_") {
			// Directory name IS the task ID (e.g., "spt_01kg2fg47nr167g2khc9gsa7dh")
			taskID = dirName
			foundOldFormat = true
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

// SpecTitleFromRequirements extracts the title from requirements.md content.
// It finds the first non-empty line, strips leading # characters, "Requirements:" prefix, and whitespace.
// Returns empty string if no usable title is found.
func SpecTitleFromRequirements(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(line, "#"))
		if line != "" {
			// Strip common prefixes
			line = strings.TrimPrefix(line, "Requirements:")
			line = strings.TrimPrefix(line, "Requirements")
			line = strings.TrimSpace(line)
			if line != "" {
				return types.TruncateSpecTaskName(line)
			}
		}
	}
	return ""
}

// MergeBranchFastForward performs a fast-forward merge of sourceBranch into targetBranch in a bare repo.
// This only works if targetBranch is an ancestor of sourceBranch (fast-forward possible).
// Returns (merged bool, error) where merged indicates if the merge was performed.
func MergeBranchFastForward(ctx context.Context, repoPath, sourceBranch, targetBranch string) (bool, error) {
	// Get source and target commit IDs
	sourceCommit, err := GetBranchCommitID(ctx, repoPath, sourceBranch)
	if err != nil {
		return false, fmt.Errorf("source branch %s not found: %w", sourceBranch, err)
	}

	targetCommit, err := GetBranchCommitID(ctx, repoPath, targetBranch)
	if err != nil {
		return false, fmt.Errorf("target branch %s not found: %w", targetBranch, err)
	}

	// Check if already merged (same commit)
	if sourceCommit == targetCommit {
		log.Debug().
			Str("source_branch", sourceBranch).
			Str("target_branch", targetBranch).
			Msg("Branches already at same commit, nothing to merge")
		return true, nil
	}

	// Check if target is ancestor of source (fast-forward possible)
	_, _, err = gitcmd.NewCommand("merge-base", "--is-ancestor").
		AddDynamicArguments(targetCommit, sourceCommit).
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
	if err != nil {
		// Not a fast-forward - would need a real merge
		return false, fmt.Errorf("cannot fast-forward: %s is not an ancestor of %s (merge conflict possible)", targetBranch, sourceBranch)
	}

	// Fast-forward: update target branch ref to point to source commit
	err = UpdateBranchRef(ctx, repoPath, targetBranch, sourceCommit)
	if err != nil {
		return false, fmt.Errorf("failed to update %s ref: %w", targetBranch, err)
	}

	log.Info().
		Str("source_branch", sourceBranch).
		Str("target_branch", targetBranch).
		Str("new_commit", ShortHash(sourceCommit)).
		Msg("Fast-forward merged branch")

	return true, nil
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
		AddDynamicArguments(ref+"^{tree}").
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
// Equivalent to: git ls-tree -z <tree-sha>
func ListTreeEntries(ctx context.Context, repoPath, treeSHA string) ([]TreeEntry, error) {
	stdout, _, err := gitcmd.NewCommand("ls-tree", "-z").
		AddDynamicArguments(treeSHA).
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
	if err != nil {
		return nil, err
	}

	var entries []TreeEntry
	for _, record := range strings.Split(stdout, "\x00") {
		if record == "" {
			continue
		}
		// Format: <mode> SP <type> SP <sha> TAB <name>
		meta, name, ok := strings.Cut(record, "\t")
		if !ok {
			return nil, fmt.Errorf("malformed ls-tree record %q", record)
		}
		fields := strings.Fields(meta)
		if len(fields) != 3 {
			return nil, fmt.Errorf("malformed ls-tree record %q", record)
		}
		entries = append(entries, TreeEntry{
			Mode: fields[0],
			Type: fields[1],
			SHA:  fields[2],
			Name: name,
		})
	}
	return entries, nil
}

// WriteTree creates a tree object from entries.
// Equivalent to: git mktree -z
func WriteTree(ctx context.Context, repoPath string, entries []TreeEntry) (string, error) {
	var input strings.Builder
	for _, e := range entries {
		if e.Name == "" || e.Name == "." || e.Name == ".." || strings.ContainsAny(e.Name, "/\x00") {
			return "", fmt.Errorf("invalid tree entry name %q", e.Name)
		}
		// Format: <mode> SP <type> SP <sha> TAB <name> NUL
		fmt.Fprintf(&input, "%s %s %s\t%s\x00", e.Mode, e.Type, e.SHA, e.Name)
	}

	stdout, stderr, err := gitcmd.NewCommand("mktree", "-z").
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

// UpdateBranchRefFrom moves a branch to commitSHA only if it currently points
// at oldSHA. Equivalent to: git update-ref refs/heads/<branch> <commit> <old>
func UpdateBranchRefFrom(ctx context.Context, repoPath, branch, commitSHA, oldSHA string) error {
	_, _, err := gitcmd.NewCommand("update-ref").
		AddDynamicArguments("refs/heads/"+branch, commitSHA, oldSHA).
		RunStdString(ctx, &gitcmd.RunOpts{Dir: repoPath})
	return err
}

// ErrRepoPathBlocked is returned when a file write would replace or descend
// through an existing entry that is not a regular file or directory.
var ErrRepoPathBlocked = errors.New("path is blocked by an existing symlink, submodule, file or directory")

// ErrBranchMoved is returned when the branch changed between reading its head
// and updating it.
var ErrBranchMoved = errors.New("branch was updated concurrently")

const (
	treeModeFile       = "100644"
	treeModeExecutable = "100755"
	treeModeDir        = "040000"
)

// CommitFileToBareBranch commits a single file to a branch in a bare repo
// directly, creating the branch when it does not exist. Nothing is checked
// out, so tree content (symlinks, .git paths) never reaches the filesystem.
// Returns (commitSHA, changed, error) where changed indicates if a commit was made.
func CommitFileToBareBranch(ctx context.Context, repoPath, branch, filePath string, content []byte, authorName, authorEmail, message string) (string, bool, error) {
	parentSHA, err := branchHead(ctx, repoPath, branch)
	if err != nil {
		return "", false, err
	}
	return commitFileOnParent(ctx, repoPath, branch, parentSHA, filePath, content, authorName, authorEmail, message)
}

// branchHead returns the commit branch points at, or "" when it does not exist.
func branchHead(ctx context.Context, repoPath, branch string) (string, error) {
	repo, err := giteagit.OpenRepository(ctx, repoPath)
	if err != nil {
		return "", fmt.Errorf("failed to open repository: %w", err)
	}
	defer repo.Close()
	commitID, err := repo.GetBranchCommitID(branch)
	if giteagit.IsErrNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to read branch %s: %w", branch, err)
	}
	return commitID, nil
}

// commitFileOnParent writes content at filePath on top of parentSHA ("" for a
// new branch) and moves branch from parentSHA to the new commit, failing with
// ErrBranchMoved if the branch no longer points at parentSHA.
func commitFileOnParent(ctx context.Context, repoPath, branch, parentSHA, filePath string, content []byte, authorName, authorEmail, message string) (string, bool, error) {
	blobSHA, err := WriteBlob(ctx, repoPath, content)
	if err != nil {
		return "", false, fmt.Errorf("failed to write blob: %w", err)
	}

	var currentTreeSHA string
	if parentSHA != "" {
		currentTreeSHA, err = GetTreeSHA(ctx, repoPath, parentSHA)
		if err != nil {
			return "", false, fmt.Errorf("failed to read tree of %s: %w", branch, err)
		}
	}

	newRootTreeSHA, err := buildTreeWithFile(ctx, repoPath, currentTreeSHA, strings.Split(filePath, "/"), blobSHA)
	if err != nil {
		return "", false, fmt.Errorf("failed to build tree for %s: %w", filePath, err)
	}

	if newRootTreeSHA == currentTreeSHA {
		log.Debug().Str("branch", branch).Str("file", filePath).Msg("File unchanged, skipping commit")
		return parentSHA, false, nil
	}

	commitSHA, err := CreateCommit(ctx, repoPath, newRootTreeSHA, parentSHA, authorName, authorEmail, message)
	if err != nil {
		return "", false, fmt.Errorf("failed to create commit: %w", err)
	}

	if err := UpdateBranchRefFrom(ctx, repoPath, branch, commitSHA, parentSHA); err != nil {
		if head, headErr := branchHead(ctx, repoPath, branch); headErr == nil && head != parentSHA {
			return "", false, fmt.Errorf("%w: %s", ErrBranchMoved, branch)
		}
		return "", false, fmt.Errorf("failed to update branch ref: %w", err)
	}

	log.Info().
		Str("branch", branch).
		Str("file", filePath).
		Str("commit", ShortHash(commitSHA)).
		Msg("Committed file directly to bare repo")

	return commitSHA, true, nil
}

// buildTreeWithFile returns a copy of currentTreeSHA ("" for none) with the
// blob at pathParts. It only replaces regular files and only descends into
// directories: a symlink, submodule or directory in the way is
// ErrRepoPathBlocked, and so is a case-variant symlink or submodule.
func buildTreeWithFile(ctx context.Context, repoPath, currentTreeSHA string, pathParts []string, blobSHA string) (string, error) {
	if len(pathParts) == 0 {
		return "", fmt.Errorf("empty path")
	}

	var entries []TreeEntry
	if currentTreeSHA != "" {
		var err error
		entries, err = ListTreeEntries(ctx, repoPath, currentTreeSHA)
		if err != nil {
			return "", fmt.Errorf("failed to list tree %s: %w", currentTreeSHA, err)
		}
	}

	name := pathParts[0]
	isLeaf := len(pathParts) == 1
	existing := -1
	for i, e := range entries {
		if e.Name == name {
			existing = i
			continue
		}
		if strings.EqualFold(e.Name, name) && e.Mode != treeModeFile && e.Mode != treeModeExecutable && e.Mode != treeModeDir {
			return "", fmt.Errorf("%w: %q (mode %s)", ErrRepoPathBlocked, e.Name, e.Mode)
		}
	}

	var entry TreeEntry
	if isLeaf {
		entry = TreeEntry{Mode: treeModeFile, Type: "blob", SHA: blobSHA, Name: name}
		if existing >= 0 {
			switch entries[existing].Mode {
			case treeModeFile:
			case treeModeExecutable:
				entry.Mode = treeModeExecutable
			default:
				return "", fmt.Errorf("%w: %q (mode %s)", ErrRepoPathBlocked, name, entries[existing].Mode)
			}
		}
	} else {
		var subtreeSHA string
		if existing >= 0 {
			if entries[existing].Mode != treeModeDir {
				return "", fmt.Errorf("%w: %q (mode %s)", ErrRepoPathBlocked, name, entries[existing].Mode)
			}
			subtreeSHA = entries[existing].SHA
		}
		newSubtreeSHA, err := buildTreeWithFile(ctx, repoPath, subtreeSHA, pathParts[1:], blobSHA)
		if err != nil {
			return "", err
		}
		entry = TreeEntry{Mode: treeModeDir, Type: "tree", SHA: newSubtreeSHA, Name: name}
	}

	if existing >= 0 {
		entries[existing] = entry
	} else {
		entries = append(entries, entry)
	}
	return WriteTree(ctx, repoPath, entries)
}
