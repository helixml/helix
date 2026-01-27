# Replace go-git with Native Git (via gitea/gitcmd)

**Date:** 2026-01-27
**Status:** Phase 1 Complete (HTTP Server)
**Author:** Claude + Luke

## Problem

go-git v6 has critical performance and reliability issues:

1. **Deadlock in file transport** - The file transport uses `io.Pipe` with zero buffer. When cloning repos with many branches (322+), the pack negotiation overflows the pipe buffer, causing both reader and writer to block indefinitely. We patched this with a buffered pipe in our fork.

2. **Unusable pack generation performance** - `transport.UploadPack` takes 77 seconds for qwen-code (31k objects) and 16+ minutes for zed (still running). This makes clone operations via our git HTTP server unusable for large repos.

3. **Accumulating trust issues** - We keep finding problems. The codebase is complex and pure-Go reimplementation of git has edge cases.

## Solution

Replace go-git entirely with native git execution via gitea's `gitcmd` package.

### Why gitea's gitcmd?

1. **Battle-tested** - Gitea has 45k+ GitHub stars, used in production by thousands of organizations
2. **Pure Go wrapper** - Shells out to native git, so we get native git's performance and correctness
3. **Clean API** - `gitcmd.Command` with `WithStdinCopy()`, `WithStdoutCopy()`, `WithEnv()` for HTTP protocol
4. **No CGO** - Unlike git2go/libgit2 which has reliability issues (FluxCD deprecated it)
5. **ADO support** - Native git supports Azure DevOps Git v2 protocol

### What we're NOT doing

- NOT using random small libraries like gitkit (300 stars, unproven)
- NOT reimplementing git-http-backend ourselves (we tried, it was hell)

### Could we use gitea's full context.Context?

Gitea's `context.Context` (from `code.gitea.io/gitea/services/context`) includes:
- Chi router integration
- Gitea's database models (repo_model, auth_model, etc.)
- Gitea's auth system and middleware
- Their settings/configuration system

**To use it fully, we'd need to:**
- Switch from gorilla/mux to chi
- Adopt gitea's GORM-based database models
- Use gitea's auth middleware
- Essentially embed gitea as a library

**Recommendation:** Use gitea's **public git packages** (`modules/git`, `modules/git/gitcmd`) but keep our own HTTP layer. This gives us:
- Battle-tested git command execution
- Our existing auth/RBAC integration
- Our custom post-push hooks
- Minimal migration effort

If we find ourselves fighting this split, we can revisit full gitea integration later.

## Architecture

### Current (go-git)

```
HTTP Request → GitHTTPServer → go-git transport.UploadPack → SLOW/DEADLOCK
                             → go-git transport.ReceivePack
```

### New (native git via gitcmd)

```
HTTP Request → GitHTTPServer → gitcmd.NewCommand("upload-pack", "--stateless-rpc")
                             → native git process (fast, correct)
                             → stdin/stdout piped to HTTP request/response
```

## Files to Modify

### Phase 1: HTTP Server (Critical Path)

**Replace:** `api/pkg/services/git_http_server_gogit.go` (1366 lines)
**With:** `api/pkg/services/git_http_server.go`

Key changes:
- Remove go-git imports
- Add `code.gitea.io/gitea/modules/git/gitcmd` import
- Replace `transport.UploadPack()` with `gitcmd.NewCommand("upload-pack", "--stateless-rpc", ".")`
- Replace `transport.ReceivePack()` with `gitcmd.NewCommand("receive-pack", "--stateless-rpc", ".")`
- Keep all custom post-push hooks (design docs, PR creation, branch restrictions)

### Phase 2: Repository Operations

**Files:**
- `api/pkg/services/git_repository_service.go` (2526 lines)
- `api/pkg/services/git_repository_service_pull.go` (466 lines)
- `api/pkg/services/git_repository_service_push.go` (276 lines)
- `api/pkg/services/git_repository_service_contents.go` (482 lines)
- `api/pkg/services/project_internal_repo_service.go` (873 lines)

Replace go-git operations with gitea's `git` package:
- `git.Clone()` instead of `git.PlainClone()`
- `git.Push()` instead of `repo.Push()`
- `git.InitRepository()` instead of `git.PlainInit()`
- Use `gitcmd` for custom operations

### Phase 3: Cleanup

- Remove go-git fork from go.mod
- Remove `api/pkg/git/git_manager.go` (381 lines)
- Remove go-git helper code

## Custom Hooks to Preserve

Our git HTTP server has significant custom business logic that MUST be preserved:

### Authentication & Authorization
- `authMiddleware()` - API key extraction and validation
- `hasReadAccess()` / `hasWriteAccess()` - RBAC checks
- `getBranchRestrictionForAPIKey()` - Agent branch restrictions (agents can only push to their assigned branches)

### Pre-Push Sync
- Before serving upload-pack for external repos, sync from upstream (GitHub/ADO)
- `SyncAllBranches()` call ensures clients get latest data

### Post-Push Hooks (receive-pack)
These run AFTER a successful push:

1. **Branch hash tracking** - `getBranchHashes()` before, compare after to detect changes
2. **Design doc processing** - `processDesignDocsForBranch()`:
   - Parse `design/*.md` files for task references
   - Update spec tasks with implementation status
3. **Feature branch handling** - `handleFeatureBranchPush()`:
   - Create design reviews
   - Trigger PR creation for external repos
4. **Main branch handling** - `handleMainBranchPush()`:
   - Update task states when PRs are merged
5. **Rollback on hook failure** - `rollbackBranchRefs()` if post-push processing fails

### Ref Advertisement Filtering
- We may need to filter refs based on branch restrictions (future)

## Implementation Plan

### Step 1: Add gitea dependency

```bash
go get code.gitea.io/gitea/modules/git
go get code.gitea.io/gitea/modules/git/gitcmd
```

### Step 2: Create new HTTP server

Create `git_http_server.go` with:

```go
package services

import (
    "code.gitea.io/gitea/modules/git/gitcmd"
    // ... our existing imports
)

func (s *GitHTTPServer) handleUploadPack(w http.ResponseWriter, r *http.Request) {
    // ... auth, repo lookup (keep existing) ...

    // Sync from upstream for external repos (keep existing)
    if repo.ExternalURL != "" {
        s.gitRepoService.SyncAllBranches(ctx, repoID, false)
    }

    // NEW: Use native git via gitcmd
    cmd := gitcmd.NewCommand("upload-pack", "--stateless-rpc", ".")

    opts := &gitcmd.RunOpts{
        Dir:    repoPath,
        Stdin:  r.Body,
        Stdout: w,
        Env:    environ,
    }

    if err := cmd.Run(r.Context(), opts); err != nil {
        log.Error().Err(err).Msg("upload-pack failed")
    }
}

func (s *GitHTTPServer) handleReceivePack(w http.ResponseWriter, r *http.Request) {
    // ... auth, repo lookup, branch restrictions (keep existing) ...

    // Capture branch hashes BEFORE push
    beforeHashes := s.getBranchHashes(repoPath)

    // NEW: Use native git via gitcmd
    cmd := gitcmd.NewCommand("receive-pack", "--stateless-rpc", ".")

    opts := &gitcmd.RunOpts{
        Dir:    repoPath,
        Stdin:  r.Body,
        Stdout: w,
        Env:    environ,
    }

    if err := cmd.Run(r.Context(), opts); err != nil {
        log.Error().Err(err).Msg("receive-pack failed")
        return
    }

    // Capture branch hashes AFTER push
    afterHashes := s.getBranchHashes(repoPath)
    changedBranches := s.detectChangedBranches(beforeHashes, afterHashes)

    // Run post-push hooks (keep existing logic)
    s.handlePostPushHook(ctx, repoID, repoPath, changedBranches)
}
```

### Step 3: Update getBranchHashes to use gitcmd

Replace go-git ref iteration with:

```go
func (s *GitHTTPServer) getBranchHashes(repoPath string) map[string]string {
    hashes := make(map[string]string)

    output, _, err := gitcmd.NewCommand("for-each-ref", "--format=%(refname:short) %(objectname)", "refs/heads/").
        WithDir(repoPath).
        RunStdString(context.Background())

    if err != nil {
        return hashes
    }

    for _, line := range strings.Split(output, "\n") {
        parts := strings.SplitN(line, " ", 2)
        if len(parts) == 2 {
            hashes[parts[0]] = parts[1]
        }
    }

    return hashes
}
```

### Step 4: Test with large repos

1. Restart API
2. Clone qwen-code - should be <5 seconds (was 77 seconds)
3. Clone zed - should be <30 seconds (was 16+ minutes, still running)
4. Test push with design docs - verify hooks fire

### Step 5: Migrate repository service operations

Replace go-git in git_repository_service*.go files.

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Native git not installed | Docker images already have git installed |
| gitcmd API changes | Pin to specific gitea version |
| Hook environment differences | Test thoroughly, compare env vars |
| Performance regression in other areas | Benchmark before/after |

## Success Criteria

1. Clone qwen-code in <10 seconds (was 77 seconds)
2. Clone zed in <60 seconds (was 16+ minutes)
3. All existing tests pass
4. Post-push hooks fire correctly
5. Branch restrictions work
6. External repo sync still works

## Progress

### 2026-01-27: Phase 1 Complete

**Completed:**
- ✅ Added gitea/gitcmd dependency
- ✅ Created new `git_http_server.go` using native git via gitcmd
- ✅ Replaced go-git's slow `transport.UploadPack` with `git upload-pack --stateless-rpc`
- ✅ Replaced go-git's `transport.ReceivePack` with `git receive-pack --stateless-rpc`
- ✅ Preserved all custom hooks (branch restrictions, upstream sync, post-push processing)
- ✅ Added proper gitcmd initialization (`gitcmd.SetExecutablePath()`)
- ✅ Backed up old go-git implementation to `git_http_server_gogit.go.bak`
- ✅ Build passes for services and server packages

**Key Implementation Notes:**
1. Used gitea's builder pattern: `gitcmd.NewCommand("upload-pack").AddArguments("--stateless-rpc").AddDynamicArguments(".")`
2. String literals work with gitcmd (implicit conversion to `internal.CmdArg`)
3. String variables must use `AddDynamicArguments()` for safety
4. Protocol v2 supported via `GIT_PROTOCOL` environment variable passthrough

**Pending:**
- [ ] Test with large repos (qwen-code: 31k objects, zed: 600k+ objects)
- [ ] Phase 2: Migrate git repository service operations (Clone, Push, etc.)
- [ ] Phase 3: Remove go-git dependency entirely

## References

- Gitea githttp.go: https://github.com/go-gitea/gitea/blob/main/routers/web/repo/githttp.go
- gitcmd package: https://pkg.go.dev/code.gitea.io/gitea/modules/git/gitcmd
- Git Smart HTTP protocol: https://git-scm.com/docs/http-protocol
