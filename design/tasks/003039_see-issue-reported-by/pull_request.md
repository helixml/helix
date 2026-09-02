# Fix legacy organization agent provider migration

## Summary
Allow organization agents with legacy provider names such as `user/anthropic` to match an available organization or global provider, so administrators can update and save those agents without restoring personal-provider access. Personal `pe_` provider IDs that are not visible to the organization remain rejected.

Hydrate synthetic global OpenAI endpoints from the configured `OPENAI_API_KEY` and `OPENAI_BASE_URL` before proxying Codex Responses API requests. This prevents coding-agent requests from failing with a misleading missing API key error when the global OpenAI provider is configured through the control-plane environment.

## Testing
Added regression coverage for saving an organization agent with a legacy provider name and for resolving a legacy global OpenAI reference with its configured credentials and base URL.

Focused provider validation and code-agent endpoint tests pass. The complete `api/pkg/server` test package was also run; changed tests pass, while unrelated in-process organization tests fail because the test binary was built with `CGO_ENABLED=0` and `go-sqlite3` requires CGO.
