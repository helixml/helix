# Requirements: Test Shared BuildKit Cache Performance

## Overview

Validate that the shared BuildKit cache (`helix-buildkit` container + `helix-shared` buildx builder) improves Docker build times across multiple spectask sessions.

## User Stories

### US-1: Verify cache sharing across sessions
**As a** platform operator  
**I want to** confirm that BuildKit cache is shared between different spectask sessions  
**So that** users benefit from faster builds when dependencies are already cached

### US-2: Measure performance improvement
**As a** developer  
**I want to** quantify the build time improvement from shared cache  
**So that** we can validate the feature works and track regressions

## Acceptance Criteria

1. **Cache hit verification**: Second build of the same Dockerfile in a different session uses cached layers
2. **Measurable speedup**: Cached build is at least 50% faster than cold build
3. **Builder routing**: `docker build` commands route through `helix-shared` builder (not local daemon)
4. **Cross-session persistence**: Cache survives session termination and sandbox restart

## Test Scenarios

| Scenario | Cold Build | Warm Build | Expected Speedup |
|----------|------------|------------|------------------|
| Simple Dockerfile (apt install) | ~60s | ~5s | >10x |
| Multi-stage with pip/npm | ~120s | ~15s | >5x |

## Out of Scope

- Performance under concurrent builds (future work)
- Cache eviction policies
- Network-based cache sharing (S3, etc.)