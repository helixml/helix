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
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/helixml/helix/api/pkg/types"
)

const (
	workspacePatchLimit     = 512 * 1024
	workspaceFileLimit      = 1024 * 1024
	workspaceTreeEntryLimit = 20000
	checkpointRefPrefix     = "refs/helix/checkpoints/"
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

	resp := types.WorkspaceReviewResponse{
		Workspace:   workspace,
		GeneratedAt: time.Now().UTC(),
		Sources:     make([]types.WorkspaceReviewSource, 0, len(ranges)),
	}
	for _, review := range ranges {
		source, err := buildReviewSource(r.Context(), workDir, resolvedBase, review, ignoreWhitespace)
		if err != nil {
			http.Error(w, fmt.Sprintf("generate %s review: %v", review.id, err), http.StatusInternalServerError)
			return
		}
		resp.Sources = append(resp.Sources, source)
	}
	writeJSON(w, http.StatusOK, resp)
}

func buildReviewSource(ctx context.Context, workDir, baseRef string, review reviewRange, ignoreWhitespace bool) (types.WorkspaceReviewSource, error) {
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
	patch, truncated, err := runGitBounded(ctx, workDir, workspacePatchLimit, nil, patchArgs...)
	if err != nil {
		return types.WorkspaceReviewSource{}, err
	}

	files, err := diffFileSummary(ctx, workDir, review.from, review.to)
	if err != nil {
		return types.WorkspaceReviewSource{}, err
	}
	if review.includeUntracked {
		untrackedPatch, untrackedFiles, untrackedTruncated, err := untrackedDiff(ctx, workDir, workspacePatchLimit-len(patch))
		if err != nil {
			return types.WorkspaceReviewSource{}, err
		}
		patch += untrackedPatch
		truncated = truncated || untrackedTruncated
		files = mergeCodeChangeFiles(files, untrackedFiles)
	}
	additions, deletions := summarizeCodeChangeFiles(files)
	return types.WorkspaceReviewSource{
		ID:             review.id,
		Title:          review.title,
		BaseRef:        baseRef,
		HeadRef:        "HEAD",
		Patch:          patch,
		PatchHash:      hashString(patch),
		Truncated:      truncated,
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
	out, _, err := runGitBounded(r.Context(), workDir, 8*1024*1024, nil,
		constGitArg("ls-files"), constGitArg("-z"), constGitArg("--cached"),
		constGitArg("--others"), constGitArg("--exclude-standard"))
	if err != nil {
		http.Error(w, fmt.Sprintf("list workspace files: %v", err), http.StatusInternalServerError)
		return
	}

	paths := strings.Split(out, "\x00")
	directories := make(map[string]struct{})
	entries := make([]types.WorkspaceFileEntry, 0, min(len(paths), workspaceTreeEntryLimit))
	truncated := false
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

func (s *Server) handleWorkspaceReviewFileContents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	workDir, _, err := resolveReviewWorkspace(r.URL.Query().Get("workspace"))
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
	source := r.URL.Query().Get("source")
	oldRef, newRef := strings.TrimSpace(mergeBase), ""
	switch source {
	case types.WorkspaceReviewSourceBranch:
		newRef = "HEAD"
	case types.WorkspaceReviewSourceWorkingTree:
		oldRef = "HEAD"
	case types.WorkspaceReviewSourceAll:
	default:
		http.Error(w, "invalid review source", http.StatusBadRequest)
		return
	}
	oldContent, err := readReviewContent(r.Context(), workDir, oldRef, r.URL.Query().Get("old_path"))
	if err != nil {
		http.Error(w, fmt.Sprintf("read old file: %v", err), http.StatusBadRequest)
		return
	}
	newContent, err := readReviewContent(r.Context(), workDir, newRef, r.URL.Query().Get("new_path"))
	if err != nil {
		http.Error(w, fmt.Sprintf("read new file: %v", err), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, types.WorkspaceReviewFileContentsResponse{Old: oldContent, New: newContent})
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
	files, err := diffFileSummary(r.Context(), workDir, req.BeforeRef, req.AfterRef)
	if err != nil {
		http.Error(w, fmt.Sprintf("summarize checkpoint diff: %v", err), http.StatusInternalServerError)
		return
	}
	additions, deletions := summarizeCodeChangeFiles(files)
	writeJSON(w, http.StatusOK, types.WorkspaceCheckpointDiffResponse{
		Workspace: workspace, BeforeRef: req.BeforeRef, AfterRef: req.AfterRef,
		Patch: patch, PatchHash: hashString(patch), Truncated: truncated, Files: files,
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
	commonDir, err := gitText(ctx, workDir, constGitArg("rev-parse"), constGitArg("--git-common-dir"))
	if err != nil {
		return "", err
	}
	commonDir = strings.TrimSpace(commonDir)
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(workDir, commonDir)
	}
	indexFile, err := os.CreateTemp(commonDir, "helix-checkpoint-index-*")
	if err != nil {
		return "", err
	}
	indexPath := indexFile.Name()
	if err := indexFile.Close(); err != nil {
		return "", err
	}
	if err := os.Remove(indexPath); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	defer os.Remove(indexPath)

	env := append(os.Environ(),
		"GIT_INDEX_FILE="+indexPath,
		"GIT_AUTHOR_NAME=Helix",
		"GIT_AUTHOR_EMAIL=helix@users.noreply.github.com",
		"GIT_COMMITTER_NAME=Helix",
		"GIT_COMMITTER_EMAIL=helix@users.noreply.github.com",
	)
	if _, _, err := runGitBounded(ctx, workDir, 64*1024, env, constGitArg("read-tree"), constGitArg("HEAD")); err != nil {
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
	return commit, nil
}

func diffFileSummary(ctx context.Context, workDir, from, to string) ([]types.InteractionCodeChangeFile, error) {
	args := []gitArg{constGitArg("diff"), constGitArg("--numstat"), constGitArg("-z")}
	fromArg, err := safeGitArg(from)
	if err != nil {
		return nil, err
	}
	args = append(args, fromArg)
	if to != "" {
		toArg, err := safeGitArg(to)
		if err != nil {
			return nil, err
		}
		args = append(args, toArg)
	}
	args = append(args, constGitArg("--"), constGitArg("."))
	out, _, err := runGitBounded(ctx, workDir, 8*1024*1024, nil, args...)
	if err != nil {
		return nil, err
	}
	files := parseNumstatZ([]byte(out))

	statusArgs := []gitArg{constGitArg("diff"), constGitArg("--name-status"), constGitArg("-z")}
	statusArgs = append(statusArgs, fromArg)
	if to != "" {
		toArg, _ := safeGitArg(to)
		statusArgs = append(statusArgs, toArg)
	}
	statusArgs = append(statusArgs, constGitArg("--"), constGitArg("."))
	statusOut, _, err := runGitBounded(ctx, workDir, 8*1024*1024, nil, statusArgs...)
	if err != nil {
		return nil, err
	}
	applyNameStatuses(files, []byte(statusOut))
	return files, nil
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

func untrackedDiff(ctx context.Context, workDir string, remaining int) (string, []types.InteractionCodeChangeFile, bool, error) {
	if remaining <= 0 {
		return "", nil, true, nil
	}
	out, _, err := runGitBounded(ctx, workDir, 8*1024*1024, nil, constGitArg("ls-files"), constGitArg("-z"), constGitArg("--others"), constGitArg("--exclude-standard"))
	if err != nil {
		return "", nil, false, err
	}
	var patch strings.Builder
	files := make([]types.InteractionCodeChangeFile, 0)
	truncated := false
	for _, rawPath := range strings.Split(out, "\x00") {
		if rawPath == "" {
			continue
		}
		rel, err := cleanRelativePath(rawPath)
		if err != nil {
			continue
		}
		pathArg, _ := safeGitPathArg(rel)
		filePatch, wasTruncated, runErr := runGitBoundedAllowExitOne(ctx, workDir, remaining-patch.Len(),
			constGitArg("diff"), constGitArg("--no-index"), constGitArg("--patch"), constGitArg("--no-color"), constGitArg("--binary"), constGitArg("--"), constGitArg("/dev/null"), pathArg)
		if runErr != nil {
			return "", nil, false, runErr
		}
		contentPath := filepath.Join(workDir, filepath.FromSlash(rel))
		content, size, _, binary, readErr := readBoundedFile(contentPath, workspaceFileLimit)
		file := types.InteractionCodeChangeFile{Path: rel, Kind: "added", Binary: binary}
		if readErr == nil && !binary {
			file.Additions = countLines(content, size)
		}
		files = append(files, file)
		patch.WriteString(filePatch)
		if wasTruncated || patch.Len() >= remaining {
			truncated = true
			break
		}
	}
	return patch.String(), files, truncated, nil
}

func readReviewContent(ctx context.Context, workDir, ref, pathValue string) (types.WorkspaceReviewFileContent, error) {
	if pathValue == "" || pathValue == "/dev/null" {
		return types.WorkspaceReviewFileContent{}, nil
	}
	rel, err := cleanRelativePath(pathValue)
	if err != nil {
		return types.WorkspaceReviewFileContent{}, err
	}
	if ref == "" {
		resolved, _, err := resolveWorkspaceFile(workDir, rel)
		if err != nil {
			if os.IsNotExist(err) {
				return types.WorkspaceReviewFileContent{Path: rel}, nil
			}
			return types.WorkspaceReviewFileContent{}, err
		}
		content, size, truncated, binary, err := readBoundedFile(resolved, workspaceFileLimit)
		return types.WorkspaceReviewFileContent{Path: rel, Contents: content, ByteLength: size, Truncated: truncated, Binary: binary}, err
	}
	if _, err := safeGitArg(ref); err != nil {
		return types.WorkspaceReviewFileContent{}, err
	}
	specArg, err := safeGitPathArg(ref + ":" + rel)
	if err != nil {
		return types.WorkspaceReviewFileContent{}, err
	}
	out, truncated, err := runGitBounded(ctx, workDir, workspaceFileLimit, nil, constGitArg("show"), specArg)
	if err != nil {
		return types.WorkspaceReviewFileContent{Path: rel}, nil
	}
	binary := !utf8.ValidString(out) || bytes.IndexByte([]byte(out), 0) >= 0
	if binary {
		out = ""
	}
	return types.WorkspaceReviewFileContent{Path: rel, Contents: out, ByteLength: int64(len(out)), Truncated: truncated, Binary: binary}, nil
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

func runGitBoundedAllowExitOne(ctx context.Context, cwd string, limit int, args ...gitArg) (string, bool, error) {
	return runGitBoundedExit(ctx, cwd, limit, nil, true, args...)
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

func mergeCodeChangeFiles(left, right []types.InteractionCodeChangeFile) []types.InteractionCodeChangeFile {
	byPath := make(map[string]types.InteractionCodeChangeFile, len(left)+len(right))
	for _, file := range append(left, right...) {
		existing, ok := byPath[file.Path]
		if ok {
			file.Additions += existing.Additions
			file.Deletions += existing.Deletions
			file.Binary = file.Binary || existing.Binary
		}
		byPath[file.Path] = file
	}
	files := make([]types.InteractionCodeChangeFile, 0, len(byPath))
	for _, file := range byPath {
		files = append(files, file)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files
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

func countLines(content string, size int64) int {
	if size == 0 {
		return 0
	}
	count := strings.Count(content, "\n")
	if !strings.HasSuffix(content, "\n") {
		count++
	}
	return count
}

func validateReviewRef(value string) (gitArg, error) {
	if strings.HasPrefix(value, "-") {
		return gitArg{}, fmt.Errorf("invalid Git ref")
	}
	return safeGitArg(value)
}
