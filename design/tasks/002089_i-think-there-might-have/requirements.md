# Requirements: Reinstate Role/Identity File Republish on Worker Activation

## Background

Every helix-org AI Worker is told, at activation time, that the org-wide
policy, its role, and its identity files live on the **helix-specs**
branch of its per-Worker repo. The activation prompt
(`DefaultHelixSpecsMandate` in
`api/pkg/org/infrastructure/runtime/helix/spawner.go:327`) states:

> helix-org pushes them on hire and re-pushes them on every activation,
> so the remote always has the current owner-edited version.

This contract is **broken**. The code only pushes these files on the
*first* `WorkerProject.Ensure` call (when no project exists yet) and on
explicit `update_role` / `update_identity` MCP-tool invocations (via
`Workspace.MirrorFile`). On every subsequent activation the fast path in
`WorkerProject.Ensure` (`api/pkg/org/infrastructure/runtime/helix/project.go:207-234`)
short-circuits before `republishWorkerFiles` runs.

## What got removed and why

The "republish on every activation" behaviour was added in
commit `4a6cb33c51` — *"feat: republish role/identity on every activation +
always git-pull in agent"* (Phil Winder, 2026-05-02). That commit was
written specifically to make `update_role` live-edits propagate to
already-spawned Workers without a fire+re-hire round trip, and was
validated end-to-end against `demos/getting-started`.

It was removed in commit `4f7837ac0c` — *"fix(api/org/runtime/helix):
settings auto-apply to existing workers"* (Phil Winder, 2026-06-03). The
commit message says:

> Files (role.md / identity.md / agent.md) are still NOT republished on
> the fast path because that would clobber external edits on the
> helix-specs branch (canonical-content updates flow through
> Workspace.MirrorFile instead).

The intent of `4f7837ac0c` was to plug a *different* hole — making the
fast path re-call `ApplyProject` so runtime / provider / model / credentials
edits made via the Settings UI propagate. Dropping the file-republish was
a deliberate side decision, motivated by a "could clobber external edits"
worry rather than a measured concern: the canonical content lives in the
DB (`Role.Content`, `Worker.IdentityContent`), and every legitimate edit
path *already* goes through `MirrorFile`. There is no documented case of
an "external edit" the system actually wants to preserve.

Net effect: the activation prompt promises an invariant
(`role.md` / `identity.md` / `agent.md` always reflect the DB) that the
code no longer maintains.

## User Stories

1. **As an operator**, when I edit a Worker's role or identity in the DB
   (or via any path that does not go through the `update_role` /
   `update_identity` MCP tools), the next activation of that Worker
   should publish the current canonical content to the helix-specs
   branch — without me having to fire and re-hire.

2. **As an agent inside a Worker desktop**, when I `git pull origin
   helix-specs` and `cat workers/$HELIX_WORKER_ID/.context/role.md` as
   the activation prompt instructs, I should see the current DB content
   — even if the file was missing, stale, or hand-edited on the branch
   since the last activation.

3. **As an operator recovering from a corrupted helix-specs branch**
   (e.g. someone force-pushed, an agent checkpoint dropped the file,
   the file was deleted by a `git rm`), the next activation should
   restore the canonical files without manual intervention.

## Acceptance Criteria

- `WorkerProject.Ensure`'s fast path re-publishes `agent.md`,
  `role.md`, and `identity.md` from the DB on every activation, in
  addition to the spec re-apply behaviour added in `4f7837ac0c`.
- The settings auto-apply behaviour from `4f7837ac0c` is preserved —
  `ApplyProject` is still called on every activation so worker runtime
  / provider / model / credentials drift propagates.
- The existing test
  `TestEnsureWithPersistedProjectFastPaths`
  (`api/pkg/org/infrastructure/runtime/helix/project_test.go:381`) is
  updated to assert the new contract: files **must** be republished on
  the fast path. The "fast path must not republish role.md" assertion
  (line 423-424) flips to "fast path MUST republish role.md".
- A new test pins the live-edit propagation path end-to-end: write a
  Worker with role v1, persist a project, then call `Ensure` again with
  the Worker's role updated to v2 in the DB — the next `Ensure` must
  push v2 to `workers/<id>/.context/role.md`.
- The `DefaultHelixSpecsMandate` prompt text stays unchanged. After the
  fix it is *accurate* (it already promises this behaviour).
- `repoLocks` in `Workspace` serialises pushes per-repo so the
  per-activation re-push does not race against a concurrent
  `MirrorFile` triggered by a same-tick `update_role`. (Already in
  place — verify it covers the new caller.)

## Out of Scope

- Changing the on-branch path layout (`workers/<id>/.context/...`).
- Reading the remote first to skip no-op pushes. The git servicer's
  `CreateOrUpdateFileContents` is already cheap and idempotent — the
  original commit measured the cost as "two HTTP calls per activation"
  and accepted it.
- Re-pushing arbitrary worker files. Only the three canonical files
  (`agent.md`, `role.md`, `identity.md`) are in scope.
- Changes to the spec-task agent's planning prompt (the one rendered
  by `BuildPlanningPrompt`) — that prompt does **not** mention role /
  identity files and is unaffected.
