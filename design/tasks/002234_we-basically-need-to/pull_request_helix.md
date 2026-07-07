# Rewrite README around projects & Kanban agentic engineering

## Summary

The old README positioned Helix as "AI Agents on a Private GenAI Stack" — it
read like an on-prem RAG appliance and led with RAG, knowledge management, and
GPU scheduling. That no longer reflects the product.

This rewrite repositions Helix as what it is: a **private agent fleet with
spec-driven coding**. You run many coding agents in parallel, each in its own
GPU-accelerated desktop sandbox, organized on a Kanban board of spec tasks, and
you review the resulting pull requests.

It leads with a screenshot of the Helix Kanban board — with the "New SpecTask"
panel showing *this very task* being created. Wonderfully meta.

Tone is tuned for a technical GitHub audience: concrete and mechanism-first,
marketing/business speak removed, unsubstantiated superlatives cut. The message
matches the helix.ml marketing site and docs.

## Changes

- New H1 + tagline: "a private agent fleet with spec-driven coding".
- Hero image committed at `docs/images/kanban-board.png` (relative reference,
  descriptive alt text, meta caption).
- New **Projects & the Kanban board** section documenting the real board flow:
  Backlog → Planning → Spec Review → In Progress → Pull Request → Merged, plus
  parallel isolated sandboxes.
- New **Why Helix is different** section (desktop per agent, fleet visibility,
  multiplayer, high-density isolation, no lock-in).
- New **Works with your stack (no lock-in)** section: agent harnesses (Claude
  Code, Codex, Gemini CLI, Qwen Code, any ACP-compatible agent) and LLM
  providers (hosted OpenAI/Anthropic; Anthropic via proxy incl. Vertex AI and
  Bedrock; self-hosted OpenAI-compatible endpoints incl. vLLM on your own GPUs).
- Existing capabilities (RAG, skills/tools, tracing, multi-tenancy, automation,
  notifications, auth) demoted to a concise **Also included** section.
- Quick Start, Configuration, Development, License, Contributing, Support kept.
- Fixed two links that were already broken in the old README: removed dead
  `api/pkg/agent/SPEC.md`; repointed Upgrading Guide to
  `charts/helix-controlplane/UPGRADE.md`.
- `.gitignore`: added a negation for `docs/images/` so README images are tracked
  despite the blanket `*.png` rule.

## Screenshots

![Helix Kanban board — hero](https://github.com/helixml/helix/raw/helix-specs/design/tasks/002234_we-basically-need-to/screenshots/01-kanban-hero.png)
