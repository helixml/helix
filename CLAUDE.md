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
- **Frontend**: Vite HMR (dev mode) or `yarn build` + refresh (prod mode)
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
Credentials in `.env` (`DRONE_SERVER_URL`, `DRONE_ACCESS_TOKEN`).
```bash
# Check build status
curl -s -H "Authorization: Bearer $DRONE_ACCESS_TOKEN" \
  "$DRONE_SERVER_URL/api/repos/helixml/helix/builds?branch=BRANCH&limit=3" | jq -r '.[] | "\(.number): \(.status)"'
# Get failing step logs
curl -s -H "Authorization: Bearer $DRONE_ACCESS_TOKEN" \
  "$DRONE_SERVER_URL/api/repos/helixml/helix/builds/BUILD/logs/1/STEP" | jq -r '.[].out' | grep -E "FAIL|Error|panic"
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

## CLI Development
Use the helix CLI for testing, not raw curl. If functionality is missing, add it to `api/pkg/cli/spectask/`.
