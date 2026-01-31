# Helix-Sway Image Versioning and Deployment Analysis

**Date:** 2025-11-26
**Status:** Implemented
**Author:** Claude (analysis requested by user)

## Executive Summary

The helix-sway image build and deployment flow has several bugs that cause changes to not be properly picked up in production. The root cause is using `:latest` tags combined with Docker-in-Docker (DinD) state persistence issues.

## The Complete Build/Deploy Flow

### Build Phase (`./stack build-and-push-helix-code`)

```
1. build-wolf           → wolf:helix-fixed (local)
2. build-moonlight-web  → helix-moonlight-web:helix-fixed (local)
3. build-sandbox:
   ├── build-sway:
   │   ├── build-zed release    → ./zed-build/zed binary
   │   └── docker build         → helix-sway:latest (from Dockerfile.sway-helix)
   │
   ├── HASH_BEFORE = $(docker images helix-sway:latest --format '{{.ID}}')
   ├── HASH_AFTER  = $(docker images helix-sway:latest --format '{{.ID}}')
   │
   ├── if HASH_BEFORE != HASH_AFTER:
   │   └── docker save helix-sway:latest > helix-sway.tar
   │       echo $HASH_AFTER > helix-sway.tar.hash
   │
   └── docker build -f Dockerfile.sandbox:
       ├── COPY helix-sway.tar /opt/images/helix-sway.tar
       ├── COPY helix-sway.tar.hash /opt/images/helix-sway.tar.hash
       └── → registry.helixml.tech/helix/helix-sandbox:${COMMIT_HASH}
```

### Runtime Phase (Sandbox Container Startup)

```
Container starts → GOW init system → /etc/cont-init.d/*.sh scripts

04-start-dockerd.sh:
├── Start dockerd (DinD)
├── Create helix_default network
└── Load helix-sway image:
    ├── EXPECTED_HASH = cat /opt/images/helix-sway.tar.hash
    ├── CURRENT_HASH = docker images helix-sway:latest --format '{{.ID}}'
    │
    ├── if EXPECTED_HASH == CURRENT_HASH:
    │   └── Skip docker load (assume already loaded)  ← BUG #1
    └── else:
        └── docker load -i /opt/images/helix-sway.tar
```

### Wolf Container Creation

```
wolf_executor_apps.go:
├── zedImage = os.Getenv("ZED_IMAGE") || "helix-sway:latest"
└── createSwayWolfAppForAppsMode(..., zedImage, ...)
    └── wolf.NewMinimalDockerApp(..., "helix-sway:latest", ...)
```

---

## Identified Bugs

### Bug #1: DinD State Can Persist Across Container Recreations

**Location:** `04-start-dockerd.sh` lines 276-286

**Problem:** The hash comparison assumes that if the helix-sway:latest image exists in DinD with a matching hash, it's the correct version. However:

1. Docker volumes can persist DinD state across container restarts
2. If the sandbox container is restarted (not fully recreated with volume cleanup), DinD keeps old images
3. The `:latest` tag masks version differences - same tag, different content

**Reproduction:**
```bash
# On remote sandbox:
docker images helix-sway:latest
# Shows IMAGE ID = abc123 (OLD version)

# Inside sandbox:
cat /opt/images/helix-sway.tar.hash
# Shows abc123 (matches because tarball wasn't updated)
# OR shows xyz789 (NEW version, but DinD has old image cached)
```

### Bug #2: Hash Comparison Can Be Bypassed

**Location:** `stack` lines 1060-1082

**Problem:** The HASH_BEFORE/HASH_AFTER comparison happens at BUILD time on the build machine. If the Docker build cache is warm and the image doesn't change (e.g., Docker incorrectly caches layers), the hash comparison passes and the tarball is NOT regenerated.

```bash
# Scenario: File changed but Docker cache hit
HASH_BEFORE=abc123
# Docker build uses cached layers (shouldn't but might)
HASH_AFTER=abc123  # Same hash!
# Result: Reuses OLD helix-sway.tar, new changes lost
```

### Bug #3: No Version Verification at Runtime

**Location:** `wolf_executor_apps.go` line 71

**Problem:** The executor blindly uses `helix-sway:latest` without verifying:
- Which version is actually loaded
- Whether it matches the expected version for this sandbox release
- Whether the image was properly loaded at all

```go
// wolf_executor_apps.go:71
zedImage:          zedImage,  // Just "helix-sway:latest", no version check
```

### Bug #4: `:latest` Tag is Semantically Meaningless

**Location:** Multiple files

**Problem:** Using `:latest` everywhere makes it impossible to:
- Know which version is deployed
- Detect version mismatches
- Roll back to previous versions
- Debug deployment issues

**Files affected:**
- `Dockerfile.sandbox` line 748: `ZED_IMAGE="helix-sway:latest"`
- `stack` build-sway: tags as `helix-sway:latest`
- `wolf_executor_apps.go`: uses environment default

### Bug #5: Tarball Hash Stores Image ID, Not Content Hash

**Location:** `stack` lines 1067, 1074

**Problem:** The hash file stores Docker's image ID (a hash of image metadata), not a content hash of the tarball or source files. This means:
- Different builds from identical source could have different image IDs
- Image ID doesn't directly reflect source file changes

---

## Implemented Solution: Self-Describing Sandbox with Versioned Tags

### Architecture

Each sandbox container is self-describing - it knows which helix-sway version it contains:

```
┌─────────────────────────────────────────────────────────────┐
│ helix-sandbox container                                      │
│                                                              │
│  /opt/images/helix-sway.tar      (the tarball)              │
│  /opt/images/helix-sway.tar.hash (image ID for skip logic)  │
│  /opt/images/helix-sway.version  (commit hash: "abc123")    │
│                                                              │
│  DinD loads → helix-sway:abc123                             │
│             → helix-sway:latest (kept for compat)           │
│                                                              │
│  Wolf executor reads /opt/images/helix-sway.version         │
│  Uses: helix-sway:abc123                                    │
└─────────────────────────────────────────────────────────────┘
```

The control plane never needs updating - each sandbox is self-contained.

### Implementation (Completed)

#### 1. Build-sandbox (`stack` lines 1060-1088)

```bash
# Tag with commit hash (in addition to :latest)
docker tag helix-sway:latest helix-sway:${COMMIT_HASH}

# Save with both tags so DinD gets versioned tag after load
docker save helix-sway:latest helix-sway:${COMMIT_HASH} > helix-sway.tar

# Write version metadata
echo "${COMMIT_HASH}" > helix-sway.version
```

#### 2. Dockerfile.sandbox (line 170)

```dockerfile
COPY helix-sway.tar /opt/images/helix-sway.tar
COPY helix-sway.tar.hash /opt/images/helix-sway.tar.hash
COPY helix-sway.version /opt/images/helix-sway.version
```

#### 3. Startup Script (`04-start-dockerd.sh` lines 273-319)

```bash
# Read version from embedded metadata
SWAY_VERSION="latest"
if [ -f /opt/images/helix-sway.version ]; then
    SWAY_VERSION=$(cat /opt/images/helix-sway.version)
fi

# Check for versioned tag (more reliable than :latest)
CURRENT_HASH=$(docker images "helix-sway:${SWAY_VERSION}" --format '{{.ID}}')

# Load if needed, tag for backwards compat if tarball predates versioning
docker load -i /opt/images/helix-sway.tar
docker tag helix-sway:latest "helix-sway:${SWAY_VERSION}" 2>/dev/null || true
```

#### 4. Wolf Executor (`wolf_executor_apps.go` lines 838-878)

```go
// resolveZedImage reads /opt/images/helix-sway.version
// Returns "helix-sway:{commit}" or fallback to "helix-sway:latest"
func resolveZedImage(envZedImage string) string {
    const versionFile = "/opt/images/helix-sway.version"

    // Try to read version from embedded metadata
    versionBytes, err := os.ReadFile(versionFile)
    if err == nil {
        version := strings.TrimSpace(string(versionBytes))
        if version != "" {
            return fmt.Sprintf("helix-sway:%s", version)
        }
    }

    // Fallback for backwards compatibility
    return "helix-sway:latest"
}
```

### Backwards Compatibility

- Old tarballs without version file: Wolf executor falls back to `:latest`
- Old tarballs without versioned tag: Startup script auto-tags after load
- No control plane changes required

---

## Immediate Workarounds

Until the full solution is implemented:

### Force Tarball Regeneration

```bash
# Before build-sandbox, remove cached tarball
rm -f helix-sway.tar helix-sway.tar.hash

# Force Docker to rebuild without cache
docker build --no-cache -f Dockerfile.sway-helix -t helix-sway:latest .
```

### Force DinD Image Reload on Deploy

```bash
# On remote sandbox, before pulling new image:
docker compose down sandbox
docker volume rm helix_sandbox_docker_data  # If such volume exists
docker compose pull sandbox
docker compose up -d sandbox
```

### Verify Deployed Version

```bash
# SSH to sandbox and check the loaded script
docker compose exec sandbox docker exec <zed-container> cat /usr/local/bin/start-zed-helix.sh | head -20
```

---

## Verification Checklist

After fix implementation, verify:

- [ ] Change to `wolf/sway-config/start-zed-helix.sh` → rebuild → new version in production
- [ ] `docker images helix-sway` shows commit-hash tagged image
- [ ] Sandbox logs show "Loading helix-sway:abc123" (not `:latest`)
- [ ] Wolf executor logs show versioned image name
- [ ] Remote deployment has matching version

---

## Appendix: File Locations

| Component | File | Lines |
|-----------|------|-------|
| Build-sway | `stack` | 832-912 |
| Build-sandbox | `stack` | 983-1167 |
| Hash comparison | `stack` | 1060-1082 |
| Dockerfile.sway | `Dockerfile.sway-helix` | Full file |
| Dockerfile.sandbox | `Dockerfile.sandbox` | Lines 166-169, 748 |
| Startup script | `Dockerfile.sandbox` | Lines 273-300 (embedded) |
| Wolf executor | `api/pkg/external-agent/wolf_executor_apps.go` | Lines 46, 71, 872 |
