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
# ✅ CORRECT: Build Zed using stack script with external_websocket_sync feature
./stack build-zed           # Default: dev mode (fast, ~1.3GB binary)
./stack build-zed dev       # Explicit dev mode (fast, debug symbols)
./stack build-zed release   # Release mode (slow, ~2GB optimized binary)

# ❌ WRONG: Direct cargo commands will fail or produce broken binaries
cargo build --package zed         # Missing feature flag!
cargo build --release --package zed  # Wrong output location!
```

**Why stack script is MANDATORY:**
- Automatically includes `--features external_websocket_sync` flag (critical for Helix integration)
- Binary is copied to `./zed-build/zed` and bind-mounted into containers
- Stack script uses correct paths and build flags automatically
- Both dev and release builds work correctly (dev is faster for iteration)

**CRITICAL: Kill Old Builds First**
```bash
# ALWAYS kill any existing cargo builds before starting new one
pkill -f "cargo build" && pkill -f rustc

# Then build with stack
./stack build-zed
```

**Why killing old builds is critical:**
- Multiple simultaneous cargo builds cause resource exhaustion
- Old builds consume CPU/RAM and slow down new builds massively
- Conflicting builds can produce corrupted binaries
- Always verify no cargo processes running: `ps aux | grep -E "cargo build|rustc" | grep -v grep`

**Zed Hot Reload Workflow:**
1. **Kill any running builds**: `pkill -f "cargo build" && pkill -f rustc`
2. Make changes to Zed source code in `~/pm/zed`
3. Build: `./stack build-zed` (~30-60 seconds for incremental dev builds)
4. Inside running PDE: Close Zed window (click X)
5. Auto-restart script launches updated binary in 2 seconds
6. No container recreation needed - directory bind-mount survives inode changes

## CRITICAL: Sway Container Image Build Process

**ALWAYS use the stack command for building the Sway image - NEVER use docker build directly**

```bash
# ✅ CORRECT: Build helix-sway image using stack script
./stack build-sway

# ❌ WRONG: Direct docker commands may miss important configurations
docker build -f Dockerfile.sway-helix -t helix-sway:latest .
```

**Why stack script is MANDATORY:**
- Ensures correct image tag (`helix-sway:latest`)
- Consistent build process with other stack operations
- Provides clear success/failure feedback with feature summary
- Used by Wolf executor for both PDEs and External Agents

**When to rebuild the Sway image:**
- After modifying startup scripts in `wolf/sway-config/`
- After modifying `Dockerfile.sway-helix`
- After updating Go daemons (settings-sync-daemon, screenshot-server)
- After changing Sway/waybar configurations

**Critical files in the Sway image:**
- `/usr/local/bin/start-zed-helix.sh` - Zed startup script with initialization wait
- `/opt/gow/startup-app.sh` - GOW launcher configuration
- `/usr/local/bin/settings-sync-daemon` - Zed settings synchronization
- `/usr/local/bin/screenshot-server` - Screenshot capture for streaming

**After rebuilding, NEW external agent sessions will use the updated image automatically.**
Existing containers will NOT pick up changes - you must create a new session to test.

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
- **WAIT DURING LONG BUILDS**: For Docker container builds that take several minutes, use `sleep 60` between status checks instead of continuously monitoring. Check completion with `docker images | grep {image_name}` after sleeping
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

## CRITICAL: Wolf API Testing
**MANDATORY: Wolf API calls must be made from INSIDE the Wolf container using the Unix socket**

To query Wolf APIs properly:
```bash
# Install curl in Wolf container first (one-time setup)
docker compose -f docker-compose.dev.yaml exec wolf bash -c "apt update && apt install -y curl jq"

# Query Wolf API via Unix socket (correct method)
docker compose -f docker-compose.dev.yaml exec wolf bash -c "curl -s --unix-socket /var/run/wolf/wolf.sock http://localhost/api/v1/apps" | jq '.'

# NOT from host (this will fail):
curl "http://localhost:47989/api/v1/apps"  # Wrong - external API doesn't exist
```

This is required because Wolf's internal API is only accessible via Unix socket from within the container.

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

## Wolf Development Workflow

**Wolf Integration Development Commands:**

```bash
# Rebuild Wolf container with latest source code changes
./stack rebuild-wolf

# Auto-build Wolf during development startup (built into ./stack start)
./stack start
```

### Wolf Development Process:
1. **Make changes** to Wolf source code in `../wolf/src/`
2. **Rebuild Wolf**: `./stack rebuild-wolf` (builds and restarts Wolf container)
3. **Test integration**: Wolf changes are immediately available to Helix API
4. **Iterate quickly**: No need to restart entire stack, just rebuild Wolf

### Key Benefits:
- ✅ **Fast iteration**: Rebuild only Wolf container (~30 seconds)
- ✅ **Automatic startup**: `./stack start` builds Wolf if container missing
- ✅ **Source integration**: Uses latest Wolf source code with fixes
- ✅ **Hot reloading ready**: Integrates with existing Helix hot reloading stack

## ENHANCED BUILD SYSTEM WITH COMMIT TRACKING

The build.sh script now supports commit messages and automatic version tracking:

### Usage:
```bash
# Standard build
./build.sh

# Build with commit message (auto-commits changes)
./build.sh "Fix PIN promise destruction in pairing flow"
./build.sh "Add certificate debugging for HTTPS server"
./build.sh "Implement incremental build optimization"
```

### Features:
- **Auto-version bumping**: Each build gets unique step8.9.X version
- **Commit message integration**: Message goes into debian/changelog and git commit
- **Automatic commits**: Changes committed after successful builds with descriptive metadata
- **Build metrics**: Duration, strategy, binary MD5 included in commit
- **Fast iteration**: Sub-30 second incremental builds enable rapid debugging

### ALWAYS use commit messages during debugging sessions:
- Makes it easy to track progress through complex issues
- Creates searchable git history of all debugging attempts
- Connects build versions to specific debugging goals
- Enables rollback to working states if needed

## HyprMoon Streaming Development Process

### Current Achievement: Complete Moonlight Streaming Infrastructure

**Status: STREAMING IS WORKING**

### Build Process for Streaming Features:
1. **Navigate to hyprmoon directory**: `cd /home/luke/pm/hyprmoon`
2. **Make code changes** to streaming components
3. **Build with descriptive commit**: `./build.sh "Add Wolf RTSP server integration"`
4. **Deploy to helix**: Copy debs and rebuild container
5. **Test streaming**: Use `./complete_pairing_test.sh` or manual Moonlight client

### Key Streaming Components Built:
- **Wolf Streaming Engine**: Battle-tested components integrated
- **Native Hyprland Frame Capture**: Zero-copy GPU integration
- **Complete Server Stack**: HTTP, HTTPS, RTSP, RTP, Control servers
- **Moonlight Protocol**: 4-phase pairing, certificates, authentication
- **Direct Streaming Integration**: Bypass complex event coordination

### Streaming Test Commands:
```bash
# Test complete pairing and streaming
./complete_pairing_test.sh

# Test manual streaming
moonlight stream localhost "Hyprland Desktop" --quit-after --1080

# Test individual components
curl "http://localhost:47989/serverinfo?uniqueid=test&uuid=test"
curl "http://localhost:47989/launch?uniqueid=test&uuid=test&appid=1"
echo -e "OPTIONS rtsp://127.0.0.1:48010 RTSP/1.0\r\n\r\n" | nc 127.0.0.1 48010
```

### Critical Lessons for Streaming Development:
1. **Use Wolf's battle-tested components directly** - don't reinvent
2. **Native Hyprland integration** better than virtual environments
3. **Direct function calls** simpler than complex event coordination
4. **Fix include paths systematically** when integrating Wolf code
5. **Test each server component individually** before full integration

### Wolf Parallel Sessions Support

**CRITICAL: Wolf has been modified to allow session persistence while enabling proper cleanup**

The Moonlight client sends various stop/cancel signals when users quit or disconnect. By default, this would stop the streaming session and container. We've modified Wolf to distinguish between:

1. **Moonlight protocol events** (client-initiated) - Sessions persist
2. **Wolf internal API** (Helix-initiated) - Proper cleanup happens

**Modification Locations - Moonlight Protocol (client-initiated, sessions persist)**:

1. `/home/luke/pm/wolf/src/moonlight-server/control/control.cpp`
   - TERMINATION packet handling (line ~205): Disabled PauseStreamEvent firing
   - DISCONNECT event handling (line ~175): Disabled PauseStreamEvent firing
   - **Result**: Sessions continue running when client quits or disconnects via control protocol

2. `/home/luke/pm/wolf/src/moonlight-server/rest/endpoints.hpp`
   - Function: `cancel` (line ~514): Disabled StopStreamEvent firing
   - **Result**: Sessions continue running when client calls `/cancel` HTTPS endpoint
   - This endpoint is part of Moonlight HTTPS protocol used during pairing/launch

3. `/home/luke/pm/wolf/src/moonlight-server/rest/servers.cpp`
   - Function: HTTPS `serverinfo` handler (line ~123): Always passes `std::nullopt` instead of `client_session`
   - **Result**: Moonlight client always sees server as available (not busy)
   - Prevents client from complaining "running game wasn't started on this PC"
   - Allows starting new sessions without client trying to stop existing ones first

4. `/home/luke/pm/wolf/src/moonlight-server/rest/endpoints.hpp`
   - Function: `launch` (line ~456): Added duplicate prevention check
   - Checks if client already has active session for the same app
   - If yes, reuses existing session instead of creating new container
   - **Result**: Prevents duplicate containers when client calls /launch repeatedly
   - Works with serverinfo hiding - client calls /launch instead of /resume

**Unmodified (intentionally) - Wolf Internal API (Helix-initiated, proper cleanup)**:

- `/home/luke/pm/wolf/src/moonlight-server/api/endpoints.cpp`
- Function: `endpoint_StreamSessionStop` - Still fires StopStreamEvent
- **Result**: When Helix calls `/api/v1/sessions/stop`, containers are properly stopped and removed
- This prevents session/container reuse bugs when deleting and recreating PDEs

**To rebuild Wolf with modifications**: `docker compose -f docker-compose.dev.yaml build wolf && docker compose -f docker-compose.dev.yaml down wolf && docker compose -f docker-compose.dev.yaml up -d wolf`

This enables:
- ✅ Session persistence when clients quit (Ctrl+Shift+Alt+Q)
- ✅ Session persistence when clients call /cancel endpoint
- ✅ Reconnection to existing sessions without restart
- ✅ Multiple parallel Personal Dev Environment sessions
- ✅ Proper container cleanup when Helix explicitly deletes PDEs
- ✅ No session/container reuse bugs

## Using Generated TypeScript Client and React Query

**IMPORTANT: Always use the generated TypeScript client instead of manual API calls**

### Regenerating the TypeScript Client
When adding new API endpoints with swagger annotations:
```bash
./stack update_openapi
```

This generates:
- Swagger documentation from Go code
- TypeScript client with proper types in `frontend/src/api/api.ts`
- Type-safe API methods matching the backend exactly

### Required Swagger Annotations for API Endpoints
All API handlers must include proper swagger annotations for client generation:

```go
// @Summary List personal development environments
// @Description Get all personal development environments for the current user
// @Tags PersonalDevEnvironments
// @Accept json
// @Produce json
// @Success 200 {array} PersonalDevEnvironmentResponse
// @Failure 401 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/personal-dev-environments [get]
func (apiServer *HelixAPIServer) listPersonalDevEnvironments(res http.ResponseWriter, req *http.Request) {
```

### Using the Generated Client with React Query (MANDATORY)
**CRITICAL: ALL frontend API interactions MUST use React Query**

```typescript
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { ServerPersonalDevEnvironmentResponse, ServerCreatePersonalDevEnvironmentRequest } from '../api/api'
import useApi from '../hooks/useApi'

const api = useApi()
const apiClient = api.getApiClient() // Get the generated API client

// Use React Query for data fetching
const { data: environments = [], isLoading, error } = useQuery({
  queryKey: ['personal-dev-environments'],
  queryFn: () => apiClient.v1PersonalDevEnvironmentsList(),
  select: (response) => response.data || [],
  enabled: !!account.user
})

// Use React Query for mutations
const queryClient = useQueryClient()
const createEnvironmentMutation = useMutation({
  mutationFn: (request: ServerCreatePersonalDevEnvironmentRequest) =>
    apiClient.v1PersonalDevEnvironmentsCreate(request),
  onSuccess: () => {
    queryClient.invalidateQueries({ queryKey: ['personal-dev-environments'] })
  }
})

// Trigger mutations
const handleCreate = () => {
  createEnvironmentMutation.mutate({
    environment_name: 'test-env',
    app_id: 'app-123'
  })
}
```

### React Query Requirements and Conventions
- **MUST use React Query** for all API calls (queries and mutations)
- **ALWAYS refactor existing code** to use React Query when touching API-related components
- **Use proper query keys** for cache management
- **Invalidate queries** after mutations to refresh data
- **Handle loading and error states** through React Query hooks

### Common Usage Patterns
```typescript
// Error handling with proper type checking
{error && (
  <Alert severity="error" sx={{ mb: 2 }}>
    {error instanceof Error ? error.message : 'Default error message'}
  </Alert>
)}

// Refresh button using query invalidation
<IconButton
  onClick={() => queryClient.invalidateQueries({ queryKey: ['query-key'] })}
  disabled={loading}
>
  <RefreshIcon />
</IconButton>

// Handle mutation errors separately
{createMutation.error && (
  <Alert severity="error" sx={{ mb: 2 }}>
    {createMutation.error instanceof Error
      ? createMutation.error.message
      : 'Failed to create item'}
  </Alert>
)}

// Use optional chaining for nullable generated types
environment.instanceID || ''
environment.configured_tools && environment.configured_tools.length > 0
```

### Key Benefits
- **Type Safety**: Automatic TypeScript types matching backend structs
- **API Consistency**: Generated methods match exact backend endpoints
- **Field Name Accuracy**: No manual field name mismatches (PascalCase vs snake_case)
- **Automatic Updates**: Regenerating updates all types and methods

This file must be kept up to date with any critical lessons learned during development.

## Wolf Streaming Platform Operations and Debugging

### CRITICAL: Wolf App Management and Persistence

**Wolf App Storage Behavior:**
- **Static apps** are defined in `/home/luke/pm/helix/wolf/config.toml` and persist across restarts
- **Dynamic apps** (created via API) are stored in Wolf's internal state and **cleared on restart**
- **Restarting Wolf is safe** and clears broken dynamic apps while preserving working static configuration

### Wolf API Access (MANDATORY METHOD)

**CRITICAL: Always access Wolf API from inside API container via shared socket:**

```bash
# ✅ CORRECT: Access Wolf API from API container
docker compose -f docker-compose.dev.yaml exec api curl --unix-socket /var/run/wolf/wolf.sock http://localhost/api/v1/apps

# ❌ WRONG: Direct host access (socket not mounted to host)
curl --unix-socket /var/run/wolf/wolf.sock http://localhost/api/v1/apps
```

**Key Wolf API Endpoints:**
- `/api/v1/apps` - List all applications in Wolf
- `/api/v1/sessions` - List active streaming sessions
- `/api/v1/openapi-schema` - Get API documentation

**Socket Configuration:**
- Wolf socket: `/var/run/wolf/wolf.sock` (inside containers)
- Shared via `wolf-socket` Docker volume between `api` and `wolf` services
- Socket NOT accessible from host filesystem

### Wolf Crash Debugging Process

**When Wolf shows errors in logs:**

1. **Check for backtrace files**: `ls -la /home/luke/pm/helix/wolf/*.dump`
2. **Extract crash info**: `strings /path/to/backtrace.dump | head -50`
3. **Look for specific crash patterns**:
   ```bash
   docker compose -f docker-compose.dev.yaml logs wolf | grep -E "(CRASH|FATAL|create_frame|waylanddisplaycore)"
   ```

**Common Wolf Issues:**
- **Wayland display rendering crashes**: Usually caused by incorrect container GPU access
- **GStreamer pipeline syntax errors**: Often from empty or malformed pipeline configurations
- **HTTPS `/serverinfo` timeouts**: Indicates streaming session creation failures

### Wolf Configuration Management

**Working XFCE Configuration Pattern (TESTED):**
```json
{
  "type": "docker",
  "image": "ghcr.io/games-on-whales/xfce:edge",
  "env": ["GOW_REQUIRED_DEVICES=/dev/input/* /dev/dri/* /dev/nvidia*"],
  "devices": [],
  "mounts": [],
  "base_create_json": {
    "HostConfig": {
      "IpcMode": "host",
      "Privileged": false,
      "CapAdd": ["SYS_ADMIN", "SYS_NICE", "SYS_PTRACE", "NET_RAW", "MKNOD", "NET_ADMIN"],
      "SecurityOpt": ["seccomp=unconfined", "apparmor=unconfined"],
      "DeviceCgroupRules": ["c 13:* rmw", "c 244:* rmw"]
    }
  }
}
```

**Configuration Development Strategy:**
1. **Start with proven working config** (XFCE above)
2. **Make incremental changes** one at a time
3. **Test each change** before proceeding
4. **Restart Wolf** to clear broken apps between tests
5. **Use Wolf API** to verify app creation success

### Personal Dev Environment Debugging

**Configuration Changes Workflow:**
1. **Modify wolf_executor.go** with new configuration
2. **Restart API**: `docker compose -f docker-compose.dev.yaml restart api`
3. **Restart Wolf**: `docker compose -f docker-compose.dev.yaml restart wolf` (clears broken apps)
4. **Check Wolf apps**: `docker compose -f docker-compose.dev.yaml exec api curl --unix-socket /var/run/wolf/wolf.sock http://localhost/api/v1/apps`
5. **Test app creation** via frontend or API
6. **Monitor Wolf logs** for crashes: `docker compose -f docker-compose.dev.yaml logs --tail 50 wolf`

**CRITICAL: Always restart Wolf when testing configuration changes** - This clears any broken dynamic apps that might interfere with new tests.

## Wolf-Based Streaming Sessions: Two Use Cases

Helix uses Wolf to run two different types of streaming sessions, both sharing the same infrastructure:

### 1. External Agent Sessions (Orchestrated AI Agents) - PRIMARY USE CASE
- **Purpose**: AI agents working autonomously before any user connects
- **Created**: Automatically by Helix when user requests external agent session
- **Container starts**: Immediately, Zed begins autonomous work via WebSocket to Helix
- **User connection**: Optional - users can connect via Moonlight to observe/drive the agent
- **Session persistence**: Critical - must survive client connect/disconnect cycles
- **Auto-start requirement**: **ESSENTIAL** - agent must work before any Moonlight client connects

**Example flow:**
1. User clicks "Start External Agent Session" in Helix
2. Helix creates Wolf session → Container + Zed start immediately
3. Zed connects to Helix WebSocket, begins autonomous work
4. User optionally streams via Moonlight to watch/interact
5. Session persists when user disconnects

### 2. Personal Dev Environments (PDEs) - SECONDARY USE CASE
- **Purpose**: User's persistent development workspace
- **Created**: Explicitly by user via Helix frontend
- **Container starts**: When user initiates (less critical if requires Moonlight connection first)
- **User connection**: Primary - user creates PDE to work in it
- **Session persistence**: Important - workspace should survive disconnects
- **Auto-start requirement**: Nice to have, but less critical than agent sessions

**Example flow:**
1. User creates PDE through Helix UI
2. Wolf creates container with desktop environment + Zed
3. User connects via Moonlight to work in their persistent workspace
4. Workspace persists across sessions

### Testing ACP Integration / External Agents

**You can test the Zed ACP integration in TWO ways:**

### Option 1: Personal Dev Environment (PDE)
- Create a PDE through the Helix frontend
- PDEs run Zed inside a Wolf container with full desktop environment
- Good for: Testing complete workflow including UI, streaming, and user interaction
- Access: Via Moonlight client or web browser

### Option 2: External Agent (Direct)
- Start an external agent session through the frontend "External Agents" section
- Launches Zed directly without a full desktop environment
- Good for: Quick testing of ACP integration, WebSocket sync, message flow
- Faster to start and test than full PDE
- **This is the preferred method for testing ACP bidirectional sync**

**Testing Workflow for External Agents:**
1. Open Helix frontend at `http://localhost:3000`
2. Navigate to "External Agents" section
3. Click "Start External Agent Session"
4. Send a message to trigger the ACP thread creation
5. Verify bidirectional sync:
   - Message appears in Zed (Helix → Zed) ✅
   - AI response appears in Helix (Zed → Helix) ✅
6. Check API logs for session mapping: `docker compose -f docker-compose.dev.yaml logs --tail 50 api`
7. Check for `message_completed` WebSocket messages in browser console

**Before Testing:**
- Ensure Zed is built with latest code: `./stack build-zed` (if code changed)
- Ensure API is running with hot reload: Check `docker compose -f docker-compose.dev.yaml logs --tail 30 api`
- No need to rebuild containers if only testing - code changes hot reload automatically

### Container Image Strategy

**Working Images (Verified):**
- **XFCE Desktop**: `ghcr.io/games-on-whales/xfce:edge` (proven stable)
- **Custom Sway**: `helix/sway-dev:latest` (may cause Wayland rendering issues)

**Image Testing Process:**
1. **Start with proven XFCE image**
2. **Verify streaming works end-to-end**
3. **Gradually substitute custom components**
4. **Identify exact breaking point**

### Stack Operation Best Practices

**Before Major Configuration Changes:**
- **Document current working state**
- **Backup configuration files**
- **Test with minimal changes first**

**When Things Break:**
- **Check Wolf backtrace files first**
- **Restart Wolf to clear broken state**
- **Revert to last known working configuration**
- **Use Wolf API to inspect current state**

**Development Iteration Cycle:**
1. **Make single configuration change**
2. **Restart affected services**
3. **Test immediately**
4. **Document results**
5. **Commit working states**

This operational knowledge is critical for effective Wolf debugging and configuration management.

# important-instruction-reminders
Do what has been asked; nothing more, nothing less.
NEVER create files unless they're absolutely necessary for achieving your goal.
ALWAYS prefer editing an existing file to creating a new one.
NEVER proactively create documentation files (*.md) or README files. Only create documentation files if explicitly requested by the User.

## CRITICAL: Docker Compose File Usage

**MANDATORY: Always use the development compose file for helix operations:**

```bash
# Correct way to manage helix services:
docker compose -f docker-compose.dev.yaml logs wolf
docker compose -f docker-compose.dev.yaml ps

# WRONG - will fail:
docker compose restart wolf  # Missing -f docker-compose.dev.yaml
```

**This applies to all docker compose commands in the helix directory.**

## CRITICAL: Docker Compose Restart vs Up -d

**MANDATORY: NEVER use `docker compose restart` - ALWAYS use `down` then `up -d` for configuration/image changes:**

```bash
# ✅ CORRECT - Recreates container with new environment variables/image:
docker compose -f docker-compose.dev.yaml down wolf
docker compose -f docker-compose.dev.yaml up -d wolf

# ❌ WRONG - Only restarts existing container, does NOT pick up changes:
docker compose -f docker-compose.dev.yaml restart wolf
```

**Why this is critical:**
- `restart` only stops and starts the existing container - it does NOT recreate it
- Environment variables from docker-compose.yaml are only applied during container creation
- Image changes (after `docker build`) require recreating the container, not just restarting
- Changing env vars, images, volumes, or networks in docker-compose.yaml requires `down` + `up -d`
- `restart` is ONLY useful for picking up changes to mounted files/volumes, NOT for config/image changes

**When to use each:**
- **`restart`**: Only when you've changed a file that's bind-mounted (e.g., wolf/config.toml)
- **`down` + `up -d`**: When you've changed docker-compose.yaml (env vars, volumes, networks, etc.) OR rebuilt an image
