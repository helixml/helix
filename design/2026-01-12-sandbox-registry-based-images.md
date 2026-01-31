# Sandbox Registry-Based Inner Images

**Date:** 2026-01-12
**Status:** Proposed
**Author:** Luke (with Claude)

## Problem

The current sandbox build process embeds desktop images (helix-sway, helix-ubuntu) as tarballs inside the sandbox container image. This causes several issues:

1. **Full layer copy on every build**: Even if only a small amount of code changes in the bottom layers, the entire 1.6GB+ tarball is re-embedded
2. **No layer deduplication**: Docker's layer caching doesn't help because the tarball is an opaque blob
3. **Slow development iteration**: Changing a few lines of Go code in screenshot-server requires waiting for the full tarball transfer
4. **Large upgrade downloads**: End users download the entire image even when most layers are unchanged

### Current Architecture

```
sandbox image (shipped to users)
├── Wolf streaming server
├── Moonlight Web
├── sandbox-images/helix-sway.tar (1.6GB opaque blob)
├── sandbox-images/helix-ubuntu.tar (1.8GB opaque blob)
└── Inner dockerd loads tarballs on first use
```

## Proposed Solution

Use a Docker registry to store and distribute inner images. The sandbox container pulls images from the registry instead of loading from embedded tarballs.

### New Architecture

```
sandbox image (shipped to users)
├── Wolf streaming server
├── Moonlight Web
├── sandbox-images/helix-sway.ref (text file: "registry.helixml.tech/helix/sway:v1.2.3")
├── sandbox-images/helix-ubuntu.ref (text file: "registry.helixml.tech/helix/ubuntu:v1.2.3")
└── Inner dockerd pulls from registry on first use (with layer deduplication)
```

### Benefits

1. **Layer deduplication**: Docker handles layer caching automatically. Changing screenshot-server only downloads ~50MB instead of 1.6GB
2. **Faster upgrades**: Users on v1.2.2 upgrading to v1.2.3 only pull changed layers
3. **Faster dev iteration**: `./stack build-sway` pushes to registry; sandbox pulls only changed layers
4. **Smaller sandbox image**: No embedded tarballs, sandbox image drops from ~4GB to ~500MB
5. **Parallel pulls**: Multiple inner images can pull simultaneously

### Tradeoffs

1. **Registry dependency**: Sandbox must reach registry to pull images (but already needs this for sandbox upgrades)
2. **First-pull latency**: Cold start requires full pull (mitigated by pre-pulling on sandbox start)
3. **Enterprise configuration**: Need mechanism to override registry URL for air-gapped deployments

## Implementation

### Phase 1: Registry Infrastructure

1. Use existing `registry.helixml.tech` or create dedicated registry
2. Define image naming convention: `registry.helixml.tech/helix/{sway,ubuntu}:{version}`
3. Update CI to push images to registry after build

### Phase 2: Build Script Changes

#### `./stack build-sway` and `./stack build-ubuntu`

```bash
# Current flow:
# 1. Build image locally
# 2. Export to tarball
# 3. Transfer tarball to sandbox's dockerd via docker save/load

docker build -t helix-sway:$TAG .
docker save helix-sway:$TAG > sandbox-images/helix-sway.tar
docker exec sandbox docker load < sandbox-images/helix-sway.tar

# New flow:
# 1. Build image locally
# 2. Push to registry (leverages layer caching)
# 3. Write reference file (for sandbox to know what to pull)
# 4. Trigger pull in local sandbox (for dev testing)

docker build -t helix-sway:$TAG .
docker tag helix-sway:$TAG $REGISTRY/helix/sway:$TAG
docker push $REGISTRY/helix/sway:$TAG
echo "$REGISTRY/helix/sway:$TAG" > sandbox-images/helix-sway.ref
# For local dev: trigger pull in running sandbox
docker exec sandbox docker pull $REGISTRY/helix/sway:$TAG
```

#### `./stack build-sandbox`

```bash
# Current flow:
# 1. Build Wolf, Moonlight Web
# 2. COPY tarballs into image (1.6GB + 1.8GB = 3.4GB copied every time)
# 3. Ship massive image

COPY sandbox-images/helix-sway.tar /sandbox-images/
COPY sandbox-images/helix-ubuntu.tar /sandbox-images/

# New flow:
# 1. Build Wolf, Moonlight Web
# 2. COPY only small reference files (~100 bytes each)
# 3. Ship slim image

COPY sandbox-images/helix-sway.ref /sandbox-images/
COPY sandbox-images/helix-ubuntu.ref /sandbox-images/
```

#### CI/CD Pipeline Changes

```yaml
# Current: Multiple jobs, 3.4GB tarball artifacts passed between them
build-desktop-images:
  - build helix-sway, helix-ubuntu
  - export tarballs as artifacts (3.4GB uploaded to CI cache)

build-sandbox:
  - download tarball artifacts (3.4GB)
  - embed in sandbox image
  - push sandbox image (~4GB)

# New: Single job, no artifact passing, Docker layer cache handles efficiency
build-sandbox:
  # Build desktop images and push to registry (layer-cached)
  - docker build helix-sway
  - docker push $REGISTRY/helix/sway:$VERSION
  - docker build helix-ubuntu
  - docker push $REGISTRY/helix/ubuntu:$VERSION

  # Write .ref files locally (same job, no artifact transfer)
  - echo "$REGISTRY/helix/sway:$VERSION" > sandbox-images/helix-sway.ref
  - echo "$REGISTRY/helix/ubuntu:$VERSION" > sandbox-images/helix-ubuntu.ref

  # Build and push slim sandbox image (~500MB)
  - docker build -f Dockerfile.sandbox .
  - docker push $REGISTRY/helix/sandbox:$VERSION
```

Benefits of single job:
- No artifact upload/download between jobs (was 3.4GB)
- Docker build cache shared across all builds in same job
- Simpler CI configuration
- Atomic: either everything succeeds or nothing is published

#### Dockerfile.sandbox Changes

```dockerfile
# Current:
COPY sandbox-images/helix-sway.tar /sandbox-images/helix-sway.tar
COPY sandbox-images/helix-ubuntu.tar /sandbox-images/helix-ubuntu.tar
# Startup script does: docker load < /sandbox-images/*.tar

# New:
COPY sandbox-images/helix-sway.ref /sandbox-images/helix-sway.ref
COPY sandbox-images/helix-ubuntu.ref /sandbox-images/helix-ubuntu.ref
# Startup script does: docker pull $(cat /sandbox-images/*.ref)
```

#### Local Development Workflow

For rapid iteration, developers need the local sandbox to use newly built images:

```bash
# Option A: Push to shared dev registry, pull in sandbox
./stack build-sway  # pushes to registry, triggers pull

# Option B: Direct load for offline/fast iteration (keep as fallback)
./stack build-sway --local  # uses docker save/load, no registry
```

### Phase 3: Sandbox Runtime Changes

**Container executor** (Go code that launches desktop containers):
```go
// Current: Assumes image is pre-loaded
containerConfig.Image = "helix-sway:latest"

// New: Read reference, pull if needed, use full registry path
ref := readFile("/sandbox-images/helix-sway.ref")  // e.g., "registry.helixml.tech/helix/sway:v1.2.3"
ensureImagePulled(ref)
containerConfig.Image = ref
```

**Startup script** changes:
- Pre-pull images on sandbox boot (background, non-blocking)
- Handle pull failures gracefully (retry, use cached version)

### Phase 4: Enterprise Registry Override

Add configuration for custom registry:

```yaml
# helix config
sandbox:
  registry: "internal-registry.corp.example.com"
  # Images will be pulled from:
  # internal-registry.corp.example.com/helix/sway:v1.2.3
```

Implementation:
1. Environment variable: `HELIX_SANDBOX_REGISTRY`
2. Passed to sandbox container
3. Container executor uses this to construct image references

For air-gapped deployments:
```bash
# Mirror images to internal registry
docker pull registry.helixml.tech/helix/sway:v1.2.3
docker tag registry.helixml.tech/helix/sway:v1.2.3 internal-registry.corp.example.com/helix/sway:v1.2.3
docker push internal-registry.corp.example.com/helix/sway:v1.2.3
```

## Migration Path

1. **Backward compatibility**: Keep tarball loading as fallback if .ref file doesn't exist
2. **Gradual rollout**: Ship both tarball and ref file initially, switch to ref-only after validation
3. **Version pinning**: Ref files contain exact version tags, not `latest`

## Development Workflow

After implementation, the dev workflow becomes:

```bash
# Make changes to screenshot-server
vim api/pkg/desktop/desktop.go

# Build and push (only changed layers uploaded)
./stack build-sway
# Output: Pushed registry.helixml.tech/helix/sway:abc123 (uploaded 47MB of 1.6GB)

# Sandbox pulls only changed layers
# Output: Pulled registry.helixml.tech/helix/sway:abc123 (downloaded 47MB)

# Test immediately - no waiting for 1.6GB transfer
```

## Open Questions

1. **Registry authentication**: How do sandbox containers authenticate to registry?
   - Option A: Anonymous pull (public read, authenticated push)
   - Option B: Pass credentials via environment/secrets

2. **Offline mode**: Should we support fully offline operation?
   - Option A: No, require registry access
   - Option B: Fallback to embedded tarball if registry unreachable

3. **Image garbage collection**: How to clean up old image versions in sandbox's dockerd?
   - **Critical**: Must pull new images BEFORE pruning old ones, otherwise we lose shared layers and download the full image anyway (defeating the whole purpose)
   - Sequence: pull new → verify success → prune old
   - Option A: Prune previous version after successful pull of new version
   - Option B: Keep N most recent versions, prune older ones
   - Option C: Disk pressure based - only prune when running low on space

4. **Version coordination**: How to ensure sandbox image version matches desktop image versions?
   - Embed expected versions in sandbox image
   - Validate on pull, warn on mismatch

## Estimated Impact

| Metric | Current | After |
|--------|---------|-------|
| Sandbox image size | ~4GB | ~500MB |
| Full sway rebuild (dev) | 3-5 min | 30-60s |
| Incremental sway change | 2-3 min | 10-20s |
| User upgrade (minor version) | 4GB download | ~100-500MB |
| Cold start (first session) | Instant (pre-loaded) | +30-60s (pull) |

## Next Steps

1. Validate registry access from sandbox containers
2. Prototype build script changes
3. Test layer deduplication savings with real image versions
4. Design enterprise registry configuration UX
