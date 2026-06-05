# feat(goose): Integrate Goose AI Agent into Zed via ACP (Phase 1)

## Summary

Adds Goose (the AAIF/Block open-source AI agent) as a fourth code-agent runtime alongside `zed_agent`, `qwen_code`, and `claude_code`. Goose speaks ACP natively, so it slots into Zed's agent panel via the same `agent_servers` plumbing the other runtimes use — no new protocol work needed.

This PR ships Phase 1 (base runtime — single "Goose" thread in Zed). Phase 2 (per-recipe custom agents driven from project YAML + the spec-task creation form) will follow in a separate PR.

## Changes

See the per-repo `pull_request_<repo>.md` for repo-specific details. Spec docs (requirements / design / tasks) live in [`helix-specs/design/tasks/002041_we-need-to-integrate/`](https://github.com/helixml/helix/tree/helix-specs/design/tasks/002041_we-need-to-integrate/).
