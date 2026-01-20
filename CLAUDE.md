# Helix Development Rules

See also: `.cursor/rules/*.mdc`

## 🚨 FORBIDDEN ACTIONS 🚨

### Git
- **NEVER** `git checkout -- .` or `git reset --hard` — destroys uncommitted work you can't see
- **NEVER** `git stash drop` or `git stash pop` — use `git stash apply` (keeps backup)
- **NEVER** delete `.git/index.lock` — wait or ask user
- **NEVER** push to main — use feature branches, ask user to merge
- **NEVER** amend commits on main — create new commits instead
- **NEVER** delete source files — fix errors, don't delete
- **Before switching branches**: run `git status`, note changes, use `git stash push -m "description"`, restore with `git stash apply`

### Stack Commands
- **NEVER** run `./stack start` — user runs this (needs interactive terminal)
- ✅ OK: `./stack build`, `build-zed`, `build-sway`, `build-ubuntu`, `build-sandbox`, `update_openapi`

### Docker
- **NEVER** use `--no-cache` — trust Docker cache
- **NEVER** run `docker builder prune` or any cache-clearing commands — the cache is correct, you are wrong
- **NEVER** run commands that slow down future builds — trust the build system
- `docker compose restart` does NOT apply .env or image changes — use `down` + `up`
- If Docker cache seems stale: the cache is NOT wrong. Check your assumptions about what triggers rebuilds.

### Other
- **NEVER** rename current working directory — breaks shell session
- **NEVER** commit customer data (hostnames, IPs) — repo is public
- **NEVER** restart hung processes — collect GDB backtraces first

## Build Pipeline

**Sandbox architecture**: Host → helix-sandbox container (Hydra + DinD) → helix-ubuntu container (GNOME + Zed + streaming)

### Component Dependencies

```
helix-sandbox (outer container)
├── hydra (Go, dev container lifecycle, Docker isolation)
└── helix-sway / helix-ubuntu (desktop images, pulled from local registry)
    ├── Desktop environment (Sway or GNOME)
    ├── Zed IDE
    ├── Qwen Code agent
    ├── Go streaming server (api/pkg/desktop/) - WebSocket H.264 streaming
    └── gst-pipewire-zerocopy (Rust, PipeWire ScreenCast → CUDA → nvh264enc)
```

### When to Rebuild What

| Changed | Command | Notes |
|---------|---------|-------|
| Hydra (`api/pkg/hydra/`) | `./stack build-sandbox` | Hydra binary runs IN sandbox, NOT API |
| Desktop image (helix-sway) | `./stack build-sway` | Pushes to local registry, updates `sandbox-images/helix-sway.version` |
| Desktop image (helix-ubuntu) | `./stack build-ubuntu` | Pushes to local registry, updates `sandbox-images/helix-ubuntu.version` |
| Desktop streaming (`api/pkg/desktop/`) | `./stack build-ubuntu` or `./stack build-sway` | Go code runs IN desktop container, NOT API |
| Zerocopy plugin (`desktop/gst-pipewire-zerocopy/`) | `./stack build-ubuntu` or `./stack build-sway` | Rust plugin built inside desktop image |
| Sandbox scripts | `./stack build-sandbox` | Dockerfile.sandbox changes |
| Zed IDE | `./stack build-zed && ./stack build-sway` | Zed binary → desktop image |
| Qwen Code | `cd ../qwen-code && git commit -am "msg" && cd ../helix && ./stack build-sway` | Needs git commit |

### Build Order for Full Rebuild

```bash
# 1. Build Zed (if changed)
./stack build-zed

# 2. Build desktop images (pushes to local registry, includes streaming + zerocopy plugin)
./stack build-sway
./stack build-ubuntu

# 3. Build sandbox (only if Hydra or sandbox scripts changed)
./stack build-sandbox

# 4. Start a new session to use the updated desktop image
# No sandbox restart needed - new sessions auto-pull from local registry
```

### Verify Build

```bash
# Check desktop image versions
cat sandbox-images/helix-sway.version
cat sandbox-images/helix-ubuntu.version

# Verify image is available in sandbox's dockerd
docker compose exec -T sandbox docker images | grep helix-
```

New sessions auto-pull from local registry. Version flow: build writes `.version` files → sandbox heartbeat reads them → API looks up version from heartbeat when starting sessions. Existing containers don't update.

## Code Patterns

### Go
- Fail fast: `return fmt.Errorf("failed: %w", err)` — never log and continue
- Use structs, not `map[string]interface{}` for API responses
- GORM AutoMigrate only — no SQL migration files
- Use gomock, not testify/mock
- **NO FALLBACKS**: Pick one approach that works and stick to it. Fallback code paths are rarely tested and add complexity. If something doesn't work, fix it properly instead of adding a fallback.

### TypeScript/React

#### 🚨 CRITICAL: ALWAYS Use Generated TypeScript API Client 🚨

**NEVER use manual `fetch()`, `api.post()`, `api.get()`, or raw HTTP calls in frontend code.**

The generated API client (`frontend/src/api/api.ts`) provides type-safe methods for ALL backend endpoints:

```typescript
// ✅ CORRECT - use generated client
const apiClient = api.getApiClient();
await apiClient.v1SessionsResumeCreate(sessionId);
await apiClient.v1ExternalAgentsUploadCreate(sessionId, { file }, { open_file_manager: false });

// ❌ WRONG - never do this
await api.post(`/api/v1/sessions/${sessionId}/resume`);
await fetch(`/api/v1/external-agents/${sessionId}/upload`, { ... });
```

**If an endpoint is missing from the generated client:**
1. Add swagger annotations to the Go handler (see `api/pkg/server/*_handlers.go`)
2. Run `./stack update_openapi` to regenerate the client
3. Then use the generated method

**Benefits:**
- Type safety for request/response bodies
- Auto-completion in IDE
- Breaking API changes caught at compile time
- Consistent error handling

- Use generated API client + React Query for ALL API calls
- Extract `.data` from Axios responses in query functions
- No `setTimeout` for async — use events/promises
- Extract components when files exceed 500 lines
- No `type="number"` inputs — use text + parseInt
- **useEffect/useCallback dependency arrays**: ONLY include primitive data values that actually change. NEVER include:
  - Context values (`streaming`, `api`, `snackbar`, `helixApi`, `account`, etc.)
  - Functions (they're stable references from hooks)
  - Refs (they're mutable and don't trigger re-renders)
  - Objects from hooks (use specific primitive properties instead)

  **Correct**: `[sessionId]` or `[sessionId, projectId]`
  **WRONG**: `[sessionId, helixApi]` or `[sessionId, snackbar]`

### Frontend
- Use ContextSidebar pattern (see `ProjectsSidebar.tsx`)
- Invalidate queries after mutations, don't use setQueryData

## Architecture

**ACP connects Zed ↔ Agent, NOT Agent ↔ LLM**
```
LLM ←(OpenAI API)→ Qwen Code Agent ←(ACP)→ Zed IDE
```

**RBAC**: Use `authorizeUserToResource()` — one unified AccessGrants system

**Enterprise context**: Support internal DNS, proxies, air-gapped networks, private CAs

## Verification

### Frontend Pre-commit Check (matches Drone CI)
**ALWAYS run before committing frontend changes:**
```bash
cd frontend && yarn test && yarn build && cd ..
```
This runs the same checks as Drone CI. Fix any errors before committing.

### Quick Checks
After frontend changes (dev mode):
```bash
docker compose -f docker-compose.dev.yaml logs --tail 50 frontend | grep -i error
# Then ask user to verify page loads
```

After API changes:
```bash
docker compose -f docker-compose.dev.yaml logs --tail 30 api | grep -E "building|running|failed"
```

## API Authentication

### 🚨 CRITICAL: Use `.env.usercreds` with explicit exports 🚨

**NEVER** use `oh-hallo-insecure-token` - this is the runner system token, NOT a user API key.
User operations (creating tasks, sessions, screenshots, etc.) require a real user API key.

**The correct file is `.env.usercreds`** which contains:
- `HELIX_API_KEY` - User's API key (starts with `hl-`)
- `HELIX_URL` - API server URL (e.g., `http://localhost:8080`)
- `HELIX_UBUNTU_AGENT` - Ubuntu agent ID for testing
- `HELIX_PROJECT` - Project ID for creating tasks

**IMPORTANT**: `source .env.usercreds` does NOT export variables! You must explicitly export:

```bash
# CORRECT - explicitly export each variable (use backticks, NOT $() - see note below)
# Use -f2- to preserve values containing = (like base64 API keys)
export HELIX_API_KEY=`grep HELIX_API_KEY .env.usercreds | cut -d= -f2-`
export HELIX_URL=`grep HELIX_URL .env.usercreds | cut -d= -f2-`

# Or use set -a to auto-export (then source)
set -a && source .env.usercreds && set +a

# Or inline export for one-off commands
export HELIX_API_KEY="hl-xxx" HELIX_URL="http://localhost:8080" && /tmp/helix spectask list
```

**⚠️ Shell escaping bug**: The Bash tool incorrectly escapes `$()` command substitution
(escapes `$` and adds space before `(`). Use backticks `` `command` `` instead of `$(command)`.

**File convention:**
- `.env.usercreds` - **Primary file** for CLI testing (HELIX_API_KEY + HELIX_URL + agent/project IDs)
- `.env.userkey` - Minimal file with just `HELIX_API_KEY=hl-xxx`

**401 Unauthorized errors?** You're probably using the wrong token. Check:
```bash
echo $HELIX_API_KEY  # Should start with "hl-", NOT "oh-hallo-insecure-token"
```

## Quick Reference

- Build helix CLI: `cd api && CGO_ENABLED=0 go build -o /tmp/helix-bin .` (creates executable)
- Regenerate API client: `./stack update_openapi`
- Kill stuck builds: `pkill -f "cargo build" && pkill -f rustc`
- Design docs and implementation plans go in `design/YYYY-MM-DD-name.md` (not `.claude/plans/`)

## Testing CLI Commands

### Helix CLI (spectask subcommand)

Build the CLI first:
```bash
cd api && CGO_ENABLED=0 go build -o /tmp/helix . && cd ..
```

Set up environment:
```bash
source .env.userkey
export HELIX_URL="http://localhost:8080"
# HELIX_API_KEY is already set from .env.userkey
```

**Session Management:**
```bash
/tmp/helix spectask list              # List sessions with external agents
/tmp/helix spectask list-agents       # List available Helix agents/apps
/tmp/helix spectask start --project <prj_id> -n "Task name"  # Create new task + sandbox
/tmp/helix spectask resume <session-id>   # Resume existing session
/tmp/helix spectask stop <session-id>     # Stop a session
/tmp/helix spectask stop --all            # Stop ALL sessions
```

**Screenshot Testing:**
```bash
/tmp/helix spectask screenshot <session-id>   # Saves screenshot to current dir
```

**Video Stream Testing:**
```bash
# Connect to WebSocket video stream and display real-time stats
/tmp/helix spectask stream <session-id>

# Run for 30 seconds then exit
/tmp/helix spectask stream <session-id> --duration 30

# Save raw video frames to file
/tmp/helix spectask stream <session-id> --output video.h264

# Verbose mode (show each frame)
/tmp/helix spectask stream <session-id> -v
```

**Interactive Testing:**
```bash
# Live interactive mode with video stats + VLC server
/tmp/helix spectask live <session-id> --vlc :8081

# Send text input to session
/tmp/helix spectask send <session-id> "hello world"
```

**MCP Testing:**
```bash
# Test MCP screenshot tool via session's MCP server
/tmp/helix spectask mcp <session-id>
```

### Sandbox Service Names
The sandbox service name depends on GPU type:
- `sandbox-nvidia` - Systems with NVIDIA GPU
- `sandbox` - Systems without GPU (uses software encoding)

Use the correct service name when running docker compose exec commands:
```bash
# NVIDIA GPU systems
docker compose exec -T sandbox-nvidia docker images | grep helix-

# Non-GPU systems
docker compose exec -T sandbox docker images | grep helix-
```

### Image Versions
```bash
# Check desktop image versions
cat sandbox-images/helix-sway.version
cat sandbox-images/helix-ubuntu.version

# Verify image is available in sandbox's dockerd
docker compose exec -T sandbox docker images | grep helix-
```

### Logs
```bash
# Desktop container logs (inside sandbox)
docker compose exec -T sandbox docker logs {CONTAINER_NAME} 2>&1 | grep -E "screenshot|capture|pipewire|zerocopy"

# Sandbox logs
docker compose logs --tail 50 sandbox 2>&1 | grep -E "session|GPU|hydra"

# API logs for external agents
docker compose logs --tail 50 api 2>&1 | grep -E "external-agent|screenshot|session"
```

### Desktop Container Log Locations
Both desktops use `desktop-bridge` which logs to stdout (visible in `docker logs`).

```bash
# Find container name:
docker compose exec -T sandbox-nvidia docker ps --format "{{.Names}}" | grep -E "ubuntu-external|sway-external"

# View logs (both Ubuntu and Sway use the same pattern now)
docker compose exec -T sandbox-nvidia docker logs {CONTAINER} 2>&1 | grep -E "PIPEWIRE|zerocopy|desktop-bridge"

# Other log files:
docker compose exec -T sandbox-nvidia docker exec {CONTAINER} cat /tmp/settings-sync.log
```

### Debugging pipewirezerocopysrc (Zero-Copy GPU Streaming)

**Step 1: Find the container name**
```bash
docker compose exec -T sandbox-nvidia docker ps --format "{{.Names}}" | grep ubuntu-external
```

**Step 2: Check docker logs (contains pipewirezerocopysrc logs)**
```bash
docker compose exec -T sandbox-nvidia docker logs {CONTAINER_NAME} 2>&1 | grep -E "PIPEWIRE_DEBUG|EXT_IMAGE_COPY|zerocopy"
```

**Step 3: Run benchmark with zerocopy mode**
```bash
# Start a session AFTER rebuilding helix-ubuntu (new sessions use new images)
/tmp/helix spectask start --agent $HELIX_UBUNTU_AGENT --project $HELIX_PROJECT -n "test"

# Wait ~15 seconds for GNOME to initialize, then run benchmark
# CRITICAL: --video-mode zerocopy forces use of pipewirezerocopysrc
/tmp/helix spectask benchmark ses_01xxx --video-mode zerocopy --duration 15
```

**Key debug patterns to look for:**
```
# Good: Our element is running
[PIPEWIRE_DEBUG] PipeWire state: Unconnected -> Connecting -> Paused

# Good: NVIDIA tiled modifier detected
[PIPEWIRE_DEBUG] Format modifier=0x300000000e08014 vendor_id=0x3 is_gpu_tiled=true

# Good: DmaBuf requested
[PIPEWIRE_DEBUG] Buffer types: 0x8 (DmaBuf (zero-copy))

# Good: Frames flowing
[PIPEWIRE_FRAME] First frame received from PipeWire

# Bad: Buffer allocation failed
[PIPEWIRE_DEBUG] PipeWire state: Paused -> Error("error alloc buffers: Invalid argument")

# Bad: Using SHM instead of DmaBuf
[PIPEWIRE_DEBUG] Buffer types: 0x4 (MemFd (SHM fallback))
```

**Common mistakes:**
1. Running benchmark on a session started BEFORE rebuilding - must start NEW session
2. Forgetting `--video-mode zerocopy` - without it, uses native pipewiresrc instead
3. GNOME ScreenCast sends multiple Format callbacks with different modifiers - this is normal

**Modifier debugging:**
- 0x0 = LINEAR (no tiling, triggers SHM fallback)
- 0x300000000xxxxx = NVIDIA tiled (vendor ID 0x03 in bits 56-63)
- 0x00ffffffffffffff = DRM_FORMAT_MOD_INVALID ("any modifier")

## Video Streaming Performance Testing

### 🚨 ALWAYS Use Benchmark CLI for Video Testing 🚨

When testing video streaming performance (FPS, latency, frame drops), **ALWAYS use the benchmark CLI** with vkcube or active screen content:

```bash
# 1. Start a NEW session (existing sessions won't have your code changes)
export HELIX_API_KEY=`grep HELIX_API_KEY .env.usercreds | cut -d= -f2-`
export HELIX_URL=`grep HELIX_URL .env.usercreds | cut -d= -f2-`
export HELIX_PROJECT=`grep HELIX_PROJECT .env.usercreds | cut -d= -f2-`
/tmp/helix spectask start --project $HELIX_PROJECT -n "video test"

# 2. Wait for GNOME to initialize (~15 seconds)
sleep 15

# 3. Run benchmark with active content (vkcube generates 60 FPS damage)
# Replace ses_xxx with your actual session ID from step 1
/tmp/helix spectask benchmark ses_xxx --duration 30

# 4. Check the output for FPS and frame timing
# Target: 60 FPS with active content, 10 FPS with static screen
```

### Why vkcube/Active Content Matters

GNOME uses **damage-based ScreenCast** in headless mode:
- Static screen → ~10 FPS (keepalive timer only)
- Terminal with output → 15-35 FPS (depends on terminal update rate)
- vkcube → 60 FPS (constant GPU rendering = constant damage)

**Never test video FPS on a static desktop** - you'll only see 10 FPS which is expected behavior.

### Frame Rate by Damage Source

| Damage Source | Expected FPS | Notes |
|---------------|--------------|-------|
| Static screen | 10 | Keepalive timer, NOT a bug |
| Kitty terminal | ~17 | Kitty has internal frame pacing |
| Terminal (ghostty) fast output | 35-40 | More damage events |
| vkcube (GPU rendering) | 55-60 | Constant damage at refresh rate |

### Debug Commands

```bash
# Check PipeWire node state (inside desktop container)
docker compose exec -T sandbox-nvidia docker exec {CONTAINER_NAME} pw-dump | grep -A20 '"state"'

# Check if zero-copy is enabled (look for modifier 0x300000000e08xxx)
docker compose exec -T sandbox-nvidia docker logs {CONTAINER_NAME} 2>&1 | grep "modifier="

# Force zerocopy mode in benchmark
/tmp/helix spectask benchmark ses_xxx --video-mode zerocopy --duration 30
```

## CLI Development

**ALWAYS use the helix CLI** for testing and debugging - never use raw curl commands to call API endpoints:

```bash
# Good - use CLI
/tmp/helix spectask screenshot ses_01xxx
/tmp/helix spectask stream ses_01xxx

# Bad - don't use curl for things the CLI can do
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/external-agents/xxx/screenshot
```

**Why?**
1. CLI has proper error handling and user-friendly output
2. CLI handles authentication via environment variables
3. CLI changes are tested and documented
4. If functionality is missing or broken, **add it to the CLI** - this improves the product

**Adding CLI functionality:**
- `helix spectask` commands are in `api/pkg/cli/spectask/`
- Follow existing patterns (cobra commands, getAPIURL/getToken helpers)
- Add new subcommands when needed rather than using curl workarounds
