# Requirements: Code Intelligence Skill

## Background

Helix has a skills marketplace with YAML-defined skills (GitHub, Jira, Gmail, etc.) and separate user-configured MCP tools. Kodit is an MCP server for code search/intelligence that Helix already uses internally (for Zed IDE sync), but it is not exposed as a skill users can enable for their agents/apps.

## User Stories

**US1**: As a Helix user, I want to see a "Code Intelligence" skill in the skills marketplace so I can enable it for my agents.

**US2**: As a Helix user, I should be able to enable the Code Intelligence skill with a single click — no URL or API key required. Helix generates the Kodit MCP URL and auth credentials internally (using the user's existing Helix API key and org membership).

**US3**: As a Helix agent, once Code Intelligence is enabled, I should have access to Kodit MCP tools (semantic_search, keyword_search, grep, list_files, read_file) to answer code-related questions.

**US4**: As a Helix developer, I want TDD-first implementation with unit and e2e tests so the feature works now and stays correct as the codebase evolves.

**US5**: As a QA manager, I want a documented manual test procedure so I can verify the end-to-end feature works in a running Helix instance.

## Acceptance Criteria

- [ ] "Code Intelligence" skill appears in the skills list (`GET /api/v1/skills`)
- [ ] The skill has a category of "Development" and an appropriate icon
- [ ] The skill's YAML definition specifies it is MCP-backed (not API-backed)
- [ ] Enabling the skill requires no user-provided configuration — Helix derives the Kodit MCP URL and auth token automatically (from the user's Helix API key and org)
- [ ] The configured skill is saved as an `AssistantMCP` tool on the app/assistant
- [ ] The agent can call Kodit MCP tools (search, grep, read_file, etc.) during inference
- [ ] The skill description explains what Kodit provides and what the URL field means

## Testing Requirements

### Unit Tests (TDD — write tests first)

- **Skill manager** (`api/pkg/agent/skill/api_skills/manager_test.go`): loading `code-intelligence.yaml` produces a `SkillDefinition` with `Spec.MCP.AutoProvision == true`
- **Enable handler** (`api/pkg/server/skills_test.go`): `POST /api/v1/apps/{id}/skills/code-intelligence/enable` returns 200 and the app's `mcpTools` contains an entry with the expected Kodit URL and the user's API key in the `Authorization` header
- **Enable handler — no Kodit URL configured**: returns a clear error (not a silent no-op)

### E2E / Integration Tests

- **Full provisioning flow** (suite-based, using `NewTestServer` + mock store): calling the enable endpoint produces an `AssistantMCP` config that the agent runtime (`NewDirectMCPClientSkills`) can pick up and use
- **Extend `mcp_backend_kodit_test.go`**: verify that an app with a Code Intelligence MCP tool configured routes calls to the Kodit backend correctly

### Frontend Tests

- Vitest test confirming the skill marketplace renders the Code Intelligence card and calls the enable endpoint on click (no dialog shown for `autoProvision` skills)

### Manual QA Test Plan

**Pre-conditions:** Helix running at `http://localhost:8080` with Kodit configured.

**Steps:**

1. **Register** — go to `http://localhost:8080/login`, click "Register here", create an account (e.g. `test@helix.local` / `testpass123`)
2. **Onboarding** — complete the onboarding flow (create an org)
3. **Add a repository** — in the org settings, add at least one Git repository so Kodit has code to index
4. **Create an agent** — go to Apps / Agents, create a new agent
5. **Enable Code Intelligence** — in the agent's Skills section, find "Code Intelligence" and click Enable (no config dialog should appear)
6. **Save the agent**
7. **Chat with the agent** — open the agent and ask a question about code in the connected repository (e.g. "What does the `RunAgent` function do?")
8. **Verify tool use** — confirm the agent's response references actual code from the repository and that the Kodit MCP tools appear in the tool-call trace (semantic_search, grep, or read_file)

**Pass criteria:** The agent answers correctly using code from the repository, and tool calls to Kodit are visible in the session trace. No manual URL or API key was entered at any point.
