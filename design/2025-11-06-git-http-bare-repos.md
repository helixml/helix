# Git HTTP Server & Bare Repository Architecture

**Date:** 2025-11-06
**Context:** SpecTask agent workflow - agents need to clone code and push design docs

## Problem Statement

Agents running in Wolf containers need to:
1. Clone code repositories to analyze existing codebase
2. Create design documents in `helix-design-docs` branch
3. Push design docs back to Helix so UI can display them
4. Later, push implementation code changes

**Initial approach (filesystem paths):**
- Cloned repos using `git clone /filestore/repos/{id}`
- Required mounting `/filestore` into every Wolf container
- Security nightmare (agents access all repos)
- Doesn't scale across multiple hosts

## Solution: Git HTTP Server + Bare Repositories

### Architecture

```
┌─────────────────────────────────────────────────────────┐
│ Wolf Container (Agent Session)                          │
│                                                          │
│  ~/work/my-repo/          ← git clone http://api:8080   │
│  ~/work/helix-design-docs ← git worktree                │
│                                                          │
│  git push origin helix-design-docs                      │
│         │                                                │
│         └─→ Uses credentials from ~/.git-credentials    │
│             (configured by startup script)              │
└─────────────────────────────┬───────────────────────────┘
                              │ HTTP POST
                              │ /git/{repo-id}/git-receive-pack
                              │ Auth: api:{runner-token}
┌─────────────────────────────▼───────────────────────────┐
│ Helix API Server                                        │
│                                                          │
│  GitHTTPServer                                          │
│  ├─ GET  /git/{id}/info/refs                            │
│  ├─ POST /git/{id}/git-upload-pack   (clone/fetch)      │
│  └─ POST /git/{id}/git-receive-pack  (push)             │
│                                                          │
│  Auth: Validates runner token via API key system        │
│  Executes: git receive-pack --stateless-rpc {bare-repo} │
└─────────────────────────────┬───────────────────────────┘
                              │
┌─────────────────────────────▼───────────────────────────┐
│ Filestore - Bare Repositories                           │
│                                                          │
│  /filestore/repos/{repo-id}/          (bare)            │
│  /filestore/projects/{id}/repo/       (bare, internal)  │
│                                                          │
│  ✅ Accept pushes from agents                           │
│  ✅ No checked-out branch (no conflicts)                │
└─────────────────────────────────────────────────────────┘
```

### Bare Repository Format

**All filestore repos are bare:**

```bash
# Bare repo structure
/filestore/repos/{repo-id}/
├── HEAD
├── config
├── description
├── hooks/
├── info/
├── objects/
└── refs/
```

**No working tree** - files only exist in git object database.

**Why bare?**
- Accept unlimited pushes without branch conflicts
- No "refusing to update checked out branch" errors
- Industry standard for git servers
- Cleaner separation: filestore = storage, agents = working copies

### API Server Operations (Read/Write to Bare Repos)

**Reading (fast):**
```go
repo := git.PlainOpen(bareRepoPath)
ref := repo.Head()
commit := repo.CommitObject(ref.Hash())
tree := commit.Tree()
file := tree.File(".helix/startup.sh")
content := file.Contents()  // Read directly from object database
```

**Writing (temp clone workflow):**
```go
// Create unique temp directory
tempClone := os.MkdirTemp("", "helix-operation-*")
defer os.RemoveAll(tempClone)

// Clone from bare repo
repo := git.PlainClone(tempClone, false, {URL: bareRepoPath})

// Modify files
os.WriteFile(tempClone + "/file.txt", content, 0644)

// Commit and push
worktree.Add("file.txt")
worktree.Commit("Update file")
repo.Push()  // Pushes back to bare repo
```

### Agent Workflow

**1. Container Startup (start-zed-helix.sh):**

```bash
# Configure git credentials for HTTP push
git config --global credential.helper 'store --file ~/.git-credentials'
echo "http://api:${HELIX_API_TOKEN}@api:8080" > ~/.git-credentials
chmod 600 ~/.git-credentials

# Set up design docs worktree
git -C ~/work/{primary-repo} worktree add ~/work/helix-design-docs helix-design-docs
```

**2. Repositories Already Cloned:**

API server clones before container starts:
```bash
# Inside API container (has /filestore mounted)
git clone http://api:{token}@api:8080/git/{repo-id} /filestore/workspaces/spectasks/{task-id}/{repo-name}
```

Wolf container mounts the workspace:
```bash
-v /filestore/workspaces/spectasks/{task-id}:/home/retro/work
```

**3. Agent Pushes Design Docs:**

```bash
cd ~/work/helix-design-docs/tasks/2025-11-06_feature_abc123/
git add .
git commit -m "Add design docs"
git push origin helix-design-docs
# ✅ Uses credentials from ~/.git-credentials
# ✅ Authenticates with runner token
# ✅ Push succeeds to bare repo
```

### Authentication

**Token Used:** Server's `RunnerToken` (from config)
- **NOT** user-specific API keys
- Shared administrative token for all agent operations
- Long-lived, stable, rarely revoked
- If revoked: ALL agents stop (intentional security feature)

**Credential Storage:**
- Location: `~/.git-credentials` (inside container, ephemeral)
- Created by startup script on every container start
- Auto-cleaned when container destroyed
- **Not** in persistent workspace (prevents accidental commits)

**Why ephemeral is good:**
- Fresh credentials on every container restart
- Token changes propagate automatically
- Can't be accidentally committed to git
- Reduces security blast radius

### Git HTTP Protocol Routes

```
GET  /git/{repo-id}/info/refs?service=git-upload-pack
→ Returns available refs for clone/fetch

POST /git/{repo-id}/git-upload-pack
→ Sends pack file for clone/fetch (read operation)

POST /git/{repo-id}/git-receive-pack
→ Receives pack file for push (write operation)
```

**Implementation:**
- Uses standard `git upload-pack` and `git receive-pack` binaries
- `--stateless-rpc` flag for HTTP compatibility
- Auth middleware validates runner token before executing git commands

### Benefits

**Security:**
- No filesystem mounts needed (agents can't access all repos)
- Token-based authentication (standard HTTP)
- Ephemeral credentials (auto-refresh)
- Works across network boundaries (multi-host deployments)

**Reliability:**
- Bare repos never have push conflicts
- HTTP is standard git protocol (tested, reliable)
- Unique temp directories prevent concurrent operation conflicts
- Proper cleanup with defer statements

**Scalability:**
- API server and Wolf can be on different hosts
- No shared filesystem required
- Standard HTTP load balancing works
- Can add caching/CDN later

### Potential Issues

**1. Token Revocation:**
- If `RunnerToken` is revoked, ALL agents immediately lose access
- Mitigation: This is intentional - global security kill switch
- Container restart picks up new token automatically

**2. Empty Bare Repos:**
- Newly created bare repos have no commits yet
- Cloning immediately after creation fails
- Mitigation: Always create initial commit via temp clone

**3. Credential Persistence:**
- Credentials lost on container restart
- Mitigation: Startup script recreates them (good for security)
- Not a bug, it's a feature!

**4. Network Performance:**
- HTTP clone/push slower than filesystem operations
- Mitigation: Local network, minimal latency
- Can optimize with git shallow clones (already using `--depth 1` in some places)

### Testing Checklist

- [ ] Create new project from sample
- [ ] Modify startup script in UI
- [ ] Create spec task
- [ ] Verify repositories cloned via HTTP
- [ ] Verify worktree created at `~/work/helix-design-docs`
- [ ] Agent creates design docs
- [ ] Agent commits and pushes successfully
- [ ] Helix UI displays pushed design docs

### Future Work: Multi-User SpecTasks & Agent Identity

**Current Limitation:**
Agents use server's `RunnerToken` for all git operations. This works for single-user scenarios but has limitations:

**Problem for multi-user SpecTasks:**
- SpecTasks can be shared across organization members
- Multiple users collaborate on same spec task
- Current design: All git commits use `RunnerToken`
- No attribution to specific user who initiated the action

**Future Solution - Agent Service Accounts:**

When SpecTasks are organization-owned and multi-user:

1. **Create agent-specific service accounts:**
   ```go
   agentIdentity := CreateAgentIdentity(specTask.ID, organization.ID)
   // Returns: API key specific to this agent
   ```

2. **Scope permissions to organization repos:**
   - Agent identity has read/write to organization's repos only
   - Can't access other organizations' repos
   - Audit trail shows which agent made which commits

3. **Rotate tokens per session:**
   - Each agent session gets fresh token
   - Token invalidated when session ends
   - Prevents long-lived credential leaks

4. **Git commit attribution:**
   ```
   Author: Helix Agent (SpecTask: {id}) <agent-{id}@helix.ml>
   Committer: User: {username} <{email}>
   ```

**Not implemented yet** - current single-user workflow is sufficient.
Will add when organization-shared SpecTasks are needed.

### Future Optimizations

**If needed:**
- Cache cloned repos per-agent (reuse across tasks)
- Implement git protocol v2 (faster)
- Add git LFS support for large files
- Batch operations (clone multiple repos in parallel)

**Not needed yet:**
- Direct git object database writes (complex, marginal benefit)
- Custom git protocol (HTTP works fine)
- Persistent credential cache (ephemeral is more secure)
