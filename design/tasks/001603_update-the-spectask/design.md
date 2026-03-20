# Design: Update Spectask Prompt with Startup Script Info

## Key Files

- **Prompt template:** `api/pkg/services/spec_task_prompts.go` — `planningPromptTemplate` (line 28)
- **Startup script location:** `/home/retro/work/helix-specs/.helix/startup.sh`
- **Script runner:** `desktop/shared/helix-workspace-setup.sh` (lines 782–810) — runs the startup script with `bash -i` inside the "Helix Setup" terminal
- **Reference to log path:** `desktop/shared/start-zed-core.sh` (line 45) mentions `/tmp/helix-workspace-setup.log` but this file is not actually created

## Startup Script Flow

1. `start-zed-core.sh` launches `helix-workspace-setup.sh` in a terminal window
2. `helix-workspace-setup.sh` clones repos, checks out branches, then runs `/home/retro/work/helix-specs/.helix/startup.sh`
3. The startup script output goes only to the terminal — not to any file the agent can read

## Log File Decision

The user's request references `/path/to/log` as a placeholder. Based on naming conventions in the codebase (e.g., `/tmp/pipewire.log`, `/tmp/waybar.log`, `/tmp/dev-server.log`), the log should be at:

**`/tmp/helix-startup.log`**

The `helix-workspace-setup.sh` needs to tee startup script output to this file. Change the invocation at line 799:

```bash
# Before:
if bash -i "$STARTUP_SCRIPT"; then

# After:
if bash -i "$STARTUP_SCRIPT" 2>&1 | tee /tmp/helix-startup.log; then
```

Note: Using a pipe changes `$?` to the exit code of `tee`, so use `${PIPESTATUS[0]}` to capture the script's actual exit code.

## Prompt Change

Add a new section to `planningPromptTemplate` (in `spec_task_prompts.go`) between the Git workflow section and "Document Your Learnings". The section should be concise:

```markdown
## Startup Script

The project startup script (installs deps, starts dev servers) runs automatically at session start:
- **Location:** `/home/retro/work/helix-specs/.helix/startup.sh`
- **Log:** `cat /tmp/helix-startup.log` (written when the script runs at startup)

If the startup script hasn't run yet, the log won't exist. You can re-run it manually: `bash /home/retro/work/helix-specs/.helix/startup.sh`
```

## Patterns Found

- The planning prompt uses Go `text/template` with `PlanningPromptData` struct
- The template has a `KoditSection` conditional with `{{if .KoditSection}}` — if startup script info needs to be conditional (e.g., only shown when a startup script exists), a similar approach could be used. But since the info is universally useful, a static section is simpler.
- The implementation prompt (`approvalPromptTemplate`) already mentions the startup script at line 865: `"Add commands to /home/retro/work/helix-specs/.helix/startup.sh (runs at sandbox startup)"` — this is the implementation phase; the planning phase currently has nothing.
