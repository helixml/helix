# Design: Fix Startup Script Failure

## Summary

The `./stack start` command fails because Docker configuration directory permissions prevent BuildKit from loading.

## Architecture

```
./stack start
    └── build-zed (function in stack script)
         └── docker build --provenance=false ...
              └── FAILS: Docker can't read ~/.docker/config.json
                         └── CLI plugins (buildx) don't load
                              └── --provenance flag unknown
```

## Root Cause

Docker was previously run with `sudo`, which created `/home/retro/.docker/` owned by root:

```
drwx------ 3 root root /home/retro/.docker/
```

The user `retro` (uid=1000) cannot access this directory, so Docker:
1. Cannot load `config.json`
2. Cannot discover CLI plugins in `~/.docker/cli/plugins/`
3. Falls back to basic `docker build` without BuildKit features

## Solution

**Option 1: Fix permissions (recommended)**
```bash
sudo chown -R retro:retro /home/retro/.docker
```

**Option 2: Delete and recreate**
```bash
sudo rm -rf /home/retro/.docker
# Docker will recreate on next run
```

## Verification

After fix:
```bash
docker buildx version  # Should show version without warnings
./stack start          # Should complete successfully
```

## Key Decisions

| Decision | Rationale |
|----------|-----------|
| Fix permissions vs code change | This is an environment issue, not a bug in the stack script |
| Use chown vs rm -rf | Preserves any existing buildx builders/cache |

## Prevention

Avoid running Docker with `sudo` - the user is already in the `docker` group (gid=984).