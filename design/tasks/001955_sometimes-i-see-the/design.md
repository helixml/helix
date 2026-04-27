# Design: Stop git from opening vim during agent session startup

## Approach

Two complementary changes to `desktop/shared/helix-workspace-setup.sh`:

### 1. Belt: change pull semantics to `--ff-only`

Replace the two unguarded `git pull` calls with `git pull --ff-only`. A fast-forward never produces a merge commit, so git never invokes the editor.

- **Line 382** — base-branch pull during "new branch" mode. With `--ff-only`, divergence becomes an error rather than a silent merge. Wrap the call so we **log a warning and continue** (don't `exit 1`) — the agent's working branch will still be created from whatever local state exists, and a stale base is far less harmful than a failed startup.
- **Line 473** — `helix-specs` worktree pull. The current code already handles failure gracefully (logs and continues with local version). `--ff-only` slots in cleanly without changing that contract.

### 2. Braces: belt-and-braces with `GIT_MERGE_AUTOEDIT=no`

Export `GIT_MERGE_AUTOEDIT=no` once at the top of the script (right after `set -e`). This is git's official knob for "use the default merge message without launching an editor". Even if a future code path adds another `git pull`/`git merge`, it cannot block startup on a vim prompt.

`GIT_MERGE_AUTOEDIT=no` is *scoped to the script's environment* — it does not affect interactive git use the agent does later in its shell, because that runs in a different process tree.

## Why not these alternatives

- **`git pull --no-edit`** — accepts the default merge commit message silently. Avoids the editor, but creates surprise merge commits on the agent's local base branch every time the agent has been running long enough for `origin/main` to advance. Hides drift instead of surfacing it.
- **`git pull --rebase`** — would replay any local commits on top of origin. The agent shouldn't have local commits on the base branch; if it does, rebasing them silently is worse than logging and skipping. Also conflicts with line 157 (`pull.rebase false`) which is set deliberately for concurrent agent work.
- **`GIT_EDITOR=true` / `EDITOR=true`** — too broad. Would also no-op interactive `git commit` invocations the user might run later in the same shell environment.
- **Removing line 157 (`pull.rebase false`)** — that config is global and exists for a reason (the comment says "merge commits for concurrent agent work"). Don't touch it as part of this fix.

## Key Decision: warn-and-continue on FF failure for BASE_BRANCH

The current line 382 calls `exit 1` if `git pull` fails. With `--ff-only`, divergence will trigger that exit, breaking startup for users who have any local commits on the base branch (rare but possible).

We change this to: log a clear warning explaining the divergence, then proceed. Rationale:
- The agent immediately branches off `BASE_BRANCH` (lines 388–401), so a slightly-stale base is harmless.
- A blocked startup is exactly the user-facing symptom we're trying to fix.
- The warning gives the operator a paper trail if it matters.

## Files Touched

- `desktop/shared/helix-workspace-setup.sh` — the only file changed.

## Deployment Notes

This script is `ADD`ed into the desktop image (`Dockerfile.ubuntu-helix:1124`, `Dockerfile.sway-helix:963`). To pick up the fix:

1. Edit the script.
2. `./stack build-ubuntu` to rebake the desktop image.
3. New agent sessions will use the updated script.

Existing running sessions are unaffected (they already booted), which is fine — the bug only manifests at startup.

## Testing

- Test fast-forward case: clean local base branch, remote ahead by one commit → pull succeeds silently, no editor.
- Test divergence case: create a local commit on the base branch, set remote ahead → script logs warning, proceeds, does not open editor, does not exit.
- Test the `helix-specs` worktree case: same divergence setup against the worktree → warning logged, local version retained.
- Verify by starting a fresh agent session end-to-end and confirming the workspace setup terminal completes without prompting.

## Notes for Future Agents

- The script is invoked by `desktop/shared/start-zed-core.sh` which launches it in a kitty/gnome-terminal so the user sees output. That's why a vim popup is so user-visible — it's literally in front of the user.
- The script has its own `cleanup_and_prompt` trap that catches *script* failures and offers a debug shell. That's separate from the git editor issue — the editor opens *during* execution, before any trap fires.
- `git config --global pull.rebase false` (line 157) is intentional; don't try to "fix" it here.
