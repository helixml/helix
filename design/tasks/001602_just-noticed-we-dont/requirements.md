# Requirements: Code Intelligence Skill

## Background

Helix has a skills marketplace with YAML-defined skills (GitHub, Jira, Gmail, etc.) and separate user-configured MCP tools. Kodit is an MCP server for code search/intelligence that Helix already uses internally (for Zed IDE sync), but it is not exposed as a skill users can enable for their agents/apps.

## User Stories

**US1**: As a Helix user, I want to see a "Code Intelligence" skill in the skills marketplace so I can enable it for my agents.

**US2**: As a Helix user, I should be able to enable the Code Intelligence skill with a single click — no URL or API key required. Helix generates the Kodit MCP URL and auth credentials internally (using the user's existing Helix API key and org membership).

**US3**: As a Helix agent, once Code Intelligence is enabled, I should have access to Kodit MCP tools (semantic_search, keyword_search, grep, list_files, read_file) to answer code-related questions.

## Acceptance Criteria

- [ ] "Code Intelligence" skill appears in the skills list (`GET /api/v1/skills`)
- [ ] The skill has a category of "Development" and an appropriate icon
- [ ] The skill's YAML definition specifies it is MCP-backed (not API-backed)
- [ ] Enabling the skill requires no user-provided configuration — Helix derives the Kodit MCP URL and auth token automatically (from the user's Helix API key and org)
- [ ] The configured skill is saved as an `AssistantMCP` tool on the app/assistant
- [ ] The agent can call Kodit MCP tools (search, grep, read_file, etc.) during inference
- [ ] The skill description explains what Kodit provides and what the URL field means
