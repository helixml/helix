# Design: Stop git from opening vim during agent session startup

## Approach

Single-line change in `desktop/shared/helix-workspace-setup.sh`: export `GIT_MERGE_AUTOEDIT=no` at the top of the script, before any git command runs.

That env var is git's official knob for "use the default merge commit message and do not invoke `$EDITOR`". With it set:

- A clean fast-forward stays a fast-forward.
- A non-fast-forward pull creates the merge commit silently with git's auto-generated message ("Merge branch '…' of …").
- A pull that hits real merge conflicts still fails — git can't auto-resolve content conflicts, and we want that to fail loudly because it's a permanent error the user/operator must investigate.

Existing failure semantics on both pull sites are preserved:

- **Line 382** (`git pull origin "$BASE_BRANCH"`) keeps its `exit 1` on failure. A merge-conflict failure here is a genuine permanent error — git couldn't reconcile the base branch — and the operator needs to see it. We do not want to swallow it and continue with a stale base.
- **Line 473** (`git -C "$WORKTREE_PATH" pull origin helix-specs`) keeps its current "log and continue with local version" path. That handler exists to tolerate transient remote/network failures, not merge conflicts; a real merge conflict here is rare (the script stashes uncommitted changes first) and will be surfaced in the same warning.

## Why not these alternatives

- **`git pull --ff-only`** — refuses any non-fast-forward, even when git could auto-merge cleanly. We *want* automatic merging when it's possible; we just don't want the editor.
- **`git pull --no-edit`** on the explicit pull lines only — works for these two call sites but leaves the door open for any future `git pull` / `git merge` added to the script to re-introduce the bug. The env-var approach is one line and covers the whole script.
- **`git pull --rebase`** — replays local commits on top of origin. The script explicitly sets `pull.rebase false` (line 157) for concurrent-agent reasons; don't fight that.
- **`GIT_EDITOR=true` / `EDITOR=true`** — too broad. Would also no-op interactive `git commit` invocations the user might run in the same shell environment. `GIT_MERGE_AUTOEDIT` is precisely scoped to the merge-message case.
- **Removing line 157 (`pull.rebase false`)** — that config is global and exists for a reason ("merge commits for concurrent agent work"). Out of scope.

## Scope of the env var

`GIT_MERGE_AUTOEDIT=no` is exported into the script's process environment only. It does not affect:

- Interactive shells the user opens later (different processes).
- Other git commands in the script that don't trigger a merge (`git fetch`, `git checkout`, `git stash`, etc.).
- The agent's own git activity inside Zed/Claude (separate process tree).

## Files Touched

- `desktop/shared/helix-workspace-setup.sh` — one `export` line near the top.

## Deployment Notes

This script is `ADD`ed into the desktop image (`Dockerfile.ubuntu-helix:1124`, `Dockerfile.sway-helix:963`). To pick up the fix:

1. Edit the script.
2. `./stack build-ubuntu` to rebake the desktop image.
3. New agent sessions will use the updated script.

Existing running sessions are unaffected (they already booted), which is fine — the bug only manifests at startup.

## Testing

- **Fast-forward case**: clean local base branch, remote ahead by one commit → pull succeeds silently, no editor.
- **Auto-mergeable divergence**: local has a non-conflicting commit on the base branch, origin has advanced → pull creates a merge commit silently using the default message, no editor, startup proceeds.
- **Merge-conflict case**: local has a commit that conflicts with origin's advance → pull fails (hard), startup exits with the existing FATAL message. This is the intended permanent-error behaviour.
- Verify by starting a fresh agent session end-to-end and confirming the workspace setup terminal completes without prompting.

## Notes for Future Agents

- The script is invoked by `desktop/shared/start-zed-core.sh` which launches it in a kitty/gnome-terminal so the user sees output. That's why a vim popup is so user-visible — it's literally in front of the user.
- The script has its own `cleanup_and_prompt` trap that catches *script* failures and offers a debug shell. That's separate from the git editor issue — the editor opens *during* execution, before any trap fires.
- `git config --global pull.rebase false` (line 157) is intentional; don't try to "fix" it here.
- `GIT_MERGE_AUTOEDIT` has been the documented git knob for this since git 1.7 — stable and safe to rely on.

## Implementation Notes

- **TTY-gated bug**: confirmed via local repro that git only opens the editor on `git pull` when stdin is a TTY (and `GIT_MERGE_AUTOEDIT` is unset). In non-TTY contexts (e.g., piped/CI), git silently uses the default message even without the env var. That's why the bug surfaces in the agent session — the script runs inside a kitty terminal — but doesn't show up when running the script piped through `tee` or under `bash -c`.
- **Verified locally** with a throwaway git repo + `script(1)` to fake a TTY:
  - Auto-mergeable divergence + `GIT_MERGE_AUTOEDIT=no` → silent merge commit, no editor (sentinel `GIT_EDITOR=false` did not trip).
  - Conflicting divergence + `GIT_MERGE_AUTOEDIT=no` → "Automatic merge failed; fix conflicts" → git exits non-zero, which the script's existing `|| { echo FATAL; exit 1; }` catches.
- The actual desktop-image rebuild (`./stack build-ubuntu`) and live agent-session verification are deployment/QA steps left to the reviewer — the script change itself is already proven by the local repro above.
