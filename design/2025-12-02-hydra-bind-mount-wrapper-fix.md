# Hydra Bind-Mount Docker Wrapper Fix

**Date:** 2025-12-02
**Status:** Implemented
**Supersedes:** 2025-12-01-hydra-bind-mount-symlink.md (incorrect assumption)

## Problem

The previous design doc (2025-12-01) assumed Docker CLI resolves symlinks before sending paths to the daemon. **This assumption was incorrect.**

Testing confirmed:
```bash
# In dev container:
$ docker run -v ~/work:/test ubuntu ls /test
# Shows EMPTY directory - sandbox's /home/retro/work, not the symlinked data

$ docker run -v /data/workspaces/spec-tasks/spt_xxx:/test ubuntu ls /test
# Shows REAL data - .git, design/, etc.

$ docker inspect <container> | jq '.[] | .Mounts[].Source'
# Shows "/home/retro/work" - NOT resolved to /data/workspaces/...
```

Docker CLI does NOT resolve symlinks. It passes the literal path to the daemon.

## Root Cause

1. Dev container has symlink: `/home/retro/work` → `/data/workspaces/spec-tasks/{id}`
2. User runs: `docker run -v ~/work/foo:/app ...`
3. Docker CLI passes `/home/retro/work/foo` to Hydra's dockerd
4. Hydra's dockerd runs in sandbox container
5. Sandbox has its own `/home/retro/work` directory (auto-created, empty)
6. Bind mount uses sandbox's empty `/home/retro/work/foo` instead of the real data

## Solution: Docker CLI Wrapper

Install a wrapper script at `/usr/bin/docker` that:
1. Intercepts all docker commands
2. Resolves symlinks in `-v`, `--volume`, and `--mount` arguments
3. Passes resolved paths to the real docker CLI at `/usr/bin/docker.real`

### Wrapper Script: `wolf/sway-config/docker-wrapper.sh`

```bash
#!/bin/bash
# Resolves symlinks in bind mount paths for Hydra compatibility

DOCKER_REAL="/usr/bin/docker.real"

resolve_path() {
    local path="$1"
    if [[ -e "$path" || -L "$path" ]]; then
        readlink -f "$path" 2>/dev/null || echo "$path"
    else
        # Path doesn't exist yet - resolve parent directory
        local dir=$(dirname "$path")
        local base=$(basename "$path")
        if [[ -e "$dir" || -L "$dir" ]]; then
            local resolved_dir=$(readlink -f "$dir" 2>/dev/null || echo "$dir")
            echo "${resolved_dir}/${base}"
        else
            echo "$path"
        fi
    fi
}

# Process -v/--volume and --mount arguments, resolve symlinks, exec real docker
```

### Dockerfile Changes

```dockerfile
# After installing docker-ce-cli:
COPY wolf/sway-config/docker-wrapper.sh /usr/local/bin/docker-wrapper.sh
RUN mv /usr/bin/docker /usr/bin/docker.real \
    && mv /usr/local/bin/docker-wrapper.sh /usr/bin/docker \
    && chmod +x /usr/bin/docker /usr/bin/docker.real
```

## How It Works

1. User runs: `docker run -v ~/work/foo:/app ubuntu bash`
2. Wrapper receives args: `-v /home/retro/work/foo:/app`
3. Wrapper resolves: `/home/retro/work` → `/data/workspaces/spec-tasks/{id}`
4. Wrapper calls: `/usr/bin/docker.real run -v /data/workspaces/spec-tasks/{id}/foo:/app ubuntu bash`
5. Hydra's dockerd receives resolved path that exists in sandbox
6. Bind mount succeeds with real data!

## Docker Compose Support

`docker compose` is a plugin that runs through the main `docker` binary, so the wrapper handles it automatically. When you run `docker compose up`, the main docker binary is invoked with the `compose` subcommand, and the wrapper processes any volume arguments.

## Testing

After building the new sway image:

```bash
# In dev container:
cd ~/work
echo "test" > testfile.txt
docker run --rm -v ~/work:/mnt alpine cat /mnt/testfile.txt
# Should output: test

# Verify the wrapper resolved the path:
docker inspect $(docker run -d -v ~/work:/mnt alpine sleep 60) | jq '.[] | .Mounts[].Source'
# Should show: /data/workspaces/spec-tasks/{id}
```

## Future Improvement

The wrapper is a temporary solution. Future options:
1. Patch Docker CLI to resolve symlinks natively
2. Use Docker's `--volume-driver` to implement custom resolution
3. Contribute upstream fix to Docker project
