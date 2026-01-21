# Git Sync and Security Architecture

**Date:** 2026-01-21
**Status:** Implemented
**Author:** Claude Code

## Overview

This document describes the security architecture for Git operations in Helix, specifically focusing on:

1. How the "middle repository" stays synchronized with upstream external repositories
2. How agent push permissions are enforced via spec_task branch restrictions
3. How conflicts are prevented through synchronous upstream sync with rollback

## Architecture

### Repository Topology

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   Upstream      │     │  Helix Middle   │     │   Agent/User    │
│   Repository    │◄───►│   Repository    │◄───►│   Local Repo    │
│  (GitHub, ADO)  │     │   (Bare Repo)   │     │  (In Sandbox)   │
└─────────────────┘     └─────────────────┘     └─────────────────┘
     External              Helix-hosted            Desktop/Zed
```

**Middle Repository Purpose:**
- Acts as an intermediary between external upstream repos and agents
- Provides Helix-controlled Git HTTP server with authentication
- Enables branch-level access control per spec_task
- Allows synchronization to be managed server-side

### Branch Directionality

| Branch Type | Direction | Description |
|-------------|-----------|-------------|
| Default branch (main/master) | PULL only | Always syncs FROM upstream. Agents cannot push here. |
| Spec task branches (feature/*) | PUSH only | Agent changes push TO upstream. Synced before accepting pushes. |

## API Key Types and Permissions

### Key Classification

```go
type BranchRestriction struct {
    IsAgentKey    bool   // True if this is a session-scoped agent key
    AllowedBranch string // The only branch the agent can push to
    ErrorMessage  string // Error message if push should be denied
}
```

### Permission Matrix

| API Key Type | Identified By | Can Pull? | Can Push? | Branch Restriction |
|--------------|---------------|-----------|-----------|-------------------|
| Regular user key | No SessionID or SpecTaskID | Yes | Yes | None |
| Agent key with spec_task | SessionID or SpecTaskID present | Yes | Yes | Only task's BranchName |
| Agent key without spec_task | SessionID present, no SpecTaskID | Yes | **No** | Error returned |
| Agent key with task, no branch | SpecTaskID present, task.BranchName empty | Yes | **No** | Error returned |

### Error Messages

- **No spec_task:** "In order to make changes to this Git repo, please create a spec_task."
- **No branch assigned:** "Spec task does not have a branch assigned. Cannot push to Git."
- **Wrong branch:** Push is silently rolled back (agent will see refs unchanged on next fetch)

## Sync Flow

### On Pull (git clone / git fetch)

```
┌─────────────────────────────────────────────────────────────────┐
│                     handleUploadPack                             │
├─────────────────────────────────────────────────────────────────┤
│ 1. Authenticate user via API key                                │
│ 2. Check read access to repository                              │
│ 3. IF external repo:                                            │
│    └─ Sync all branches from upstream (non-blocking on failure) │
│ 4. Serve git-upload-pack response                               │
└─────────────────────────────────────────────────────────────────┘
```

**Sync failure handling:** If upstream sync fails, we log a warning and serve cached data. This ensures users can still pull even if upstream is temporarily unavailable.

### On Push (git push)

```
┌─────────────────────────────────────────────────────────────────┐
│                     handleReceivePack                            │
├─────────────────────────────────────────────────────────────────┤
│ 1. Authenticate user via API key                                │
│ 2. Check write access to repository                             │
│ 3. IF external repo:                                            │
│    └─ Sync all branches from upstream                           │
│    └─ REJECT push if sync fails (HTTP 409)                      │
│ 4. Record current ref states (branchesBefore)                   │
│ 5. Accept push via transport.ReceivePack                        │
│ 6. Detect which branches were pushed (branchesAfter - before)   │
│ 7. Check branch restrictions:                                   │
│    ├─ Get BranchRestriction for API key                         │
│    ├─ IF agent key without spec_task → ROLLBACK                 │
│    ├─ IF agent key pushing wrong branch → ROLLBACK              │
│    └─ IF regular user key → no restriction                      │
│ 8. IF external repo:                                            │
│    ├─ Push each branch to upstream (SYNCHRONOUS)                │
│    └─ IF upstream push fails → ROLLBACK                         │
│ 9. Run post-push hooks (async) - only if everything succeeded   │
└─────────────────────────────────────────────────────────────────┘
```

## Rollback Mechanism

When a push needs to be rolled back (unauthorized branch, upstream push failure, etc.):

```go
func rollbackBranchRefs(st storage.Storer, previousHashes map[string]string, branches []string) {
    for _, branch := range branches {
        if prevHash, existed := previousHashes[branch]; existed {
            // Branch existed before - restore to previous hash
            ref := plumbing.NewHashReference(refName, plumbing.NewHash(prevHash))
            st.SetReference(ref)
        } else {
            // Branch was newly created - delete it
            st.RemoveReference(refName)
        }
    }
}
```

**Important:** The git client may see a "successful" push (the HTTP response completes), but the refs are rolled back. On the next fetch or push, the client will see that their refs weren't actually updated and will need to retry.

## Conflict Prevention

### The Conflict Window

Without synchronous upstream push, there's a window for conflicts:

```
Timeline without sync:
T1: Agent pushes to helix ────────────────────────────────────────►
T2: User pushes to upstream ──────────────────────────────────────►
T3: Helix async push to upstream → CONFLICT (silent failure)
```

With synchronous push and rollback:

```
Timeline with sync:
T1: Sync from upstream ─────┐
T2: Accept agent push ──────┤ (atomic window ~ms)
T3: Push to upstream ───────┘
    └─ If fails: ROLLBACK, agent retries with fresh data
```

### Guarantees

1. **Middle repo never diverges from upstream** - rollback ensures this
2. **Agents see conflicts immediately** - on next fetch/push, not silently lost
3. **No accumulated state** - failed pushes don't leave partial changes
4. **Branch isolation** - agents can only affect their assigned branch

## Security Considerations

### Agent Isolation

Each agent session receives an API key scoped to:
- A specific SessionID
- A specific SpecTaskID
- Therefore, a specific BranchName

This prevents:
- Agent A pushing to Agent B's branch
- Any agent pushing to main/default branch
- Agents without tasks making any changes

### API Key Lookup

```go
apiKeyRecord, _ := store.GetAPIKey(ctx, &types.ApiKey{Key: apiKey})

// Agent key if either is set
isAgentKey := apiKeyRecord.SessionID != "" || apiKeyRecord.SpecTaskID != ""

if isAgentKey {
    task, _ := store.GetSpecTask(ctx, apiKeyRecord.SpecTaskID)
    allowedBranch := task.BranchName
    // Enforce branch restriction
}
```

## Implementation Files

| File | Purpose |
|------|---------|
| `api/pkg/services/git_http_server_gogit.go` | Git HTTP server with sync and security logic |
| `api/pkg/types/types.go` | ApiKey struct with SessionID, SpecTaskID fields |
| `api/pkg/types/simple_spec_task.go` | SpecTask struct with BranchName field |

### Key Functions

- `handleReceivePack()` - Main push handler with all security checks
- `handleUploadPack()` - Pull handler with pre-fetch sync
- `getBranchRestrictionForAPIKey()` - Determines what an API key can push
- `rollbackBranchRefs()` - Restores refs to pre-push state
- `SyncAllBranches()` - Fetches all branches from upstream

## Frontend Integration

A "Sync from Upstream" button is available on the GitRepoDetail page for external repositories:

- Location: Header area, next to repository chips
- Visibility: Only for repos with `external_url` set
- Action: Calls `POST /api/v1/git/repositories/{id}/sync-all`
- Effect: Syncs all branches from upstream to helix middle repo

## Future Considerations

1. **Webhook-based sync** - Could add webhooks from upstream to trigger sync on external changes
2. **Selective branch sync** - Currently syncs all branches; could optimize to sync only affected branches
3. **Conflict notification** - Could notify users/agents when rollback occurs
4. **Audit logging** - Could log all branch restriction violations for security review
