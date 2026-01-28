# Design: Shared Docker BuildKit Cache

## Primary Use Case: Helix-in-Helix Development

When developing Helix inside a Helix desktop session ("Helix-in-Helix"), running `./stack start` triggers `docker compose up -d` which builds multiple services (api, frontend, haystack, etc.). Each new session currently rebuilds everything from scratch because BuildKit cache isn't shared between Hydra-spawned dockerd instances.

**Pain point**: Starting a new Helix-in-Helix dev session means waiting 10+ minutes for builds that should be cached.

## Current Architecture

```
helix-sandbox container
├── Sandbox dockerd (/var/lib/docker)
│   └── Desktop images (helix-ubuntu, helix-sway)
│
└── Hydra manager
    └── /hydra-data/ (hydra-storage volume - persists across sandbox restarts)
        ├── sessions/ses_001/docker (Session 1 dockerd --data-root)
        │   └── User builds cached here (isolated)
        ├── sessions/ses_002/docker (Session 2 dockerd --data-root)
        │   └── User builds cached here (isolated, no sharing!)
        └── sessions/ses_N/...
```

Each Hydra-spawned dockerd has its own `--data-root`, so BuildKit cache is NOT shared.

## Proposed Solution: Shared BuildKit Cache Directory

BuildKit stores cache in `<data-root>/buildkit/`. We can use BuildKit's local cache export/import to share cache across dockerd instances via a shared volume.

### Option A: Inline Cache (Simplest)

Use `--cache-from` and `--cache-to` with a shared directory:

```bash
docker buildx build \
  --cache-from type=local,src=/shared-cache/myimage \
  --cache-to type=local,dest=/shared-cache/myimage,mode=max \
  -t myimage .
```

**Pros**: Works with any dockerd, no daemon config changes
**Cons**: Requires user/agent to pass cache flags explicitly

### Option B: Registry Cache (Alternative)

Use the sandbox's local registry for caching:

```bash
docker buildx build \
  --cache-from type=registry,ref=registry:5000/cache/myimage \
  --cache-to type=registry,ref=registry:5000/cache/myimage,mode=max \
  -t myimage .
```

**Pros**: Standard pattern, works across sandboxes
**Cons**: More network overhead, registry must be running

## Recommended Approach: Option A with Go Wrappers

1. **Hydra creates shared cache directory** at `/hydra-data/buildkit-cache/`
2. **Hydra mounts this directory** into all dev containers it creates
3. **Rewrite `docker-wrapper.sh` in Go** - inject cache flags for `docker build` and `docker buildx build`
4. **Rewrite `docker-compose-wrapper.sh` in Go** - inject cache flags for `docker compose build` (requires special handling - see below)
5. **BuildKit handles concurrency** via content-addressed storage (safe for concurrent access)

### Prerequisite: Rewrite Bash Wrappers in Go

The existing bash wrappers (`docker-wrapper.sh`, `docker-compose-wrapper.sh`) are already complex and hard to maintain. Adding version detection, YAML preprocessing, and cache injection in bash would be a nightmare.

**New approach**: Single Go binary `docker-shim` that handles both `docker` and `docker compose` commands.

Benefits:
- Proper argument parsing (not bash string manipulation)
- Testable with unit tests
- YAML parsing for compose files (use `gopkg.in/yaml.v3`)
- Version comparison without `sort -V` hacks
- Single binary, easier to deploy into desktop images
- Can share code with Hydra

The shim will be symlinked as both `docker` and `docker-compose` in the PATH, and detect which mode based on `argv[0]` or first argument.

### Implementation Details

**Hydra creates shared cache directory** (manager.go):
```go
// In NewManager or Start, create the shared cache directory
cacheDir := filepath.Join(m.dataDir, "buildkit-cache")
os.MkdirAll(cacheDir, 0755)
```

**Hydra passes mount to dev containers** (devcontainer.go):
```go
// Add shared buildkit cache to all dev containers
mounts = append(mounts, mount.Mount{
    Type:   mount.TypeBind,
    Source: filepath.Join(dm.manager.dataDir, "buildkit-cache"),
    Target: "/buildkit-cache",
})
```

This is entirely internal to Hydra - no docker-compose changes needed. The `/hydra-data` volume already exists and persists across sandbox restarts.

**Go docker-shim for `docker build`**:

```go
// In docker-shim, detect "buildx build" or "build" commands and inject cache flags

func handleDockerCommand(args []string) []string {
    if !isBuildCommand(args) {
        return args
    }
    
    if _, err := os.Stat("/buildkit-cache"); os.IsNotExist(err) {
        return args // No cache directory, pass through
    }
    
    // Extract image name from -t flag for cache key
    imageName := extractImageTag(args)
    cacheKey := sanitizeForPath(imageName)
    if cacheKey == "" {
        cacheKey = "default"
    }
    cacheDir := filepath.Join("/buildkit-cache", cacheKey)
    
    // Inject cache flags
    args = append(args,
        "--cache-from", fmt.Sprintf("type=local,src=%s", cacheDir),
        "--cache-to", fmt.Sprintf("type=local,dest=%s,mode=max", cacheDir),
    )
    
    return args
}
```

### ⚠️ Docker Compose v2 Caveat

**IMPORTANT**: Docker Compose v2 (the Go rewrite, `docker compose` as a plugin) does **NOT** shell out to `docker build`. Instead, it:

1. Uses the Docker SDK/API directly
2. Communicates with the BuildKit daemon over gRPC
3. Never invokes the `docker` CLI binary

This means **the docker shim won't intercept builds triggered by `docker compose build`** when compose calls BuildKit directly.

**Solution for Docker Compose v2**: The shim must inject cache configuration when wrapping compose commands, using one of these approaches:

#### Approach 1: Use `--set` flag (Compose v2.24+)

Docker Compose v2.24+ supports `--set` to override build options:

```go
func handleComposeCommand(args []string) []string {
    if !isBuildOrUpBuild(args) {
        return args
    }
    
    if _, err := os.Stat("/buildkit-cache"); os.IsNotExist(err) {
        return args
    }
    
    version := getComposeVersion()
    if version.AtLeast("2.24") {
        // Inject cache config for all services using wildcard
        args = append(args,
            "--set", `*.build.cache_from=["type=local,src=/buildkit-cache"]`,
            "--set", `*.build.cache_to=["type=local,dest=/buildkit-cache,mode=max"]`,
        )
    } else {
        // Fall back to compose file preprocessing
        args = preprocessComposeFile(args)
    }
    
    return args
}
```

**Pros**: Clean, no file manipulation
**Cons**: Requires Compose v2.24+, wildcard syntax

#### Approach 2: Preprocess Compose File (Fallback for older Compose)

Generate a modified compose file with cache configuration injected:

```go
func preprocessComposeFile(args []string) []string {
    // Find the compose file being used
    composeFile := findComposeFile(args)
    
    // Parse YAML
    var compose map[string]interface{}
    data, _ := os.ReadFile(composeFile)
    yaml.Unmarshal(data, &compose)
    
    // Inject cache config into all services with build sections
    services := compose["services"].(map[string]interface{})
    for name, svc := range services {
        service := svc.(map[string]interface{})
        if build, ok := service["build"]; ok {
            buildConfig := normalizeBuildConfig(build)
            buildConfig["cache_from"] = []string{"type=local,src=/buildkit-cache"}
            buildConfig["cache_to"] = []string{"type=local,dest=/buildkit-cache,mode=max"}
            service["build"] = buildConfig
        }
    }
    
    // Write modified file
    tmpFile, _ := os.CreateTemp("", "compose-cache-*.yaml")
    yaml.NewEncoder(tmpFile).Encode(compose)
    
    // Prepend -f flag (compose uses last -f for conflicts)
    return append([]string{"-f", tmpFile.Name()}, args...)
}
```

**Pros**: Works with any Compose v2 version, full control
**Cons**: More complex, must handle compose file merging semantics

#### Recommended: Approach 1 with Approach 2 Fallback

Check Compose version and use the appropriate method.

## Concurrency Safety

BuildKit's local cache exporter uses content-addressed storage (blobs identified by SHA256). This is safe for concurrent access:

- **Reads**: Multiple readers can access the same blob simultaneously
- **Writes**: Each writer creates new blobs atomically (write to temp, rename)
- **No corruption**: Same content = same hash = same file (idempotent)

The BuildKit team confirms this works for concurrent builds: https://github.com/moby/buildkit/issues/1512

## Disk Space Considerations

- **Cache location**: `/hydra-data/buildkit-cache/` (inside existing hydra-storage volume)
- **Deduplication**: Content-addressed storage means identical layers stored once
- **Pruning**: Use `docker buildx prune` periodically, or let Docker's LRU handle it
- **Estimated savings**: 10 identical builds go from ~50GB to ~5GB

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Cache method | Local directory | Simplest, no registry dependency |
| Cache location | `/hydra-data/buildkit-cache/` | Uses existing volume, Hydra implementation detail |
| Mount type | Bind mount | Share across Hydra dockerd instances |
| Concurrency | Trust BuildKit | Content-addressed, proven safe |
| Wrapper language | Go (rewrite from bash) | Testable, proper arg parsing, YAML handling, maintainable |
| docker build | Go shim injects `--cache-from/--cache-to` | Intercepts CLI invocations |
| docker compose build | Go shim uses `--set` or file preprocessing | Compose v2 uses BuildKit API directly, doesn't invoke docker CLI |