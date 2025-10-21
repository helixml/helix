# Claude Rules for Helix Development

This file contains critical development guidelines and context that MUST be followed at all times during Helix development.

## Documentation Organization

**CRITICAL: LLM-generated design documents MUST live in `design/` folder ONLY**

- **`design/`**: All LLM-generated design docs, architecture decisions, status reports, debugging logs
  - These are internal development artifacts created during the feature development process
  - Naming convention: `YYYY-MM-DD-descriptive-name.md` (e.g., `2025-09-23-wolf-streaming-architecture.md`)
  - Date should reflect when the document was written, enabling chronological navigation

- **`docs/`**: External user-facing documentation ONLY
  - User guides, API documentation, deployment instructions
  - Documentation meant for external consumption
  - Should NOT contain LLM-generated design artifacts

- **Root level**: Only `README.md`, `CONTRIBUTING.md`, and `CLAUDE.md` (this file)

**Why this matters:**
- Keeps internal design artifacts separate from user-facing documentation
- Dates in filenames provide clear chronological history of development decisions
- Makes it easy to follow the evolution of architectural decisions over time

## Hot Reloading Development Stack

The Helix development stack has hot reloading enabled in multiple components for fast iteration:

- **Frontend**: Vite-based hot reloading for React/TypeScript changes
- **API Server**: Air-based hot reloading for Go API changes
- **GPU Runner**: Live code reloading for runner modifications
- **Wolf Integration**: Real-time config and code updates
- **Zed Editor**: Directory bind-mount + auto-restart loop for binary updates

This means you often don't need to rebuild containers - just save files and changes are picked up automatically.

## CRITICAL: Zed Editor Build Process

**ALWAYS use the stack command for building Zed - NEVER use cargo directly**

```bash

./stack up        # Default: dev mode (fast, ~1.3GB binary)
./stack dev       # Explicit dev mode (fast, debug symbols)
./stack release   # Release mode (slow, ~2GB optimized binary)

# ❌ WRONG: Direct cargo commands will fail or produce broken binaries
cargo build --package zed         # Missing feature flag!
cargo build --release --package zed  # Wrong output location!
```

**Why stack script is MANDATORY:**

- Stack script uses correct paths and build flags automatically
- Both dev and release builds work correctly (dev is faster for iteration)

## Key Development Rules (from design.md)

### ALWAYS Follow These Rules:
1. **CRITICAL: ALL BUILDS HAPPEN IN CONTAINERS**: NEVER inspect system packages or attempt to build on the host. ALL builds must happen in containers. NEVER check host system package versions or dependencies.
2. **CRITICAL: NEVER USE BACKGROUND BUILDS**: ALWAYS run builds in foreground only. NEVER use `run_in_background: true` or `&` with build commands. Builds MUST be run synchronously with full output visible. This prevents confusion and lost builds.
3. **CRITICAL: ONLY ONE BUILD AT A TIME**: Never run multiple builds simultaneously. Always wait for current build to complete before starting another.
4. **CRITICAL: HOST ≠ CONTAINER ENVIRONMENTS**: NEVER check package availability on host system. Host runs Ubuntu 24.04, containers run Ubuntu 25.04 - completely different package sets. ALWAYS check package availability using container commands like `docker run ubuntu:25.04 apt search package`. The host system has NO relevance to what's available in build containers.
5. **CRITICAL: NEVER USE --no-cache**: NEVER use `--no-cache` with Docker builds. We trust Docker's caching system completely and --no-cache is wasteful and unnecessary. Docker's layer caching is reliable and speeds up builds significantly.
3. **Build caches are critical**: Without ccache/meson cache, iteration takes too long
4. **Test after every change**: Big-bang approaches are impossible to debug
5. **Use exact Ubuntu source**: Don't deviate from what Ubuntu ships
6. **Container build caches matter**: Use BuildKit mount caches for Docker builds
7. **Git commit discipline**: Make commits after every substantial change
8. **Phase milestone commits**: ALWAYS commit when reaching phase milestones
9. **Manual testing required**: Human verification at every step, no automation
10. **CRITICAL: Always start helix container before manual testing**: MUST check `docker ps | grep helix` and start container if needed before asking user to test via VNC
11. **DOCKERFILE FILENAME UPDATES CRITICAL**: When moving from one Step to the next (e.g., Step 7 -> Step 8), you MUST update the Dockerfile COPY lines to reference the new Step filenames BEFORE running docker build. If you update the Dockerfile after build has started, Docker will use cached layers with the old filenames. ALWAYS verify the Dockerfile has correct Step filenames before building.
12. **CRITICAL: NEVER WRITE FALLBACK CODE**: NEVER write fallback code without explicit permission from the user. Fallbacks hide real problems and make debugging harder. If something fails (e.g., can't read a file, missing dependency), FAIL LOUDLY with a clear error message. Let the user know exactly what went wrong so it can be fixed properly.
13. **CRITICAL: ALWAYS CHECK API HOT RELOAD LOGS AFTER CODE CHANGES**: After making Go code changes, IMMEDIATELY tail the API logs to verify the hot reloader built successfully: `docker compose -f docker-compose.dev.yaml logs --tail 30 api`. The Air hot reloader provides instant feedback on build errors. NEVER assume code compiled successfully without checking these logs.



## MUST ALWAYS DO BEFORE MANUAL TESTING:
1. Check if helix container is running: `docker ps | grep helix`
2. If not running, start it before asking user to test
3. Provide VNC connection details (port 5901)
4. Only then ask user to manually test via VNC
