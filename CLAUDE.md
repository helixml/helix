# Claude Rules for Helix Development

The following rule files should be consulted when working on the codebase:

@.cursor/rules/helix.mdc
@.cursor/rules/go-api-handlers.mdc
@.cursor/rules/use-gorm-for-database.mdc
@.cursor/rules/use-frontend-api-client.mdc

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
5. **CRITICAL: NEVER USE --no-cache ON DOCKER BUILDS**: NEVER use `--no-cache` flag with `docker build` commands. We TRUST Docker's caching system completely. Docker's layer caching is reliable, correct, and speeds up builds significantly. Using `--no-cache` is wasteful and unnecessary. Docker BuildKit intelligently invalidates caches when source files change - you don't need to manually force cache invalidation.
6. **CRITICAL: NEVER CLEAR BUILDKIT CACHE - EVER**: NEVER run `docker builder prune` or clear Docker BuildKit cache. If you suspect the Docker build cache is wrong, YOU ARE WRONG - the issue lies somewhere else. Docker's caching is correct and reliable. If builds aren't picking up changes, investigate the actual root cause (wrong file paths, incorrect COPY commands, missing file changes, etc.) instead of blaming the cache. Build caches are carefully managed and clearing them wastes significant time on subsequent builds.
7. **CRITICAL: NEVER BUILD OR PUSH IMAGES WITHOUT PERMISSION**: NEVER run docker build, docker tag, or docker push commands that create versioned images (rc tags, version numbers, commit hashes as tags) without explicit user permission. If you build unauthorized images and push them to the registry, it creates permanent confusion in the registry that has to be cleaned up manually. Building and pushing images is ONLY allowed when explicitly requested by the user. The user manages releases and versioning - you write code, the user builds and releases it.
8. **Build caches are critical**: Without ccache/meson cache, iteration takes too long
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



### Wolf Version and Modifications

**IMPORTANT: We use upstream wolf-ui branch with minimal modifications**

**Wolf Repository**: `/home/luke/pm/wolf` on branch `wolf-ui`
**Base**: Upstream games-on-whales/wolf wolf-ui branch (lobbies support)

**Our Modifications** (commits by Luke Marsden only):

1. **57321eb**: Auto-pairing PIN support
   - File: `src/moonlight-server/rest/servers.cpp`
   - Reads `MOONLIGHT_INTERNAL_PAIRING_PIN` env var to auto-fulfill pairing
   - Enables automated pairing with moonlight-web without manual PIN entry

2. **84d4c01**: Phase 5 HTTP support for Moonlight pairing protocol
   - Adds HTTP endpoint support for pairing phase 5
   - Works with auto-pairing feature

3. **307c3de + 45339fe**: These cancel each other out (remove then revert)

**Result**: Essentially running **upstream wolf-ui branch** with only auto-pairing additions.

**Known Issues from Upstream**:
- GStreamer refcount errors (`gst_mini_object_unref`) flooding logs
- Occasional zombie process (PID becomes unresponsive)
- These are upstream wolf-ui branch bugs, not from our changes

**To rebuild Wolf**: `docker compose -f docker-compose.dev.yaml build wolf && docker compose -f docker-compose.dev.yaml down wolf && docker compose -f docker-compose.dev.yaml up -d wolf`

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

## CRITICAL: Database Migrations - GORM AutoMigrate Only

**MANDATORY: ALWAYS use GORM AutoMigrate for schema changes - NEVER create SQL migration files**

```go
// ✅ CORRECT: Add new fields/tables by updating GORM structs
type StreamingAccessGrant struct {
    ID        string    `gorm:"type:varchar(255);primaryKey"`
    SessionID string    `gorm:"type:varchar(255);index;not null"`
    UserID    string    `gorm:"type:varchar(255);index;not null"`
    CreatedAt time.Time `gorm:"autoCreateTime"`
}

// GORM AutoMigrate handles this automatically on startup
db.AutoMigrate(&StreamingAccessGrant{})

// ❌ WRONG: Creating SQL migration files for schema changes
// DO NOT create files like: api/pkg/store/migrations/0003_add_streaming_rbac.up.sql
```

**Why this is CRITICAL:**
- GORM AutoMigrate handles ALL schema changes automatically (tables, columns, indexes)
- SQL migrations are ONLY for complex data migrations that require special handling
- Creating SQL migrations for schema changes causes conflicts with AutoMigrate
- AutoMigrate is safe, idempotent, and works across dev/staging/prod

**The ONLY valid use of SQL migrations:**
- Complex data transformations that can't be expressed in GORM
- One-time data cleanup operations
- Backfilling data based on complex business logic
- Renaming tables/columns (requires explicit SQL to preserve data)

**Examples of what AutoMigrate handles (no SQL migration needed):**
- ✅ Adding new tables
- ✅ Adding new columns
- ✅ Adding indexes
- ✅ Changing column types (with data compatibility)
- ✅ Adding NOT NULL constraints
- ✅ Adding foreign keys

**The workflow:**
1. Update your GORM struct definitions in Go
2. GORM AutoMigrate runs on API startup
3. Schema changes apply automatically
4. No manual migration files needed

**If you catch yourself writing a SQL migration for schema changes, STOP and use GORM structs instead.**
