# Claude Rules for HyprMoon Development

This file contains critical development guidelines and context that MUST be followed at all times during HyprMoon development.

## CRITICAL: Always Reference design.md

Before taking any action, ALWAYS read and follow the current `/home/luke/pm/hyprmoon/design.md` file. This file contains:
- Current development context and directory structure
- Key development lessons learned
- Previous findings and known issues
- Current problem description
- Methodical approach phases and steps
- Success criteria

## Key Development Rules (from design.md)

### ALWAYS Follow These Rules:
1. **CRITICAL: ALL BUILDS HAPPEN IN CONTAINERS**: NEVER inspect system packages or attempt to build on the host. ALL builds must happen in containers. NEVER check host system package versions or dependencies.
2. **CRITICAL: NEVER USE BACKGROUND BUILDS**: ALWAYS run builds in foreground only. NEVER use `run_in_background: true` or `&` with build commands. Builds MUST be run synchronously with full output visible. This prevents confusion and lost builds.
3. **CRITICAL: ONLY ONE BUILD AT A TIME**: Never run multiple builds simultaneously. Always wait for current build to complete before starting another.
4. **CRITICAL: HOST ≠ CONTAINER ENVIRONMENTS**: NEVER check package availability on host system. Host runs Ubuntu 24.04, containers run Ubuntu 25.04 - completely different package sets. ALWAYS check package availability using container commands like `docker run ubuntu:25.04 apt search package`. The host system has NO relevance to what's available in build containers.
3. **Build caches are critical**: Without ccache/meson cache, iteration takes too long
4. **Test after every change**: Big-bang approaches are impossible to debug
5. **Use exact Ubuntu source**: Don't deviate from what Ubuntu ships
6. **Container build caches matter**: Use BuildKit mount caches for Docker builds
7. **Git commit discipline**: Make commits after every substantial change
8. **Phase milestone commits**: ALWAYS commit when reaching phase milestones
9. **Manual testing required**: Human verification at every step, no automation
10. **CRITICAL: Always start helix container before manual testing**: MUST check `docker ps | grep helix` and start container if needed before asking user to test via VNC
11. **DOCKERFILE FILENAME UPDATES CRITICAL**: When moving from one Step to the next (e.g., Step 7 -> Step 8), you MUST update the Dockerfile COPY lines to reference the new Step filenames BEFORE running docker build. If you update the Dockerfile after build has started, Docker will use cached layers with the old filenames. ALWAYS verify the Dockerfile has correct Step filenames before building.

### Current Development Context:
- **New methodical repo**: `~/pm/hyprmoon/` (this directory)
- **Previous attempt**: `~/pm/Hyprland-wlroots/` (big-bang approach, caused grey screen)
- **Helix container environment**: `/home/luke/pm/helix/` (test environment)
- **Ubuntu source**: `~/pm/hyprmoon/hyprland-0.41.2+ds/` (baseline)

### Current Problem:
- HyprMoon container shows grey screen in VNC instead of working helix desktop
- Need to isolate which modification broke the rendering/capture
- Using methodical incremental approach instead of big-bang

### Current Status:
Phase 1 Complete: Raw Ubuntu package built and ready for testing
Next: Test baseline, then incremental moonlight integration

## CONSOLIDATED BUILD AND DEPLOYMENT PROCESS (CRITICAL REFERENCE)

**ALWAYS use this EXACT process for building AND deploying HyprMoon deb packages:**

### Step 1: Build the deb packages
```bash
# Navigate to hyprmoon directory
cd /home/luke/pm/hyprmoon

# Run the consolidated build script (handles everything)
./build.sh

# This script:
# - Uses bind mounts with hyprmoon-build-env container
# - Captures timestamped build output automatically
# - Runs container-build.sh inside the container
# - Provides clear success/failure feedback
# - Generates properly named deb files
```

### Step 2: Deploy to helix container (MANDATORY BEFORE TESTING)
```bash
# Copy deb files to helix directory
cp hyprmoon_*.deb hyprland-backgrounds_*.deb /home/luke/pm/helix/

# Navigate to helix directory
cd /home/luke/pm/helix

# Update Dockerfile.zed-agent-vnc with EXACT deb filenames
# (Update COPY lines to match the generated deb filenames)

# Rebuild helix container with new debs
docker compose -f docker-compose.dev.yaml build zed-runner

# CRITICAL: Recreate container to get clean state from image
# NEVER just restart - that preserves modified container state!
docker compose -f docker-compose.dev.yaml down zed-runner
docker compose -f docker-compose.dev.yaml up -d zed-runner
```

### Step 3: Verify deployment before testing
```bash
# Check container is running
docker ps | grep helix

# Wait for container to fully start (give it 30-60 seconds)
# Then connect via VNC on port 5901 for testing
```

**CRITICAL: NEVER test without completing ALL steps above!**

**Why This Process is Mandatory:**
- ✅ **Uses bind mounts**: No Docker layer copying overhead
- ✅ **Ubuntu 25.04 container**: Only environment with required dev packages
- ✅ **Dependency caching**: Build container pre-loaded with all deps
- ✅ **Fast iteration**: 5-10 minutes vs 30+ for full rebuilds
- ✅ **Proven reliable**: Successfully builds with correct versioning
- ✅ **Always latest**: Ensures we test the actual code we just built
- ✅ **Incremental safety**: Prevents testing stale versions that invalidate our methodology

**CRITICAL BUILD REQUIREMENTS:**
- **MANDATORY FOREGROUND ONLY**: NEVER use `run_in_background: true` - ALL builds must run in foreground with full output visible
- **CRITICAL: 15-MINUTE TIMEOUT**: ALWAYS use 15-minute timeout (900000ms) for builds - the default 2-minute timeout is too short and causes hanging with tail -f commands
- **ONE BUILD AT A TIME**: Never run multiple concurrent builds - wait for completion before starting another
- **CRITICAL: DEPENDENCY SYNCHRONIZATION**: When adding new dependencies to `debian/control`, ALWAYS also add them to `Dockerfile.build` in the same session. Both files must be kept in sync to ensure build environment has all required packages.
- **BUILD CACHE ENABLED**: debian/rules has been modified to disable `make clear` - this preserves build cache between runs for much faster iterations
- **BUILD CACHE LOCATION**: Build cache is stored in `/workspace/hyprland-0.41.2+ds/build/` which is bind-mounted to host filesystem
- ALWAYS capture build output to timestamped log files: `command 2>&1 | tee build-$(date +%s).log`
- NEVER use --no-cache flags - we DO want build caching for speed
- NEVER EVER use --no-cache with docker builds - we trust Docker's caching system completely
- **CRITICAL: Monitor CONTAINER logs for actual compiler errors**: The outer build-*.log only shows package management
- **MUST monitor container-build-*.log files**: These contain the actual compilation output and error details
- Builds can take 10+ minutes - be patient and wait for completion
- When builds fail, inspect the complete CONTAINER log file for compiler errors
- Use `find . -name "container-build-*.log" | sort | tail -1` to find the latest container build log
- **Check both logs**: outer build-*.log for overall status, container-build-*.log for compilation errors

## MUST ALWAYS DO BEFORE MANUAL TESTING:
1. Check if helix container is running: `docker ps | grep helix`
2. If not running, start it before asking user to test
3. Provide VNC connection details (port 5901)
4. Only then ask user to manually test via VNC

## CRITICAL: ALWAYS CHECK REFERENCE IMPLEMENTATIONS FOR MISSING CODE

**MANDATORY ORDER when encountering missing files, headers, or dependencies:**

1. **FIRST: Check `~/pm/Hyprland-wlroots/`** - Previous working implementation that compiled successfully
2. **SECOND: Check `~/pm/wolf/`** - Original Wolf moonlight implementation source
3. **LAST RESORT: Comment out with TODO** - Only if not found in either reference

**When to use this process:**
- Missing include files (e.g., `eventbus/event_bus.hpp`, `helpers/logger.hpp`)
- Missing dependencies or libraries
- Unknown moonlight components or modules
- Build errors about missing symbols or definitions
- Any Wolf-specific functionality that needs implementation

**How to use reference implementations:**
- Search for the missing component by name in both directories
- Copy the file structure and adapt include paths for hyprmoon layout
- Maintain the same functionality while fixing include paths
- Reference working patterns for integration approaches

**Previous Implementation Locations:**
- **Previous attempt**: `~/pm/Hyprland-wlroots/` (big-bang approach, caused grey screen but did compile successfully)
- **Original Wolf source**: `~/pm/wolf/` (battle-tested Wolf moonlight implementation)
- Use these as authoritative references for patterns, includes, and integration points
- Specifically useful for Step 3: Global Manager Integration and all Wolf components

This file must be kept up to date with any critical lessons learned during development.