# Requirements: Chrome MCP Testing Documentation

## User Story

As a planning/implementation agent working on web application tasks, I want clear guidance that Chrome MCP is available for browser-based testing, so that I can reproduce bugs and verify fixes interactively during both planning and implementation phases.

## Background

The Chrome DevTools MCP server (`chrome-devtools`) is available locally in agent environments. Currently, the planning and implementation prompts mention it briefly under "Visual Testing" but don't emphasize its use for:
- Bug reproduction during planning (understanding the issue before designing a fix)
- Fix verification during implementation (testing that code changes work)

## Acceptance Criteria

1. **Planning prompt updated**: The planning prompt in `spec_task_prompts.go` includes a note that for web app bug/feature tasks, agents should attempt to reproduce issues via Chrome MCP before writing specs

2. **Implementation prompt updated**: The implementation prompt in `agent_instruction_service.go` includes a note that agents should test fixes via Chrome MCP after making code changes

3. **Non-intrusive**: Changes are additions to existing sections, not restructuring - keeps the prompts concise

4. **Conditional guidance**: Notes clarify this applies to web/frontend tasks (not backend-only or CLI tasks)

## Out of Scope

- Adding new MCP tools or capabilities
- Changing Chrome MCP configuration
- Mandatory testing requirements (guidance only)