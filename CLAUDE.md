# Helix Development Rules

## Communication Style
- Be a skeptical staff engineer — no sycophancy, challenge bad assumptions, focus on facts
- If uncertain, investigate rather than confirming beliefs
- **Current year: 2026** — include "2026" in web searches for current info
- If you find yourself adding hacks or workarounds, **stop** — take a step back, root cause the issue, understand the wider context, and fix it properly. Don't always take the path of least resistance — we need maintainable code.
- See also: `.cursor/rules/*.mdc`

## FORBIDDEN ACTIONS

### Git
- **NEVER** `git checkout -- .`, `git reset --hard`, `git checkout -f` — destroys uncommitted work
- **NEVER** `git stash drop/pop` — use `git stash apply` (keeps backup)
- **NEVER** squash merge — always use regular merge commits (`gh pr merge --merge`)
- **NEVER** push to main, amend commits on main, delete source files, delete `.git/index.lock`
- **NEVER** `rm -rf *` or `rm -rf .*` in a git repo
- **NEVER** `git checkout --orphan` then clear files — use a separate temp directory instead
- **Before switching branches**: `git status` → `git stash push -u -m "desc"` → checkout → `git stash apply`
- Diff EVERYTHING before destructive operations — don't spot-check

### Filesystem
- **NEVER** `rm -rf` without **EXPLICIT USER CONSENT**
- **NEVER** delete backups, VM images, or files >1GB without asking
- Use `mv` to temp location instead of `rm` when uncertain

### Commits & Debugging
- Commit and push frequently, keep commits atomic, update design docs
- No unsubstantiated claims about code severity/importance without evidence
- Ask user to verify UI changes; when stuck, use `git bisect`
- Write debugging notes to `design/YYYY-MM-DD-*.md` (survives compaction)
- **TEST EVERY CHANGE** — never commit without deploying and testing. If untestable, flag it: "WARNING: NOT tested yet"

### Sessions & Workflow
- **NEVER** `spectask stop --all` or stop sessions you didn't create without permission
- **NEVER** ask "should I continue?" or suggest breaks — just keep working

### Stack Commands
- **NEVER** `./stack start-tmux` (needs interactive terminal)
- OK: `./stack start`, `./stack build`, `build-zed`, `build-ubuntu`, `build-sandbox`, `update_openapi`

### Hot Reloading
- **API**: Air auto-rebuilds Go changes
- **Frontend**: Vite HMR (dev mode) or `yarn build` + refresh (prod mode). The `helix-frontend-1` container runs Vite dev server on **port 8081** — changes to `frontend/src/` are live immediately, no rebuild needed. The main app at port 8080 proxies to it.
- **Settings-sync-daemon**: does NOT hot reload — requires `./stack build-ubuntu` + new session

### Production Frontend Mode
If `FRONTEND_URL=/www` in `.env`, frontend is served from `./frontend/dist:/www:ro` bind mount.

```bash
# Setup: cd frontend && yarn build && echo "FRONTEND_URL=/www" >> ../.env
# After changes: cd frontend && yarn build  # then browser refresh
# Switch to dev: sed -i '/^FRONTEND_URL=/d' .env && docker compose -f docker-compose.dev.yaml up -d api
```

**NEVER `rm -rf frontend/dist`** — breaks the bind mount. Use `rm -rf frontend/dist/*` instead.

### UTM Virtual Machines
See `design/2026-02-04-macos-dev-environment-setup.md` for setup.

- Control VMs: `utmctl list|start|stop|status <UUID>` (in `/Applications/UTM.app/Contents/MacOS/`)
- Expand disks: `qemu-img resize /path/to/vm.qcow2 1T` (stopped), then inside VM: `growpart /dev/vda 2 && resize2fs /dev/vda2`
- QEMU builds must include `--enable-spice`; NEVER modify UTM source
- Build QEMU: **ALWAYS use `cd for-mac && make rebuild-qemu`** (stop VM first). This builds, installs to app bundle, fixes dylib rpaths, copies to dev-qemu, and signs. **NEVER** use raw `ninja install` + manual `cp` + `codesign` — this breaks rpaths (`@rpath/pixman-1.0.framework` etc. won't resolve).
- QEMU source: `~/pm/qemu-utm` (default branch)
- If signing fails with `errSecInternalComponent`, use `codesign --force --sign - --timestamp=none --options runtime --entitlements build/darwin/entitlements.plist build/dev-qemu/*`
- QEMU version string is in `hw/display/helix/helix-frame-export.m` `helix_frame_export_init()` — update it when making QEMU changes
- Dev-mode uses `build/dev-qemu/qemu-system-aarch64` (separate from app bundle)

### Docker
- **NEVER** `docker builder prune`, `docker system prune`, `--no-cache` — destroys hours of build cache
- **IF DISK IS FULL**: delete old image tags, not build cache
- **ALWAYS** use `docker-compose.dev.yaml` — never the prod compose file (breaks dev networking)
- `docker compose restart` does NOT apply .env/image changes — use `down` + `up`

### Other
- **NEVER** rename CWD, commit customer data, or restart hung processes (collect GDB traces first)

## Build Pipeline

**Architecture**: Host → helix-sandbox (Hydra + DinD) → helix-ubuntu (GNOME + Zed + streaming)

### When to Rebuild What

| Changed | Command | Notes |
|---------|---------|-------|
| Hydra (`api/pkg/hydra/`) | `./stack build-sandbox` | Runs IN sandbox |
| Desktop image / desktop-bridge / zerocopy | `./stack build-ubuntu` | Pushes to local registry |
| Sandbox scripts | `./stack build-sandbox` | Dockerfile.sandbox |
| Zed IDE | `./stack build-zed release` | Binary → desktop image. **Must use `release` on ARM** (`gemm-f16` fullfp16 asm fails in debug) |
| Qwen Code | `cd ../qwen-code && git commit -am "msg" && cd ../helix && ./stack build-ubuntu` | |
| DRM manager (`api/pkg/drm/`, `api/cmd/helix-drm-manager/`) | `./stack build-drm-manager` | Systemd service on VM guest |
| Zed config (`zed_config.go`) | No rebuild | API-side, Air hot reloads. Start NEW session |
| Settings-sync-daemon | `./stack build-ubuntu` | Start NEW session after |

Full rebuild order: `build-zed` → `build-ubuntu` → `build-sandbox` (if needed) → start new session.

### Verify Build
```bash
cat sandbox-images/helix-ubuntu.version
docker compose exec -T sandbox-nvidia docker images helix-ubuntu:$(cat sandbox-images/helix-ubuntu.version) --format "Tag: {{.Tag}}, Created: {{.CreatedAt}}"
```
New sessions auto-pull; existing containers keep old images.

### macOS Desktop App — Deploying to VM
The API runs inside Docker in the VM at `~/helix`. Desktop app Go (`for-mac/`) auto-rebuilds via `wails dev`, but API changes need manual deploy:
```bash
git add -A && git commit -m "desc" && git push
ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -p 41222 ubuntu@localhost \
  "cd ~/helix && git fetch && git checkout BRANCH && git pull"
ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -p 41222 ubuntu@localhost \
  "cd ~/helix && docker compose -f docker-compose.dev.yaml restart api"
```

## Code Patterns

### Go
- Fail fast: `return fmt.Errorf("failed: %w", err)` — never log and continue
- Error on missing config — don't silently skip
- Structs for API responses, not `map[string]interface{}`
- GORM AutoMigrate only, gomock not testify/mock
- **NO FALLBACKS** — one approach, fix properly, no dead code paths
- **CLEAN UP DEAD CODE** immediately

Test suite pattern:
```golang
type MySuite struct {
	suite.Suite
	ctrl  *gomock.Controller
	store *store.MockStore
	server *HelixAPIServer
}
func TestMySuite(t *testing.T) { suite.Run(t, new(MySuite)) }
func (s *MySuite) SetupTest() { /* init ctrl, store, server */ }
```

### TypeScript/React
- **ALWAYS use generated API client** (`api.getApiClient()`) — NEVER raw `fetch/api.post/api.get`
- If endpoint missing: add swagger annotations → `./stack update_openapi` → use generated method
- React Query for all API calls, extract `.data` from Axios in query functions
- No `setTimeout` for async, no `type="number"` inputs (use text + parseInt)
- Extract components at 500+ lines
- **Dependency arrays**: ONLY primitives that change. NEVER include context values, functions, refs, or objects from hooks
- **Routing**: `useRouter()` with `router.navigate('name', { params })` — NOT `<Link>` or `<a href>` (react-router5)
- Invalidate queries after mutations, don't use `setQueryData`
- Use ContextSidebar pattern (see `ProjectsSidebar.tsx`)

## Architecture
- **ACP**: `LLM ←(OpenAI API)→ Qwen Code Agent ←(ACP)→ Zed IDE`
- **RBAC**: `authorizeUserToResource()` — unified AccessGrants
- **Enterprise**: Support internal DNS, proxies, air-gapped, private CAs

## Dev Environment (Helix-in-Helix)

**`helix-4` is a symlink to `helix`** — they are the same directory. Always use `/home/retro/work/helix/`.

When running as a spec task agent, the **inner Helix** at `http://localhost:8080` has the full sandbox running (helix-sandbox-nvidia-1 + Zed agent). You HAVE a complete dev environment — don't give up on testing.

### Test Credentials (inner Helix at localhost:8080)
- URL: `http://localhost:8080`
- Email: `test@helix.local` / Password: `testpass123`
- Or check `.env.usercreds` in the helix directory for real API keys

### Browser Testing Setup (inner Helix)
The inner Helix starts with **no users**. **You will almost always need to register before you can do anything.** Before testing any UI:
1. **Always try to register first** — go to `/login`, click "Register here", use `test@helix.local` / `testpass123`. Even if you think the account exists, registration fails gracefully if it does, so just try it.
2. **Complete onboarding** — after registering you land on `/onboarding`; create an org before you can access any other pages
3. Check DB to confirm: `docker exec helix-postgres-1 psql -U postgres -d postgres -c "SELECT email FROM users LIMIT 5;"`

### Go Local Tests (CGo fix)
`go test ./pkg/server/...` requires CGo for tree-sitter. Fix:
```bash
sudo apt-get update && sudo apt-get install -y gcc libc6-dev
CGO_ENABLED=1 go test -v -run TestSuiteName ./pkg/server/ -count=1
```

### Never Give Up on Testing
- Always test changes end-to-end in the inner Helix browser (MCP Chrome DevTools available)
- Check DB state: `docker exec helix-postgres-1 psql -U postgres -d postgres -c "SQL"`
- Investigate logs yourself — don't tell user to check logs (exception: ask user to verify UI)

## Verification

### Testing
- **Go**: `go build ./pkg/server/ ./pkg/store/ ./pkg/types/` (don't run store tests locally — need Postgres)
- **Frontend**: `cd frontend && yarn build` before committing
- **Full tests**: Push and check CI (`gh pr checks` or Drone API)
- Investigate logs yourself — don't tell user to check logs (exception: ask user to verify UI)

### Quick Log Checks
```bash
docker compose -f docker-compose.dev.yaml logs --tail 50 frontend | grep -i error  # frontend
docker compose -f docker-compose.dev.yaml logs --tail 30 api | grep -E "building|running|failed"  # api
docker compose logs --tail 100 sandbox-nvidia 2>&1 | grep -E "error|failed"  # sandbox
```

## API Authentication

**NEVER** use `oh-hallo-insecure-token` — that's the runner token, not a user key.

Use `.env.usercreds` (contains `HELIX_API_KEY`, `HELIX_URL`, `HELIX_UBUNTU_AGENT`, `HELIX_PROJECT`):
```bash
set -a && source .env.usercreds && set +a  # or:
export HELIX_API_KEY=`grep HELIX_API_KEY .env.usercreds | cut -d= -f2-`
```
**Shell bug**: Use backticks, not `$()` (tool escapes `$` incorrectly).

## Quick Reference
- Build CLI: `cd api && CGO_ENABLED=0 go build -o /tmp/helix-bin .`
- Regenerate API client: `./stack update_openapi`
- Kill stuck builds: `pkill -f "cargo build" && pkill -f rustc`
- Design docs: `design/YYYY-MM-DD-name.md`

## CI (Drone)
**ALWAYS use the Drone MCP tools** (`drone_build_info`, `drone_fetch_logs`, `drone_search_logs`, `drone_tail_logs`, `drone_read_logs`) when they are available. Do NOT try to extract credentials from `.env` files or use raw `curl` — the MCP tools handle authentication automatically.

Workflow for investigating CI failures:
1. `drone_build_info` — get build overview, see which steps failed
2. `drone_fetch_logs` — download logs for the failing step
3. `drone_search_logs` — search for `FAIL:`, `panic:`, `error` patterns
4. `drone_tail_logs` / `drone_read_logs` — read specific sections around failures

Fallback (only if Drone MCP tools are unavailable): credentials are in `.env` (`DRONE_SERVER_URL`, `DRONE_ACCESS_TOKEN`).
```bash
curl -s -H "Authorization: Bearer $DRONE_ACCESS_TOKEN" \
  "$DRONE_SERVER_URL/api/repos/helixml/helix/builds?branch=BRANCH&limit=3" | jq -r '.[] | "\(.number): \(.status)"'
```

## Database
```bash
docker exec helix-postgres-1 psql -U postgres -d postgres -c "SQL_HERE"
```
DB: `postgres`, user: `postgres`. Git repos at `/filestore/git-repositories/` in API container.

## CLI Testing

### Where Commands Run
Helix stack runs **inside the UTM VM** (SSH: `ssh -p 2222 luke@127.0.0.1`). Only UTM/QEMU and build scripts run on macOS host.

### Build & Deploy CLI
```bash
# macOS: cd api && CGO_ENABLED=0 go build -o /tmp/helix .
# Linux VM: cd api && GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o /tmp/helix-linux . && scp -P 2222 /tmp/helix-linux luke@127.0.0.1:/tmp/helix
```

### Key Commands
```bash
/tmp/helix spectask list|list-agents|start|resume|stop|screenshot|stream|benchmark|send|mcp|live
```

### Sandbox Service Names
- `sandbox-nvidia` (Linux GPU), `sandbox` (Linux no-GPU), `sandbox-macos` (macOS, container: `helix-sandbox-macos-1`)

### Container Logs
```bash
docker compose exec -T sandbox-nvidia docker ps --format "{{.Names}}" | grep ubuntu-external
docker compose exec -T sandbox-nvidia docker logs {CONTAINER} 2>&1 | grep -E "PIPEWIRE|zerocopy|desktop-bridge"
```

## Video Streaming Testing

**ALWAYS use benchmark CLI** — never test FPS on a static desktop (expected: 10 FPS keepalive).

```bash
/tmp/helix spectask start --project $HELIX_PROJECT -n "test"
sleep 15
/tmp/helix spectask benchmark ses_xxx --duration 30  # --video-mode zerocopy for zero-copy
```

Expected FPS: static=10, terminal=15-35, vkcube=55-60.

### Zerocopy Debug Patterns
- Good: `PipeWire state: Connecting`, `Buffer types: 0x8 (DmaBuf)`, `modifier=0x300000000e08xxx`
- Bad: `Error("error alloc buffers")`, `Buffer types: 0x4 (MemFd (SHM fallback))`
- Modifiers: `0x0`=LINEAR (bad), `0x300000000xxxxx`=NVIDIA tiled (good), `0x00ffffffffffffff`=DRM_FORMAT_MOD_INVALID

## Multi-Desktop Streaming E2E Test

Tests container video pipeline (PipeWire → GStreamer → H.264 → WebSocket) across multiple sessions.

```bash
# In VM: start 3 sessions, wait 20s, stream all in parallel
/tmp/helix spectask stream ses_01xxx --duration 30 > /tmp/s1.log 2>&1 &
/tmp/helix spectask stream ses_01yyy --duration 30 > /tmp/s2.log 2>&1 &
/tmp/helix spectask stream ses_01zzz --duration 30 > /tmp/s3.log 2>&1 &
wait && cat /tmp/s1.log /tmp/s2.log /tmp/s3.log
```
Look for: StreamInit received, video frames > 0, FPS > 0. Common issues: 0 frames (wait longer), OOM (try 2 sessions), "Sandbox not connected" (start fresh session).

From macOS host via SSH: `ssh -p 2222 -o StrictHostKeyChecking=no luke@127.0.0.1 "export HELIX_API_KEY=... && /tmp/helix spectask list"`. Don't combine `run_in_background` with `&` in SSH.

## Zed WebSocket Sync E2E Testing

The WebSocket sync protocol between Helix (Go) and Zed (Rust) has E2E tests that run a real Zed binary against a Go test server importing real production Helix server code.

### Architecture
- **Go test server**: `zed-repo/crates/external_websocket_sync/e2e-test/helix-ws-test-server/main.go`
- **Imports real code**: `server.NewTestServer()` from `api/pkg/server/test_helpers.go` + `memorystore` (in-memory, no Postgres)
- **9-phase test**: thread creation, follow-up, new thread, follow-up to non-visible thread, simulate user input, UI state query, open_thread + follow-up, mid-stream interrupt, rapid 3-turn cancel
- **Multi-agent rounds**: Tests run for both `zed-agent` and `claude` (Claude Code). Set `E2E_AGENTS` env var to control which agents are tested.
- **Screenshots**: Periodic Xvfb screenshots captured in `/test/screenshots/`

### Running Locally (from Zed repo)

**Both repos must be checked out as siblings** (`~/pm/helix` and `~/pm/zed`). The Go test server imports Helix server code via a `replace` directive pointing to `../../../../../helix`.

```bash
# 1. Build Zed binary (if not already built)
#    Use 'dev' for faster iteration (~3min), 'release' for CI/production (~12min)
cd ~/pm/helix && ./stack build-zed dev

# 2. Copy Zed binary to e2e-test dir
cp ~/pm/helix/zed-build/zed ~/pm/zed/crates/external_websocket_sync/e2e-test/zed-binary

# 3. Run E2E tests (builds Go test server from current checkout + Docker image)
cd ~/pm/zed/crates/external_websocket_sync/e2e-test

# Single agent (fast, ~2min):
./run_docker_e2e.sh

# Both agents (zed-agent + claude, ~5min):
E2E_AGENTS="zed-agent,claude" ./run_docker_e2e.sh

# Skip Go rebuild (use cached binary — only safe if Go code hasn't changed):
E2E_AGENTS="zed-agent,claude" ./run_docker_e2e.sh --no-build
```

**What gets tested**: The Go test server is built from `~/pm/zed/.../helix-ws-test-server/` which imports the Helix Go code from `~/pm/helix/api/` via the `replace` directive. So you're testing the **currently checked out versions of both repos**.

**Rebuild checklist** — if you change code, rebuild the affected component:
| Changed | Rebuild |
|---------|---------|
| Helix Go code (`api/`) | `./run_docker_e2e.sh` (rebuilds Go test server) |
| E2E test server (`helix-ws-test-server/main.go`) | `./run_docker_e2e.sh` (rebuilds Go test server) |
| Zed Rust code (`crates/`) | `cd ~/pm/helix && ./stack build-zed dev` then copy binary |
| Dockerfiles or `run_e2e.sh` | `./run_docker_e2e.sh` (rebuilds Docker image) |

The script prints binary timestamps and checksums — check these to verify you're testing the right code.

**ANTHROPIC_API_KEY**: Auto-sourced from `~/pm/helix/.env` or `~/pm/helix/.env.usercreds`. Must be set for tests to work.

### Go Unit Tests (server-side)
```bash
cd api && go test -v -run TestWebSocketSyncSuite ./pkg/server/ -count=1
```

### CI (Drone)
The `zed-e2e-test` step in `.drone.yml` runs automatically on the sandbox-build pipeline:
1. Clones Zed at commit pinned in `sandbox-versions.txt` (`ZED_COMMIT=...`)
2. Builds Zed binary (cached by commit hash)
3. Multi-stage Docker build: Go test server (with current helix source) + runtime
4. Runs 9-phase E2E test for both `zed-agent` and `claude` with `ANTHROPIC_API_KEY` from Drone secrets

**Updating pinned Zed version**: After pushing Zed changes, update `sandbox-versions.txt` with the new commit hash. The Go test server's `go.mod` has a `replace` directive for local dev; CI overrides it to point to `/drone/src`.

### Key Files
| File | Purpose |
|------|---------|
| `sandbox-versions.txt` | Pins `ZED_COMMIT` for CI builds |
| `api/pkg/server/test_helpers.go` | `NewTestServer`, `QueueCommand`, `SetSyncEventHook` |
| `api/pkg/store/memorystore/` | In-memory store for tests (no Postgres) |
| `api/pkg/server/websocket_external_agent_sync_test.go` | 46 Go unit tests for handler paths |
| `design/2026-03-20-multi-agent-e2e-tests.md` | Multi-agent E2E test design and roadmap |

## CLI Development
Use the helix CLI for testing, not raw curl. If functionality is missing, add it to `api/pkg/cli/spectask/`.
