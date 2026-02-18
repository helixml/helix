# Design: Fix Startup Script for New Environments

## Summary

Modify the `./stack` script to work around Docker config permission issues by setting `DOCKER_CONFIG` to a fallback directory when the default `~/.docker/` is inaccessible.

## Root Cause

When `~/.docker/` is owned by root (common when Docker was first run with `sudo`), the Docker CLI:
1. Cannot read `config.json`
2. Cannot load CLI plugins from `~/.docker/cli/plugins/`
3. Falls back to basic `docker` without buildx

The `--provenance=false` flag requires buildx, so builds fail.

## Solution

Instead of fixing permissions (which requires `sudo`), set `DOCKER_CONFIG` to bypass the broken directory:

```bash
# Check if ~/.docker is accessible, use fallback if not
if [ -d "$HOME/.docker" ] && [ ! -r "$HOME/.docker" ]; then
    export DOCKER_CONFIG="${XDG_CONFIG_HOME:-$HOME/.config}/docker"
    mkdir -p "$DOCKER_CONFIG"
    echo "⚠️  ~/.docker not readable, using $DOCKER_CONFIG"
fi
```

This approach:
- **No sudo required** - doesn't try to fix permissions
- **Preserves user's config** - only uses fallback when needed
- **Idempotent** - safe to run multiple times
- **Standard location** - uses XDG config dir as fallback

## Implementation Location

File: `helix/stack`

Add the check near the top of the script, after the initial variable exports (around line 15), before any Docker commands run.

## Key Decisions

| Decision | Rationale |
|----------|-----------|
| Use `DOCKER_CONFIG` vs fix permissions | Simpler, no sudo, works immediately |
| Use XDG fallback vs /tmp | Persistent across sessions, follows XDG standard |
| Check readability vs ownership | More direct test of the actual problem |

## Testing

1. Simulate broken permissions: `sudo chown root:root ~/.docker`
2. Run `./stack start`
3. Verify it uses fallback config and builds succeed
4. Run on working environment to verify no change in behavior