# Design: Fix Startup Script Failure

## Summary

Modify `17-start-dockerd.sh` to fix ownership of `~/.docker/` after creating the buildx builder, so the `retro` user can access Docker CLI plugins.

## Root Cause

```
17-start-dockerd.sh (runs as root)
    └── docker buildx create --name helix-shared ...
         └── Creates /home/retro/.docker/ owned by root
              └── retro user cannot read it
                   └── docker buildx fails to load
                        └── --provenance flag unknown
```

## Solution

Add a `chown` at the end of `17-start-dockerd.sh` after the buildx setup:

```bash
# Fix ownership of .docker directory for retro user
if id -u retro >/dev/null 2>&1 && [ -d /home/retro/.docker ]; then
    chown -R retro:retro /home/retro/.docker
    echo "[dockerd] Fixed /home/retro/.docker ownership"
fi
```

## Implementation Location

File: `helix/desktop/shared/17-start-dockerd.sh`

Add after line 157 (after `docker buildx rm default` and the echo), before the script ends.

## Key Decisions

| Decision | Rationale |
|----------|-----------|
| Fix in 17-start-dockerd.sh vs stack script | Fix at the source - this script creates the problem |
| Check if retro user exists | Script may run in different environments |
| Use chown -R | Covers all files in .docker/ including buildx/ subdirectory |

## Testing

1. Start a new Helix-in-Helix session
2. Verify `ls -la ~/.docker` shows `retro:retro` ownership
3. Run `docker buildx version` as retro user (no warnings)
4. Run `./stack start` - should complete without permission errors