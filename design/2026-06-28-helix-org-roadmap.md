# Helix-Org Roadmap

_Date: 2026-06-28 · Status: draft for discussion_

## Thesis

Automate traditional, information-centric back-office jobs by employing AI to do
the small, recurring tasks a business needs done. These jobs typically share a
shape: **ingest information → take some action → pass an approval / acknowledgement
gate**. Helix-Org aims to be generic across these workloads.

**Anchor example — the documentation bot.** Simple, has an approval gate, requires
teaching. We use it throughout this roadmap to keep ideas concrete.

## Mental model (the domain today)

Modelled deliberately as an **org chart** so it can be explained in ordinary
business language.

- **Role** — the job description. A markdown `role.md`: what the job is, who/what it
  interacts with, how the work is done, the workflow to follow. Editable by anyone
  with edit permission (usually a manager) in the UI.
  _(`orgchart.Role`, `api/pkg/org/domain/orgchart/role.go`)_
- **Worker** — hired into a role; implemented as an AI agent. Has an **identity**
  (most importantly a name, used to refer to it; plus a markdown `identity.md`).
  _(`orgchart.Worker` / `orgchart.Position`)_
- **Topic** — the connection to the outside world; an event stream the worker
  subscribes to (e.g. GitHub, Slack). **Streams** carry the messages; filters /
  processors can be applied mid-stream to cut noise.
  _(`api/pkg/org/domain/streaming/`, wake via `infrastructure/wakebus`)_
- **Activation** — a topic message spins up a new ACP agent session with a special
  briefing prompt that points the agent at its `role.md`, lists its tools, and
  includes the trigger. The agent then decides what to do (e.g. `gh` for GitHub,
  Slack REST for Slack).
  _(`briefing.BuildPrompt`, `api/pkg/org/domain/briefing/prompt.go`)_
- **Credential provider** — wraps a third-party service; Helix mints a short-lived
  OAuth credential on demand so an agent can call a token-secured API (e.g. Slack).
- **Learnings** — the agent improves via its own mechanism (e.g. Claude memories /
  a markdown file). Managers improve behaviour by editing the role.

**Design stance: no procedural pipelines.** The agents are good enough; improve the
role prompt or the model rather than writing and maintaining procedural workflow
code. Everything assumes ACP communication — a deliberate simplification.

## Where we are today

The mental model is highly flexible and the system "just about works" end to end.
Day-to-day usage is flaky: rough edges in the UI and the Helix interface, not yet
100% stable, personal-subscription token exhaustion, imperfect org configuration.
**Priority: actually use the system and find the rough edges.**

---

## Roadmap

Ordered top-down: highest-level domain flexibility first, then domain enhancements,
then lower-level implementation, productionization, adoption, and UX.

### Horizon at a glance

| Horizon | Theme | Headline items |
|---|---|---|
| **Now** | Stabilise & exercise | Dogfood real bots; fix rough edges; finish humans + hierarchy demos |
| **Next** | Observability & events | Audit/record activity; rich domain events + UI status pills; reliable stream queues |
| **Next** | Productionize | Remove gating, ship to prod; Agent + Sandbox as the workspace |
| **Later** | Performance & reach | Fast agent harness; many more topic types; UX overhaul |

---

### 1. Complete the domain (top-level flexibility)

Two domain concepts exist as placeholders but are not yet exercised.

#### 1.1 Humans in the org
**Goal.** Humans act as placeholders for real people, holding their identifiers
across systems and a description of what they are responsible for.

- A human (e.g. "Luke") carries cross-system identifiers: Slack handle, email,
  GitHub login, plus a responsibility description (e.g. "point of contact for Helix
  code; commercial sales meetings").
- AI workers can query the org for "who is responsible for X", resolve to a human,
  and reach them across the right channels (ping on GitHub, email, etc.).

**Demo.** Worker finds a bug → asks the org who owns Helix code → tags Luke on
GitHub and emails him.

#### 1.2 Hierarchy — approvals & escalation
**Goal.** Make the existing manager/subordinate structure useful. Tools already
expose who a worker's manager and subordinates are; the open question is _when a
worker would use it_.

- Lead use case: **approval gates / escalation.** Role prompt states the worker must
  get line-manager approval before a given action.
- Build a demo that genuinely requires hierarchy (approval → approved/denied loop).

---

### 2. Domain enhancements — observability & events ("two heads")

The biggest enhancement is multifaceted, spanning both the **observational** end and
the **event-emission** front end.

#### 2.1 Observability / audit (record, then expose)
**Problem.** Very hard to see what's happening inside the org and hard to debug. Fix
by first **recording** activity, then **exposing** it.

- Decide the right **granularity** — likely not every raw interaction/log, but enough
  for visibility and debugging (some cases may warrant full capture).
- Address both the **technical** challenge (high traffic volume) and the **UX**
  challenge (presenting it usefully).
- _Scaffolding exists:_ a `domainevent` package and project-level audit logging
  (`org_domain_events`, `services/audit_log_service.go`) — build on these rather than
  starting fresh.

#### 2.2 Domain events (DDD sense)
**Goal.** Rich, first-class domain events emitted throughout the org. Today there are
many slots for them but effectively none in use.

- Examples: `approval.required` → `approval.approved` / `approval.denied`; worker
  lifecycle (sleeping → woken → handling message → finalising).
- Drives real-time visibility in the org chart and unlocks observability broadly.

#### 2.3 UI status pills
**Goal.** Per-worker status pills in the UI showing what each worker is doing
("sleeping", "woken", "awaiting approval", "finalising work").

- Useful, accurate states — driven by domain events, not cosmetic noise.

#### 2.4 Reliable streams — proper queues with ack
**Problem.** Streams are currently lightweight/in-memory with no read-acknowledgement.
Future use cases (e.g. several workers in one role pulling tickets off a queue) need
**pull + ack** so a message is handled exactly once.

- Add acknowledgement / "message read" semantics so multiple workers don't double-handle.
- **Option:** back streams with **NATS** (already used elsewhere, incl. JetStream) for
  durable, reliable delivery. Trade-off: heavier than the current simple ~few-hundred-line
  in-memory model. Weigh durability vs. simplicity.

#### 2.5 Outbound communication — settled principle (revisit only if it breaks down)
**Decision (default for now).** Only **incoming** streams exist. All outbound
communication is done by the **agent itself** via tools/actions it writes — no
custom outbound adapters/APIs.

- **Why.** Avoids writing a per-app outbound translation layer (e.g. a single JSON
  packet + Helix-side parser fanned out into Slack's separate "react / post text /
  attach file" REST calls). Let the agent make those calls directly.
- Inbound stays flexible: a message is just a string in whatever format, parsed
  Helix-side; one message = one complete result.
- Revisit only when a concrete case makes agent-owned outbound impractical.

---

### 3. Connectivity — more incoming topic types

**Definition of a good topic.** Any system that **emits events** usable as a trigger.
Poor topics are external systems that are stores of data rather than event streams —
for those, trigger the agent another way (a person asking, a cron) and let it pull the
data via API/MCP with the right credentials.

- **Harden existing:** email path exists but is untested/may need rewiring; Sentry.
- **Good candidates:** Postmark (email), WhatsApp, Discord, Teams, Azure DevOps, other
  comms/edit-in-motion event sources.
- **Not topics (data stores):** Google Analytics and similar — access via tool/MCP on
  demand, don't model as a stream.

---

### 4. Productionize

#### 4.1 Release to prod (remove gating)
Currently gated by two things:
1. An environment variable disabling most org code paths — **`HELIX_ORG_ENABLED`**
   (`api/pkg/config/config.go`).
2. A **per-user feature flag** on the users table.

**Plan.** Deploy Helix-Org to prod and run the more stable bots there. Create a
top-level org and a personal space for bots.

#### 4.2 Workspace: Agent + Sandbox (replace spec-task infra)
Org currently rides on the spec-task infrastructure (well-hardened from heavy coding
use, with useful helpers). **Target:** move to the first-class **sandbox** concept.
A worker's workspace = a **Helix agent** + a **Helix sandbox** (the agent concept
already exists and is good).

#### 4.3 Fast agent harness
**Problem.** Today everything runs through Zed + MCP — fine, but cold-starting the
desktop, starting a session, and basic cloud interactions are slow.

**Goal.** A much faster agent harness in Helix Agents (shape TBD).

- **Targets:** quick-response use cases — chatbots, real-time replies, voice / phone,
  and faster Slack interactions.

---

### 5. Adoption & dogfooding

**High value:** real usage finds rough edges _and_ delivers value when the bot itself
is worthwhile.

- Build genuinely useful agents (e.g. Chris automating sales tasks) — double whammy of
  new demos/use cases plus real value.
- Migrate internally-run systems over to Helix-Org.

---

### 6. UX (owned by Karolis)

**Audience.** Aim at the average **technical-ish business user** — someone comfortable
with Excel and SaaS tools, not a non-technical consumer. Don't dumb it down to the
point of losing flexibility (false economy).

- Preserve **power-user flexibility**: ability to change agent type / model.
- Fix the current pain of hopping between disjoint parts of the Helix UI — unify the
  org experience.

---

## Open questions

- Audit **granularity** — what to record vs. drop, and how to present it at scale.
- Streams: adopt **NATS/JetStream** for durable queues, or keep the simple in-memory
  model? Where's the line?
- What is the **fast agent harness** actually — architecture and surface?
- The right **hierarchy demo** that genuinely needs manager/subordinate awareness.
