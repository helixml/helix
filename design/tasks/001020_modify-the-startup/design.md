# Design: Rename Repo Directories in helix-specs Startup

## Overview

Modify the helix-specs startup script to rename numbered repo directories to their canonical names so the `./stack` script works correctly.

## Root Cause

The API's `CreateRepository` function in `api/pkg/services/git_repository_service.go` (lines 135-143) auto-increments repository names when a user already has a repo with that name:

```go
// Auto-increment name if it already exists
baseName := request.Name
uniqueName := baseName
suffix := 1
for existingNames[uniqueName] {
    uniqueName = fmt.Sprintf("%s-%d", baseName, suffix)
    suffix++
}
```

So if you already have `helix`, `zed`, `qwen-code` repos, new ones become `helix-1`, `zed-1`, `qwen-code-1`.

## Why This Matters

The `./stack` script uses `$PROJECTS_ROOT` (parent of helix) and expects:
- `$PROJECTS_ROOT/zed`
- `$PROJECTS_ROOT/qwen-code`

When repos are cloned with numbered names, `./stack build-zed` and other commands fail.

## Solution

Add a renaming step to `helix-specs/.helix/startup.sh` that:
1. Detects numbered directories (e.g., `helix-1`, `zed-2`, `qwen-code-3`)
2. Renames them to canonical names (`helix`, `zed`, `qwen-code`)
3. Handles edge cases (already correct, target exists, etc.)

## Implementation

```bash
# Add to startup.sh before finding HELIX_DIR
cd ~/work

# Rename numbered repos to canonical names
for pattern in "helix-" "zed-" "qwen-code-"; do
    canonical="${pattern%-}"  # Remove trailing dash
    for numbered in ${pattern}[0-9]*; do
        [ -d "$numbered" ] || continue
        if [ ! -e "$canonical" ]; then
            mv "$numbered" "$canonical"
            echo "Renamed $numbered → $canonical"
        fi
    done
done
```

## Verification (Already Done)

Checked helix codebase for numbered repo assumptions:
- **Prompts**: All use `/home/retro/work/helix-specs/` (correct)
- **sample_project_code_service.go**: Clones as `helix`, `zed`, `qwen-code` (correct)
- **helix-dev-setup.sh**: Clones as `helix`, `zed`, `qwen-code` (correct)
- **stack script**: Uses `$PROJECTS_ROOT/zed`, `$PROJECTS_ROOT/qwen-code` (correct)

No code assumes numbered names. The `zed-1`, `zed-2` in server.go are agent IDs, not repo names.