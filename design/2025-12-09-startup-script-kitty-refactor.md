# Startup Script Refactor: Always Show Kitty Terminal

**Date:** 2025-12-09
**Status:** Implemented
**Files Changed:** `wolf/sway-config/start-zed-helix.sh`

## Problem

Customer reported Azure DevOps repo cloning failing silently in production. The Zed editor opened on `.` (empty work directory) instead of the cloned repository. No error was visible to the user.

### Root Causes Identified

1. **Kitty terminal only launched conditionally** - Only when `.helix/startup.sh` existed in the repo
2. **Clone failures were silent** - Script logged to file but user couldn't see errors
3. **Branch detection bug** - Script assumed `main` as default branch when `refs/remotes/origin/HEAD` wasn't set (Azure DevOps often doesn't set this for repos using `master`)

### Original Flow (Broken)

```
start-zed-helix.sh
    |
    |--> Log to file (user can't see)
    |--> Clone repos (failures logged but hidden)
    |--> Setup worktree
    |--> IF startup script exists:
    |        Launch kitty with wrapper
    |--> Launch Zed with folders (or fallback to ".")
```

If clone failed, user saw Zed open on empty directory with no explanation.

## Solution

### New Architecture

Move ALL setup work into kitty terminal so user can see progress and errors:

```
start-zed-helix.sh (main script)
    |
    |--> Minimal checks (Zed binary, WAYLAND_DISPLAY)
    |--> Generate setup script
    |--> ALWAYS launch kitty with setup script (background)
    |--> Wait for completion signal file
    |--> Read folder list from file
    |--> Launch Zed with folders

Setup script (runs in kitty, user sees output)
    |
    |--> Git configuration
    |--> Clone repositories (with visible errors)
    |--> Setup helix-specs worktree
    |--> Additional setup (Claude state, keybindings, SSH)
    |--> Determine Zed folders, write to file
    |--> Run project startup script (if exists)
    |--> Touch completion signal
    |--> Offer interactive shell
```

### Inter-Process Communication

Signal files enable kitty and main script to coordinate:

- `~/.helix-startup-complete` - Touched when setup work is done
- `~/.helix-zed-folders` - Contains folder paths for Zed to open

This keeps Zed and kitty in **separate process trees** so closing kitty doesn't kill Zed.

### Branch Detection Fix

Old code (line 123, broken for Azure DevOps):
```bash
DEFAULT_BRANCH=$(git -C "$CLONE_DIR" symbolic-ref refs/remotes/origin/HEAD 2>/dev/null \
    | sed 's@^refs/remotes/origin/@@' || echo "main")
```

New code (robust, matches `helix-specs-create.sh`):
```bash
DEFAULT_BRANCH=$(git -C "$CLONE_DIR" symbolic-ref refs/remotes/origin/HEAD 2>/dev/null \
    | sed 's@^refs/remotes/origin/@@')
if [ -z "$DEFAULT_BRANCH" ]; then
    # Fallback: check for common branch names
    if git -C "$CLONE_DIR" show-ref --verify refs/remotes/origin/main >/dev/null 2>&1; then
        DEFAULT_BRANCH="main"
    elif git -C "$CLONE_DIR" show-ref --verify refs/remotes/origin/master >/dev/null 2>&1; then
        DEFAULT_BRANCH="master"
    else
        DEFAULT_BRANCH="main"  # Last resort
    fi
fi
```

## Testing Notes

### Scenarios to Test

1. **Normal clone** - Repo with `main` branch, `.helix/startup.sh` exists
2. **Azure DevOps repo** - Uses `master`, no `origin/HEAD` symbolic ref
3. **Clone failure** - Invalid credentials or network error
4. **No startup script** - Repo exists but no `.helix/startup.sh`
5. **No repos configured** - `HELIX_REPOSITORIES` not set
6. **Empty repo** - No commits yet

### What User Should See

In all cases, kitty terminal opens showing:
- Environment variables (sanitized)
- Git configuration progress
- Clone progress with clear success/failure messages
- Worktree setup status
- Startup script output (if exists)
- Interactive shell option

## Related Code

### Test Coverage Gap

`test-helix-specs-creation.sh` tests `helix-specs-create.sh` (the helper), but doesn't test `start-zed-helix.sh`. The branch detection bug was in the untested outer script.

Consider adding a test case for Azure DevOps style repos:
```bash
run_test "Repo with master branch (Azure DevOps style)" '
    git checkout -b master 2>/dev/null
    echo "test" > file.txt
    git add file.txt
    git commit -m "Initial commit"
    git push origin master
    # Simulate Azure DevOps: remove origin/HEAD symbolic ref
    rm -f .git/refs/remotes/origin/HEAD
'
```

### Refactoring Opportunity

The robust `get_default_branch()` logic now exists in two places:
1. `helix-specs-create.sh` lines 84-99
2. `start-zed-helix.sh` setup script lines 159-170

Consider extracting to a shared function in `helix-specs-create.sh` (or new `git-helpers.sh`).

## Deployment Notes

### Dev Environment
- Bind mounts `./wolf/sway-config:/helix-dev/sway-config:ro`
- Changes are picked up automatically (no rebuild needed)

### Production
- Scripts are copied individually in `Dockerfile.sway-helix`
- No new files added, so no Dockerfile changes needed for this refactor
- Requires image rebuild: `./stack build-sway`

## Version

Script version bumped from v3 to v4 (line 16).
