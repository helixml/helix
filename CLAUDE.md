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
- ✅ OK: `./stack build`, `build-zed`, `build-sway`, `build-ubuntu`, `build-wolf`, `update_openapi`

### Docker
- **NEVER** use `--no-cache` — trust Docker cache
- `docker compose restart` does NOT apply .env or image changes — use `down` + `up`

### Other
- **NEVER** rename current working directory — breaks shell session
- **NEVER** commit customer data (hostnames, IPs) — repo is public
- **NEVER** restart hung processes — collect GDB backtraces first

## Build Pipeline

**Sandbox architecture**: Host → helix-sandbox container (Hydra + Wolf + Moonlight Web + DinD) → helix-sway container (Zed + Qwen Code + daemons)

### Component Dependencies

```
helix-sandbox (outer container)
├── hydra (Go, dev container lifecycle, Docker isolation)
├── wolf:helix-fixed (streaming server, GStreamer plugins)
│   ├── Wolf C++ server
│   ├── gst-wayland-display (Rust, for Sway compositor capture)
│   └── gst-pipewire-zerocopy (Rust, for GNOME ScreenCast capture)
├── helix-moonlight-web:helix-fixed (web streaming)
└── helix-sway.tar / helix-ubuntu.tar (desktop images)
    ├── Zed IDE
    └── Qwen Code agent
```

### When to Rebuild What

| Changed | Command | Notes |
|---------|---------|-------|
| Wolf C++ (`~/pm/wolf/src/`) | `./stack build-wolf && ./stack build-sandbox` | Wolf is embedded in sandbox |
| Wolf Rust plugins (`~/pm/wolf/gst-pipewire-zerocopy/`) | `./stack build-wolf && ./stack build-sandbox` | Same as above |
| Hydra (`api/pkg/hydra/`) | `./stack build-sandbox` | Hydra binary runs IN sandbox, NOT API |
| Desktop image (helix-sway) | `./stack build-sway` | Creates tarball in sandbox-images/ |
| Desktop image (helix-ubuntu) | `./stack build-ubuntu` | Creates tarball in sandbox-images/ |
| Desktop streaming (`api/pkg/desktop/`) | `./stack build-ubuntu` or `./stack build-sway` | Go code runs IN desktop container, NOT API |
| Sandbox scripts | `./stack build-sandbox` | Dockerfile.sandbox changes |
| Zed IDE | `./stack build-zed && ./stack build-sway` | Zed binary → desktop image |
| Qwen Code | `cd ../qwen-code && git commit -am "msg" && cd ../helix && ./stack build-sway` | Needs git commit |
| Moonlight Web | `./stack build-moonlight-web && ./stack build-sandbox` | Embedded in sandbox |

### Build Order for Full Rebuild

```bash
# 1. Build Wolf (streaming server + GStreamer plugins)
./stack build-wolf

# 2. Build Moonlight Web (optional, if changed)
./stack build-moonlight-web

# 3. Build Zed (if changed)
./stack build-zed

# 4. Build desktop images (creates tarballs)
./stack build-sway
./stack build-ubuntu

# 5. Build sandbox (embeds Wolf, Moonlight Web, and desktop tarballs)
./stack build-sandbox

# 6. Restart sandbox to use new image
docker compose down sandbox && docker compose up -d sandbox
```

### Verify Build

```bash
# Check desktop image versions
cat sandbox-images/helix-sway.version
cat sandbox-images/helix-ubuntu.version

# Check Wolf image
docker images wolf:helix-fixed

# Check sandbox has latest Wolf
docker compose exec -T sandbox ls -la /wolf/wolf
```

New sessions use updated image; existing containers don't update.

## Code Patterns

### Go
- Fail fast: `return fmt.Errorf("failed: %w", err)` — never log and continue
- Use structs, not `map[string]interface{}` for API responses
- GORM AutoMigrate only — no SQL migration files
- Use gomock, not testify/mock

### TypeScript/React
- Use generated API client + React Query for ALL API calls
- Extract `.data` from Axios responses in query functions
- No `setTimeout` for async — use events/promises
- Extract components when files exceed 500 lines
- No `type="number"` inputs — use text + parseInt

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

After frontend changes:
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
- Design docs go in `design/YYYY-MM-DD-name.md`

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

### Wolf API (streaming infrastructure)
```bash
# List active lobbies (sandbox sessions)
docker compose exec -T sandbox curl -s --unix-socket /var/run/wolf/wolf.sock \
  http://localhost/api/v1/lobbies | jq '.[].name'

# List Wolf streaming sessions (active Moonlight connections)
docker compose exec -T sandbox curl -s --unix-socket /var/run/wolf/wolf.sock \
  http://localhost/api/v1/sessions | jq

# Get GPU and memory stats
docker compose exec -T sandbox curl -s --unix-socket /var/run/wolf/wolf.sock \
  http://localhost/api/v1/system/memory | jq '{gpu: .gpu_stats, lobbies: (.lobbies | length)}'
```

### Image Versions
```bash
# Check current desktop image versions
cat sandbox-images/helix-sway.version
cat sandbox-images/helix-ubuntu.version

# Verify image is loaded in sandbox's dockerd
docker compose exec -T sandbox docker images | grep helix-
```

### Logs
```bash
# Screenshot server logs (inside sandbox container)
docker compose exec -T sandbox docker logs {CONTAINER_NAME} 2>&1 | grep -E "screenshot|capture"

# Wolf logs
docker compose logs --tail 50 sandbox 2>&1 | grep -E "lobby|session|GPU"

# API logs for external agents
docker compose logs --tail 50 api 2>&1 | grep -E "external-agent|screenshot|Wolf"
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
