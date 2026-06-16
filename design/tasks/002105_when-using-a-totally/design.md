# Design: Ensure 'main' Is Pushed Before 'helix-specs' on New GitHub Repos

## Summary

The bug is fixed primarily in the **shell script that actually creates and
pushes both `main` and `helix-specs` for an empty repo**:
`desktop/shared/helix-specs-create.sh` → `create_helix_specs_branch`.

That function already tries to seed the default branch before `helix-specs`
(added in commit `ee00cc926`, "seed default branch on empty repos before
helix-specs"). The remaining gap is that the seed step is **best-effort**: if
seeding the default branch fails or is skipped, the function *still* pushes the
`helix-specs` orphan, which then becomes the only branch on the empty upstream
and GitHub promotes it to default. We make the default-branch seed
**authoritative** so `helix-specs` is never the first branch on an empty repo.

## The shell script that pushes both branches

`desktop/shared/helix-specs-create.sh`, `create_helix_specs_branch()`:

```bash
# Detect empty repo
local REPO_IS_EMPTY=false
if ! git -C "$REPO_PATH" rev-parse HEAD >/dev/null 2>&1; then
    REPO_IS_EMPTY=true
fi

local RETURN_BRANCH="${CURRENT_BRANCH:-$REPO_DEFAULT_BRANCH}"

# Seed default branch FIRST so the upstream adopts it as default
if [ "$REPO_IS_EMPTY" = true ]; then
    if ! git -C "$REPO_PATH" show-ref --verify "refs/heads/$RETURN_BRANCH" ...; then
        git -C "$REPO_PATH" checkout --orphan "$RETURN_BRANCH" ...
        git -C "$REPO_PATH" commit --allow-empty -m "Initial commit" ...
        if git -C "$REPO_PATH" push -u origin "$RETURN_BRANCH" ...; then
            echo "  Seeded $RETURN_BRANCH on upstream as default"
        else
            echo "  Warning: failed to seed $RETURN_BRANCH on upstream"   # <-- bug: continues anyway
        fi
    fi
fi

# Then create + push the helix-specs orphan
git -C "$REPO_PATH" checkout --orphan helix-specs ...
git -C "$REPO_PATH" commit --allow-empty -m "Initialize helix-specs branch" ...
git -C "$REPO_PATH" push origin helix-specs ...                            # <-- runs even if seed failed
```

This is also where the desktop session-startup path lands: `start-zed-core.sh`
launches `helix-workspace-setup.sh`, which `source`s `helix-specs-create.sh` and
calls `create_helix_specs_branch` for the primary repo (after its own empty-repo
init block).

## Root cause (shell-script level)

For a brand-new empty upstream, GitHub makes the **first branch pushed** the
default. `create_helix_specs_branch` intends to push the default branch first,
but the seed is not enforced:

1. **Seed push not enforced.** If `git push -u origin "$RETURN_BRANCH"` fails
   (transient error, perms, internal→external forwarding hiccup), the code only
   logs a warning and proceeds to push `helix-specs`. The empty upstream then
   has `helix-specs` as its only/first branch → default = `helix-specs`.
2. **`RETURN_BRANCH` can be wrong/empty.** It is `${CURRENT_BRANCH:-$REPO_DEFAULT_BRANCH}`.
   On a freshly cloned empty repo `CURRENT_BRANCH` is whatever the client's
   `init.defaultBranch` produced (could be `master`, or empty), so the seeded
   default may not be the intended `main`.
3. **Redundant second handler.** `helix-workspace-setup.sh` also has an
   empty-repo init (~line 324) that pushes `main` (with a README) before calling
   `create_helix_specs_branch`. When it runs, the repo is no longer empty by the
   time the function runs, so the function's own (fixed) seeding is bypassed —
   meaning the function's seeding is only exercised in the standalone/edge path,
   exactly where the gap above bites.

## Fix

### Primary — make the default-branch seed authoritative (helix-specs-create.sh)

In `create_helix_specs_branch`, for the empty-repo case:

1. **Normalize `RETURN_BRANCH`** to a sensible default before seeding: prefer
   the detected upstream default, else `main` (fall back to `master` only if
   that is clearly the repo's convention). Never let it be empty.
2. **Gate the helix-specs push on a successful seed.** If the default-branch
   seed push does not succeed for an empty repo, do **not** push `helix-specs`.
   Instead retry, or skip helix-specs creation and surface a clear error — so we
   never leave `helix-specs` as the first/only branch on the upstream.
3. **(Optional) Verify** the upstream default after seeding (e.g.
   `git ls-remote --symref origin HEAD`) and log/fail if it resolved to
   `helix-specs`.

This keeps the change localized to the one function that pushes both branches,
matches the existing approach in commit `ee00cc926`, and needs no new
dependencies.

#### Concrete change to `create_helix_specs_branch` (illustrative)

```bash
local SEED_OK=true
if [ "$REPO_IS_EMPTY" = true ]; then
    # (2) Never seed an empty/branchless value; force a sensible default.
    [ -z "$RETURN_BRANCH" ] && RETURN_BRANCH="main"

    if ! git -C "$REPO_PATH" show-ref --verify "refs/heads/$RETURN_BRANCH" >/dev/null 2>&1; then
        git -C "$REPO_PATH" checkout --orphan "$RETURN_BRANCH" 2>&1 || true
        git -C "$REPO_PATH" commit --allow-empty -m "Initial commit" 2>&1 || true
        if git -C "$REPO_PATH" push -u origin "$RETURN_BRANCH" 2>&1; then
            echo "  Seeded $RETURN_BRANCH on upstream as default"
        else
            # (1) Was: log a warning and fall through to push helix-specs anyway.
            echo "  ERROR: failed to seed $RETURN_BRANCH; refusing to push helix-specs first"
            SEED_OK=false
        fi
    fi
fi

# (1) Gate: on an empty repo, never push helix-specs unless the default
#     branch was seeded first. This is the one-line behavioural fix.
if [ "$REPO_IS_EMPTY" = true ] && [ "$SEED_OK" = false ]; then
    HELIX_SPECS_BRANCH_EXISTS=false
    return 1
fi

# ... unchanged: create + push the helix-specs orphan ...
```

The single behavioural change is that the empty-repo path **stops** before
pushing `helix-specs` whenever the default-branch seed did not succeed, so
`helix-specs` can never be the first branch GitHub sees.

### Cleanup — remove the redundant empty-repo init (helix-workspace-setup.sh)

Consolidate empty-repo initialization so there is a single source of truth.
Either remove the `helix-workspace-setup.sh` empty-repo block (~line 324) and
rely on `create_helix_specs_branch`'s now-authoritative seeding, or keep it but
ensure it pushes `main` first and is consistent with the function. Removing the
duplicate avoids the "README Initial commit" noise and the masking interaction
described above. (Behaviour-preserving cleanup — verify with the test harness.)

No backend (Go) changes are required. The shell setup pushes `main` and
`helix-specs` as separate `git push` operations, so once the script guarantees
`main` is pushed first, it reaches the upstream first. The fix lives entirely in
the shell script.

## Key decisions

- **Decision:** Fix in `helix-specs-create.sh` (`create_helix_specs_branch`), the
  script that pushes both branches for empty repos. **Rationale:** it is the
  real, in-flow location; matches the existing `ee00cc926` approach; smallest
  correct change.
- **Decision:** Make the default-branch seed a hard precondition for pushing
  `helix-specs`. **Rationale:** the current best-effort seed is exactly why the
  bug still occurs; never allow helix-specs to be the first branch on an empty
  upstream.
- **Decision:** Fix entirely in the shell script; no backend (Go) changes.
  **Rationale:** the shell script is what pushes both branches for an empty repo
  and is the real lever; separate pushes mean ordering there is sufficient.

## Risks / gotchas

- Don't break the non-empty path — only the empty-repo branch changes.
- Preserve the existing return-to-original-branch / stash-restore behaviour.
- `master`-convention repos must still work; normalization must not force `main`
  when the upstream genuinely uses `master`.
- The internal→external forwarding means a "successful" local push still relies
  on forwarding succeeding; the seed-gate (do not push helix-specs unless the
  default-branch push succeeded) is what protects against the empty upstream
  ever defaulting to `helix-specs`.

## Implementation Notes

- **Files changed (helix repo, branch `feature/002105-ensure-main-is-pushed`):**
  - `desktop/shared/helix-specs-create.sh` — in `create_helix_specs_branch`:
    (1) normalize `RETURN_BRANCH` to `main` when it would otherwise be empty;
    (2) track `SEED_OK` and, for empty repos, **bail out (`return 1`, set
    `HELIX_SPECS_BRANCH_EXISTS=false`, restore stash) before touching
    helix-specs** if the default-branch seed push fails; (3) after a successful
    helix-specs push on an empty repo, verify `git ls-remote --symref origin
    HEAD` did not resolve to `helix-specs` and warn if it did.
  - `desktop/shared/helix-workspace-setup.sh` — the empty-repo init block
    (~line 350) now **hard-fails (`exit 1`)** if `git push -u origin main`
    fails, instead of logging and continuing.
  - `desktop/shared/test-helix-specs-creation.sh` — added a remote-default-branch
    assertion to every `run_test` case, plus a new "Empty repo with failing seed
    push" test.

- **Key discovery — do NOT remove the `helix-workspace-setup.sh` empty-repo init
  block.** The later "Checkout correct branch" section (`new` branch mode) runs
  `git checkout "$BASE_BRANCH"` + `git pull origin "$BASE_BRANCH"` and FATALs if
  `main` does not yet exist on the upstream. The init block is what seeds `main`
  first, so it must stay. Both empty-repo handlers now share one rule: the
  default branch MUST land on the upstream first, or we stop.

- **Local-test caveat:** a `git init --bare` remote does not auto-promote the
  first pushed branch to default the way GitHub does (its `HEAD` is fixed at
  init time, here `master`). The test's `git worktree add helix-specs` check and
  the new seed-failure test are the meaningful regression guards; the
  remote-`HEAD`-not-helix-specs assertion documents intent and catches an
  explicit misconfiguration.

- **Test result:** `bash desktop/shared/test-helix-specs-creation.sh` → 12
  passed, 0 failed.

## Test plan

- Extend `desktop/shared/test-helix-specs-creation.sh`:
  - Empty repo → assert remote default branch is **not** `helix-specs` (e.g.
    inspect `git symbolic-ref refs/remotes/origin/HEAD` / `ls-remote --symref`)
    and that `git worktree add helix-specs` succeeds (existing assertion).
  - Empty repo where the seed push is forced to fail → assert `helix-specs` is
    **not** created/pushed (no helix-specs-as-default).
- Run the existing 11-case suite to confirm no regression.
- Manual: connect a brand new empty GitHub repo, run setup repeatedly, confirm
  the GitHub default branch is `main` every time.
