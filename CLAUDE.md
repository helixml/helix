# Helix Development Rules

## Communication Style
- Be a skeptical staff engineer — no sycophancy, challenge bad assumptions, focus on facts
- If uncertain, investigate rather than confirming beliefs
- **Current year: 2026** — include "2026" in web searches for current info
- If you find yourself adding hacks or workarounds, **stop** — take a step back, root cause the issue, understand the wider context, and fix it properly. Don't always take the path of least resistance — we need maintainable code.
- **Always give full URLs for PRs and issues** — never use the `owner/repo#123` shorthand format. Use `https://github.com/helixml/helix/pull/123` etc.
- **Be concise. Only write comments when necessary for external documentation.**
- See also: `.cursor/rules/*.mdc`

## FORBIDDEN ACTIONS

### Git
- **NEVER** `git checkout -- .`, `git reset --hard`, `git checkout -f` — destroys uncommitted work
- **NEVER** `git stash drop/pop` — use `git stash apply` (keeps backup)
- **NEVER** squash merge — always use regular merge commits (`gh pr merge --merge`)
- **NEVER** `gh pr create` without `--repo helixml/zed` when in the Zed repo — the upstream `zed-industries/zed` remote causes `gh` to target the wrong repo by default
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
- **Use conventional commit format**: `type(scope): description`
  - Types: `feat`, `fix`, `refactor`, `chore`, `docs`, `test`, `style`, `perf`, `ci`, `build`, `revert`
  - Scope is optional but encouraged (e.g., `api`, `frontend`, `specs`, `zed`)
  - Subject ≤ 72 chars, imperative mood, no trailing period
  - Examples: `feat(api): add PR content reading from helix-specs`, `fix(frontend): handle empty task list`, `chore(specs): update progress`
  - The `commit-msg` hook enforces this — non-conforming commits are rejected
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

### Dev Stack Networking
- The **local dev stack** is at `localhost:8080`. Use this for all API calls when testing your changes.
- `api:8080` is the **outer Helix stack** (the one running your agent session). Requests to `api:8080` hit the production/outer API, NOT your local dev code. The `$USER_API_TOKEN` / `$HELIX_API_URL` env vars also point at the outer stack.
- When using `curl` or the browser to test, always use `http://localhost:8080`, never `http://api:8080`.

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

**Experimental desktop pulls.** The sandbox startup script
(`sandbox/04-start-dockerd.sh`) only pulls the *production* desktop image
(`helix-ubuntu`) on every container start. Experimental desktops
(`helix-sway`, `helix-zorin`, `helix-xfce`, `helix-kde`) are gated behind
the `HELIX_EXPERIMENTAL_DESKTOPS` env var (space-separated, default
empty). Set e.g. `HELIX_EXPERIMENTAL_DESKTOPS="sway"` in your environment
to pre-pull sway at sandbox startup; otherwise it's pulled lazily by
Docker the first time someone launches a sway desktop session.

### **CRITICAL: Bumping sandbox-versions.txt after Zed or Qwen changes**

`sandbox-versions.txt` pins the exact commits CI uses to build the sandbox:
```
ZED_COMMIT=<full git sha>
QWEN_COMMIT=<full git sha>
```

**If you modify Zed or Qwen, you MUST follow this order:**

1. Commit your changes in the Zed/Qwen repo (do NOT push yet).
2. Copy the local commit hash: `git rev-parse HEAD`
3. Update `sandbox-versions.txt` in this repo with that hash.
4. **Open the Helix PR** (with the bumped hash) *before* pushing the Zed/Qwen branch.
5. Push the Zed/Qwen branch and open that PR.
6. Merge the Zed/Qwen PR.
7. Merge the Helix PR.

**Why this order matters:** The spec task system marks a task done when all its PRs are merged. If the Zed PR is merged first, the system may close the task before `sandbox-versions.txt` is updated — leaving CI pointing at the wrong commit indefinitely. Getting the commit hash from a local commit (before pushing) solves the chicken-and-egg problem.

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
- **Org lookup in API handlers**: when a handler reads an `{org_id}` URL segment (or any `org_id` request field), ALWAYS resolve it via `s.lookupOrg(ctx, orgStr)` (in `wallet_handlers.go`) before using it as a key for store lookups, authorization, or persisting onto a row. `lookupOrg` accepts both an `org_…` id AND an org name/slug, so handlers transparently work whether the frontend sent the canonical id or the URL-facing slug. Never write the raw URL segment into a row's `OrganizationID` column — the slug doesn't match wallet/membership rows keyed by id (this caused sandbox delete to fail with `get org wallet: not found`).

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
- **Search/filter**: use `matchesAllTokens()` from `utils/searchUtils.ts` — splits on whitespace, requires all tokens to match (AND logic). NEVER use raw `.includes(query)` for search boxes — it fails on multi-word queries

### UI Styles

These rules keep our list pages visually consistent. When in doubt, mirror `Sandboxes.tsx` / `Tasks.tsx`.

#### Tables
- **ALWAYS use `SimpleTable`** from `frontend/src/components/widgets/SimpleTable.tsx`. Don't reach for raw MUI `<Table>` / `<TableContainer>`. References: `TasksTable.tsx`, `SandboxesTable.tsx`, `AppsTable.tsx`.
- Build rows via a `tableData` `useMemo` that maps each entity to `{ id, _data: <entity>, <field>: <ReactNode>, ... }`. Always include `_data` so action handlers can recover the typed entity.
- Cells are `<Typography>` nodes, not raw strings. Use `variant="body2" color="text.secondary"` for non-primary cells. Make the name cell a bold link via an inline `<a>` (see `TasksTable.tsx`); call `e.preventDefault()` + `e.stopPropagation()` in its `onClick`.
- **Row actions go in a single vertical-dot menu**, never a row of icon buttons. Implement `getActions` as one `<IconButton><MoreVertIcon/></IconButton>` that opens a `<Menu>` with `<MenuItem>` entries (each item has a leading icon: `<Icon sx={{ mr: 1, fontSize: 20 }} />`). Track the active row via `currentX` state set on menu open; clear it on close.
- `e.stopPropagation()` in every menu/icon click handler so row clicks don't fire.
- Status chips: build a small dedicated component (e.g. `SandboxStatusBadge`) rather than inlining `<Chip>` styling.

#### Cards (cards view)
- Render via the shared `CardGrid` (`components/widgets/CardGrid.tsx`) — never roll a new MUI `Grid container` (its negative margins break flush alignment with the page title).
- Card chrome: `<Card>` with `border: '1px solid rgba(0, 0, 0, 0.08)'`, `borderRadius: 1`, `boxShadow: 'none'`, hover bumps `borderColor` to `rgba(0,0,0,0.12)` and tints `backgroundColor: 'rgba(0,0,0,0.01)'`. `height: '100%'` + `display: 'flex'; flexDirection: 'column'` so the grid rows align.
- Inside, `<CardContent>` uses `p: 2`, `'&:last-child': { pb: 2 }`, `cursor: 'pointer'`, and an `onClick` that opens the detail view.
- **Top-right corner gets the vertical-dot menu** (`<IconButton><MoreVertIcon sx={{ fontSize: 16 }}/></IconButton>` → `<Menu>`). Same menu items as the table actions — keep the two surfaces in sync. Don't put separate Open/Delete icons at the bottom of the card.
- Status indicator goes inline next to the dot menu (e.g. status badge), or as the leading icon by the title (see `CronTaskCard.tsx` — green clock vs paused-circle, with tooltip).
- Dense stat strip uses the gradient panel: `background: 'linear-gradient(145deg, rgba(255,255,255,0.03) 0%, rgba(255,255,255,0.01) 100%)'`, `border: '1px solid rgba(255,255,255,0.06)'`, `borderRadius: 2`, `p: 1.5`. Stats are label (caption, `0.65rem`, `text.secondary`) + value (body2, `0.8rem`, `monospace`, `fontWeight: 600`).

#### Table ↔ cards toggle
- Use `ViewModeToggle` (`components/widgets/ViewModeToggle.tsx`) + the `useViewMode(storageKey, defaultMode)` hook. State persists via URL `?view=` param + localStorage automatically.
- Toggle sits **directly above the table/grid, right-aligned**, in its own row inside the page `Stack`. Don't park it in `topbarContent` — the topbar is reserved for the primary action button (e.g. "New Sandbox").
- Page header (`<Typography variant="h5">Title</Typography>` + secondary description) sits above the toggle in the same `Stack`.

## Architecture
- **ACP**: `LLM ←(OpenAI API)→ Qwen Code Agent ←(ACP)→ Zed IDE`
- **RBAC**: `authorizeUserToResource()` — unified AccessGrants
- **Enterprise**: Support internal DNS, proxies, air-gapped, private CAs

## helix-org design philosophy

Anything under `api/pkg/org/` is the org-graph runtime (Workers, Positions, Roles, Streams). Behaviour lives in the prompt/profile, not in Go code. The code is scaffolding.

- **Prefer data and text over code.** If a feature can be expressed as a Role/Position prompt edit, a scope value, or a tool description, do that before adding Go logic.
- **Keep the MCP surface small.** MCP tools are reserved for org-graph primitives (reads + mutations of Workers, Positions, Roles, Streams). Anything else a Worker needs goes through shell tools provisioned in their environment (`bash`, `curl`, `git`, `gh`, `python`). Don't add MCP wrappers like `publish_to_blog` or `fetch_url` — describe the shell usage in the Role text instead. **One recorded exception:** `mint_credential` is a generic credential-minting *primitive* (it is what makes the shell tools usable on long-running sessions whose boot-time tokens have expired). A *primitive* is different from a per-action wrapper; the per-action ban stands. See `design/tasks/002092_helix-org-mintcredential/design.md` §2 for the full rationale.
- **Complete a user action in as few steps as possible.** A tool should do the whole of what the user means by one action, not force a chain of follow-up calls. `create_bot`, for example, grants the new Bot its initial tools AND subscribes it to the topics named at creation — in one call — because a manager creating a Bot almost always wants it tooled-up and listening immediately. Prefer bulk arguments (arrays) over one-at-a-time calls for the same reason. This supersedes the older "no workflow in code / `Role.Streams` stays prompt-driven" rule: creation-time subscription is a supported convenience, not forbidden orchestration. Keep the *implementation* DRY — `create_bot` reuses the same `subscriptions.Subscribe` use case the standalone `subscribe` tool calls; it does not reimplement it. Structural derivation still holds: `Bot.Tools` is the live MCP surface, and editing it changes the Bot's capability. When reviewing a tool, ask: "does this complete the user's intent, reusing existing use cases, without hiding a decision the agent should make?"
- **Social enforcement first.** A Worker reads scope from its prompt and complies. Reach for hard enforcement only when the cost of a violation is high.
- **Keep the core generic.** Tool definitions and scope shapes live with the tool, not in the registry, server, or domain layer. New tools must be addable without editing the core.

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
- **PREFER end-to-end testing in the inner Helix over every other form of verification.** Setup is fast, not "substantial work" — register (`test@helix.ml` / `helixtest`), complete onboarding (testorg → testproj → claude-opus-4-6 auto-selects), create a spectask, navigate to its detail page. Do this *every time* a UI change is testable in the inner Helix. The inner Helix exists for exactly this — there is no point having it and not using it. Isolated DOM harnesses, JS-only algorithm replays, and unit tests are NOT substitutes; they verify the algorithm, not the wired-up production component.
- Always test changes end-to-end in the inner Helix browser using the `mcp__chrome-devtools__*` MCP tools (they're under that prefix — easy to miss in the tool listing).
- **Be patient with the stack booting — do NOT give up after one failed check.** At session start the stack pulls images and brings up containers in stages; `localhost:8080` returning `000`/connection-refused, or a container showing `Restarting`/`health: starting`, means it is still coming up, NOT that it is broken. Poll `docker compose -f docker-compose.dev.yaml ps` and `curl -s -o /dev/null -w '%{http_code}' http://localhost:8080` on a loop, waiting **several minutes** (the full bring-up can take 5–10 min) before concluding the stack is unavailable. The signal that it is ready is `helix-api-1`, `helix-frontend-1`, and `helix-postgres-1` all `Up` and `8080` returning `200`. Never downgrade to "couldn't verify in the UI" while containers are still mid-startup — wait for them.
- Check DB state: `docker exec helix-postgres-1 psql -U postgres -d postgres -c "SQL"`
- Investigate logs yourself — don't tell user to check logs (exception: ask user to verify UI)

### Exposing the inner Helix to public webhooks (Notion, Stripe, GitHub, …)
When a third-party service needs to POST a webhook into the inner Helix from the public internet, use **cloudflared quick tunnels** — no signup, no auth token, no account. Anonymous, ephemeral.
```bash
curl -sSL https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64 -o /tmp/cloudflared && chmod +x /tmp/cloudflared
/tmp/cloudflared tunnel --url http://localhost:8080 --no-autoupdate > /tmp/cloudflared.log 2>&1 &
sleep 6 && grep -oE 'https://[a-z0-9-]+\.trycloudflare\.com' /tmp/cloudflared.log | head -1
```
The printed `https://<random>.trycloudflare.com` URL fronts `localhost:8080`. Paste it (plus your `/api/v1/webhooks/{trigger_id}` path) into the third-party's webhook config. Tunnel dies when the cloudflared process does.

Avoid ngrok — it now requires an authtoken even for short tests, which means asking the user for credentials and storing them somewhere. cloudflared's quick-tunnel mode is the same idea with no friction.

## Verification

### Testing
- **Go**: `go build ./pkg/server/ ./pkg/store/ ./pkg/types/` (don't run store tests locally — need Postgres)
- **Frontend**: `cd frontend && yarn build` before committing
- **Full tests**: Push and check CI (`gh pr checks` or Drone API)
- Investigate logs yourself — don't tell user to check logs (exception: ask user to verify UI)

### Don't claim confidence you didn't earn
- NEVER report quantified confidence ("95% tested", "last 5% works"). Either you ran
  it end-to-end (say so, with the output) or you did not (say "NOT tested: <what/why>").
- A unit test that checks a state change (field reset, row deleted) is NOT evidence the
  feature works. Do not write "covered by unit tests" when the test only asserts the
  mechanism, not the user-visible outcome.
- Reasoning by analogy ("same as the fork/X path") is a hypothesis, not a result.
  Verify the precondition state matches — a live/connected resource is a DIFFERENT
  state from a fresh/offline one, and lifecycle bugs hide in that gap.

### Test the next operation, not just the state change
- For any reset/clear/delete/cancel/switch feature, always exercise the IMMEDIATELY
  FOLLOWING normal operation: clear → send a message; delete → recreate; cancel →
  resume; reset thread → next turn. Bugs live in that seam, not in the mutation itself.

### Live external-agent (Zed) testing is mandatory for lifecycle changes
- Features touching session/thread lifecycle (clear, fork, cancel, resume, switch-agent)
  MUST be tested against a LIVE, connected Zed — not seeded DB rows. Seeded rows only
  exercise the no-connection branch and miss thread_created routing entirely.
- To get a live session fast: create a **spec task** (it provisions a git repo, so Zed's
  workspace setup completes and it opens the sync WebSocket). A bare
  `agent_type=zed_external` chat session does NOT work — no repo → workspace setup
  FATALs after 300s → Zed never connects. Liveness check: `config->>'zed_thread_id'` is a
  non-empty UUID.

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

**Helix-in-Helix**: `.env.usercreds` is NOT available in the inner instance. Use the browser at `http://localhost:8080`. In dev mode, the first registered account is automatically admin. Use these fixed credentials so sessions are idempotent:
- Email: `test@helix.ml`
- Password: `helixtest`
- Full Name: `Test User`

If already registered, click "Sign in here" and use the same credentials.

## Quick Reference
- Build CLI: `cd api && CGO_ENABLED=0 go build -o /tmp/helix-bin .`
- Regenerate API client: `./stack update_openapi`
- Kill stuck builds: `pkill -f "cargo build" && pkill -f rustc`
- Design docs: `design/YYYY-MM-DD-name.md`

## Releases & Continuous Delivery
**Cut a release with the GH CLI, always auto-generating the notes:**
```bash
gh release create <tag> --target main --generate-notes   # e.g. 2.11.32
```
- `--generate-notes` == GitHub's "Generate release notes" button: a per-PR changelog with
  contributors + a Full Changelog diff vs the previous tag. **Never** hand-write `--notes`.
- Tags are bare semver `2.11.x` (no `v` prefix). Bump the patch from the latest:
  `git tag --sort=-v:refname | head`.
- To regenerate notes on an existing release:
  `gh api -X POST repos/helixml/helix/releases/generate-notes -f tag_name=<tag> -f target_commitish=main --jq .body > /tmp/n.md && gh release edit <tag> --notes-file /tmp/n.md`.

**Cutting the tag self-deploys to prod.** The tag triggers the Drone tag build, whose
`deploy-prod` pipeline (`scripts/deploy-prod.sh`) SSHes to prod and rolls the new version:
the **london controlplane** (`helix-cloud-london`: ZFS-snapshots the DB, bumps `HELIX_VERSION`
in `/data/helix-app/helix/.env`, `docker compose pull/up api`, health-checks, auto-rolls-back
on failure) and the **code.helix.ml runner** (bumps `SANDBOX_TAG`). See
`design/2026-06-25-prod-version-bump-runbook.md`. Watch the build with `gh pr checks` / the
Drone MCP tools and confirm `deploy-prod` goes green.

## CI (Drone)
**ALWAYS check CI yourself after pushing a PR.** Don't make the user discover the failure and tell you. As soon as a push is up, use `gh pr checks <num>` (or the Drone MCP tools / fallback below) to confirm green or surface failures. If failing, drill into the logs, fix, push the fix, and re-check — all without being asked.

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

## Sandboxes API

User-facing ephemeral containers for exec / files / terminal — different from spec-task sandboxes (those run the full Zed/agent stack). Source of truth: `design/2026-04-29-sandboxes-api.md`.

### Architecture

```
helix CLI / UI ─REST/WS─▶ helix API (sandbox controller)
                                  │ store.Sandbox row
                                  ▼ Postgres
                                  │
                       picks hydra host (heartbeat-versioned for desktop, any online host for headless/custom)
                                  │
                                  ▼ RevDial
                          hydra (in helix-sandbox-nvidia-1)
                                  │ docker exec / cp / inspect
                                  ▼
                          /sbx-{id} container (configured runtime image)
```

- Org-scoped, optional `project_id`. Cross-org id-guessing is blocked by `loadAuthorizedSandbox`.
- 1 vCPU / 2GB RAM, default TTL 1h, soft-delete on expiry by the reaper goroutine (`StartReaper`, polls 60s).
- Headless containers use plain public images (`ubuntu:22.04`, `node:22-bookworm-slim`, …); `SkipImageValidation=true` triggers a proactive `docker pull` in hydra.
- Desktop runtime is heartbeat-versioned (`helix-ubuntu:<sha>`) and runs the full GNOME shell. Currently broken in pure sandbox-API mode (gnome-shell exits) — prefer headless runtimes.

### Runtime configuration

Operators configure runtimes via env vars (parsed by `config.Sandboxes`):

| Env | Default |
|---|---|
| `HELIX_SANDBOX_RUNTIMES` | `headless-ubuntu=ubuntu:22.04\|sleep infinity,node22=node:22-bookworm-slim\|tail -f /dev/null,python313=python:3.13-slim\|tail -f /dev/null` |
| `HELIX_SANDBOX_DEFAULT_RUNTIME` | `headless-ubuntu` |
| `HELIX_SANDBOX_ALLOW_CUSTOM_IMAGE` | `false` (set true to let API callers pass arbitrary `image`) |

Format: `name=image[\|keep-alive-shell-cmd]`. Add new runtimes (go, rust, java, …) by extending the CSV — no code change.

### CLI

All commands honour `$HELIX_API_KEY` and `$HELIX_URL`. Org defaults to `$HELIX_ORG` or your first org.

```bash
helix sandbox runtimes                                    # discover what's available
helix sandbox create --name x --runtime node22 --ttl 600  # create + wait for running
helix sandbox create --image alpine:3.19                  # custom image (requires gate enabled)
helix sandbox list [--project prj_…]                      # filter by project
helix sandbox get <sbx_…>                                 # full row as JSON

# exec
helix sandbox exec <sbx_…> -- <cmd> [args...]             # synchronous, prints stdout/stderr
helix sandbox exec <sbx_…> --detached -- <cmd>            # fire-and-forget, prints cmd id
helix sandbox commands <sbx_…>                            # list commands tracked
helix sandbox logs <sbx_…> <sbcmd_…> [--follow]           # SSE log stream
helix sandbox kill <sbx_…> <sbcmd_…> [--signal TERM]

# files
helix sandbox ls <sbx_…> --path /tmp
echo data | helix sandbox write <sbx_…> /tmp/x.txt --mode 644
helix sandbox read <sbx_…> /tmp/x.txt

# terminal
helix sandbox terminal <sbx_…>                            # interactive PTY (xterm)

# cleanup
helix sandbox delete <sbx_…>                              # immediate teardown
```

### Workflow expectations

- `create → wait` is fast (~1-2s for headless after the image is pulled). First create of a new image incurs a `docker pull`.
- `exec` synchronous: 60s default per-command timeout. Use `--detached` for anything longer or interactive — then poll via `commands` / stream via `logs --follow`.
- TTL is enforced server-side: row's `expires_at` is set on create, refreshed on `PATCH timeout_seconds`. Reaper deletes after expiry.
- Container is ephemeral. Nothing survives `delete` — no snapshots in v1.

### Where things live

| File | Purpose |
|---|---|
| `api/pkg/types/sandbox.go` | `Sandbox`, `CreateSandboxRequest`, status enums |
| `api/pkg/sandbox/runtimes.go` | `RuntimeRegistry` — config-driven runtime spec resolution |
| `api/pkg/sandbox/controller.go` | Lifecycle: create, provision, delete, reaper |
| `api/pkg/server/sandboxes_api_handlers.go` | REST handlers (auth, JSON, ws bridge for terminal) |
| `api/pkg/hydra/sandbox_handlers.go` / `sandbox_ops.go` | hydra-side exec/file/terminal implementation |
| `api/pkg/hydra/client_sandbox.go` | RevDial client used by API → hydra |
| `api/pkg/client/sandbox.go` | Public Go client (used by CLI; available to external SDKs) |
| `api/pkg/cli/sandbox/sandbox.go` | `helix sandbox …` cobra subcommands |
| `frontend/src/pages/Sandboxes.tsx`, `SandboxDetail.tsx` | UI list + detail (Overview/Terminal/Commands/Files) |

### Hot-rebuild loop

- API code changes → Air rebuilds `helix-api-1` automatically.
- **hydra code changes** (`api/pkg/hydra/`) require redeploying the binary into the sandbox container — Air does NOT rebuild it:
  ```bash
  cd api && CGO_ENABLED=0 GOOS=linux go build -o /tmp/hydra-linux ./cmd/hydra
  docker cp /tmp/hydra-linux helix-sandbox-nvidia-1:/usr/local/bin/hydra
  docker compose -f docker-compose.dev.yaml exec -T sandbox-nvidia pkill -TERM hydra
  ```
  hydra restarts automatically; tail `docker logs helix-sandbox-nvidia-1` and look for `RevDial control connection established` to confirm.
- Frontend → Vite HMR (port 8081 mounted into `helix-frontend-1`).
