# Requirements: Add Credentials Backstop Paragraph to worker-policy.md

## Background

`api/pkg/org/application/agent/worker-policy.md` is the org-wide policy
text every AI Worker reads at the start of every activation. It is
embedded into the Helix binary via `//go:embed worker-policy.md`
(`policy.go:27`) and is pushed by the Helix runtime to
`.context/worker-policy.md` on each Worker's repo.

Today the policy covers *being* an AI Worker — speaking discipline,
AI-vs-human priority, chain of command, errors-and-exits. It says
nothing about **credentials**.

After task 002092 (`mint_credential`), every Worker holds the
`mint_credential` tool in its baseline tool set, but the shell no
longer contains a `GH_TOKEN` env var at boot. The recovery contract
(*mint → export → retry on 401/403*) currently lives only in
per-Role prompts. As of writing, only one Role (`owner_role.md`) has
been updated; five-plus existing Roles authored against the old
`SecretInjector` contract still describe shell-authenticated commands
as if `GH_TOKEN` were present. A Worker on one of those Roles will,
on its next activation, silently burn the turn trying to
`gh api`/`git push`/`curl -H "Authorization: ..."` with no token.

## User Story

**As a** Helix operator,
**I want** the credential contract documented at the org-policy
altitude (not just per-Role),
**so that** every existing and future Worker — regardless of which
Role's prompt it was hired against — knows to mint a token before
authenticated shell work and to re-mint on 401/403 without me having
to hand-edit every Role prompt in the org.

## Scope

Add a short, prescriptive section to `worker-policy.md` that:

1. States the default: external-provider tokens are **not** present
   in the shell at boot.
2. States the action: call `mint_credential` (with the provider name)
   *before* any `gh`, `git`, or authenticated `curl` invocation and
   `export` the returned token (e.g. `export GH_TOKEN=$(...)`).
3. States the recovery: any 401/403 from such a command means the
   token has expired — re-mint, re-export, retry. This is expected
   on any session that outlives ~1h.
4. Defers detail to the per-Role prompt for *which* providers and
   *which* commands apply — worker-policy.md gives the rule, the
   Role describes the workflow.

## Acceptance Criteria

- **AC1.** `worker-policy.md` contains a new section (placed between
  "Chain of command" and "Errors and exits") that covers the three
  bullets above in fewer than ~15 lines of prose. The tone matches
  the rest of the file (terse, prescriptive, no exclamations).
- **AC2.** The section names `mint_credential` explicitly and shows
  the export pattern (`export GH_TOKEN=$(...)`) so the agent can
  copy-paste rather than infer it.
- **AC3.** The section names 401/403 explicitly as the re-mint
  trigger, mirroring the wording in `mint_credential`'s tool
  description so the agent gets the same signal from both surfaces.
- **AC4.** The section does not duplicate per-Role workflow detail —
  it is a *backstop*, not a tutorial. (Reviewer test: would a Role
  that doesn't use any external provider be confused or misled by
  this paragraph? Answer must be no — it should read as
  "if-you-use-any-of-these, here's the rule.")
- **AC5.** (Revised after review feedback.) The existing
  "External-provider credentials: `mint_credential`" section in
  `owner_role.md` is **deleted** in this task. With the rule living
  in `worker-policy.md` (which every Worker reads on every
  activation), owners no longer need to write a credential paragraph
  into Roles they create — the policy text already covers it. One
  source of truth, no duplication for owners to keep in sync.
- **AC6.** `Policy` is the embedded `worker-policy.md` string
  (`policy.go:14`). The Go build must remain green — no `//go:embed`
  or test breakage. No existing test asserts the literal contents of
  `Policy`, so the edit is text-only.
- **AC7.** The change ships as a single commit on the helix repo —
  this is the systemic fix promised in the parent task notes and
  should not be bundled with unrelated Role edits.

## Out of Scope

- Editing the five-plus stale per-Role prompts to remove their
  old `GH_TOKEN`-assumes-it-exists guidance. Those Roles get fixed
  organically when next touched; the backstop in worker-policy.md
  is what makes that organic timeline safe.
- Changing `mint_credential`'s tool description or the credential
  provider registry — both already shipped in task 002092.
- Adding new credential providers (Slack etc.) — out of scope; the
  worker-policy.md text must read provider-agnostically so a future
  Slack provider doesn't require another edit.
