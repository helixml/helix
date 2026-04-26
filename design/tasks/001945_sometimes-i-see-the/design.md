# Design

## Approach

Set `GIT_MERGE_AUTOEDIT=no` at the top of `desktop/shared/helix-workspace-setup.sh`. Git honors this env var by skipping the editor on auto-generated merge commit messages and committing them directly. This is the documented mechanism for exactly this case (see `git-merge(1)`, "FAQ").

As a belt-and-suspenders safeguard, also export `GIT_EDITOR=true`. If any future git operation in the script (or anything it sources) tries to launch an editor for a different reason — `git commit` without `-m`, `git rebase -i`, `git tag -a` without `-m`, etc. — the editor will be `/bin/true`, which exits 0 immediately. Git interprets an empty/unchanged message in those contexts as "abort" for some commands and "accept default" for others; both are vastly better than hanging on vim.

## Why not the alternatives

- **`git pull --no-edit`**: Works, but only for the two existing `git pull` lines. Doesn't protect against future git calls added to the script (or to scripts it sources, like `helix-specs-create.sh` and `helix-git-hooks.sh`). The env var is set-and-forget for the whole script process.
- **`git pull --rebase`**: Conflicts with the project's `git config --global pull.rebase false` set by this same script (line 157), and the project explicitly chose merge commits for concurrent agent work.
- **`git config --global core.editor true`**: Persists across the user's other interactive shells in the container — they'd lose their editor in any future manual git work. Env-var scope is limited to the script.
- **Set `EDITOR=true`**: Too broad — would also affect any non-git command in the script that respects `$EDITOR`.

## Where to set it

In `helix-workspace-setup.sh`, right after `set -e` and before any git operation. Both env vars (`GIT_MERGE_AUTOEDIT=no` and `GIT_EDITOR=true`) get exported so they're inherited by sourced scripts (`helix-specs-create.sh`, `helix-git-hooks.sh`) and by the startup script that this one execs at the end.

`GIT_MERGE_AUTOEDIT=no` is the load-bearing fix. `GIT_EDITOR=true` is defense-in-depth.

## What the change looks like

A ~3-line addition near the top of `desktop/shared/helix-workspace-setup.sh`:

```bash
# Prevent any git operation from launching an editor and blocking the session
# on user input (e.g., merge commit message confirmation on a divergent pull).
export GIT_MERGE_AUTOEDIT=no
export GIT_EDITOR=true
```

No other code changes needed. No tests need to change. The two existing `git pull` lines stay as-is.

## Verification

1. Static check: `grep -nE 'git\s+(pull|merge|commit|rebase)' desktop/shared/helix-workspace-setup.sh` — confirm exports come before any git command.
2. Runtime check: deliberately diverge a helix-specs branch (commit locally, push different commits remotely from another agent), start a session, observe that `git pull origin helix-specs` produces a merge commit and the script proceeds without prompting.
3. Regression check: a fast-forward `git pull` should still produce no merge commit (env vars only affect the editor, not the merge strategy).

## Files touched

- `helix/desktop/shared/helix-workspace-setup.sh` — add 2 export lines

## Notes for future agents

- The script is sourced into the desktop container at build time via `./stack build-ubuntu`. Changes here require a `build-ubuntu` and a fresh session to take effect on running stacks.
- The script is **not** the same as `~/work/helix-specs/.helix/startup.sh` (the project startup script visible to the user). The git work happens in the workspace-setup script that runs *before* startup.sh; the user perceives both as "the startup script."
- The CLAUDE.md at the repo root forbids `git pull --rebase` style changes ("NEVER squash merge — always use regular merge commits") — the env-var approach respects this policy.
