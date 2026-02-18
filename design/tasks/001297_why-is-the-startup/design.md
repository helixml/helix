# Design: Fix Startup Script for New Environments

## Summary

Modify the `./stack` script to automatically detect and fix Docker config directory permission issues before running Docker commands.

## Architecture

```
./stack start
    └── fix_docker_permissions() [NEW]
         ├── Check if ~/.docker exists and is owned by root
         ├── Fix ownership with sudo chown if needed
         └── Continue to existing logic
    └── build-zed (existing)
         └── docker build --provenance=false ... [now works]
```

## Solution

Add a new function `fix_docker_permissions()` to the `stack` script that:

1. Checks if `~/.docker/` exists
2. Checks if it's owned by root (common issue when Docker was run with `sudo`)
3. Runs `sudo chown -R $USER:$USER ~/.docker` to fix ownership
4. Logs what it did for transparency

## Implementation Location

File: `helix/stack`

Insert the new function near the top with other setup functions (around line 25, after `setup_dev_networking()`), then call it early in commands that use Docker.

## Code Changes

```bash
# Fix Docker config directory permissions if owned by root
# This commonly happens when Docker is first run with sudo
function fix_docker_permissions() {
  local DOCKER_DIR="$HOME/.docker"
  
  if [ -d "$DOCKER_DIR" ]; then
    local OWNER=$(stat -c '%U' "$DOCKER_DIR" 2>/dev/null || stat -f '%Su' "$DOCKER_DIR" 2>/dev/null)
    if [ "$OWNER" = "root" ] && [ "$USER" != "root" ]; then
      echo "🔧 Fixing Docker config directory permissions (owned by root)..."
      sudo chown -R "$USER:$USER" "$DOCKER_DIR"
      echo "✅ Fixed ~/.docker ownership"
    fi
  fi
}
```

Call `fix_docker_permissions` at the start of:
- `build_zed()` function
- `start` command handler
- Any other command that uses `docker build` or `docker buildx`

## Key Decisions

| Decision | Rationale |
|----------|-----------|
| Fix in stack script vs documentation | Every new environment should work out of the box |
| Use `stat` for ownership check | Portable across Linux and macOS |
| Only fix if owned by root | Don't change permissions unnecessarily |
| Silent success, verbose fix | Don't clutter output when everything is fine |

## Testing

1. Create a fresh environment with `sudo mkdir ~/.docker && sudo chown root:root ~/.docker`
2. Run `./stack start`
3. Verify it fixes permissions and continues without errors
4. Run again to verify idempotency (no unnecessary sudo prompts)