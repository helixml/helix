# Hydra Bind-Mount Solution

**Date:** 2025-12-01
**Updated:** 2025-12-10
**Status:** Implemented (updated to use double bind mount instead of symlinks)

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

There are TWO separate Docker volumes:
- **helix-filestore**: Mounted to API container at `/filestore`
- **sandbox-data**: Mounted to sandbox container at `/data`

```
┌─────────────────────────────────────────────────────────────────────┐
│ API Container                                                        │
│   helix-filestore volume → /filestore/workspaces/...                │
│   (git clone happens here)                                          │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│ Sandbox Container                                                    │
│                                                                      │
│   sandbox-data volume → /data/workspaces/spec-tasks/{id}/           │
│                                                                      │
│   ┌──────────────────────────────────────────────────────────────┐  │
│   │ Hydra dockerd (per-session)                                  │  │
│   │   - Sees sandbox filesystem                                  │  │
│   │   - Can access /data/workspaces/...                          │  │
│   └──────────────────────────────────────────────────────────────┘  │
│                                                                      │
│   ┌──────────────────────────────────────────────────────────────┐  │
│   │ Dev Container (helix-sway)                                   │  │
│   │   - /data/workspaces/spec-tasks/{id}/ (bind mount)           │  │
│   │   - /home/retro/work (SAME dir, second bind mount)           │  │
│   │   - Docker CLI connects to Hydra's socket                    │  │
│   │   - Startup script clones repos on first boot                │  │
│   └──────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
```

## Solution

~~Use Docker's symlink resolution behavior combined with a dedicated sandbox-data volume.~~

**Updated (2025-12-10):** Now uses double bind mount instead of symlinks. This eliminates issues where tools (like Qwen Code's file operations) resolve symlinks and get confused about paths.

**Key insight:** Mount the same directory at TWO paths - both the actual data path AND the user-friendly path.

### Implementation

1. **Sandbox container mounts sandbox-data volume at `/data`**
   ```yaml
   # docker-compose.dev.yaml
   sandbox:
     volumes:
       - sandbox-data:/data
   ```

2. **Mount workspace at BOTH paths in dev container (wolf_executor.go):**
   ```go
   mounts := []string{
       fmt.Sprintf("%s:%s", workspaceDir, workspaceDir),       // e.g., /data/workspaces/spec-tasks/{id}
       fmt.Sprintf("%s:/home/retro/work", workspaceDir),       // Same dir at user-friendly path
   }
   ```

3. ~~**Create symlink in dev container:**~~ **NO LONGER NEEDED**
   ```bash
   # OLD: ln -sf /data/workspaces/spec-tasks/{id} /home/retro/work
   # NEW: Wolf executor handles this with double bind mount
   ```

4. **User experience unchanged:**
   ```bash
   cd /home/retro/work        # Works (real directory, not symlink)
   docker run -v /home/retro/work/foo:/app ...  # Works!
   ```

5. **How it works:**
   - User runs `docker run -v /home/retro/work/foo:/app`
   - Docker CLI sees `/home/retro/work` as a REAL directory (bind mount, not symlink)
   - Docker wrapper script (`docker-wrapper.sh`) resolves to actual path via readlink
   - Actual path sent to daemon: `/data/workspaces/spec-tasks/{id}/foo`
   - Hydra's dockerd CAN access this path (sandbox has `/data` mounted)
   - Bind mount succeeds!

6. **Why double bind mount is better than symlinks:**
   - Tools don't get confused by symlink resolution
   - `/home/retro/work` appears as a real directory in all contexts
   - No race conditions with symlink creation
   - Fail-fast if mount is missing (no silent fallbacks)

7. **Workspace initialization:**
   - Docker creates empty directories when bind-mounting non-existent paths
   - The startup script (start-zed-helix.sh) clones repos into the workspace on first boot
   - API container does NOT need write access to `/data` - workspace init is inside sandbox

## Code Changes

### 1. docker-compose.dev.yaml - Add sandbox-data volume

```yaml
sandbox:
  volumes:
    - sandbox-data:/data
  environment:
    - SANDBOX_DATA_PATH=/data

volumes:
  sandbox-data:  # Workspace data for dev containers
```

### 2. wolf_executor.go - Translate paths

```go
func (w *WolfExecutor) translateToHostPath(containerPath string) string {
    // Convert /filestore/workspaces/... to /data/workspaces/...
    // This maps API container paths to sandbox container paths
    if strings.HasPrefix(containerPath, "/filestore/workspaces/") {
        return strings.Replace(containerPath, "/filestore/workspaces/", "/data/workspaces/", 1)
    }
    return containerPath
}
```

### 3. wolf_executor.go - Mount at BOTH paths (Updated 2025-12-10)

```go
mounts := []string{
    fmt.Sprintf("%s:%s", config.WorkspaceDir, config.WorkspaceDir),   // Actual data path
    fmt.Sprintf("%s:/home/retro/work", config.WorkspaceDir),          // User-friendly path
}
```

### 4. wolf_executor.go - Add WORKSPACE_DIR env var

```go
env = append(env, fmt.Sprintf("WORKSPACE_DIR=%s", config.WorkspaceDir))
```

### 5. startup-app.sh - Verify mounts exist (Updated 2025-12-10)

```bash
# No longer create symlinks - Wolf executor handles double bind mount
# Just verify both paths exist and fail fast if not
if [ -z "$WORKSPACE_DIR" ]; then
    echo "FATAL: WORKSPACE_DIR environment variable not set"
    exit 1
fi
if [ ! -d "$WORKSPACE_DIR" ]; then
    echo "FATAL: WORKSPACE_DIR does not exist: $WORKSPACE_DIR"
    exit 1
fi
if [ ! -d /home/retro/work ]; then
    echo "FATAL: /home/retro/work bind mount not present"
    exit 1
fi
```

### 6. install.sh - Add sandbox-data volume for production

```bash
# sandbox.sh script includes:
-v sandbox-data:/data \
-e SANDBOX_DATA_PATH=/data \
```

## Security Considerations

- Only the specific task's workspace directory is mounted, not all of `/data`
- Each dev container has its own isolated symlink
- No additional permissions required
- API container cannot write to sandbox data (separate volumes)

## Scope

This solution works for:
- **Local dev (docker-compose.dev.yaml):** Sandbox has `sandbox-data:/data` mounted
- **Production sandboxes:** Each has its own `sandbox-data` volume

## Testing

1. Start a spec task with Hydra enabled
2. In the dev container terminal:
   ```bash
   cd /home/retro/work
   echo "test" > testfile.txt
   docker run --rm -v /home/retro/work:/mnt alpine cat /mnt/testfile.txt
   ```
3. Should output: `test`
