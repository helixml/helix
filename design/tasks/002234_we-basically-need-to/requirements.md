# Requirements: Rewrite README Around Projects & Kanban Agentic Engineering

## Background

The current `helix/README.md` positions Helix as an "AI Agents on a Private
GenAI Stack" — an on-prem RAG platform. It leads with RAG, knowledge
management, and GPU scheduling. This no longer reflects the product.

Helix has become an **agentic engineering platform**: you organize work into
**projects**, each with a **spec-driven Kanban board**. Tasks flow through
stages (Draft → Planning → Approved → Implementing → Review → Done). Each task
runs a coding agent (Claude Code, Codex, Gemini CLI, Qwen) inside its own full
GPU-accelerated streaming desktop, and you can run many in parallel, watch each
one live, and hand off across time zones.

The README must be rewritten to lead with this story. It should open with a
screenshot of the project Kanban board — which is "wonderfully meta" because
this very README rewrite is itself a spec task moving across that board.

The new marketing positioning already exists in `helix-next` (see
`components/features.tsx` and `content/docs/guide-manage-backlog.mdx`). The
README should be consistent with that language, not invent a new one.

## User Stories

### US-1: New visitor understands what Helix is in 10 seconds
**As a** developer landing on the GitHub repo,
**I want** the first screen (title, tagline, hero screenshot) to show the
projects + Kanban agentic engineering product,
**so that** I immediately understand Helix runs fleets of coding agents, not
that it's an old-school RAG appliance.

**Acceptance Criteria:**
- H1 / tagline reflect agentic engineering (fleets of coding agents in
  isolated desktops, spec-driven Kanban), not "AI Agents on a Private GenAI Stack".
- The first image below the header is a screenshot of the project Kanban board,
  committed into the helix repo (e.g. `docs/images/kanban-board.png`) and
  referenced with a **relative path** — not a hosted/external URL.
- The screenshot has descriptive alt text and a one-line meta caption.

### US-2: Reader understands the projects + Kanban workflow
**As a** prospective user,
**I want** a section explaining projects and the spec-driven Kanban flow,
**so that** I understand how work moves from a plain-language spec to a merged PR.

**Acceptance Criteria:**
- A "Projects & Kanban" section describes the board stages
  (Draft → Planning → Approved → Implementing → Review → Done).
- It explains: write a spec → planning agent produces a plan → approve →
  implementation agent codes in an isolated desktop → review → PR.
- Mentions running multiple tasks in parallel, each in its own sandbox.

### US-3: Reader understands the differentiators
**As a** technical evaluator,
**I want** the headline features to match the current positioning,
**so that** I see why Helix differs from a terminal-only agent runner.

**Acceptance Criteria:**
- Key features cover: full desktop per agent (not just a terminal),
  fleet visibility (watch/step into any agent), multiplayer/follow-the-sun,
  high-density isolation (multiple isolated agents per machine), and
  swap-any-agent (ACP-compatible: Claude Code, Codex, Gemini CLI, Qwen).
- Existing capabilities (RAG/knowledge, tracing, multi-tenancy, self-host)
  are retained but demoted below the agentic-engineering story.

### US-4: Existing practical content is preserved
**As an** operator,
**I want** Quick Start, self-host/deploy, license, and contributing sections
to remain,
**so that** the rewrite doesn't lose install and licensing information.

**Acceptance Criteria:**
- Quick Start (Docker install, Kubernetes), configuration pointer,
  development setup, documentation links, license, support sections are kept
  (edited for consistency, not deleted).
- All existing external links (docs, Discord, launchpad, license) still work.

## Non-Goals
- No changes to product code, docs site, or `helix-next`.
- No new screenshots need to be produced by code. The Kanban hero image is the
  attachment the user provided; it is committed into the helix repo and
  referenced relatively (see Open Questions for the source-file note).
- Not rewriting `CONTRIBUTING.md`, `local-development.md`, or other docs.

## Open Questions
- **Hero screenshot file:** Per review feedback, the Kanban screenshot (the
  user's attachment) will be committed into the helix repo and referenced by
  relative path. Proposed location: `docs/images/kanban-board.png`. One
  practical note: the attachment image is **not currently present on disk in
  this workspace**, so the implementer must save the provided attachment to
  that path before the README will render. Is `docs/images/` the preferred
  location, or should it live elsewhere (e.g. `frontend/assets/img/`)?
- **Keep RAG/knowledge as a section or a one-liner?** Assumption: keep a
  short "Also included" section so existing users still find it, but demote it
  well below the agentic-engineering content.
- **Tagline wording:** helix-next uses "Your Role Is Changing. Here's Your
  Agent Control Room." Should the README reuse that exact tagline or a
  README-specific variant? Assumption: use a consistent, README-appropriate
  variant in the same spirit (agent control room / fleets of coding agents).
- **Cloud vs self-host framing:** Should the lead CTA point to Helix Cloud
  (app.helix.ml) or self-host? Assumption: mention both, keep the existing
  SaaS / Private Deployment link row.
