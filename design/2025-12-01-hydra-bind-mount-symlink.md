# Hydra Bind-Mount Symlink Solution

**Date:** 2025-12-01
**Status:** Implementing

## Problem

When Hydra is enabled, each dev container (helix-sway) gets its own isolated dockerd instance running inside the sandbox container. When users try to bind-mount files from their workspace:

```bash
docker run -v /home/retro/work/mycode:/app myimage
```

This fails because:
1. The Docker CLI runs inside the dev container
2. The bind-mount path `/home/retro/work/mycode` is sent to Hydra's dockerd
3. Hydra's dockerd runs in the **sandbox container**, not the dev container
4. The path `/home/retro/work` doesn't exist in the sandbox - it only exists inside the dev container

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│ Sandbox Container                                               │
│                                                                 │
│   /filestore/workspaces/spec-tasks/{id}/  ← mounted volume     │
│                                                                 │
│   ┌─────────────────────────────────────────────────────────┐  │
│   │ Hydra dockerd (per-session)                             │  │
│   │   - Sees sandbox filesystem                             │  │
│   │   - Can access /filestore/...                           │  │
│   └─────────────────────────────────────────────────────────┘  │
│                                                                 │
│   ┌─────────────────────────────────────────────────────────┐  │
│   │ Dev Container (helix-sway)                              │  │
│   │   - /home/retro/work → user's workspace                 │  │
│   │   - Docker CLI connects to Hydra's socket               │  │
│   └─────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

## Solution

Use Docker's symlink resolution behavior to make paths consistent.

**Key insight:** Docker CLI resolves symlinks before sending mount paths to the daemon.

### Implementation

1. **Mount workspace at the SAME path in both sandbox and dev container:**
   ```
   /filestore/workspaces/spec-tasks/{id} → /filestore/workspaces/spec-tasks/{id}
   ```

2. **Create symlink in dev container:**
   ```bash
   ln -sf /filestore/workspaces/spec-tasks/{id} /home/retro/work
   ```

3. **User experience unchanged:**
   ```bash
   cd /home/retro/work        # Works (via symlink)
   docker run -v /home/retro/work/foo:/app ...  # Works!
   ```

4. **How it works:**
   - User runs `docker run -v /home/retro/work/foo:/app`
   - Docker CLI resolves symlink: `/home/retro/work` → `/filestore/workspaces/spec-tasks/{id}`
   - Actual path sent to daemon: `/filestore/workspaces/spec-tasks/{id}/foo`
   - Hydra's dockerd CAN access this path (sandbox has `/filestore` mounted)
   - Bind mount succeeds!

## Code Changes

### 1. wolf_executor.go - Mount at same path

**Before:**
```go
mounts := []string{
    fmt.Sprintf("%s:/home/retro/work", config.WorkspaceDir),
}
```

**After:**
```go
mounts := []string{
    fmt.Sprintf("%s:%s", config.WorkspaceDir, config.WorkspaceDir),
}
```

### 2. wolf_executor.go - Add WORKSPACE_DIR env var

```go
env = append(env, fmt.Sprintf("WORKSPACE_DIR=%s", config.WorkspaceDir))
```

### 3. startup-app.sh - Create symlink

```bash
# Create symlink for user-friendly path that also enables Hydra bind-mounts
if [ -n "$WORKSPACE_DIR" ] && [ -d "$WORKSPACE_DIR" ]; then
    ln -sfn "$WORKSPACE_DIR" /home/retro/work
fi
```

## Security Considerations

- Only the specific task's workspace directory is mounted, not all of `/filestore`
- Each dev container has its own isolated symlink
- No additional permissions required

## Scope

This solution works for:
- **Local dev (docker-compose.dev.yaml):** Sandbox has `/filestore` mounted
- **Production with shared storage:** If sandbox has filestore access

This does NOT work for:
- **Remote sandboxes without filestore:** Workspaces are ephemeral inside sandbox's DinD
  - This is a separate issue requiring workspace-in-sandbox architecture

## Testing

1. Start a spec task with Hydra enabled
2. In the dev container terminal:
   ```bash
   cd /home/retro/work
   echo "test" > testfile.txt
   docker run --rm -v /home/retro/work:/mnt alpine cat /mnt/testfile.txt
   ```
3. Should output: `test`
