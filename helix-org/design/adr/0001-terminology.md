# ADR-0001 — Terminology pinned for the helix-org redesign

## Status

Accepted — 2026-05-21.

## Context

helix-org's domain layer is well named (one struct per noun, IDs
centralised, enums isolated) but the seam between code, prompts, and
`CLAUDE.md` has accumulated synonyms and homonyms. The redesign
analysis under [`../2026-05-21-redesign/03-ubiquitous-language.md`](../2026-05-21-redesign/03-ubiquitous-language.md)
catalogued them; eight terminological resolutions are recommended in
§6 of that doc. Two further resolutions arrive at the helix↔helix-org
seam now that the module is being absorbed into helix
([`../2026-05-21-redesign/09-integration-reframe.md`](../2026-05-21-redesign/09-integration-reframe.md)
§3.03).

Pinning these names *before* the substantive redesign moves removes a
recurring round of "which word do we use?" from every subsequent PR.
Reviewers can then reject drift on sight.

## Decision

The following ten names are canonical. Synonyms and homonyms below
the line are banned in new code, new prose, and new docs.

### 1. Stream, not Channel

`Stream` is the canonical term for the org-graph noun:
ordered-sequence-of-events-you-can-subscribe-to. `Channel` is banned
in prose, code, and `CLAUDE.md`. Demos and role markdown using
"channel" colloquially are grandfathered; new role markdown uses
`Stream`.

*Why:* `Stream` is already the type (`domain.Stream`), the table
(`streams`), the ID prefix (`s-…`), the `## Streams` section header
in `prompts/templates/role.md`, and the MCP tool family
(`create_stream`, `list_streams`, `stream_members`). `Channel` exists
only in `CLAUDE.md:19` and in colloquial role prose.

### 2. AI Worker (domain) + Agent (LLM-client); worker-policy.md not agent.md

The domain keeps `AIWorker` as the type and "AI Worker" as the prose
noun. The org-wide policy file ships as `worker-policy.md`, not
`agent.md`. The word "agent" is reserved for the LLM-client-binary
sense (Claude Code, the helix `external-agent` runtime) — i.e. the
thing that runs an AI Worker, not the AI Worker itself.

*Why:* `agent` was doing five jobs in the codebase
([`../2026-05-21-redesign/03-ubiquitous-language.md` §2.1](../2026-05-21-redesign/03-ubiquitous-language.md)).
Releasing it from "the AI side of a Worker" duty resolves the worst
homonym; releasing it from `agent.md` lets the file name follow the
domain term.

### 3. No scope

"Scope" is not a domain concept. The only authorisation primitive is
`(WorkerID, ToolName)`. Per-tool granularity comes from designing the
tool, not from gating a generic tool with a scope field. If per-grant
constraints are ever needed, they will land as an explicit
`Constraints` blob on `ToolGrant`; we do not retrofit prose first.

*Why:* `domain/grant.go:5-9` is already an explicit polemic against a
Scope field; `CLAUDE.md:17-18` contradicts it. The contradiction goes;
the code's stance wins.

### 4. Identity (canonical) — retire persona / profile / candidate

The per-Worker free-text description is **Identity**. The column is
`Worker.IdentityContent`; the file is `identity.md`; the MCP tool is
`update_identity`. "Persona", "profile", and "candidate" are banned
in new code and new prose; recruiter role markdown calls its outputs
"identities".

*Why:* the three synonyms appear scattered across `domain/worker.go`,
`demos/newsroom/roles/recruiter.md`, and various Role markdowns —
five names for one column.

### 5. Role typed manifests (`Tools` and `Streams`)

Today the role markdown body has prose conventions
(`## Tools (MCP)`, `## Streams`, `## Triggers`, `## Constraints`,
`## Files`) that the code does not parse. We pin the intent that
**`Tools` and `Streams` become typed manifests on `role.Role`**
(`[]tool.Name` and `[]stream.ID` respectively), shipping in migration
B7. These are **reference data only** — `hire_worker` does not
enforce them, does not auto-grant, and does not auto-subscribe. The
hiring caller's prompt reads them programmatically (via `get_role`)
to populate the `grants` arg and to call `subscribe` for each Stream
after the hire. The fields lift the lists out of unparsed markdown
sections so the chat brain can read them as JSON instead of parsing
prose.

The field names are deliberately bare — not `DefaultTools` /
`DefaultStreams` (an earlier proposal). "Default" implied "applied
unless overridden," but nothing overrides them: the hirer is fully
responsible. Bare names + a doc comment explaining "reference data,
no enforcement" reads more honestly.

*Why:* lifting the lists onto `Role` closes the unenforced "Workers
must subscribe to their declared streams" invariant (`TODO.md` item
1). Pinning the name now means B7's PR is uncontroversial.

### 6. Scheduler — one interface, replacing three "Dispatcher"s

The thing that fans events out to subscribed Workers and serialises
their activations is named **Scheduler**. The current three
interfaces (`dispatch.Dispatcher`, `server.Dispatcher`,
`tools.EventDispatcher`) collapse into one `activation.Scheduler`
when migration B2 lands. The word "Dispatcher" is reserved for the
prose-only sense ("the runtime dispatches activations") if it appears
at all.

*Why:* three interfaces existed to dodge import cycles; one name
removes the duplication. Renaming to `Scheduler` reflects that
post-B2 the responsibility narrows from "fan-out + outbound emit" to
just "fan-out + per-Worker queueing" — the outbound emit moves to a
separate `Outbox`.

### 7. MirrorFile, not PublishFile

`agent.WorkspaceSync.PublishFile` is renamed to
`WorkspaceSync.MirrorFile`. "Publish" is reserved for the MCP-tool
sense ("append an Event to a Stream").

*Why:* using `publish` for both "append to a Stream" and "mirror
role.md into the runtime workspace" causes every reader to do a
double-take.

### 8. Activation is a first-class noun

**Activation** = one Spawner invocation = one fresh agent turn = the
unit being batched, logged, and audited. It will become a typed
aggregate in migration B5; until then the *word* is pinned so that
event-stream IDs (`s-activations-<workerID>`), log keys
(`activation_id`), and prose all use it consistently. "Spawn", "wake",
"reactivate", "fire" are not synonyms for Activation:

- **Spawn** is the verb on `Spawner` (the port).
- **Wake** is reserved for `Broadcaster` long-poll wake-ups.
- "Fire" and "reactivate" are not used.

*Why:* today there is no shared term for "the thing being batched"
([`../2026-05-21-redesign/03-ubiquitous-language.md` §6.8](../2026-05-21-redesign/03-ubiquitous-language.md)),
which makes `TODO.md` item 6 ("agents go through events one by one")
hard to even discuss precisely.

### 9. Org Graph (helix-org) vs Organization (helix)

`helix.Organization` is the helix tenancy unit (a customer org).
helix-org's structural state is the **Org Graph** — never "the org"
on its own, because that collides with `helix.Organization` now that
the two live in one repo. After multi-tenancy lands (migration H5)
each `helix.Organization` will own one Org Graph instance; if at that
point a shorter noun is wanted (e.g. **Roster**) it lands in a
follow-up ADR.

*Why:* PR #2286 ships with one shared `w-owner` across all gated
users — multi-tenancy is OOS for alpha. When it arrives, the
"Org Graph belongs to an Organization" framing makes the schema
change straightforward.

### 10. Worker ↔ helix.Project is 1:1 and explicit

Every AI Worker corresponds to exactly one `helix.Project` (provisioned
lazily by `WorkerProject.Ensure`). This binding becomes an explicit
typed field `Worker.HelixProjectID`, not a kv-row in
`WorkerRuntimeState` keyed by `(workerID, backend, "project_id")`.

*Why:* the relationship is load-bearing for the production runtime
but invisible to the schema. Making it explicit unblocks the H1
migration (delete helixclient) — direct controller calls can take a
typed Project ID instead of fishing it out of a sidecar kv-store.

## Consequences

### Positive

- Every subsequent PR uses canonical terms. Reviewers reject drift on
  sight.
- The `CLAUDE.md:17-19` edits resolve three contradictions the
  analysis caught between doc and code (Channel, scope, and the
  Tool-vs-shell-tool split).
- `Scheduler`, `MirrorFile`, `Activation`, and `HelixProjectID` are
  pinned but not yet introduced; their introducing PRs (B2, B5, H1
  per the migration plan) inherit settled names.
- The `agent.md` → `worker-policy.md` rename releases the most
  overloaded word in the project; runtime-side prose can talk about
  "agents" (Claude Code, external-agent) without colliding with the
  domain.

### Negative

- Existing demo role markdowns that use "channel" colloquially are
  inconsistent until rewritten. Acceptable: they are prose for
  LLMs, which handle the synonym gracefully.
- `MirrorFile` rename touches 14 call sites including tests. Single
  mechanical sweep in the same PR; low risk.
- The owner role template uses "firing" as a verb (`bootstrap/templates/owner_role.md:20`).
  No `fire_worker` tool exists. Left as colloquial prose; no decision
  here either way.

### Out of scope

- Renaming `Dispatcher` to `Scheduler` *in code* (waits for B2; the
  ADR pins the destination name only).
- Introducing `Activation` as a type (waits for B5).
- Introducing `Worker.HelixProjectID` as a field (waits for H1).
- Multi-tenanting the Org Graph (waits for H5; if "Roster" is preferred
  over "Org Graph" then, that's a follow-up ADR).
- Removing `agent` from package names (`helix-org/agent/`,
  `agent/claude/`, `agent/helix/`). Package renames have a long blast
  radius; the source-file rename to `worker-policy.md` is enough for
  now.
- Renaming the **on-disk projection** of the policy file. Today both
  runtimes write the content as `agent.md` in the Worker's
  Environment (claude: `<envsDir>/<workerID>/agent.md`; helix:
  `.context/agent.md` on the helix-specs branch) and the activation
  prompt tells the Worker to read that path. Renaming the projection
  to `worker-policy.md` is a follow-up PR — it has to coordinate
  with existing helix-specs branches that already carry `agent.md`,
  and with the prompt strings in `agent/helix/spawner.go` and
  `agent/claude/spawner.go` that `cat` the old name. Until then, the
  Go field `agenthelix.AgentConfig.AgentMD` and the path
  `.context/agent.md` stay as-is.

## References

- [`../2026-05-21-redesign/03-ubiquitous-language.md`](../2026-05-21-redesign/03-ubiquitous-language.md) §2, §3, §6
- [`../2026-05-21-redesign/04-bounded-contexts.md`](../2026-05-21-redesign/04-bounded-contexts.md) §6
- [`../2026-05-21-redesign/08-migration-plan.md`](../2026-05-21-redesign/08-migration-plan.md) §B M0
- [`../2026-05-21-redesign/09-integration-reframe.md`](../2026-05-21-redesign/09-integration-reframe.md) §3.03, §4 B0
- helix PR [#2286](https://github.com/helixml/helix/pull/2286) — embedded-in-helix integration
