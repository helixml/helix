# Design: Reinstate Role/Identity File Republish on Worker Activation

## Summary

Move the `republishWorkerFiles` call in
`api/pkg/org/infrastructure/runtime/helix/project.go` so it runs on
**both** the fast path (project already exists) **and** the slow path
(fresh apply). Keep the spec re-apply behaviour added in `4f7837ac0c`.
Flip the one test assertion that pins "do not republish on the fast
path" to its opposite, and add a regression test for live-edit
propagation.

## Affected Files

| File | Change |
|---|---|
| `api/pkg/org/infrastructure/runtime/helix/project.go` | Call `republishWorkerFiles` in the fast-path branch before returning the persisted IDs. |
| `api/pkg/org/infrastructure/runtime/helix/project_test.go` | Update `TestEnsureWithPersistedProjectFastPaths` to assert files **are** republished on the fast path; add `TestEnsureFastPathPropagatesRoleEdits`. |

No other code changes. The Workspace, MirrorFile, and Spawner code are
already correct.

## Code Change

Today (`project.go:207-234`, abbreviated):

```go
if state.ProjectID != "" {
    if _, err := a.Service.GetProject(ctx, state.ProjectID); err != nil {
        // ... stale-state cleanup, fall through to fresh apply ...
    } else {
        // Fast path: project exists.
        // Re-apply spec for settings drift.
        if _, err := a.Service.ApplyProject(ctx, applyReq); err != nil {
            return "", "", "", fmt.Errorf("refresh project spec for %s: %w", workerID, err)
        }
        return state.ProjectID, state.AgentAppID, state.RepoID, nil
    }
}
```

After:

```go
if state.ProjectID != "" {
    if _, err := a.Service.GetProject(ctx, state.ProjectID); err != nil {
        // ... stale-state cleanup, fall through to fresh apply ...
    } else {
        // Fast path: project exists.
        // Re-apply spec for settings drift.
        if _, err := a.Service.ApplyProject(ctx, applyReq); err != nil {
            return "", "", "", fmt.Errorf("refresh project spec for %s: %w", workerID, err)
        }
        // Re-publish canonical files so DB edits to role/identity/agent.md
        // propagate to the helix-specs branch on every activation —
        // matches the contract in DefaultHelixSpecsMandate.
        a.republishWorkerFiles(ctx, workerID, state.RepoID, roleContent, worker.IdentityContent())
        return state.ProjectID, state.AgentAppID, state.RepoID, nil
    }
}
```

`republishWorkerFiles` is unchanged. It already:
- No-ops when `repoID == ""` or `Workspace == nil`.
- Calls `EnsureBranch` (idempotent — `CreateBranch` returns 200 on
  pre-existing branches).
- Logs warnings on per-file errors but never fails the activation
  (matches the existing first-apply call site).

## Why this is safe

1. **Idempotent operations.** `EnsureBranch` and the underlying
   `CreateOrUpdateFileContents` are both upsert-shaped. Re-pushing the
   same content produces no commit (`CreateOrUpdateFileContents` is a
   PUT-with-same-content no-op in the git servicer); re-pushing changed
   content is exactly what we want.
2. **Per-repo serialisation already in place.** `Workspace.lockFor`
   (`api/pkg/org/infrastructure/runtime/helix/workspace.go:124`) takes
   a per-repo mutex on every write. A simultaneous `MirrorFile` from a
   tool call and a re-push from `Ensure` will serialise rather than
   race on the helix-side working copy.
3. **DB is the source of truth.** `roleContent` and
   `worker.IdentityContent()` are loaded from the store on every
   `Ensure` (project.go:156-170). Republishing them just makes the
   branch reflect the DB, which is precisely what the activation
   prompt promises.
4. **"Could clobber external edits" concern, examined.** The only
   writers of these files outside `republishWorkerFiles` are
   `MirrorFile` (called by `update_role` / `update_identity` tools,
   which themselves persist the new content to the DB *before* the
   push). So a republish that lands after a `MirrorFile` push reads
   the same DB content and produces a no-op. There is no documented
   producer of "external edits to role.md on the helix-specs branch
   that the DB doesn't know about", and the existing fast-path
   `ApplyProject` call already clobbers external edits to the Helix
   *project spec* without complaint.
5. **Cost.** The original commit `4a6cb33c5` measured this as two HTTP
   calls per activation (CreateBranch + PutFile). Activations are
   bursty rather than steady, and `Ensure` already does multiple HTTP
   calls (`GetProject`, `ApplyProject`). The marginal cost is
   negligible.

## Why this is not the wrong fix

We considered, and rejected, three alternatives:

1. **Update the prompt to match the code** — i.e. rewrite
   `DefaultHelixSpecsMandate` to drop the "re-pushes on every
   activation" promise. Rejected: this would silently regress the
   live-edit UX (the explicit goal of `4a6cb33c5`), and operators
   reasonably expect DB-state to drive on-branch state.
2. **Read remote before push, skip on no-op** — fetch the file from
   helix-specs and only push when content differs. Rejected: more
   round-trips, more code, and the underlying servicer already
   short-circuits same-content writes.
3. **Republish only when DB content has changed since last
   activation** — track a `LastPublished{Role,Identity}Hash` in
   `WorkerRuntimeState`, push only on mismatch. Rejected as
   over-engineering for the same end state — the unconditional push
   is already cheap.

## Activation sequence (after fix)

1. Spawner receives trigger → `cfg.ensureProject(actCtx, ...)` →
   `WorkerProject.Ensure(...)`.
2. `Ensure` loads `Worker` and `WorkerRuntimeState` from the DB.
3. If `state.ProjectID == ""` (slow path):
   - `ApplyProject`, create repo, attach repo, **republish files**,
     `SaveProject`. (Unchanged.)
4. If `state.ProjectID != ""` and `GetProject` succeeds (fast path):
   - `ApplyProject` to refresh spec drift. (Unchanged.)
   - **`republishWorkerFiles` to refresh role/identity/agent.md.** (New.)
   - Return persisted IDs.
5. Spawner proceeds to `ensureHelixOrgMCP`, `ensureSession`,
   `pollUntilDone`. (Unchanged.)

## Test Plan

### Updated test: `TestEnsureWithPersistedProjectFastPaths`

Today (`project_test.go:418-425`):

```go
if atomic.LoadInt32(&git.branchCalls) != 0 {
    t.Errorf("fast path must not create-branch; got %d", ...)
}
git.mu.Lock()
defer git.mu.Unlock()
if _, ok := git.putFileByPath["workers/w-eng/.context/role.md"]; ok {
    t.Errorf("fast path must not republish role.md (would clobber external edits)")
}
```

After:

```go
if atomic.LoadInt32(&git.branchCalls) == 0 {
    t.Errorf("fast path MUST ensure-branch before republish; got 0")
}
git.mu.Lock()
defer git.mu.Unlock()
if got := git.putFileByPath["workers/w-eng/.context/role.md"]; got != "# Role v1" {
    t.Errorf("fast path MUST republish role.md from DB; got %q", got)
}
```

(Test setup at line 383 already seeds the role with `"# Role v1"`.)

### New test: `TestEnsureFastPathPropagatesRoleEdits`

Pins the live-edit propagation behaviour the original commit
`4a6cb33c5` validated end-to-end:

1. Build store with Worker `w-eng`, role `r-eng` content `"# Role v1"`.
2. Persist project state (`SaveProject` with stubbed IDs).
3. Call `Ensure` once — expect `putFileByPath["workers/w-eng/.context/role.md"] == "# Role v1"`.
4. Update role in store to `"# Role v2"`.
5. Reset `git.putFileByPath`.
6. Call `Ensure` again — expect `putFileByPath["workers/w-eng/.context/role.md"] == "# Role v2"`.

### Existing tests that should keep passing without modification

- `TestEnsureFastPathRefreshesAgentSpec` (project_test.go:439): pins
  the spec-refresh behaviour from `4f7837ac0c`. Republish doesn't
  affect `ApplyProject` call count.
- `TestEnsureRolePropagatesFromFirstPosition` (line 577): slow-path
  publish, unaffected.
- `TestEnsureSkipsRolePushIfRoleMissing` (line 595): empty role
  content is skipped by `republishWorkerFiles` (guard at project.go:331).
- `TestEnsureLogsButDoesNotFailOnPutFileError` (line 620): error
  handling in `republishWorkerFiles` already logs and continues.

### Manual / staging validation

Repro the scenario from the original commit `4a6cb33c5`:

1. Boot helix-org dev stack.
2. Hire a Worker against `r-echo` (role content: "echo: $BODY").
3. Activate and verify the Worker responds with `echo: hello`.
4. Call `update_role r-echo` with body `"loud: <BODY UPPERCASED>"`.
5. Activate again and verify the Worker responds with `loud: HELLO`.
6. Then: edit the role *directly in the DB* (bypassing `update_role`,
   to simulate the gap this fix closes). E.g.
   `UPDATE roles SET content = '...' WHERE id = 'r-echo';`.
7. Activate again — the next activation's `Ensure` must repush the new
   role to helix-specs, and the Worker must respond with the new
   behaviour.

## Notes for the implementer

- `republishWorkerFiles` lives at `project.go:312` and takes
  `(ctx, workerID, repoID, roleContent, identityContent)`. The fast
  path already has `roleContent` in scope (loaded at line 164) and
  `worker.IdentityContent()` is on the `worker` variable loaded at
  line 156. Use `state.RepoID` for the repo (the fast path returns it
  on line 233).
- Do **not** move `republishWorkerFiles` outside the fast-path `else`
  block. The stale-state branch (`errors.Is(err, ErrProjectNotFound)`)
  falls through to the slow path, which already calls
  `republishWorkerFiles` on line 290 — running it twice would be
  wasted writes (idempotent but pointless).
- The comment block at `project.go:223-229` ("We do NOT re-push
  canonical files…") must be deleted or rewritten — leaving it in
  place after the behaviour flips will confuse the next reader.
- `Workspace.MirrorFile` invalidates the warm session on `role.md` /
  `identity.md` writes (workspace.go:114-120) so the next activation
  re-reads from a fresh Claude context. The new fast-path republish
  does **not** invalidate the session — it can't, because by the time
  `Ensure` runs we're already inside the activation; the agent will
  read the freshly-published file on its next `git pull`. This is the
  correct behaviour.

## Risk

Low. The change is one call site, the function being called already
exists and is exercised by the slow path, the operations are
idempotent, per-repo serialisation is already in place, and the
behaviour matches what the user-facing prompt already promises.

The one test assertion that needs to flip is the *only* place in the
code that codifies "do not republish on fast path" — there are no
other defenders of that contract to update.
