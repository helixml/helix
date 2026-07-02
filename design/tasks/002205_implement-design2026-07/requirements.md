# Requirements: Project VCS Connection Lozenge & Loud Push-Failure Surfacing

## Background

Spec-task pushes to external repos can fail **silently**. The internal git
server returns HTTP 200 to the git client *before* mirroring to the external
provider (`git_http_server.go:614-617`). When the external push fails, refs are
rolled back (`rollbackBranchRefs`, `git_http_server.go:775-805`) but the client
already got its 200 — so the agent reports "pushed and ready for review", the UI
shows nothing, and the user is left guessing.

Root cause of the motivating incident: the acting user's connected GitHub
account (`@linuxrecruit`) had **no access** to the private repo `helixml/find-ai`.
GitHub returned `404 Repository not found` (not 403), which is misleading. The
user could not see *which* account he was pushing as, could not tell it lacked
access, and could not find where to switch it (Connected Services is buried in a
`FullScreenDialog` off the account menu — `Layout.tsx:123-131`).

Source design: `helix/design/2026-07-02-project-vcs-connection-lozenge.md`.

## Goals

1. **Never report a success the system can't verify** — the highest priority,
   shippable on its own.
2. Give users a visible, actionable **per-provider connection lozenge** on the
   project Kanban board (who you push as, whether it can reach the repo,
   connect/switch/disconnect in one place).
3. Verify access **loudly** on the push path — never silently fall back to
   another credential for user-initiated pushes.
4. Standardize OAuth connect scopes and force account re-selection so users stop
   silently re-attaching the wrong account.
5. Keep the whole thing **generic across VCS providers** (GitHub, GitLab, ADO,
   Bitbucket) — driven by the provider abstraction, not GitHub-shaped code.

## User Stories & Acceptance Criteria

### Story 1 — Loud push-failure surfacing (highest priority, independent)
**As** a user whose spec-task push failed, **I want** to be told it failed and
why, **so that** I never believe a false "ready for review".

- [ ] When the external mirror push fails and refs are rolled back, the task's
      completion signal reflects failure — the agent does not report success.
- [ ] A structured `last_push_error` is persisted on the spec task, carrying:
      provider, the account used (e.g. `@linuxrecruit`), the repo
      (`helixml/find-ai`), and the raw provider message.
- [ ] The task/board shows an explicit error state (not silence) where the user
      is looking.
- [ ] The raw provider error is translated into cause + next step, e.g.
      *"@linuxrecruit can't access helixml/find-ai. If the repo is private, this
      account isn't a member — switch to an account with access."*

### Story 2 — See who I'm pushing as (the lozenge)
**As** a project owner, **I want** a lozenge on the Kanban board per VCS
provider my repos use, **so that** I can see my acting identity and push access.

- [ ] One lozenge renders per **distinct provider** among the project's external
      repos (`distinct(repo.ExternalType) where repo.IsExternal`). No repos of a
      provider ⇒ no lozenge for it. Local-only repos contribute nothing.
- [ ] Lozenge state is one of: **Verified** (`⎇ repo · @user ✓`),
      **Needs attention** (`⚠ @user · no access to repo`), **Disconnected**
      (`Connect <Provider>`).
- [ ] When the acting Helix user differs from the pushing VCS account, both are
      shown (`acting as Tony · pushing as @tonychapman-prog`).

### Story 3 — Connect / switch / disconnect from the lozenge
**As** a user, **I want** the connection lifecycle in the lozenge's menu, **so
that** I can fix access without hunting through settings.

- [ ] Menu offers: **Switch account**, **Reconnect**, **Disconnect (unbind this
      project)**, **Remove from my account**, **View on <provider>**.
- [ ] **Switch account** and **Reconnect** force the provider's account picker
      (must not silently re-attach the existing session's account).
- [ ] **Disconnect** unbinds the connection from *this project only* — it must
      NOT revoke the user's global token.

### Story 4 — Pre-flight & on-failure verify-and-prompt
**As** a user, **I want** access checked before a full planning run is burned,
**so that** I fix the account first instead of waiting ~285s for a rollback.

- [ ] A provider access-verify probe (interface method per provider) checks the
      acting user's connection can reach the repo.
- [ ] A push auth failure is translated into an actionable "switch account"
      prompt wired to the lozenge — **never** a silent fallback to the repo-level
      / admin credential for user-initiated actions.

### Story 5 — Correct scopes & forced account selection
**As** a user connecting GitHub, **I want** the right scopes requested and to
choose which account, **so that** the lozenge can do its job and I don't
re-attach the wrong account.

- [ ] GitHub connect requests `repo` + `read:user` + `user:email` + `read:org`
      (stop storing `["repo"]` only).
- [ ] The connect/switch flow forces the provider account picker.
- [ ] Scope sets are provider-specific, sourced from each provider's capability
      entry.

## Non-goals (decided)

- Do **not** make a repo/project-level credential authoritative over the acting
  user for user-initiated pushes — acting-as-user is intentional (attribution).
- Do **not** silently fall back to another credential when the user's account
  lacks access — fail loud.
- Keep OAuth connections per-user/global; "project-level" = a per-project
  binding + verified-access surface, not a separate credential store.

## Open questions (from design)

- Multi-repo project, same provider, differing access: one aggregated
  worst-case lozenge, or per-repo detail in the menu?
- Exact board placement (top-right) and coexistence with existing controls.
- Whether pre-flight runs at repo-link time, at "Start planning", or both.
