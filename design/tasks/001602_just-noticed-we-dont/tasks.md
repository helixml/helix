# Implementation Tasks

Tests are written first (TDD). Each implementation task is paired with the test that drives it.

## Types & Skill Definition

- [x] Add `YAMLSkillMCPSpec` struct and `MCP` field to `YAMLSkillSpec` in `api/pkg/types/skill.go`
- [x] Write `manager_test.go` test: loading `code-intelligence.yaml` produces `Spec.MCP.AutoProvision == true`
- [x] Create `api/pkg/agent/skill/api_skills/code-intelligence.yaml` (make the test pass)

## Enable Endpoint (backend)

- [~] Write `skills_test.go` unit tests (testify/suite + gomock):
  - `POST /api/v1/apps/{id}/skills/code-intelligence/enable` returns 200, `mcpTools` contains Kodit URL + user API key in `Authorization` header
  - Returns error when no Kodit URL is configured in platform config
- [ ] Implement `POST /api/v1/apps/{id}/skills/{skillName}/enable` handler (make tests pass)

## E2E / Integration Tests

- [ ] Write suite-based integration test using `NewTestServer` + mock store: enable endpoint produces an `AssistantMCP` that `NewDirectMCPClientSkills` can consume
- [ ] Extend `api/pkg/server/mcp_backend_kodit_test.go`: verify an app with a Code Intelligence MCP tool routes calls to the Kodit backend correctly

## Frontend

- [ ] Write Vitest test: skill marketplace renders Code Intelligence card and calls the enable endpoint on click (no dialog shown for `autoProvision` skills)
- [ ] Implement frontend marketplace changes to support `autoProvision` one-click enable (make test pass)

## Verification

- [ ] Run `go build ./pkg/server/ ./pkg/types/ ./pkg/agent/skill/...` — no compile errors
- [ ] Run `CGO_ENABLED=1 go test -v ./pkg/server/ ./pkg/agent/skill/...` — all tests pass
- [ ] Run `cd frontend && yarn build && yarn test` — no errors
- [ ] **Manual QA** (follow the test plan in `requirements.md`):
  - [ ] Register account and complete onboarding at `http://localhost:8080`
  - [ ] Add a repository to the org
  - [ ] Create an agent, enable Code Intelligence skill (no config dialog)
  - [ ] Chat with the agent about code in the repository
  - [ ] Confirm Kodit tool calls appear in the session trace and the answer is correct
