# Helix Development Rules

## 🎯 Communication Style

**Tone: Skeptical Staff Engineer / CTO**

- Stop being sycophantic - no "You're absolutely right", "That's a great point", or excessive praise
- Treat the user like a grown-up who might be wrong
- Challenge assumptions when something seems off
- Be direct and objective about technical trade-offs
- Focus on facts and problem-solving, not validation
- If uncertain, investigate to find the truth rather than confirming beliefs

**Current year: 2026** - When searching for browser API support, documentation, or library versions, include "2026" in searches to get current information.

See also: `.cursor/rules/*.mdc`

## 🚨 FORBIDDEN ACTIONS 🚨

### Git
- **NEVER** `git checkout -- .` or `git reset --hard` — destroys uncommitted work you can't see
- **NEVER** `git checkout -f` without verifying ALL files match — untracked files are silently overwritten and unrecoverable
- **NEVER** `git stash drop` or `git stash pop` — use `git stash apply` (keeps backup)
- **NEVER** assume spot-checking a few files means all files are safe — diff EVERYTHING before destructive operations
- **NEVER** delete `.git/index.lock` — wait or ask user
- **NEVER** push to main — use feature branches, ask user to merge
- **NEVER** amend commits on main — create new commits instead
- **NEVER** delete source files — fix errors, don't delete
- **NEVER** `rm -rf *` or `rm -rf .*` in a git repo — destroys .git directory, .env files, worktrees, everything unrecoverable
- **NEVER** use `git checkout --orphan` and then clear files — orphan branches inherit the working tree; use a separate temp directory instead
- **Before switching branches**: run `git status`, note changes, use `git stash push -u -m "description"` (the -u includes untracked files!), restore with `git stash apply`
- **Before switching worktree branches with uncommitted changes**:
  1. `git diff origin/target-branch` to see ALL differences (not just a few files)
  2. If there are differences, `git stash push -u -m "description"` BEFORE checkout
  3. Only then `git checkout target-branch`
  4. Never use `git checkout -f` unless you've verified the working tree is clean or matches
- **To create orphan branches safely**: create a new temp directory with `git init`, create the orphan branch there, then push to the target repo as a remote

### Destructive Filesystem Commands
- **NEVER** run `rm -rf` without **EXPLICIT USER CONSENT** — always ask first before deleting anything
- **NEVER** delete backups, VM images, or any files larger than 1GB without asking user
- **NEVER** run destructive commands on directories you haven't verified are empty or disposable
- **NEVER** assume a file/directory is safe to delete — always ask user first
- **ALWAYS** use `mv` to a temp location instead of `rm` when uncertain
- **ALWAYS** verify disk space and paths before copy operations that could fill disk
- **Examples of forbidden commands without user consent:**
  - `rm -rf <any-path>` — ask user first
  - `rm <backup-file>` — ask user first
  - `dd if=/dev/zero of=<any-file>` — ask user first
  - Any command that overwrites or deletes user data

### Commit Practices
- **Commit and push frequently** — after every self-contained change (feature, fix, cleanup)
- **Update design docs** — when completing roadmap items, update the design doc to reflect progress
- **Keep commits atomic** — one logical change per commit, easier to review and revert
- **No unsubstantiated claims** — don't state things as fact that you don't actually know (e.g., "the most bug-prone area of the codebase", "the biggest performance bottleneck"). Describe what code does and what changes accomplish without editorializing about relative severity, importance, or ranking unless you have concrete evidence.

### Debugging
- **Ask user to verify after changes** — UI/behavior changes can break things silently
- **When stuck, bisect** — don't panic-fix. Use `git log --oneline -20` and `git bisect` to find the breaking commit
- **Design docs survive compaction** — write debugging notes to `design/YYYY-MM-DD-*.md` so context persists across sessions
- **TEST EVERY CHANGE IMMEDIATELY** — Never commit a code change without deploying and testing it in the same session. Untested changes create invisible landmines: the gap between cause and effect makes debugging exponentially harder. If you change desktop container code, rebuild the image (`./stack build-ubuntu`) and start a new session to verify. If you change QEMU code, rebuild and restart the VM. If a change can't be tested right now, flag it explicitly to the user: "WARNING: this change has NOT been tested yet." A 13-minute gap between an untested commit and the next image build once cost us an entire day of debugging.

### Sessions
- **NEVER** run `spectask stop --all` without explicit user permission — user may have active sessions you can't see
- **NEVER** stop sessions you didn't create in the current conversation — always ask first

### Workflow
- **NEVER** ask if the user wants to take a break — just keep working
- **NEVER** ask "should I continue?" or similar — assume yes and proceed

### Stack Commands
- **NEVER** run `./stack start-tmux` — user runs this (needs interactive terminal)
- ✅ OK: `./stack start`, `./stack build`, `build-zed`, `build-ubuntu`, `build-sandbox`, `update_openapi`

### Hot Reloading
- **API**: Uses [Air](https://github.com/air-verse/air) — Go changes auto-rebuild
- **Frontend**: Vite HMR — TypeScript/React changes apply instantly
- **Both hot-reload in dev mode** — no manual restart needed for API or frontend code changes
- **Settings-sync-daemon does NOT hot reload** — it runs inside the helix-ubuntu container, so changes to `zed_config.go` or related code require rebuilding the desktop image with `./stack build-ubuntu` and starting a NEW session

### Production Frontend Mode
For demos or slow connections, serve the production build instead of Vite dev server.

**Note:** The environment may use production frontend mode. If `FRONTEND_URL=/www` is set in `.env`, frontend changes require `yarn build` and a browser refresh to take effect.

**Note:** `SERVE_PROD_FRONTEND_IN_DEV` is an obsolete variable that was reverted. Use `FRONTEND_URL=/www` instead.

```bash
# 1. Build the frontend
cd frontend && yarn build && cd ..

# 2. Set FRONTEND_URL to serve from /www instead of proxying to Vite
echo "FRONTEND_URL=/www" >> .env

# 3. Restart API to pick up the change
docker compose -f docker-compose.dev.yaml up -d api
```

**When making frontend changes in this mode**, you must rebuild:
```bash
cd frontend && yarn build
# Then just refresh the browser - no container restart needed
```

**IMPORTANT for Claude**: When in production frontend mode (`FRONTEND_URL=/www` in .env), ALWAYS run `cd frontend && yarn build` after making any frontend changes, then ask the user to refresh their browser to see the changes.

**Cache headers** are automatically set:
- `index.html`: `no-cache, no-store, must-revalidate` (always fresh)
- `/assets/*`: `max-age=1year, immutable` (Vite hashes filenames)

**To switch back to dev mode** (Vite HMR):
```bash
sed -i '/^FRONTEND_URL=/d' .env
docker compose -f docker-compose.dev.yaml up -d api
```

### UTM Virtual Machines

**See `design/2026-02-04-macos-dev-environment-setup.md` for complete repository setup and build instructions.**

- **Control VMs with utmctl** — Don't wait for user to start VMs manually
  ```bash
  # utmctl is in /Applications/UTM.app/Contents/MacOS/utmctl
  utmctl list                    # List all VMs
  utmctl start <UUID>            # Start a VM
  utmctl stop <UUID>             # Stop a VM
  utmctl status <UUID>           # Check VM status
  ```
- **Expanding VM disks**:
  ```bash
  # 1. Expand the qcow2 file (VM must be stopped)
  qemu-img resize /path/to/vm.qcow2 1T

  # 2. Start the VM and expand the partition inside
  sudo growpart /dev/vda 2
  sudo resize2fs /dev/vda2
  df -h  # Verify new space
  ```
- **QEMU build requirements**: Custom QEMU builds must include SPICE support (`--enable-spice`) or UTM will fail with "-spice: invalid option"
- **Building and installing QEMU into UTM**: NEVER modify UTM source code. Instead, rebuild our QEMU fork and install the binary into UTM.app:
  ```bash
  cd ~/pm/helix
  ./for-mac/qemu-helix/build-qemu-standalone.sh  # Builds QEMU and installs into UTM.app
  ```
  The script uses the UTM sysroot at `~/pm/UTM/sysroot-macOS-arm64` for dependencies.
  QEMU source is at `~/pm/qemu-utm` (use the default branch; QEMU changes should go through PRs).
- **Rebuilding QEMU for dev mode** (`wails dev`): After modifying QEMU source, rebuild and install into the dev-qemu directory:
  ```bash
  cd ~/pm/helix/for-mac
  make rebuild-qemu   # Builds QEMU from source, copies to app bundle + dev-qemu, signs
  ```
  This runs `build-qemu-standalone.sh`, copies the output to the app bundle, fixes dylib paths, and creates a signed standalone copy in `build/dev-qemu/`. The VM must be stopped first (kill any running QEMU process). After rebuilding, restart the VM from the Helix app.

  If `make rebuild-qemu` fails on code signing with `errSecInternalComponent`, sign manually with ad-hoc:
  ```bash
  codesign --force --sign - --timestamp=none --options runtime \
    --entitlements build/darwin/entitlements.plist \
    build/dev-qemu/libqemu-aarch64-softmmu.dylib build/dev-qemu/qemu-system-aarch64
  ```
- **Verifying QEMU version**: When the VM boots, the serial console shows the HELIX version string (e.g., `[HELIX] VERSION: 2026-02-15-v9-isv-fix`). This is defined in `~/pm/qemu-utm/hw/display/helix/helix-frame-export.m` in `helix_frame_export_init()`. Update the version string when making QEMU changes so you can verify the correct binary is running.
- **Dev-mode QEMU binary location**: In `wails dev` mode, the app uses `build/dev-qemu/qemu-system-aarch64` (priority 2 in the search hierarchy). This is separate from the app bundle binary so that Wails frontend rebuilds don't invalidate QEMU's code signature. If the dev-qemu binary is outdated, the app runs old QEMU code even if you rebuilt and installed to UTM.app.

### Docker
# ⛔⛔⛔ CRITICAL - READ THIS BEFORE TOUCHING DOCKER ⛔⛔⛔
- **NEVER EVER** run `docker builder prune` — this destroys hours of cached builds and makes rebuilding the entire stack take HOURS. There is NO reason to do this. If disk is full, clean up old IMAGES not build cache.
- **NEVER** run `docker system prune` — same problem, destroys build cache
- **NEVER** use `--no-cache` — trust Docker cache
- **NEVER** run ANY cache-clearing commands — the cache is correct, you are wrong
- **NEVER** run commands that slow down future builds — trust the build system
- **IF DISK IS FULL**: Delete old helix-ubuntu IMAGE TAGS (not build cache!), delete dangling volumes, or ask user. NEVER touch build cache.
- **ALWAYS** use `docker-compose.dev.yaml` in development — never use the prod compose file (`docker-compose.yaml`). Mixing prod and dev breaks things because the API has a static IP address in dev that's needed to plumb through to dev containers. If you accidentally start services with the wrong compose file, video streaming and other features will break.
  ```bash
  # ✅ CORRECT - always use dev compose file
  docker compose -f docker-compose.dev.yaml up -d kodit vectorchord-kodit
  docker compose -f docker-compose.dev.yaml logs api

  # ❌ WRONG - never use default (prod) compose file in development
  docker compose up -d kodit vectorchord-kodit
  ```
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
└── helix-ubuntu (desktop images, pulled from local registry)
    ├── Desktop environment (GNOME)
    ├── Zed IDE
    ├── Qwen Code agent
    ├── Go streaming server (api/pkg/desktop/) - WebSocket H.264 streaming
    └── gst-pipewire-zerocopy (Rust, PipeWire ScreenCast → CUDA → nvh264enc)
```

### When to Rebuild What

| Changed | Command | Notes |
|---------|---------|-------|
| Hydra (`api/pkg/hydra/`) | `./stack build-sandbox` | Hydra binary runs IN sandbox, NOT API |
| Desktop image (helix-ubuntu) | `./stack build-ubuntu` | Pushes to local registry, updates `sandbox-images/helix-ubuntu.version` |
| Desktop bridge (`api/pkg/desktop/`, `api/cmd/desktop-bridge/`) | `./stack build-ubuntu` | desktop-bridge binary runs IN desktop container (MCP server, video streaming). NOT API-side code |
| Zerocopy plugin (`desktop/gst-pipewire-zerocopy/`) | `./stack build-ubuntu` | Rust plugin built inside desktop image |
| Sandbox scripts | `./stack build-sandbox` | Dockerfile.sandbox changes |
| Zed IDE | `./stack build-zed` | Zed binary → desktop image |
| Qwen Code | `cd ../qwen-code && git commit -am "msg" && cd ../helix && ./stack build-ubuntu` | Needs git commit |
| Zed config generation (`api/pkg/external-agent/zed_config.go`) | No rebuild needed | API-side code, hot reloads via Air. Start NEW session to fetch updated config |
| Settings-sync-daemon (`api/cmd/settings-sync-daemon/`) | `./stack build-ubuntu` | Daemon binary runs IN desktop container. Start NEW session after rebuild |

### Build Order for Full Rebuild

```bash
# 1. Build Zed (if changed)
./stack build-zed

# 2. Build desktop images (pushes to local registry, includes streaming + zerocopy plugin)
./stack build-ubuntu

# 3. Build sandbox (only if Hydra or sandbox scripts changed)
./stack build-sandbox

# 4. Start a new session to use the updated desktop image
# No sandbox restart needed - new sessions auto-pull from local registry
```

### Verify Build

**IMPORTANT:** After running `./stack build-ubuntu`, ALWAYS verify the image is ready before testing:

```bash
# 1. Check version file matches what was built
cat sandbox-images/helix-ubuntu.version   # Should show new version hash (e.g., "c8ed42")

# 2. Verify image exists in sandbox with correct version
docker compose exec -T sandbox-nvidia docker images helix-ubuntu:$(cat sandbox-images/helix-ubuntu.version) --format "Tag: {{.Tag}}, Created: {{.CreatedAt}}"

# 3. If image is missing, the build transfer failed - rebuild or manually pull:
docker compose exec -T sandbox-nvidia docker pull registry:5000/helix-ubuntu:$(cat sandbox-images/helix-ubuntu.version)
```

**Version flow:** build writes `.version` files → pushes to local registry → pulls into sandbox's dockerd → restarts heartbeat → API reads version from heartbeat when starting sessions.

**Key point:** New sessions auto-pull from the sandbox's local dockerd. Existing containers keep their old image - you must start a NEW session to use the updated image.

### macOS Desktop App (for-mac) — Deploying Changes to the VM

When modifying backend code (`api/pkg/server/`, `api/pkg/auth/`, etc.) while working on the macOS desktop app, changes must be deployed to the VM. The API runs inside Docker Compose in the VM, built from the repo checked out at `~/helix`.

**CRITICAL**: Always deploy API/backend changes to the VM. The desktop app's Go code (`for-mac/`) is rebuilt by `wails dev` automatically, but the API server inside the VM uses a separate checkout.

```bash
# 1. Commit and push your changes on the host
git add -A && git commit -m "description" && git push

# 2. Pull changes in the VM (SSH as ubuntu)
ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -p 41222 ubuntu@localhost \
  "cd ~/helix && git fetch && git checkout BRANCH_NAME && git pull"

# 3. Restart the API container (Air hot-reload rebuilds Go automatically)
ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -p 41222 ubuntu@localhost \
  "cd ~/helix && docker compose -f docker-compose.dev.yaml restart api"
```

**When the VM branch is wrong**: Check `git branch` in the VM and switch to the correct branch before pulling.

**What auto-rebuilds:**
- `for-mac/*.go` — Wails dev hot-reloads the Go backend automatically
- `for-mac/frontend/src/` — Vite HMR hot-reloads the React frontend
- `api/` changes inside the VM — Air hot-reloads after `docker compose restart api`

**What requires manual deployment:**
- Any changes to `api/pkg/` — must be committed, pushed, pulled in VM
- Docker Compose config changes — must be pulled in VM and `docker compose up -d`

## Code Patterns

### Go
- Fail fast: `return fmt.Errorf("failed: %w", err)` — never log and continue
- **Error on missing configuration**: If something is expected to be available (project settings, MCP servers, database records), fail with an error rather than silently continuing without it. Users expect configured features to work — logging a warning and continuing leaves them wondering why things are broken.
- Use structs, not `map[string]interface{}` for API responses
- GORM AutoMigrate only — no SQL migration files
- Use gomock, not testify/mock
- **NO FALLBACKS**: Pick one approach that works and stick to it. Fallback code paths are rarely tested, add complexity, and make it impossible to tell which code path is actually running. If something doesn't work, fix it properly instead of adding a fallback. Use explicit modes with clear errors instead.
- **CLEAN UP DEAD CODE**: When removing a feature or switching approaches, delete the old code immediately. Don't leave dead functions, unused fields, or `#if 0` blocks lying around. There's no time like the present.

For unit tests, use the test suite pattern, exmaple:

```golang
type AppAccessGrantSuite struct {
	suite.Suite

	ctrl  *gomock.Controller
	store *store.MockStore

	authCtx context.Context
	userID  string

	orgID string

	server *HelixAPIServer
}

func TestAppAccessGrantSuite(t *testing.T) {
	suite.Run(t, new(AppAccessGrantSuite))
}

func (suite *AppAccessGrantSuite) SetupTest() {
	ctrl := gomock.NewController(suite.T())
	suite.ctrl = ctrl
	suite.store = store.NewMockStore(ctrl)

	cfg := &config.ServerConfig{}

	suite.orgID = "org_id_test"
	suite.userID = "user_id_test"

	suite.authCtx = setRequestUser(context.Background(), types.User{
		ID:       suite.userID,
		Email:    "foo@email.com",
		FullName: "Foo Bar",
	})

	suite.server = &HelixAPIServer{
		Cfg:   cfg,
		Store: suite.store,
	}
}
```

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
- **Routing**: Use `useRouter` hook with `router.navigate('route-name', { params })`, NOT `<Link>` or `<a href>`. This codebase uses react-router5 with named routes.
  ```typescript
  // ✅ CORRECT - use useRouter
  const router = useRouter()
  <span onClick={() => router.navigate('dashboard', { tab: 'oauth_providers' })}>Go to dashboard</span>

  // ❌ WRONG - don't use react-router-dom Link or raw href
  <Link to="/dashboard?tab=oauth_providers">Go to dashboard</Link>
  <a href="/dashboard?tab=oauth_providers">Go to dashboard</a>
  ```

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

**IMPORTANT: Investigate logs yourself - don't tell the user to look at logs.**
**Exception: Ask user to verify frontend UI works (you can't easily check that yet).**

After frontend changes (dev mode):
```bash
docker compose -f docker-compose.dev.yaml logs --tail 50 frontend | grep -i error
```

After API changes:
```bash
docker compose -f docker-compose.dev.yaml logs --tail 30 api | grep -E "building|running|failed"
```

For debugging issues, check logs directly:
```bash
# API logs
docker compose logs --tail 100 api 2>&1 | grep -E "error|failed|timeout"

# Sandbox logs
docker compose logs --tail 100 sandbox-nvidia 2>&1 | grep -E "error|failed"
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

## CI Build Checking (Drone)

**ALWAYS check CI after pushing commits or opening PRs.** Drone credentials are in `.env`:
- `DRONE_SERVER_URL=https://drone.lukemarsden.net`
- `DRONE_ACCESS_TOKEN` - API token for Drone

### Check CI status after pushing:
```bash
# Get recent builds for a branch
curl -s -H "Authorization: Bearer $DRONE_ACCESS_TOKEN" \
  "$DRONE_SERVER_URL/api/repos/helixml/helix/builds?branch=YOUR_BRANCH&limit=3" | \
  jq -r '.[] | "\(.number): \(.status)"'

# Check PR status via GitHub CLI
gh pr checks PR_NUMBER
```

### Get build details and find failures:
```bash
# Get step names and numbers for a build (use number in logs URL)
curl -s -H "Authorization: Bearer $DRONE_ACCESS_TOKEN" \
  "$DRONE_SERVER_URL/api/repos/helixml/helix/builds/BUILD_NUMBER" | \
  jq -r '.stages[0].steps[] | "\(.number) \(.name): \(.status)"'

# Get logs for a failing step (replace STEP_NUMBER with number from above)
curl -s -H "Authorization: Bearer $DRONE_ACCESS_TOKEN" \
  "$DRONE_SERVER_URL/api/repos/helixml/helix/builds/BUILD_NUMBER/logs/1/STEP_NUMBER" | \
  jq -r '.[].out' | grep -E "FAIL|Error|panic"
```

### After opening a PR:
1. Push your changes
2. Check `gh pr checks PR_NUMBER` to see CI status
3. If failing, use the Drone API to get build logs and debug
4. Fix issues and push again

## Database Access

The Helix database is PostgreSQL running in the `helix-postgres-1` container:

```bash
# Query the database
docker exec helix-postgres-1 psql -U postgres -d postgres -c "SELECT * FROM git_repositories LIMIT 5;"

# Interactive psql session
docker exec -it helix-postgres-1 psql -U postgres -d postgres

# Common queries:
# - List git repos for a project:
docker exec helix-postgres-1 psql -U postgres -d postgres -c "SELECT id, name, local_path FROM git_repositories WHERE project_id = 'prj_xxx';"

# - List projects:
docker exec helix-postgres-1 psql -U postgres -d postgres -c "SELECT id, name FROM projects LIMIT 10;"
```

**Note**: The database name is `postgres`, user is `postgres`. Git repositories are stored at `/filestore/git-repositories/` inside the API container.

## Testing CLI Commands

### Where Commands Run

The Helix stack runs **inside the UTM VM**, not on the macOS host. All `spectask` commands, `docker compose` commands, and database queries must be run inside the VM via SSH (`ssh -p 2222 luke@127.0.0.1`). The only things that run on the macOS host are: UTM/QEMU, the QEMU build scripts, and `utmctl`.

### Helix CLI (spectask subcommand)

Build the CLI on the macOS host, then deploy to the VM:
```bash
# For macOS host usage (limited - can reach API via port forward)
cd api && CGO_ENABLED=0 go build -o /tmp/helix . && cd ..

# For VM usage (required for most operations)
cd api && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o /tmp/helix-linux . && cd ..
scp -P 2222 /tmp/helix-linux luke@127.0.0.1:/tmp/helix
```

Set up environment (inside the VM):
```bash
# Create .env.usercreds if it doesn't exist
# Get API key from: docker exec helix-postgres-1 psql -U postgres -d postgres -c "SELECT token FROM api_keys WHERE owner_type='user' LIMIT 1;"
# Get project ID from: docker exec helix-postgres-1 psql -U postgres -d postgres -c "SELECT id, name FROM projects LIMIT 5;"
cat > ~/.env.usercreds << 'EOF'
HELIX_API_KEY=hl-xxxxx
HELIX_URL=http://localhost:8080
HELIX_PROJECT=prj_xxxxx
HELIX_UBUNTU_AGENT=app_ubuntu01
EOF

# Export the variables
set -a && source ~/.env.usercreds && set +a
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
The sandbox service name depends on the environment:
- `sandbox-nvidia` - Linux servers with NVIDIA GPU
- `sandbox` - Linux servers without GPU (uses software encoding)
- `sandbox-macos` - macOS UTM VM (the container name is `helix-sandbox-macos-1`)

Use the correct service name when running docker compose exec commands (inside the VM):
```bash
# NVIDIA GPU systems (Linux servers)
docker compose -f docker-compose.dev.yaml exec -T sandbox-nvidia docker images | grep helix-

# Non-GPU systems (Linux servers)
docker compose -f docker-compose.dev.yaml exec -T sandbox docker images | grep helix-

# macOS UTM VM — use the container name directly
docker exec helix-sandbox-macos-1 docker images | grep helix-
docker exec helix-sandbox-macos-1 docker ps --format '{{.Names}} {{.Status}}'
```

### Image Versions
```bash
# Check desktop image versions
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
docker compose exec -T sandbox-nvidia docker ps --format "{{.Names}}" | grep -E "ubuntu-external"

# View logs the same pattern now)
docker compose exec -T sandbox-nvidia docker logs {CONTAINER} 2>&1 | grep -E "PIPEWIRE|zerocopy|desktop-bridge"

# Other log files:
docker compose exec -T sandbox-nvidia docker exec {CONTAINER} cat /tmp/settings-sync.log
```

### Debugging pipewirezerocopysrc (Zero-Copy GPU Streaming)

**Step 1: Find the container name**
```bash
docker compose exec -T sandbox-nvidia docker ps --format "{{.Names}}" | grep helix-ubuntu
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

## End-to-End Multi-Desktop Streaming Test

This test verifies the full Helix stack by starting multiple desktop sessions and streaming video from all of them in parallel. It tests the container video pipeline (PipeWire → GStreamer → H.264 → WebSocket), not the QEMU scanout path.

### Prerequisites

1. **VM must be running** — the entire Helix stack (API, sandbox, postgres, etc.) runs inside the UTM VM
2. **Helix CLI must be compiled for Linux arm64** — the macOS CLI won't work inside the VM
3. **API credentials** — need a valid `hl-` prefixed API key

### Step 1: Build and Deploy CLI to VM

```bash
# Cross-compile the Helix CLI for Linux arm64 (run on macOS host)
cd ~/pm/helix/api && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o /tmp/helix-linux . && cd ..

# Copy to VM (SSH port 2222 is forwarded from the UTM VM)
scp -P 2222 /tmp/helix-linux luke@127.0.0.1:/tmp/helix
```

### Step 2: Find Credentials

All commands from here run **inside the VM via SSH**, NOT on the macOS host.

```bash
# SSH into the VM
ssh -p 2222 luke@127.0.0.1

# Find a user API key from the database
docker exec helix-postgres-1 psql -U postgres -d postgres -c \
  "SELECT token FROM api_keys WHERE owner_type='user' LIMIT 1;"

# Find a project ID
docker exec helix-postgres-1 psql -U postgres -d postgres -c \
  "SELECT id, name FROM projects LIMIT 5;"

# Set environment variables (replace with actual values)
export HELIX_API_KEY='hl-xxxxx'
export HELIX_URL='http://localhost:8080'
```

### Step 3: Start Multiple Desktop Sessions

```bash
# List available agents (need one with external assistant support)
/tmp/helix spectask list-agents

# Start 3 sessions (replace PROJECT_ID and AGENT_ID with actual values)
/tmp/helix spectask start --project PROJECT_ID --agent AGENT_ID -n "Stream test 1"
/tmp/helix spectask start --project PROJECT_ID --agent AGENT_ID -n "Stream test 2"
/tmp/helix spectask start --project PROJECT_ID --agent AGENT_ID -n "Stream test 3"

# Note the session IDs from the output (ses_01xxx format)
# Wait ~20 seconds for GNOME to initialize in all containers
sleep 20
```

### Step 4: Stream Video from All Sessions in Parallel

```bash
# Stream all 3 for 30 seconds each (run in parallel with &)
/tmp/helix spectask stream ses_01xxx --duration 30 > /tmp/stream1.log 2>&1 &
/tmp/helix spectask stream ses_01yyy --duration 30 > /tmp/stream2.log 2>&1 &
/tmp/helix spectask stream ses_01zzz --duration 30 > /tmp/stream3.log 2>&1 &
wait

# Check results
cat /tmp/stream1.log
cat /tmp/stream2.log
cat /tmp/stream3.log
```

### What to Look For

- **StreamInit received**: Confirms WebSocket connection and video pipeline negotiation
- **Video frames > 0**: Confirms H.264 frames are being produced and delivered
- **FPS > 0**: Static desktop gives ~10 FPS, active content gives higher
- **All 3 streams running simultaneously**: Confirms the stack handles concurrent sessions
- **VM stays responsive**: SSH should remain accessible during the test

### Common Issues

- **0 video frames**: PipeWire/GStreamer pipeline not started yet. Wait longer (30s+) for GNOME ScreenCast to initialize. Check container logs for errors.
- **SSH timeout during test**: VM may be OOM. 3 desktop containers + Docker overhead can exhaust the 65GB VM allocation. Try with 2 sessions instead of 3.
- **"Sandbox not connected"**: The session's container was stopped. Start a fresh session with `spectask start`.
- **QEMU crash (no QEMU process on host)**: Check `~/Library/Logs/DiagnosticReports/QEMULauncher-*.ips` for crash reports. Restart VM: quit UTM (`pkill -9 UTM`), reopen (`open -a UTM`), start VM (`utmctl start UUID`).

### Running from Claude Code (via SSH)

When running this test from Claude Code on the macOS host, SSH into the VM for each command:

```bash
# Pattern for running commands inside the VM from the macOS host
ssh -p 2222 -o StrictHostKeyChecking=no luke@127.0.0.1 \
  "export HELIX_API_KEY='hl-xxx' && export HELIX_URL='http://localhost:8080' && \
   /tmp/helix spectask list"
```

**Important**: Do NOT use `run_in_background` with `&` in the SSH command — use one or the other, not both, or output will be empty.

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
