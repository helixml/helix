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

**Sandbox architecture**: Host → Wolf container → helix-sway container (Zed + Qwen Code + daemons)

| Component | Rebuild trigger | Commit needed? |
|-----------|----------------|----------------|
| helix files | Docker cache | No |
| qwen-code | `git rev-parse HEAD` | **Yes** |
| Zed | `./stack build-zed` | No |

```bash
# Helix changes: ./stack build-sway
# Qwen changes: cd ../qwen-code && git commit -am "msg" && cd ../helix && ./stack build-sway
# Zed changes: ./stack build-zed && ./stack build-sway
# Verify: cat sandbox-images/helix-sway.version
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

## Quick Reference

- Regenerate API client: `./stack update_openapi`
- Wolf API: `docker compose exec api curl --unix-socket /var/run/wolf/wolf.sock http://localhost/api/v1/apps`
- Kill stuck builds: `pkill -f "cargo build" && pkill -f rustc`
- Design docs go in `design/YYYY-MM-DD-name.md`
