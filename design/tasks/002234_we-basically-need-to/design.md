# Design: Rewrite README Around Projects & Kanban Agentic Engineering

## Overview

This is a documentation change: rewrite `helix/README.md` to reposition Helix
from an "on-prem RAG platform" to an **agentic engineering platform** built
around **projects** and a **spec-driven Kanban board**. Match complexity to the
task — this is prose + image references, no code.

## Source of Truth for Positioning

Do not invent new marketing language. Reuse what already ships in `helix-next`:

- `helix-next/components/features.tsx` — the five headline pillars:
  1. **The Agent Computer** — full GPU-accelerated desktop per agent, not just a terminal.
  2. **Fastest agentic engineering stack** — Rust IDE, server-side agents, HW video.
  3. **Fleet Visibility** — see every agent, zoom into any one, multi-cursor pairing.
  4. **Multiplayer** — shared agent environments, follow-the-sun handoff.
  5. **Density** — many isolated agent desktops per machine, dedup filesystem.
- `helix-next/content/docs/guide-manage-backlog.mdx` — the Kanban stages:
  **Draft → Planning → Approved → Implementing → Review → Done**, spec-first
  workflow, parallel tasks in isolated sandboxes, PR as the review gate.
- `helix-next/app/page.tsx` — tagline spirit: "Your Role Is Changing. Here's
  Your Agent Control Room."

Backend confirms these concepts are real (not just marketing):
- `helix/api/pkg/types/project.go` — `ProjectSpec` (repositories, kanban WIP
  limits, agent runtime: claude_code/zed/qwen_code/gemini_cli/codex_cli).
- `helix/api/pkg/types/simple_spec_task.go` — `SpecTaskStatus` lifecycle
  (backlog → spec_generation → spec_review → spec_approved → implementation →
  implementation_review → pull_request → done).

## Proposed README Structure

Rewrite top-to-bottom; keep practical sections lower down.

1. **Header** — logo, link row (SaaS · Private Deployment · Docs · Discord).
2. **H1 + tagline** — agentic engineering / agent control room framing.
   One-paragraph pitch: run fleets of coding agents, each in its own desktop,
   organized on a spec-driven Kanban board.
3. **Hero screenshot** — the project Kanban board, with a meta caption
   (this README rewrite was itself a task on that board). The image file (the
   user's attachment) is committed into the helix repo at
   `docs/images/kanban-board.png` and referenced with a relative Markdown path.
4. **Projects & Kanban** (the new core section) — explain the board stages and
   the spec → plan → approve → implement → review → PR flow; parallel tasks in
   isolated sandboxes.
5. **Why Helix is different** — the five pillars condensed, each 1–2 lines,
   with existing screenshots where available.
6. **Also included** (demoted) — RAG/knowledge, skills & tools, tracing,
   multi-tenancy, scheduled tasks/webhooks, notifications, auth. Short bullets.
7. **Quick Start** — Docker installer + Kubernetes (keep as-is).
8. **Configuration / Development / Documentation** — keep, light edits for
   consistency.
9. **License / Contributing / Support / Star history** — keep as-is.

## Key Decisions

- **Reposition, don't delete.** Existing capabilities (RAG, tracing,
  self-host, licensing) are demoted, not removed — current users still rely on
  them and the install flow is unchanged.
- **Single source of positioning, retuned voice.** Keep the *message* from
  `helix-next` (agent control room; engineers' roles changing; fleets of agents
  on a spec-driven Kanban board) but rewrite the *voice* for GitHub's technical
  audience per the requirements Tone Guideline — concrete and mechanism-first,
  minimal marketing/business speak, unsubstantiated superlatives cut.
- **Screenshots.** Commit the Kanban hero screenshot into the repo
  (`docs/images/kanban-board.png`) and reference it relatively — do not use a
  hosted/external URL for the hero (review feedback). The demoted feature
  sections may keep their existing GitHub user-attachment image URLs.
- **Length.** Keep it scannable. The old README is ~270 lines; the rewrite
  should be similar or shorter, front-loading the new story.

## Risks / Gotchas
- Don't break existing anchor/link references pointed at by other docs.
- Keep the license section verbatim (legal text) — only reflow if needed.
- Image alt text matters for the "meta" framing; write it deliberately.

## Testing / Verification
- Render the Markdown (GitHub preview or a local Markdown renderer) and confirm
  headings, images, and links resolve.
- Confirm no broken relative links (`./CONTRIBUTING.md`,
  `./local-development.md`, `api/pkg/agent/SPEC.md`).
- Proofread for consistency with helix-next wording.
