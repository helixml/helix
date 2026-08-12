package desktop

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

const (
	workspacePatchLimit     = 512 * 1024
	workspaceFileLimit      = 1024 * 1024
	workspaceTreeEntryLimit = 20000
	workspaceListLimit      = 8 * 1024 * 1024
	workspaceSkillFileLimit = 128 * 1024
	workspaceSkillScanLimit = 500
	workspaceSkillDirLimit  = 5000
	checkpointRefPrefix     = "refs/helix/checkpoints/"
	// checkpointRefsPerSession caps how many checkpoint refs a single
	// session may retain. Each turn publishes a before/after pair, so this
	// keeps roughly the last 100 turns reviewable while stopping refs (and
	// the objects they pin) from growing without bound in a repository the
	// user also pushes from.
	checkpointRefsPerSession = 200
)

type reviewRange struct {
	id               string
	title            string
	from             string
	to               string
	includeUntracked bool
}

func (s *Server) handleWorkspaceReview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	workDir, workspace, err := resolveReviewWorkspace(r.URL.Query().Get("workspace"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	base := r.URL.Query().Get("base")
	if base == "" {
		base = "main"
	}
	if _, err := validateReviewRef(base); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resolvedBase := resolveReviewBaseBranch(r.Context(), workDir, base)
	if resolvedBase == "" {
		http.Error(w, fmt.Sprintf("base ref %q not found", base), http.StatusBadRequest)
		return
	}
	resolvedBaseArg, err := safeGitArg(resolvedBase)
	if err != nil {
		http.Error(w, "resolved base ref is invalid", http.StatusBadRequest)
		return
	}
	mergeBase, err := gitText(r.Context(), workDir, constGitArg("merge-base"), resolvedBaseArg, constGitArg("HEAD"))
	if err != nil {
		http.Error(w, fmt.Sprintf("resolve merge base: %v", err), http.StatusInternalServerError)
		return
	}
	mergeBase = strings.TrimSpace(mergeBase)
	ignoreWhitespace := r.URL.Query().Get("ignore_whitespace") == "true"
	ranges := []reviewRange{
		{id: types.WorkspaceReviewSourceAll, title: "All task changes", from: mergeBase, includeUntracked: true},
		{id: types.WorkspaceReviewSourceBranch, title: "Branch changes", from: mergeBase, to: "HEAD"},
		{id: types.WorkspaceReviewSourceWorkingTree, title: "Working tree", from: "HEAD", includeUntracked: true},
	}

	// One throwaway index makes untracked files visible to every scope that
	// wants them, so each scope stays a single coherent `git diff` instead of
	// a tracked patch with a synthesised untracked tail concatenated on.
	untrackedEnv, releaseIndex, err := untrackedIndexEnv(r.Context(), workDir)
	if err != nil {
		http.Error(w, fmt.Sprintf("prepare untracked index: %v", err), http.StatusInternalServerError)
		return
	}
	defer releaseIndex()

	resp := types.WorkspaceReviewResponse{
		Workspace:   workspace,
		GeneratedAt: time.Now().UTC(),
		Sources:     make([]types.WorkspaceReviewSource, 0, len(ranges)),
	}
	for _, review := range ranges {
		var env []string
		if review.includeUntracked {
			env = untrackedEnv
		}
		source, err := buildReviewSource(r.Context(), workDir, resolvedBase, review, ignoreWhitespace, env)
		if err != nil {
			http.Error(w, fmt.Sprintf("generate %s review: %v", review.id, err), http.StatusInternalServerError)
			return
		}
		resp.Sources = append(resp.Sources, source)
	}
	writeJSON(w, http.StatusOK, resp)
}

func buildReviewSource(ctx context.Context, workDir, baseRef string, review reviewRange, ignoreWhitespace bool, env []string) (types.WorkspaceReviewSource, error) {
	patchArgs := []gitArg{
		constGitArg("diff"), constGitArg("--patch"), constGitArg("--no-color"),
		constGitArg("--no-ext-diff"), constGitArg("--no-textconv"), constGitArg("--binary"),
	}
	if ignoreWhitespace {
		patchArgs = append(patchArgs, constGitArg("--ignore-all-space"))
	}
	fromArg, err := safeGitArg(review.from)
	if err != nil {
		return types.WorkspaceReviewSource{}, err
	}
	patchArgs = append(patchArgs, fromArg)
	if review.to != "" {
		toArg, err := safeGitArg(review.to)
		if err != nil {
			return types.WorkspaceReviewSource{}, err
		}
		patchArgs = append(patchArgs, toArg)
	}
	patchArgs = append(patchArgs, constGitArg("--"), constGitArg("."))
	patch, truncated, err := runGitBounded(ctx, workDir, workspacePatchLimit, env, patchArgs...)
	if err != nil {
		return types.WorkspaceReviewSource{}, err
	}

	files, summaryTruncated, err := diffFileSummary(ctx, workDir, review.from, review.to, env)
	if err != nil {
		return types.WorkspaceReviewSource{}, err
	}
	additions, deletions := summarizeCodeChangeFiles(files)
	return types.WorkspaceReviewSource{
		ID:             review.id,
		Title:          review.title,
		BaseRef:        baseRef,
		HeadRef:        "HEAD",
		Patch:          patch,
		PatchHash:      hashString(patch),
		Truncated:      truncated || summaryTruncated,
		Files:          files,
		TotalAdditions: additions,
		TotalDeletions: deletions,
	}, nil
}

func (s *Server) handleWorkspaceFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	workDir, workspace, err := resolveReviewWorkspace(r.URL.Query().Get("workspace"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	// listTruncated matters: a workspace whose path listing exceeds the output
	// bound must not be presented as a complete tree.
	out, listTruncated, err := runGitBounded(r.Context(), workDir, workspaceListLimit, nil,
		constGitArg("ls-files"), constGitArg("-z"), constGitArg("--cached"),
		constGitArg("--others"), constGitArg("--exclude-standard"))
	if err != nil {
		http.Error(w, fmt.Sprintf("list workspace files: %v", err), http.StatusInternalServerError)
		return
	}

	paths := strings.Split(out, "\x00")
	directories := make(map[string]struct{})
	entries := make([]types.WorkspaceFileEntry, 0, min(len(paths), workspaceTreeEntryLimit))
	truncated := listTruncated
	for _, pathValue := range paths {
		if pathValue == "" {
			continue
		}
		rel, err := cleanRelativePath(pathValue)
		if err != nil {
			continue
		}
		for parent := filepath.ToSlash(filepath.Dir(rel)); parent != "."; parent = filepath.ToSlash(filepath.Dir(parent)) {
			directories[parent] = struct{}{}
		}
		if len(entries) >= workspaceTreeEntryLimit {
			truncated = true
			continue
		}
		entry := types.WorkspaceFileEntry{Path: rel, Kind: "file"}
		if info, err := os.Lstat(filepath.Join(workDir, filepath.FromSlash(rel))); err == nil {
			entry.Size = info.Size()
		}
		entries = append(entries, entry)
	}
	for dir := range directories {
		if len(entries) >= workspaceTreeEntryLimit {
			truncated = true
			break
		}
		entries = append(entries, types.WorkspaceFileEntry{Path: dir, Kind: "directory"})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Path == entries[j].Path {
			return entries[i].Kind < entries[j].Kind
		}
		return entries[i].Path < entries[j].Path
	})
	writeJSON(w, http.StatusOK, types.WorkspaceFilesResponse{Workspace: workspace, Entries: entries, Truncated: truncated})
}

func (s *Server) handleWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPut {
		s.handleWorkspaceFileWrite(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	workDir, workspace, err := resolveReviewWorkspace(r.URL.Query().Get("workspace"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	pathValue := r.URL.Query().Get("path")
	resolved, rel, err := resolveWorkspaceFile(workDir, pathValue)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !workspaceFileIsBrowsable(r.Context(), workDir, rel) {
		http.Error(w, "path is not a browsable workspace file", http.StatusBadRequest)
		return
	}
	content, size, truncated, binary, err := readBoundedFile(resolved, workspaceFileLimit)
	if err != nil {
		http.Error(w, fmt.Sprintf("read workspace file: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, types.WorkspaceFileResponse{
		Workspace: workspace, Path: rel, Contents: content, ByteLength: size,
		ContentHash: hashString(content), Truncated: truncated, Binary: binary,
	})
}

func (s *Server) handleWorkspaceFileWrite(w http.ResponseWriter, r *http.Request) {
	var request types.WorkspaceFileWriteRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, int64(workspaceFileLimit*8)))
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if len(request.Contents) > workspaceFileLimit {
		http.Error(w, "file exceeds the editable size limit", http.StatusRequestEntityTooLarge)
		return
	}
	if request.ExpectedContentHash == "" {
		http.Error(w, "expected_content_hash is required", http.StatusBadRequest)
		return
	}

	workDir, workspace, err := resolveReviewWorkspace(request.Workspace)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	resolved, rel, err := resolveWorkspaceFile(workDir, request.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !workspaceFileIsBrowsable(r.Context(), workDir, rel) {
		http.Error(w, "path is not an editable workspace file", http.StatusBadRequest)
		return
	}
	currentContents, _, truncated, binary, err := readBoundedFile(resolved, workspaceFileLimit)
	if err != nil {
		http.Error(w, fmt.Sprintf("read workspace file before write: %v", err), http.StatusInternalServerError)
		return
	}
	if truncated || binary {
		http.Error(w, "file is not editable", http.StatusBadRequest)
		return
	}
	if hashString(currentContents) != request.ExpectedContentHash {
		http.Error(w, "workspace file changed since it was opened", http.StatusConflict)
		return
	}

	info, err := os.Stat(resolved)
	if err != nil {
		http.Error(w, fmt.Sprintf("stat workspace file: %v", err), http.StatusInternalServerError)
		return
	}
	if err := replaceWorkspaceFile(resolved, []byte(request.Contents), info.Mode().Perm()); err != nil {
		http.Error(w, fmt.Sprintf("write workspace file: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, types.WorkspaceFileResponse{
		Workspace:   workspace,
		Path:        rel,
		Contents:    request.Contents,
		ByteLength:  int64(len(request.Contents)),
		ContentHash: hashString(request.Contents),
	})
}

func replaceWorkspaceFile(pathValue string, contents []byte, mode os.FileMode) error {
	temporary, err := os.CreateTemp(filepath.Dir(pathValue), ".helix-file-edit-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(contents); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, pathValue)
}

type workspaceSkillFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

type workspaceSkillRoot struct {
	path  string
	scope string
}

func (s *Server) handleWorkspaceSkills(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	workDir, _, err := resolveReviewWorkspace(r.URL.Query().Get("workspace"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, fmt.Sprintf("resolve sandbox home: %v", err), http.StatusInternalServerError)
		return
	}

	skills, err := listWorkspaceSkills(workDir, homeDir)
	if err != nil {
		http.Error(w, fmt.Sprintf("list workspace skills: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, types.WorkspaceSkillsResponse{Skills: skills})
}

func listWorkspaceSkills(workDir, homeDir string) ([]types.WorkspaceSkillEntry, error) {
	roots := []workspaceSkillRoot{
		{path: filepath.Join(workDir, ".agents", "skills"), scope: "project"},
		{path: filepath.Join(workDir, ".codex", "skills"), scope: "project"},
		{path: filepath.Join(workDir, ".claude", "skills"), scope: "project"},
		{path: filepath.Join(homeDir, ".agents", "skills"), scope: "personal"},
		{path: filepath.Join(homeDir, ".codex", "skills"), scope: "personal"},
		{path: filepath.Join(homeDir, ".claude", "skills"), scope: "personal"},
		{path: filepath.Join(homeDir, ".codex", "plugins"), scope: "app"},
		{path: filepath.Join(homeDir, ".agents", "plugins"), scope: "app"},
	}

	byName := make(map[string]types.WorkspaceSkillEntry)
	for _, root := range roots {
		if len(byName) >= workspaceSkillScanLimit {
			break
		}
		if err := collectWorkspaceSkills(root, byName); err != nil {
			return nil, err
		}
	}

	skills := make([]types.WorkspaceSkillEntry, 0, len(byName))
	for _, skill := range byName {
		skills = append(skills, skill)
	}
	sort.Slice(skills, func(i, j int) bool {
		return strings.ToLower(skills[i].Name) < strings.ToLower(skills[j].Name)
	})
	return skills, nil
}

func collectWorkspaceSkills(root workspaceSkillRoot, byName map[string]types.WorkspaceSkillEntry) error {
	visited := make(map[string]struct{})
	directoriesVisited := 0
	var walk func(string) error
	walk = func(directory string) error {
		if len(byName) >= workspaceSkillScanLimit || directoriesVisited >= workspaceSkillDirLimit {
			return nil
		}
		resolved, err := filepath.EvalSymlinks(directory)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("resolve skill directory %s: %w", directory, err)
		}
		info, err := os.Stat(resolved)
		if err != nil {
			return fmt.Errorf("inspect skill directory %s: %w", directory, err)
		}
		if !info.IsDir() {
			return nil
		}
		if _, exists := visited[resolved]; exists {
			return nil
		}
		visited[resolved] = struct{}{}
		directoriesVisited++

		entries, err := os.ReadDir(resolved)
		if err != nil {
			return fmt.Errorf("read skill directory %s: %w", directory, err)
		}
		for _, entry := range entries {
			if len(byName) >= workspaceSkillScanLimit {
				return nil
			}
			if entry.Name() == ".git" || entry.Name() == "node_modules" {
				continue
			}
			entryPath := filepath.Join(resolved, entry.Name())
			if strings.EqualFold(entry.Name(), "SKILL.md") {
				skill, ok, err := readWorkspaceSkill(root, entryPath)
				if err != nil {
					return err
				}
				if ok {
					key := strings.ToLower(skill.Name)
					if _, exists := byName[key]; !exists {
						byName[key] = skill
					}
				}
				continue
			}
			if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				if err := walk(entryPath); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return walk(root.path)
}

func readWorkspaceSkill(root workspaceSkillRoot, skillPath string) (types.WorkspaceSkillEntry, bool, error) {
	file, err := os.Open(skillPath)
	if err != nil {
		return types.WorkspaceSkillEntry{}, false, fmt.Errorf("open skill %s: %w", skillPath, err)
	}
	defer file.Close()

	content, err := io.ReadAll(io.LimitReader(file, workspaceSkillFileLimit+1))
	if err != nil {
		return types.WorkspaceSkillEntry{}, false, fmt.Errorf("read skill %s: %w", skillPath, err)
	}
	if len(content) > workspaceSkillFileLimit {
		return types.WorkspaceSkillEntry{}, false, nil
	}
	text := string(content)
	if !strings.HasPrefix(text, "---\n") && !strings.HasPrefix(text, "---\r\n") {
		return types.WorkspaceSkillEntry{}, false, nil
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	frontmatterEnd := strings.Index(text[4:], "\n---")
	if frontmatterEnd < 0 {
		return types.WorkspaceSkillEntry{}, false, nil
	}
	var metadata workspaceSkillFrontmatter
	if err := yaml.Unmarshal([]byte(text[4:4+frontmatterEnd]), &metadata); err != nil {
		return types.WorkspaceSkillEntry{}, false, nil
	}
	metadata.Name = strings.TrimSpace(metadata.Name)
	if metadata.Name == "" {
		metadata.Name = filepath.Base(filepath.Dir(skillPath))
	}
	if metadata.Name == "" || strings.ContainsAny(metadata.Name, " \t\r\n$") {
		return types.WorkspaceSkillEntry{}, false, nil
	}
	relativePath, err := filepath.Rel(root.path, skillPath)
	if err != nil {
		return types.WorkspaceSkillEntry{}, false, fmt.Errorf("relativize skill path %s: %w", skillPath, err)
	}
	return types.WorkspaceSkillEntry{
		Name:        metadata.Name,
		Description: strings.TrimSpace(metadata.Description),
		Scope:       root.scope,
		Path:        filepath.ToSlash(relativePath),
	}, true, nil
}

func (s *Server) handleWorkspaceCheckpointCapture(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req types.WorkspaceCheckpointCaptureRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	workDir, workspace, err := resolveReviewWorkspace(req.Workspace)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err := validateCheckpointRef(req.Ref); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	commit, err := captureWorkspaceCheckpoint(r.Context(), workDir, req.Ref)
	if err != nil {
		http.Error(w, fmt.Sprintf("capture checkpoint: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, types.WorkspaceCheckpointCaptureResponse{
		Workspace: workspace, Ref: req.Ref, Commit: commit, Captured: time.Now().UTC(),
	})
}

func (s *Server) handleWorkspaceCheckpointDiff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req types.WorkspaceCheckpointDiffRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	workDir, workspace, err := resolveReviewWorkspace(req.Workspace)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err := validateCheckpointRef(req.BeforeRef); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateCheckpointRef(req.AfterRef); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	beforeArg, _ := safeGitArg(req.BeforeRef)
	afterArg, _ := safeGitArg(req.AfterRef)
	args := []gitArg{constGitArg("diff"), constGitArg("--patch"), constGitArg("--no-color"), constGitArg("--no-ext-diff"), constGitArg("--no-textconv"), constGitArg("--binary")}
	if req.IgnoreWhitespace {
		args = append(args, constGitArg("--ignore-all-space"))
	}
	args = append(args, beforeArg, afterArg, constGitArg("--"), constGitArg("."))
	patch, truncated, err := runGitBounded(r.Context(), workDir, workspacePatchLimit, nil, args...)
	if err != nil {
		http.Error(w, fmt.Sprintf("diff checkpoints: %v", err), http.StatusInternalServerError)
		return
	}
	files, summaryTruncated, err := diffFileSummary(r.Context(), workDir, req.BeforeRef, req.AfterRef, nil)
	if err != nil {
		http.Error(w, fmt.Sprintf("summarize checkpoint diff: %v", err), http.StatusInternalServerError)
		return
	}
	additions, deletions := summarizeCodeChangeFiles(files)
	writeJSON(w, http.StatusOK, types.WorkspaceCheckpointDiffResponse{
		Workspace: workspace, BeforeRef: req.BeforeRef, AfterRef: req.AfterRef,
		Patch: patch, PatchHash: hashString(patch), Truncated: truncated || summaryTruncated, Files: files,
		TotalAdditions: additions, TotalDeletions: deletions,
	})
}

func resolveReviewWorkspace(name string) (string, string, error) {
	if name != "" {
		pathValue := findWorkspaceByNameFunc(name)
		if pathValue == "" {
			return "", "", fmt.Errorf("workspace %q not found", name)
		}
		return pathValue, name, nil
	}
	workspaces := findAllWorkspaces()
	if len(workspaces) == 0 {
		return "", "", fmt.Errorf("no workspace found")
	}
	for _, workspace := range workspaces {
		if workspace.IsPrimary {
			return workspace.Path, workspace.Name, nil
		}
	}
	return workspaces[0].Path, workspaces[0].Name, nil
}

func resolveReviewBaseBranch(ctx context.Context, workDir, base string) string {
	candidates := []string{base}
	if !strings.HasPrefix(base, "origin/") {
		candidates = append(candidates, "origin/"+base)
	}
	if base == "main" {
		candidates = append(candidates, "master", "origin/master")
	} else if base == "master" {
		candidates = append(candidates, "main", "origin/main")
	}
	for _, candidate := range candidates {
		arg, err := validateReviewRef(candidate)
		if err != nil {
			continue
		}
		if _, _, err := runGitBounded(ctx, workDir, 64*1024, nil,
			constGitArg("rev-parse"), constGitArg("--verify"), constGitArg("--quiet"), arg); err == nil {
			return candidate
		}
	}
	return ""
}

func captureWorkspaceCheckpoint(ctx context.Context, workDir, ref string) (string, error) {
	indexPath, release, err := newTempGitIndex(ctx, workDir)
	if err != nil {
		return "", err
	}
	defer release()

	env := append(os.Environ(),
		"GIT_INDEX_FILE="+indexPath,
		"GIT_AUTHOR_NAME=Helix",
		"GIT_AUTHOR_EMAIL=helix@users.noreply.github.com",
		"GIT_COMMITTER_NAME=Helix",
		"GIT_COMMITTER_EMAIL=helix@users.noreply.github.com",
	)
	if err := seedTempIndexFromHEAD(ctx, workDir, env); err != nil {
		return "", err
	}
	if _, _, err := runGitBounded(ctx, workDir, 64*1024, env, constGitArg("add"), constGitArg("-A"), constGitArg("--"), constGitArg(".")); err != nil {
		return "", err
	}
	tree, err := gitTextEnv(ctx, workDir, env, constGitArg("write-tree"))
	if err != nil {
		return "", err
	}
	treeArg, err := safeGitArg(strings.TrimSpace(tree))
	if err != nil {
		return "", err
	}
	commit, err := gitTextEnv(ctx, workDir, env, constGitArg("commit-tree"), treeArg, constGitArg("-m"), constGitArg("helix-workspace-checkpoint"))
	if err != nil {
		return "", err
	}
	commit = strings.TrimSpace(commit)
	refArg, _ := safeGitArg(ref)
	commitArg, err := safeGitArg(commit)
	if err != nil {
		return "", err
	}
	if _, err := gitText(ctx, workDir, constGitArg("update-ref"), refArg, commitArg); err != nil {
		return "", err
	}
	pruneSessionCheckpointRefs(ctx, workDir, ref)
	return commit, nil
}

// pruneSessionCheckpointRefs drops the oldest checkpoint refs for the session
// that owns ref, keeping the most recent checkpointRefsPerSession. Without it
// every turn leaves two permanent refs behind, and those refs pin the tree
// objects `git add -A` wrote — unbounded growth inside the repository the user
// pushes from. Pruning is best effort: a failure here must not fail a capture
// that already succeeded.
func pruneSessionCheckpointRefs(ctx context.Context, workDir, ref string) {
	// refs/helix/checkpoints/<session>/<interaction>/<phase>
	sessionPrefix := path.Dir(path.Dir(ref))
	if !strings.HasPrefix(sessionPrefix, checkpointRefPrefix) || sessionPrefix == strings.TrimSuffix(checkpointRefPrefix, "/") {
		return
	}
	prefixArg, err := safeGitArg(sessionPrefix)
	if err != nil {
		return
	}
	out, _, err := runGitBounded(ctx, workDir, workspaceListLimit, nil,
		constGitArg("for-each-ref"), constGitArg("--sort=-committerdate"),
		constGitArg("--format=%(refname)"), prefixArg)
	if err != nil {
		return
	}
	refs := strings.Split(strings.TrimSpace(out), "\n")
	if len(refs) <= checkpointRefsPerSession {
		return
	}
	for _, stale := range refs[checkpointRefsPerSession:] {
		stale = strings.TrimSpace(stale)
		if stale == "" || stale == ref {
			continue
		}
		staleArg, err := safeGitArg(stale)
		if err != nil {
			continue
		}
		if _, err := gitText(ctx, workDir, constGitArg("update-ref"), constGitArg("-d"), staleArg); err != nil {
			log.Warn().Err(err).Str("ref", stale).Msg("Failed to prune stale workspace checkpoint ref")
		}
	}
}

// diffFileSummary reports the changed-file summary for a range. The second
// return value is true when either underlying git listing hit its output
// bound: the summary is then incomplete and callers must surface that rather
// than presenting a partial file list as the whole change.
func diffFileSummary(ctx context.Context, workDir, from, to string, env []string) ([]types.InteractionCodeChangeFile, bool, error) {
	args := []gitArg{constGitArg("diff"), constGitArg("--numstat"), constGitArg("-z")}
	fromArg, err := safeGitArg(from)
	if err != nil {
		return nil, false, err
	}
	args = append(args, fromArg)
	if to != "" {
		toArg, err := safeGitArg(to)
		if err != nil {
			return nil, false, err
		}
		args = append(args, toArg)
	}
	args = append(args, constGitArg("--"), constGitArg("."))
	out, numstatTruncated, err := runGitBounded(ctx, workDir, workspaceListLimit, env, args...)
	if err != nil {
		return nil, false, err
	}
	files := parseNumstatZ([]byte(out))

	statusArgs := []gitArg{constGitArg("diff"), constGitArg("--name-status"), constGitArg("-z")}
	statusArgs = append(statusArgs, fromArg)
	if to != "" {
		toArg, _ := safeGitArg(to)
		statusArgs = append(statusArgs, toArg)
	}
	statusArgs = append(statusArgs, constGitArg("--"), constGitArg("."))
	statusOut, statusTruncated, err := runGitBounded(ctx, workDir, workspaceListLimit, env, statusArgs...)
	if err != nil {
		return nil, false, err
	}
	applyNameStatuses(files, []byte(statusOut))
	return files, numstatTruncated || statusTruncated, nil
}

func parseNumstatZ(data []byte) []types.InteractionCodeChangeFile {
	parts := bytes.Split(data, []byte{0})
	files := make([]types.InteractionCodeChangeFile, 0, len(parts))
	for index := 0; index < len(parts); index++ {
		part := string(parts[index])
		if part == "" {
			continue
		}
		fields := strings.SplitN(part, "\t", 3)
		if len(fields) != 3 {
			continue
		}
		pathValue := fields[2]
		oldPath := ""
		if pathValue == "" && index+2 < len(parts) {
			oldPath = string(parts[index+1])
			pathValue = string(parts[index+2])
			index += 2
		}
		file := types.InteractionCodeChangeFile{Path: filepath.ToSlash(pathValue), OldPath: filepath.ToSlash(oldPath), Kind: "modified"}
		if fields[0] == "-" || fields[1] == "-" {
			file.Binary = true
		} else {
			file.Additions, _ = strconv.Atoi(fields[0])
			file.Deletions, _ = strconv.Atoi(fields[1])
		}
		files = append(files, file)
	}
	return files
}

func applyNameStatuses(files []types.InteractionCodeChangeFile, data []byte) {
	parts := bytes.Split(data, []byte{0})
	byPath := make(map[string]*types.InteractionCodeChangeFile, len(files))
	for index := range files {
		byPath[files[index].Path] = &files[index]
	}
	for index := 0; index+1 < len(parts); {
		status := string(parts[index])
		index++
		if status == "" || index >= len(parts) {
			continue
		}
		pathValue := filepath.ToSlash(string(parts[index]))
		index++
		oldPath := ""
		if (strings.HasPrefix(status, "R") || strings.HasPrefix(status, "C")) && index < len(parts) {
			oldPath = pathValue
			pathValue = filepath.ToSlash(string(parts[index]))
			index++
		}
		file := byPath[pathValue]
		if file == nil {
			continue
		}
		file.OldPath = oldPath
		switch status[0] {
		case 'A':
			file.Kind = "added"
		case 'D':
			file.Kind = "deleted"
		case 'R':
			file.Kind = "renamed"
		case 'C':
			file.Kind = "copied"
		default:
			file.Kind = "modified"
		}
	}
}

// untrackedIndexEnv builds a throwaway GIT_INDEX_FILE seeded from HEAD with
// intent-to-add entries for every untracked, non-ignored path. Running the
// review's diff commands against it makes untracked files show up as ordinary
// additions inside a single coherent patch — with git's own rename detection,
// blob hashes and numstat counts — instead of needing one `git diff --no-index`
// subprocess per untracked file plus a hand-rolled line count. The review
// endpoint is polled, and workspaces routinely carry hundreds of untracked
// paths, so the per-file fan-out was the dominant cost of the request.
//
// The user's real index, worktree, HEAD and branch refs are untouched: every
// write lands in the temporary index, which is removed on the returned cleanup.
func untrackedIndexEnv(ctx context.Context, workDir string) ([]string, func(), error) {
	indexPath, release, err := newTempGitIndex(ctx, workDir)
	if err != nil {
		return nil, nil, err
	}
	env := append(os.Environ(), "GIT_INDEX_FILE="+indexPath)
	if err := seedTempIndexFromHEAD(ctx, workDir, env); err != nil {
		release()
		return nil, nil, err
	}
	if _, _, err := runGitBounded(ctx, workDir, 64*1024, env,
		constGitArg("add"), constGitArg("-N"), constGitArg("-A"), constGitArg("--"), constGitArg(".")); err != nil {
		release()
		return nil, nil, err
	}
	return env, release, nil
}

// newTempGitIndex reserves a unique index path under the repository's Git
// common directory and returns a cleanup that removes it on every exit path.
func newTempGitIndex(ctx context.Context, workDir string) (string, func(), error) {
	commonDir, err := gitText(ctx, workDir, constGitArg("rev-parse"), constGitArg("--git-common-dir"))
	if err != nil {
		return "", nil, err
	}
	commonDir = strings.TrimSpace(commonDir)
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(workDir, commonDir)
	}
	indexFile, err := os.CreateTemp(commonDir, "helix-checkpoint-index-*")
	if err != nil {
		return "", nil, err
	}
	indexPath := indexFile.Name()
	if err := indexFile.Close(); err != nil {
		return "", nil, err
	}
	if err := os.Remove(indexPath); err != nil && !os.IsNotExist(err) {
		return "", nil, err
	}
	return indexPath, func() { _ = os.Remove(indexPath) }, nil
}

// seedTempIndexFromHEAD populates the temporary index from HEAD, tolerating a
// repository whose HEAD is unborn (freshly initialised, no commit yet) — there
// `git read-tree HEAD` fails and an empty index is the correct starting point.
func seedTempIndexFromHEAD(ctx context.Context, workDir string, env []string) error {
	if _, _, err := runGitBounded(ctx, workDir, 64*1024, env,
		constGitArg("rev-parse"), constGitArg("--verify"), constGitArg("--quiet"), constGitArg("HEAD")); err != nil {
		return nil
	}
	_, _, err := runGitBounded(ctx, workDir, 64*1024, env, constGitArg("read-tree"), constGitArg("HEAD"))
	return err
}

// workspaceFileIsBrowsable reports whether a repository-relative path is one
// the workspace tree would list: tracked, or untracked and not ignored.
//
// Real-path containment alone is not enough. It keeps reads inside the
// workspace root, but `.git` lives inside that root too, so `.git/config` and
// `.git/credentials` were readable by anyone with session read access — which,
// through project grants, includes org members who are not the session owner.
// A remote URL routinely carries a token, so that is credential disclosure and
// not merely surprising. Ignored files (`.env`, build output) are the same
// class of problem: the design scopes the browser to tracked and non-ignored
// content, and the listing endpoint already honours that.
//
// Gating reads on the same `git ls-files` query the tree is built from keeps
// the two endpoints from disagreeing about what exists. A path the browser
// never shows is a path the browser cannot read.
func workspaceFileIsBrowsable(ctx context.Context, workDir, rel string) bool {
	pathArg, err := safeGitPathArg(rel)
	if err != nil {
		return false
	}
	out, _, err := runGitBounded(ctx, workDir, 64*1024, nil,
		constGitArg("ls-files"), constGitArg("-z"), constGitArg("--cached"),
		constGitArg("--others"), constGitArg("--exclude-standard"),
		constGitArg("--"), pathArg)
	if err != nil {
		return false
	}
	for _, listed := range strings.Split(out, "\x00") {
		if listed == rel {
			return true
		}
	}
	return false
}

func resolveWorkspaceFile(root, pathValue string) (string, string, error) {
	rel, err := cleanRelativePath(pathValue)
	if err != nil {
		return "", "", err
	}
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", "", err
	}
	target := filepath.Join(rootReal, filepath.FromSlash(rel))
	targetReal, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", "", err
	}
	contained, err := filepath.Rel(rootReal, targetReal)
	if err != nil || contained == ".." || strings.HasPrefix(contained, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("path escapes workspace")
	}
	info, err := os.Stat(targetReal)
	if err != nil {
		return "", "", err
	}
	if !info.Mode().IsRegular() {
		return "", "", fmt.Errorf("path is not a regular file")
	}
	return targetReal, rel, nil
}

func cleanRelativePath(pathValue string) (string, error) {
	if pathValue == "" || filepath.IsAbs(pathValue) || strings.ContainsAny(pathValue, "\x00\r\n") {
		return "", fmt.Errorf("invalid relative path")
	}
	cleaned := filepath.ToSlash(filepath.Clean(pathValue))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("invalid relative path")
	}
	return cleaned, nil
}

func readBoundedFile(pathValue string, limit int) (string, int64, bool, bool, error) {
	file, err := os.Open(pathValue)
	if err != nil {
		return "", 0, false, false, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", 0, false, false, err
	}
	data, err := io.ReadAll(io.LimitReader(file, int64(limit+1)))
	if err != nil {
		return "", 0, false, false, err
	}
	truncated := len(data) > limit
	if truncated {
		data = data[:limit]
	}
	binary := !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0
	if binary {
		data = nil
	}
	return string(data), info.Size(), truncated, binary, nil
}

func validateCheckpointRef(ref string) error {
	if !strings.HasPrefix(ref, checkpointRefPrefix) {
		return fmt.Errorf("invalid checkpoint ref")
	}
	arg, err := safeGitArg(ref)
	if err != nil || arg.value != ref {
		return fmt.Errorf("invalid checkpoint ref")
	}
	return nil
}

func runGitBounded(ctx context.Context, cwd string, limit int, env []string, args ...gitArg) (string, bool, error) {
	return runGitBoundedExit(ctx, cwd, limit, env, false, args...)
}

func runGitBoundedExit(ctx context.Context, cwd string, limit int, env []string, allowExitOne bool, args ...gitArg) (string, bool, error) {
	if limit < 0 {
		limit = 0
	}
	strs := make([]string, len(args))
	for index, arg := range args {
		strs[index] = arg.value
	}
	cmd := exec.CommandContext(ctx, "git", strs...)
	cmd.Dir = cwd
	if env != nil {
		cmd.Env = env
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", false, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return "", false, err
	}
	data, readErr := io.ReadAll(io.LimitReader(stdout, int64(limit+1)))
	if readErr != nil {
		_ = cmd.Wait()
		return "", false, readErr
	}
	truncated := len(data) > limit
	if truncated {
		data = data[:limit]
	}
	if _, err := io.Copy(io.Discard, stdout); err != nil {
		_ = cmd.Wait()
		return "", false, err
	}
	waitErr := cmd.Wait()
	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); !ok || !allowExitOne || exitErr.ExitCode() != 1 {
			return "", false, fmt.Errorf("git %s: %w: %s", strings.Join(strs, " "), waitErr, strings.TrimSpace(stderr.String()))
		}
	}
	return string(data), truncated, nil
}

func gitText(ctx context.Context, cwd string, args ...gitArg) (string, error) {
	out, _, err := runGitBounded(ctx, cwd, 8*1024*1024, nil, args...)
	return out, err
}

func gitTextEnv(ctx context.Context, cwd string, env []string, args ...gitArg) (string, error) {
	out, _, err := runGitBounded(ctx, cwd, 8*1024*1024, env, args...)
	return out, err
}

func summarizeCodeChangeFiles(files []types.InteractionCodeChangeFile) (int, int) {
	var additions, deletions int
	for _, file := range files {
		additions += file.Additions
		deletions += file.Deletions
	}
	return additions, deletions
}

func hashString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func validateReviewRef(value string) (gitArg, error) {
	if strings.HasPrefix(value, "-") {
		return gitArg{}, fmt.Errorf("invalid Git ref")
	}
	return safeGitArg(value)
}
