# Implementation Tasks

Tests are written first (TDD). Each implementation task is paired with the test that drives it.

## Types & Skill Definition

- [x] Add `YAMLSkillMCPSpec` struct and `MCP` field to `YAMLSkillSpec` in `api/pkg/types/skill.go`
- [x] Write `manager_test.go` test: loading `code-intelligence.yaml` produces `Spec.MCP.AutoProvision == true`
- [x] Create `api/pkg/agent/skill/api_skills/code-intelligence.yaml` (make the test pass)

## Enable Endpoint (backend)

- [x] Write `skills_test.go` unit tests (testify/suite + gomock):
  - `POST /api/v1/apps/{id}/skills/code-intelligence/enable` returns 200, `mcpTools` contains Kodit URL + user API key in `Authorization` header
  - Returns error when no Kodit URL is configured in platform config
- [x] Implement `POST /api/v1/apps/{id}/skills/{skillName}/enable` handler (make tests pass)

## E2E / Integration Tests

- [x] Write suite-based integration test using `NewTestServer` + mock store: enable endpoint produces an `AssistantMCP` that `NewDirectMCPClientSkills` can consume
- [x] Extend `api/pkg/server/mcp_backend_kodit_test.go`: verify an app with a Code Intelligence MCP tool routes calls to the Kodit backend correctly

## Frontend

- [x] Write Vitest test: `isAutoProvisionMCPSkill` helper correctly identifies autoProvision skills
- [x] Implement frontend marketplace changes to support `autoProvision` one-click enable (make test pass)

## Verification

- [x] Run `go build ./pkg/server/ ./pkg/types/ ./pkg/agent/skill/...` — no compile errors
- [x] Run `CGO_ENABLED=1 go test -v ./pkg/server/ ./pkg/agent/skill/...` — all tests pass
- [x] Run `cd frontend && npx tsc --noEmit && yarn test` — no errors
- [x] **Manual QA** (follow the test plan in `requirements.md`):
  - [x] Register account and complete onboarding at `http://localhost:8080`
  - [x] Create an agent, enable Code Intelligence skill (no config dialog — enabled with single click)
  - [x] Confirmed: skill auto-provisioned URL `http://localhost:8080/api/v1/mcp/kodit`, shows "ENABLED" button, no dialog appeared
