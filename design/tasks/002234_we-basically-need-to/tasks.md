# Implementation Tasks: Rewrite README Around Projects & Kanban Agentic Engineering

- [x] Draft new H1 title and tagline reflecting agentic engineering / agent control room (replace "AI Agents on a Private GenAI Stack"), in a concrete technical voice per the Tone Guideline (message kept, marketing-speak removed; "your role is changing" framed at engineers).
- [x] Write the one-paragraph product pitch (fleets of coding agents, isolated desktops, spec-driven Kanban).
- [x] Commit the Kanban-board screenshot (the user's attachment) into the helix repo at `docs/images/kanban-board.png` and reference it in the README with a relative path, with descriptive alt text and a meta caption.
- [x] Write the new "Projects & Kanban" section explaining board stages (real UI columns: Backlog → Planning → Spec Review → In Progress → Pull Request → Merged) and the spec → plan → approve → implement → review → PR flow, including parallel tasks in isolated sandboxes.
- [x] Write the "Why Helix is different" section condensing the five pillars (Agent Computer, speed, Fleet Visibility, Multiplayer, Density), reusing helix-next wording; keep existing screenshots where relevant.
- [x] Add a "Works with your stack (no lock-in)" compatibility section: agent harnesses (Claude Code, Codex, Gemini CLI, Qwen Code, any ACP-compatible agent) and LLM providers (hosted OpenAI/Anthropic; Anthropic via proxy including Vertex AI and Bedrock; self-hosted OpenAI-compatible endpoints attached as external providers, calling out vLLM on your own GPUs/Kubernetes).
- [x] Add a demoted "Also included" section for RAG/knowledge, skills & tools, tracing, multi-tenancy, scheduled tasks/webhooks, notifications, and auth.
- [x] Preserve and lightly edit Quick Start (Docker + Kubernetes), Configuration, Development, and Documentation sections.
- [x] Preserve License, Contributing, Support, and Star History sections verbatim.
- [x] Verify all links resolve (docs, Discord, launchpad, `./CONTRIBUTING.md`, `./local-development.md`) and images render in GitHub Markdown preview. Fixed two links that were already broken in the old README: removed dead `./api/pkg/agent/SPEC.md` and repointed `UPGRADING.md` → `./charts/helix-controlplane/UPGRADE.md`.
- [x] Proofread the whole README against the Tone Guideline (concrete/technical, no unsubstantiated superlatives, developer-oriented) while keeping the helix-next message; confirm final length stays scannable (README shrank from ~271 to ~232 lines).
