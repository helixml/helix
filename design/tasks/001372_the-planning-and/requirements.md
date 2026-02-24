# Requirements: Primary Repository Highlighting in Spectask Prompts

## Problem Statement

When multiple repositories are attached to a project, agents don't clearly understand which repository is the PRIMARY target for work vs which are for reference or dependent changes. This causes work to be done in the wrong repository.

**Example:** User attaches "website" and "launchpad" repos, asks for a simple change. Agent makes the change in launchpad instead of website because it wasn't clear that website was the primary repo.

## User Stories

### US-1: Clear Primary Repo Indication in Planning Phase
As an AI agent in the planning phase, I need to know which repository is the PRIMARY target for my work so that I analyze and plan changes for the correct codebase.

**Acceptance Criteria:**
- Planning prompt explicitly states which repo is PRIMARY vs reference/dependent
- Primary repo is visually distinguished (e.g., marked with "(PRIMARY)" or similar)
- Reference repos are clearly labeled as "for reference or dependent changes"

### US-2: Clear Primary Repo Indication in Implementation Phase
As an AI agent in the implementation phase, I need clear guidance that my main code changes go to the PRIMARY repository so that I don't accidentally modify reference repositories.

**Acceptance Criteria:**
- Implementation prompt explicitly names the primary repository
- The distinction between primary and reference repos is stated early in the prompt
- Guidance clarifies when it's OK to touch reference repos (dependent changes only)

## Out of Scope

- Changing how primary repo is set in the UI (already exists via `DefaultRepoID`)
- Multi-primary repo scenarios (one primary is the design)
- Auto-detection of which repo should be primary