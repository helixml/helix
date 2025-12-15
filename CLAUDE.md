# Helix Development Rules

See also: @.cursor/rules/helix.mdc, @.cursor/rules/go-api-handlers.mdc, @.cursor/rules/use-gorm-for-database.mdc, @.cursor/rules/use-frontend-api-client.mdc

## 🚨 CRITICAL: NEVER RUN ./stack start 🚨

**NEVER run `./stack start` - only the user runs this command**

```bash
# ❌ ABSOLUTELY FORBIDDEN
./stack start                          # NEVER DO THIS
bash -c "./stack start"                # OR THIS
```

**Why this is forbidden:**
- `./stack start` creates a tmux session that requires interactive terminal
- You cannot interact with tmux sessions (you'll get "not a terminal" errors)
- Starting services disrupts user's workflow and terminal setup
- User manages their own development environment startup

**What to do instead:**
- ✅ Tell user to run `./stack start` if services need starting
- ✅ Use `./stack up` for specific services if absolutely necessary
- ✅ Check service status with `docker compose ps`
- ✅ View logs with `docker compose logs`

**Other stack commands you CAN use:**
- `./stack build` - Build containers
- `./stack build-zed` - Build Zed binary
- `./stack build-sway` - Build Sway desktop container (Dockerfile.sway-helix)
- `./stack build-ubuntu` - Build Ubuntu desktop container (Dockerfile.ubuntu-helix)
- `./stack build-wolf` - Build Wolf
- `./stack update_openapi` - Update OpenAPI docs
- `./stack up <service>` - Start specific service (use sparingly)

## 🚨 CRITICAL: Sandbox Build Pipeline - COMMIT EVERYTHING FIRST 🚨

**The sandbox architecture is deeply nested. Changes MUST be committed before rebuilding.**

```
┌─────────────────────────────────────────────────────────────────┐
│ Host Machine                                                     │
│  └── Wolf Container (runs Docker-in-Docker)                     │
│       └── helix-sway Container (the sandbox)                    │
│            ├── Zed binary (built from ~/pm/zed)                 │
│            ├── Qwen Code (built from ~/pm/qwen-code)            │
│            └── Settings Sync Daemon (built from helix/api/cmd)  │
└─────────────────────────────────────────────────────────────────┘
```

**The problem: `./stack build-sway` copies from LOCAL builds, not git repos:**
- It copies `~/pm/qwen-code/packages/cli/dist/` → into Docker image
- It copies `~/pm/zed/target/release/zed` → into Docker image
- Wolf's inner Docker pulls the helix-sway image by TAG (based on git commit)

**If you don't commit before rebuild:**
1. Build runs → creates new helix-sway image
2. Image gets tagged with current git commit hash
3. BUT if repos aren't committed, the tag doesn't change
4. Wolf's inner Docker sees same tag → uses cached image
5. **YOUR CHANGES DON'T RUN** ← This wastes hours of debugging

**CORRECT WORKFLOW for sandbox changes:**

```bash
# Step 1: COMMIT ALL THREE REPOS FIRST
cd ~/pm/qwen-code && git add -A && git commit -m "feat: description"
cd ~/pm/zed && git add -A && git commit -m "feat: description"
cd ~/pm/helix && git add -A && git commit -m "feat: description"

# Step 2: Build Zed binary (if Zed code changed)
./stack build-zed

# Step 3: Build sway container (picks up qwen-code, zed binary, helix changes)
./stack build-sway

# Step 4: VERIFY the build worked
docker images helix-sway --format "{{.Tag}} {{.CreatedAt}}" | head -1
# Tag should match your latest helix commit hash
# CreatedAt should be just now

# Step 5: Start a NEW session (existing containers use old image)
# Ask user to create new agent session to test
```

**Common mistakes that waste hours:**
- ❌ Editing qwen-code but not committing before build-sway
- ❌ Editing zed but not running build-zed before build-sway
- ❌ Assuming existing sandbox containers got updated (they didn't)
- ❌ Not verifying the image tag/timestamp after build

**Signs your changes didn't deploy:**
- Same bug still happens after "fix"
- Console logs you added don't appear
- Image timestamp is old

**ALWAYS verify after building:**
```bash
# Check helix-sway image was just created
docker images helix-sway --format "table {{.Tag}}\t{{.CreatedAt}}" | head -3

# For Wolf changes specifically:
docker run --rm --entrypoint="" helix-sandbox:latest stat /wolf/wolf | grep Modify
```

## 🚨 CRITICAL: NEVER DELETE GIT INDEX LOCK 🚨

**NEVER delete .git/index.lock - it causes git index corruption**

```bash
# ❌ ABSOLUTELY FORBIDDEN: Deleting git lock files
rm .git/index.lock                    # NEVER DO THIS
rm -f .git/index.lock                 # OR THIS
find .git -name "*.lock" -delete      # OR THIS
```

**Why this is forbidden:**
- Lock file exists because another git process is ACTUALLY running
- Deleting it while git process is active corrupts the git index
- Corrupted index requires git fsck or re-cloning repository
- Data loss risk if index is corrupted during commit

**What to do instead:**
1. **Wait for the git process to complete** (usually < 10 seconds)
2. **Check for hung processes:** `ps aux | grep git`
3. **If lock persists:** ASK THE USER FOR HELP - never automate lock removal
4. **Never automate lock deletion** - always investigate why it exists

**Correct approach:**
```bash
# ✅ Wait for existing process
sleep 10
git status  # Will work when lock is released

# ✅ If still locked after waiting, check for processes
ps aux | grep git

# ✅ If you find hung processes: ASK USER FOR HELP
# DO NOT automatically kill processes or remove locks
# User may need to investigate why git is stuck
```

**IMPORTANT: ASK USER FOR HELP if git lock persists**

When encountering `.git/index.lock`:
- ❌ NEVER delete the lock file
- ❌ NEVER kill git processes automatically
- ✅ WAIT for processes to complete (sleep 10-30 seconds)
- ✅ ASK USER if lock persists after waiting
- ✅ Let user decide whether to kill processes or investigate

**NEVER automate git lock removal or process killing.**

## 🚨 CRITICAL: NEVER PUSH TO MAIN WITHOUT PERMISSION 🚨

**NEVER push directly to main branch - use feature/fix branches and get user approval**

```bash
# ❌ ABSOLUTELY FORBIDDEN
git push origin main                   # NEVER DO THIS
git push origin main --force           # DEFINITELY NEVER THIS

# ✅ CORRECT: Use feature or fix branches
git checkout -b fix/descriptive-name
git commit -m "fix: description"
git push origin fix/descriptive-name
# Then ASK USER to review and merge
```

**Why this is forbidden:**
- Main branch has protection rules requiring pull requests
- Direct pushes bypass code review and CI checks
- User needs to approve changes before they go to main
- Mistakes on main affect all developers immediately

**Branch naming conventions:**
- `fix/short-description` - Bug fixes
- `feature/short-description` - New features
- `refactor/short-description` - Code refactoring

**Correct workflow:**
1. Create a `fix/` or `feature/` branch for your changes
2. Commit changes to your branch
3. Push the branch to origin
4. **ASK USER** to review and merge (or create PR)
5. User decides when/how to merge to main

**Exception:** User may explicitly grant permission to push to main for urgent fixes. Always confirm first.

## 🚨 CRITICAL: NEVER AMEND COMMITS ON MAIN 🚨

**NEVER use `git commit --amend` on commits that are on the main branch**

```bash
# ❌ ABSOLUTELY FORBIDDEN on main branch
git commit --amend                     # NEVER DO THIS ON MAIN
git commit --amend -m "new message"    # OR THIS
git commit --amend --no-edit           # OR THIS

# ✅ CORRECT: Create a new commit instead
git commit -m "fix: correct the previous change"
```

**Why this is forbidden:**
- Amending rewrites history, which breaks other developers' work
- Main branch is shared - rewriting it causes merge conflicts for everyone
- User may have already pulled the commit you're amending
- Force push would be required, which is also forbidden

**What to do instead:**
1. **Create a new commit** with the fix
2. **If you made a mistake in a commit message**, create a new commit with a note
3. **If the user asks you to amend**, confirm they understand the implications
4. **Only amend on feature branches** before they're merged to main

**If you accidentally committed to main:**
- DON'T try to fix it with amend or reset
- Create a new commit that corrects the mistake
- Or ASK THE USER how they want to handle it

## 🚨 CRITICAL: NEVER USE git checkout/reset ON ENTIRE DIRECTORY 🚨

**NEVER use `git checkout -- .` or `git reset` with `.` or without specific file paths**

```bash
# ❌ ABSOLUTELY FORBIDDEN: Operations on entire directory
git checkout HEAD -- .                 # DESTROYS ALL UNCOMMITTED CHANGES
git checkout -- .                      # DESTROYS ALL UNCOMMITTED CHANGES
git reset --hard                       # DESTROYS ALL UNCOMMITTED CHANGES
git reset --hard HEAD                  # DESTROYS ALL UNCOMMITTED CHANGES
git clean -fd                          # DELETES ALL UNTRACKED FILES

# ✅ CORRECT: Always specify exact file paths
git checkout HEAD -- path/to/specific/file.tsx
git restore path/to/specific/file.tsx
git checkout -- path/to/specific/file.go
```

**Why this is forbidden:**
- Other agents or the user may have uncommitted work in progress
- You only have visibility into files YOU modified in this session
- Using `.` or omitting paths affects THE ENTIRE REPOSITORY
- Lost uncommitted changes are nearly impossible to recover
- This has caused significant data loss - see `design/2025-12-08-git-checkout-data-loss-incident.md`

**Real incident that caused this rule:**
1. Agent was fixing a single file (MoonlightStreamViewer.tsx)
2. Attempted `git commit --amend` which failed (protected branch)
3. Ran `git reset --soft HEAD~1 && git checkout HEAD -- .` to "recover"
4. The `-- .` reverted ALL uncommitted files in the repo, not just the one being worked on
5. Lost 7 files of uncommitted work from other sessions

**What to do instead:**
1. **Always specify exact file paths** in git checkout/reset commands
2. **Run `git status` first** to see what other changes exist
3. **If you need to discard YOUR changes**, only discard the specific files you modified
4. **If unsure**, ASK THE USER before running any git reset/checkout commands
5. **Never assume** you're the only one with uncommitted changes

**Before any git checkout/reset, ALWAYS:**
```bash
# ✅ Check what uncommitted changes exist
git status

# ✅ If you see files YOU DIDN'T MODIFY, STOP and ask user
# Those are someone else's work in progress!

# ✅ Only then, restore SPECIFIC files you need to change
git checkout HEAD -- frontend/src/components/specific/File.tsx
```

**NEVER use `.` or `--all` or omit paths in git checkout/reset commands.**

## 🚨 CRITICAL: NEVER RENAME CURRENT WORKING DIRECTORY 🚨

**NEVER rename or move your present working directory - it breaks your shell session**

```bash
# ❌ ABSOLUTELY FORBIDDEN: Renaming current directory
mv /home/luke/pm/helix /home/luke/pm/helix-backup    # NEVER DO THIS
mv . ../helix-renamed                                 # OR THIS
# EVEN WITH ABSOLUTE PATHS - if you're in that directory, DON'T RENAME IT
```

**Why this is forbidden:**
- Shell maintains a reference to current working directory by inode
- Renaming the directory breaks the shell's internal state
- All subsequent commands will fail or behave unpredictably
- You cannot cd, cannot run commands, session becomes unusable
- Forces session restart and complete loss of context
- Using absolute paths doesn't help - if you're IN the directory, don't rename it

**What to do instead:**

**ONLY ONE CORRECT APPROACH: ASK THE USER**

If you need to rename a directory:
1. ✅ **ASK THE USER to stop Claude/exit the session**
2. ✅ **Let user rename the directory themselves**
3. ✅ **User restarts Claude in the new location**

**DO NOT attempt to:**
- ❌ Navigate out and rename (still breaks context)
- ❌ Use absolute paths (still breaks if you're in that directory)
- ❌ Automate directory renaming in any way

**ASK USER FOR HELP - Never rename directories yourself.**

## 🚨 CRITICAL: NEVER DELETE SOURCE FILES 🚨

**NEVER delete source code files, even if they have compilation errors**

```bash
# ❌ ABSOLUTELY FORBIDDEN: Deleting source files
rm api/pkg/server/some_handler.go         # NEVER DO THIS
rm -rf frontend/src/components/broken/     # OR THIS
```

**Why this is forbidden:**
- File may be work from another agent running in parallel
- File may be incomplete work in progress that needs fixing
- Compilation errors should be FIXED, not deleted
- You don't own all code - respect other developers' work

**What to do instead:**
1. **If you created the file:** Fix the compilation errors
2. **If another agent created it:** Ask user what to do
3. **If unsure who created it:** Ask user what to do
4. **If blocking your work:** Comment out the problematic code and add a TODO

**Example of correct approach:**
```go
// TODO: Fix compilation errors in this function
// Error: undefined type Foo
// func BrokenFunction() {
//     var x Foo
// }
```

**NEVER assume you can delete someone else's code.**

## 🚨 CRITICAL: COMMIT BEFORE BUILDING SANDBOX IMAGE 🚨

**ALWAYS commit before running `./stack build-sway` or `./stack build-sandbox`**

This applies to BOTH repos:
- **helix** - for sway config, Wolf executor, API changes
- **qwen-code** (at `../qwen-code`) - for Qwen Code tool/agent changes

`./stack build-sway` is faster if you only modified the sway image. `./stack build-sandbox` also builds wolf and moonlight-web-stream.

```bash
# ❌ WRONG: Build without committing
./stack build-sway                    # Changes won't be detected!
./stack build-sandbox                 # Changes won't be detected!

# ✅ CORRECT: Commit in BOTH repos if you changed both
# In helix:
git add -A && git commit -m "changes" && git push

# In qwen-code (if modified):
cd ../qwen-code
git add -A && git commit -m "changes" && git push
cd ../helix

./stack build-sway                    # Now detects all changes
```

**Why:**
- The helix-sway image tag is derived from the helix git commit hash
- Qwen-code rebuild detection compares git commit hashes (uncommitted changes are invisible!)
- Push is required for the version link in the UI to work

## 🚨 CRITICAL: VERIFY DOCKER CACHE BUSTING ON REBUILDS 🚨

**When rebuilding Wolf, Moonlight Web, or sandbox images, VERIFY the cache was actually busted**

Docker BuildKit can cache `FROM` layers even when the referenced image has been rebuilt with the same tag. This caused hours of debugging when a Wolf fix was deployed but the sandbox kept using the old cached binary.

**After running `./stack build-wolf`, `./stack build-moonlight-web`, or `./stack build-sandbox`, check:**

```bash
# Check Wolf binary timestamp in the image
docker run --rm --entrypoint="" wolf:helix-fixed stat /wolf/wolf | grep Modify
docker run --rm --entrypoint="" helix-sandbox:latest stat /wolf/wolf | grep Modify

# Check Moonlight Web binary timestamp
docker run --rm --entrypoint="" helix-moonlight-web:helix-fixed stat /app/web-server | grep Modify
docker run --rm --entrypoint="" helix-sandbox:latest stat /moonlight-web/web-server | grep Modify

# Timestamps should be AFTER your source code changes
# If they're old, the cache wasn't busted properly
```

**Signs the cache wasn't busted:**
- Build output shows `CACHED` for layers that should have rebuilt
- Binary timestamps are older than your source changes
- `COPY . /wolf/` or `COPY . /build/` shows `CACHED` when you changed source files

**The build-sandbox script now passes `--build-arg WOLF_IMAGE_ID=...` and `--build-arg MOONLIGHT_IMAGE_ID=...` to bust the cache**, but always verify when debugging deployment issues.

## 🚨 CRITICAL: NEVER RESTART HUNG PRODUCTION PROCESSES 🚨

**DEBUGGING HUNG PROCESSES IS ALWAYS MORE IMPORTANT THAN QUICK RECOVERY**

When a production process is hung/deadlocked:

```bash
# ✅ CORRECT: Collect debugging info FIRST
# 1. Get process ID
PID=$(docker inspect --format '{{.State.Pid}}' wolf-1)

# 2. Attach GDB and collect thread backtraces
sudo gdb -p $PID
(gdb) thread apply all bt        # Full backtraces of all threads
(gdb) info threads               # Thread states
(gdb) thread <N>                 # Switch to specific thread
(gdb) bt full                    # Full backtrace with local variables
(gdb) p *mutex_ptr               # Examine mutex state
(gdb) detach                     # Detach without killing
(gdb) quit

# 3. Check for deadlock cycles
sudo gdb -p $PID -batch \
  -ex 'thread apply all bt' \
  -ex 'info threads' > /tmp/deadlock-$(date +%Y%m%d-%H%M%S).txt

# 4. Collect system state
cat /proc/$PID/status
ls -la /proc/$PID/task/          # List all threads
cat /proc/$PID/task/*/syscall    # Current syscalls for all threads

# 5. ONLY AFTER collecting all above: Consider restart

# ❌ WRONG: Immediate restart destroys debugging info
docker compose restart wolf      # NEVER DO THIS
docker compose down wolf && docker compose up -d wolf  # OR THIS
```

**Why:** Hung processes contain irreplaceable debugging information:
- Thread backtraces show exact deadlock location
- Mutex states reveal which thread holds which lock
- Memory dumps show corruption patterns
- Syscall states show kernel blocking points

**Restarting destroys ALL of this. You get ONE chance to debug a deadlock. Don't waste it.**

**Production recovery vs debugging:**
- Hung process = debugging opportunity (happens rarely, need data to fix root cause)
- If you restart immediately, the deadlock WILL happen again
- Spending 10 minutes debugging now saves hours of blind debugging later

**Document what you collect:**
```bash
# Save to design/ directory with timestamp
mkdir -p /root/helix/design/
gdb -p $PID -batch \
  -ex 'thread apply all bt full' \
  -ex 'info threads' \
  > /root/helix/design/$(date +%Y-%m-%d)-wolf-deadlock-${PID}.txt
```

## CRITICAL: Fail Fast with Clear Errors

**NEVER write fallback code or silently continue after failures**

```go
// ❌ WRONG: Hiding failures
if err != nil {
    log.Warn().Err(err).Msg("Failed to setup worktree (continuing)")
}

// ✅ CORRECT: Fail fast
if err != nil {
    return fmt.Errorf("failed to setup design docs worktree: %w", err)
}
```

**Why:** Fallbacks hide problems, confuse debugging, waste time. This is customer-facing software.

## CRITICAL: Use Structs, Not Maps

**NEVER use `map[string]interface{}` for API responses**

```go
// ❌ WRONG
response := map[string]interface{}{"status": status}

// ✅ CORRECT
type Response struct { Status string `json:"status"` }
response := &Response{Status: status}
```

**Why:** Type safety, OpenAPI generation, compile-time checks. Place types in `api/pkg/types/`.

## CRITICAL: No Timeouts in Frontend Code

**NEVER use setTimeout/delay for asynchronous operations - use event-driven patterns**

```typescript
// ❌ WRONG: Arbitrary timeout hoping things complete
await new Promise(resolve => setTimeout(resolve, 500))
setShowTestSession(true)

// ✅ CORRECT: Event-driven - wait for actual event
await queryClient.refetchQueries({ queryKey: sessionQueryKey(id) })
setShowTestSession(true)

// ✅ CORRECT: Use component lifecycle hooks
useEffect(() => {
  return () => cleanup() // Runs when component unmounts
}, [])
```

**Why:** Timeouts are unreliable (race conditions, arbitrary delays), hide timing bugs, and make code fragile. Use promises, callbacks, or React lifecycle instead.

**Exception:** Short delays for UI animations (< 100ms) are acceptable if there's no alternative.

## CRITICAL: Extract Components Before Files Get Too Long

**Break up large files BEFORE they become difficult to edit**

```typescript
// ❌ WRONG: 1800-line monolithic component
// SpecTaskKanbanBoard.tsx - 1807 lines, impossible to edit cleanly

// ✅ CORRECT: Extract into focused components
// SpecTaskKanbanBoard.tsx - 200 lines (orchestration only)
// TaskCard.tsx - 150 lines
// DroppableColumn.tsx - 180 lines
// DesignReviewViewer.tsx - 400 lines
```

**When to extract:**
- File exceeds 500 lines → consider extraction
- File exceeds 800 lines → extraction mandatory
- Component has distinct responsibilities → extract immediately

**Why:** LLMs struggle with large files (context limits, edit precision, bug risk). Extract components proactively while code is still manageable.

## Documentation Organization

- **`design/`**: LLM-generated docs, architecture decisions, debugging logs. Format: `YYYY-MM-DD-descriptive-name.md`
- **`docs/`**: User-facing documentation only
- **Root**: Only `README.md`, `CONTRIBUTING.md`, `CLAUDE.md`

## Hot Reloading Stack

Frontend (Vite), API (Air), GPU Runner, Wolf, Zed all support hot reloading. Save files → changes picked up automatically.

## CRITICAL: Always Verify Build Status

**MANDATORY: Ask user to verify page loads BEFORE declaring success**

After ANY frontend code changes, you MUST:

```bash
# 1. Check for HMR update
docker compose -f docker-compose.dev.yaml logs --tail 50 frontend
# Look for: "hmr update" (success) or "error"/"Error" (failure)

# 2. Verify no errors after HMR
docker compose -f docker-compose.dev.yaml logs --since "1m" frontend | grep -i "error"
# Should return nothing. If errors appear, BUILD IS BROKEN.

# 3. ASK USER TO VERIFY
# Tell user: "Please load the page in your browser to verify it renders correctly"
# DO NOT declare success until user confirms
```

**For API changes:**
```bash
docker compose -f docker-compose.dev.yaml logs --tail 30 api
# Look for: "building..." → "running..." (success) or "failed to build" (error)
```

**CRITICAL REQUIREMENTS:**
1. **NEVER declare success without user verification**
2. **NEVER commit frontend code without user confirming page loads**
3. **NEVER commit code with build errors**
4. **Check logs AFTER every file edit**
5. **Compilation/parse errors = broken code = UNACCEPTABLE**
6. **"hmr update" ≠ success** - Must verify: (a) no errors in logs AND (b) user confirms page loads

**Why:** Build logs don't catch all runtime errors. JSX syntax errors, missing imports, and broken conditionals only appear when page actually loads in browser.

## CRITICAL: ACP Architecture - IDE ↔ Agent, NOT Agent ↔ LLM

**ACP (Agent Client Protocol) connects the Agent to the IDE, NOT to the LLM!**

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        HELIX AGENT ARCHITECTURE                         │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│   ┌──────────┐   OpenAI API    ┌─────────────┐    ACP     ┌──────────┐ │
│   │   LLM    │ ←──────────────→│ Qwen Code   │←──────────→│   Zed    │ │
│   │(Claude/  │  function calls │  (Agent)    │  messages  │  (IDE)   │ │
│   │ Qwen3)   │  & responses    │             │  & prompts │          │ │
│   └──────────┘                 └─────────────┘            └──────────┘ │
│                                                                         │
│   LLM handles:                 Agent handles:       IDE handles:        │
│   - Understanding prompts      - Tool execution     - UI rendering      │
│   - Function calling           - State management   - User input        │
│   - Response generation        - LLM communication  - Thread display    │
│                                - ACP communication                      │
└─────────────────────────────────────────────────────────────────────────┘
```

**Common confusion to avoid:**
- ❌ WRONG: "ACP handles tool calls from the LLM"
- ✅ RIGHT: ACP is a protocol between Zed (IDE) and the Agent (Qwen Code)
- ✅ RIGHT: Tool calls happen internally between the Agent and the LLM via OpenAI API

**When debugging agent issues:**
- **Model outputs raw XML in text** → LLM issue (model doesn't understand function calling)
- **Messages not appearing in Zed** → ACP/Zed issue (protocol or UI rendering)
- **Tools failing silently** → Agent issue (Qwen Code's tool execution)

**Key files:**
- `qwen-code/packages/cli/src/` → Qwen Code agent (talks to LLM AND Zed)
- `zed/crates/agent_servers/src/acp.rs` → Zed's ACP client (talks to agent)
- `api/cmd/settings-sync-daemon/` → Configures agent_servers in Zed's settings.json

## Zed Build Process

```bash
# ✅ CORRECT: Use stack script
./stack build-zed        # Dev mode (fast, ~1.3GB)
./stack build-zed release # Release mode (slow, ~2GB)

# ❌ WRONG: Missing feature flag
cargo build --package zed
```

**Kill old builds first:** `pkill -f "cargo build" && pkill -f rustc`

**Hot reload:** Kill builds → Build with stack → Close Zed window → Auto-restart in 2s

## Desktop Container Builds

```bash
./stack build-sway    # Build Sway desktop (Dockerfile.sway-helix)
./stack build-ubuntu  # Build Ubuntu GNOME desktop (Dockerfile.ubuntu-helix)
```

**Which command to use based on files modified:**
- `wolf/sway-config/*` → `./stack build-sway`
- `wolf/ubuntu-config/*`, `Dockerfile.ubuntu-helix` → `./stack build-ubuntu`
- `api/cmd/settings-sync-daemon/*` → Both (if used by both desktops)
- `qwen-code` changes → Both (rebuild whichever desktop you're testing)

Both commands:
1. Build Zed binary if missing (uses existing in dev mode)
2. Build qwen-code using containerized build
3. Build the Docker image tagged as `helix-<name>:latest` and `helix-<name>:<commit-hash>`

**Rebuild when:** modifying desktop config files, Dockerfiles, Go daemons, or qwen-code.
**New sessions use updated image; existing containers don't.**

## Testing & Mocking

```bash
# ✅ Use gomock
mockgen -source api/pkg/external-agent/wolf_client_interface.go \
  -destination api/pkg/external-agent/wolf_client_interface_mocks.go \
  -package external_agent

# ❌ NEVER use testify/mock manually
```

## Key Development Rules

1. **Customer-facing software**: Must work on fresh installs, no manual setup
2. **All builds in containers**: Never check host packages
3. **Foreground builds only**: Never use `run_in_background: true` with builds
4. **One build at a time**: Wait for completion before starting another
5. **Host ≠ Container**: Host=Ubuntu 24.04, containers=Ubuntu 25.04
6. **Trust Docker cache**: NEVER use `--no-cache`
7. **Never clear BuildKit cache**: Cache is reliable; investigate root causes
8. **No unauthorized images**: Never build/push versioned images without permission
9. **Test after every change**: Big-bang approaches impossible to debug
10. **Check logs after changes**: Verify hot reload succeeded

## Enterprise Deployment Context

**Helix is typically deployed on enterprise networks.** Design decisions should account for:

1. **Internal DNS servers**: Enterprises have internal DNS for intranet TLDs and internal services
   - Never hardcode public DNS servers (like 8.8.8.8) as the only option
   - Always inherit DNS configuration from `/etc/resolv.conf` when possible
   - Example: Hydra passes sandbox's DNS servers to container daemons and DNS proxies

2. **Proxy servers**: HTTP/HTTPS proxies are common in enterprise environments
   - Respect `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY` environment variables
   - Container builds should pass through proxy settings

3. **Air-gapped networks**: Some deployments have limited or no internet access
   - All required images should be pullable from configurable registries
   - Don't assume external services are reachable

4. **Private certificate authorities**: Enterprises use internal CAs
   - Support custom CA certificates for TLS verification
   - Never skip certificate verification as a "solution"

5. **Network segmentation**: Services may be on different network segments
   - Don't assume all services can directly reach each other
   - Design for configurable endpoints and routing

## Wolf Development

```bash
./stack build-wolf   # Build Wolf (~30s)
./stack start        # Auto-builds Wolf if missing
```

**Wolf API (from API container only):**
```bash
docker compose -f docker-compose.dev.yaml exec api \
  curl --unix-socket /var/run/wolf/wolf.sock http://localhost/api/v1/apps
```

**Wolf app storage:**
- Static apps (config.toml): Persist across restarts
- Dynamic apps (API created): Cleared on restart
- Restart Wolf to clear broken apps

**Wolf version:** Upstream wolf-ui + auto-pairing PIN support

## Generated TypeScript Client & React Query

**MANDATORY: Use generated client + React Query**

```typescript
// ❌ WRONG
const response = await api.get('/api/v1/spec-tasks/board-settings')

// ✅ CORRECT
const { data } = useQuery({
  queryKey: ['board-settings'],
  queryFn: () => apiClient.v1SpecTasksBoardSettingsList(),
})
```

**CRITICAL: Generated client returns full Axios response**

The OpenAPI-generated client methods return the **complete Axios response object**, not just the data:

```typescript
// ❌ WRONG: Using the full response object
const result = await apiClient.v1WolfHealthList()
// result = {data: {...}, status: 200, headers: {...}, config: {...}}
return result  // Component receives {data: {...}} instead of just {...}

// ✅ CORRECT: Extract .data from response
const result = await apiClient.v1WolfHealthList()
return result.data  // Component receives the actual data object
```

**This is a VERY common bug - always extract `.data` in React Query hooks!**

**Regenerate client:** `./stack update_openapi`

**Required Swagger annotations:**
```go
// @Summary List personal development environments
// @Description Get all personal development environments
// @Tags PersonalDevEnvironments
// @Success 200 {array} PersonalDevEnvironmentResponse
// @Router /api/v1/personal-dev-environments [get]
// @Security ApiKeyAuth
```

**React Query requirements:**
- Use for ALL API calls (queries + mutations)
- Proper query keys for cache management
- Invalidate queries after mutations (standard pattern)
- Handle loading/error states

**React Query mutation pattern** (see `frontend/src/services/questionSetsService.ts`):
```typescript
// ✅ CORRECT: Always use invalidateQueries in onSuccess
export function useUpdateResource(id: string) {
  return useMutation({
    mutationFn: async (request) => {
      const response = await apiClient.v1ResourceUpdate(id, request);
      return response.data;
    },
    onSuccess: () => {
      // Standard React Query pattern: invalidate to refetch
      queryClient.invalidateQueries({ queryKey: resourceQueryKey(id) });
      queryClient.invalidateQueries({ queryKey: resourceListQueryKey() });
    },
  });
}

// ❌ WRONG: Don't use setQueryData (breaks form re-initialization)
onSuccess: (data) => {
  queryClient.setQueryData(key, data);
}
```

**Forms with React Query - Standard Pattern:**
```typescript
// 1. Initialize form from server data (runs on load AND refetch)
useEffect(() => {
  if (data) {
    setName(data.name || '')
    setDescription(data.description || '')
  }
}, [data]) // Dependency on data, not data.id

// 2. Add safety check in save handler
const handleSave = async () => {
  if (!data || !name) return // Don't save uninitialized form
  await updateMutation.mutateAsync({ name, description })
}
```

**Why this works:**
- Form re-initializes after refetch with THE VALUES YOU JUST SAVED
- User sees no change (saved "Foo" → refetch → form shows "Foo")
- Loading guard prevents form rendering until data loads
- Safety check prevents saving empty state

## Frontend Sidebar Pattern

**Use ContextSidebar for consistent navigation across pages**

```typescript
// 1. Create sidebar component (e.g., frontend/src/components/project/ProjectsSidebar.tsx)
import ContextSidebar, { ContextSidebarSection } from '../system/ContextSidebar'

const ProjectsSidebar: FC = () => {
  const router = useRouter()

  const sections: ContextSidebarSection[] = [{
    items: [
      {
        id: 'projects',
        label: 'Projects',
        icon: <Kanban size={18} />,
        isActive: currentView === 'projects',
        onClick: () => navigate('projects')
      }
    ]
  }]

  return <ContextSidebar menuType="projects" sections={sections} />
}

// 2. Register in Layout.tsx getSidebarForRoute()
import ProjectsSidebar from '../components/project/ProjectsSidebar'

function getSidebarForRoute(routeName: string) {
  switch (routeName) {
    case 'projects':
      return <ProjectsSidebar />
    // ...
  }
}

// 3. Enable drawer in router.tsx
{
  name: 'projects',
  path: '/projects',
  meta: { drawer: true }, // Must be true!
}
```

**Never create inline sidebars in page components.** Always use the global drawer + ContextSidebar pattern.

## Frontend UX

**Never use `type="number"`** - Spinners have terrible UX. Use text inputs + `parseInt()`/`parseFloat()`

**Extract reusable components** - Never duplicate complex UI logic

## Wolf Streaming

**Two use cases:**
1. **External Agents (PRIMARY)**: AI agents work autonomously, user connection optional
2. **Personal Dev Environments**: User workspace, connection required

**Testing External Agents:**
1. Navigate to "External Agents" → "Start Session"
2. Send message → verify bidirectional sync
3. Check logs: `docker compose -f docker-compose.dev.yaml logs --tail 50 api`

**Working XFCE config (tested):**
```json
{
  "type": "docker",
  "image": "ghcr.io/games-on-whales/xfce:edge",
  "env": ["GOW_REQUIRED_DEVICES=/dev/input/* /dev/dri/* /dev/nvidia*"],
  "base_create_json": {
    "HostConfig": {
      "IpcMode": "host",
      "CapAdd": ["SYS_ADMIN", "SYS_NICE", "SYS_PTRACE", "NET_RAW", "MKNOD", "NET_ADMIN"],
      "SecurityOpt": ["seccomp=unconfined", "apparmor=unconfined"]
    }
  }
}
```

## 🚨 CRITICAL: Docker Compose restart Does NOT Update Env or Images 🚨

**`docker compose restart` does NOT:**
- Re-read `.env` file changes
- Pull or use new images
- Recreate containers

**It ONLY restarts the existing container with its original configuration.**

```bash
# ❌ WRONG: This does NOT pick up .env changes or new images
docker compose -f docker-compose.dev.yaml restart api
docker compose -f docker-compose.dev.yaml restart sandbox

# ✅ CORRECT: Must down+up to apply .env changes or new images
docker compose -f docker-compose.dev.yaml down api sandbox
docker compose -f docker-compose.dev.yaml up -d api sandbox
```

**When to use each:**
- `restart` - ONLY for bind-mounted file changes (code hot-reload handles this anyway)
- `down` + `up` - For .env changes, image updates, or any config changes

**Always use:** `docker compose -f docker-compose.dev.yaml`

## Database Migrations

**Use GORM AutoMigrate ONLY** - Never create SQL migration files for schema changes

```go
// ✅ CORRECT
type StreamingAccessGrant struct {
    ID        string `gorm:"type:varchar(255);primaryKey"`
    SessionID string `gorm:"type:varchar(255);index;not null"`
}
db.AutoMigrate(&StreamingAccessGrant{})

// ❌ WRONG: SQL migration files for schema changes
```

SQL migrations only for: complex data transformations, one-time cleanup, renaming tables/columns.

## RBAC - AccessGrants System

**ONE unified RBAC: AccessGrants + Roles + RoleBindings**

```go
// ✅ CORRECT
err := apiServer.authorizeUserToResource(ctx, user, orgID, projectID,
  types.ResourceProject, types.ActionUpdate)

// ❌ WRONG: Separate membership tables
type ProjectMembership struct { ... }
```

**Only membership tables:** `OrganizationMembership`, `TeamMembership` (implementation details)

**Adding new resource type:**
1. Add to `types.Resource` constants
2. Create authorization helper in `{resource}_access_grant_handlers.go`
3. Create access grant handlers (list/create/update/delete)
4. Register routes
5. Add Swagger docs
6. Run `./stack update_openapi`
7. Create React Query hooks
8. Implement frontend UI

## General Guidelines

- Never create files unless absolutely necessary
- Prefer editing existing files
- Never proactively create markdown/README files
- **Never use `| tail` or `| head` on long-running commands** - piping to tail/head buffers all output until the command completes, which prevents watching progress. If you need to limit output, use `run_in_background: true` and check with `BashOutput` instead.
