# Requirements: Better Pull Request Titles and Descriptions

## Problem Statement

When PRs are created, they use `task.Name` as title and a simple `"> **Helix**: " + task.Description` as body. These often come from the original user prompt which is informal/verbose. We can't use an LLM at PR creation time because Claude Code subscriptions only have agent access (no direct LLM API). However, the agent IS running during implementation and HAS LLM capability.

## User Stories

### US-1: Agent writes PR content during implementation
As a developer, when my implementation is complete, I want the PR to have a clear, professional title and description that summarizes what was actually implemented (not just the original informal prompt).

### US-2: Fallback for missing PR file
As a system, when no `pull_request.md` file exists, I want to fall back to the current behavior (task.Name + task.Description) so existing workflows don't break.

## Acceptance Criteria

### AC-1: Agent writes pull_request.md
- [ ] Update agent prompts to instruct writing `pull_request.md` in the task's helix-specs directory
- [ ] File location: `/home/retro/work/helix-specs/design/tasks/{task_dir}/pull_request.md`
- [ ] Simple format: First line = title, rest = description body

### AC-2: Backend reads pull_request.md when creating PR
- [ ] When creating a PR, check for `pull_request.md` in helix-specs branch
- [ ] Parse first line as title, remaining content as description
- [ ] If file exists and parses successfully, use it instead of task.Name/Description

### AC-3: Graceful fallback
- [ ] If file doesn't exist or can't be parsed, use current behavior
- [ ] Log when using fallback vs custom content for debugging

### AC-4: File format
- [ ] First line (after optional `# `) = PR title
- [ ] Everything after first blank line = PR description (markdown)
- [ ] Keep it simple - no YAML frontmatter needed