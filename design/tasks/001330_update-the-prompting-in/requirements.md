# Requirements: Agent Screenshot Testing Capability

## Overview

Update the planning and implementation prompts to instruct agents that they can and should test changes themselves using browser automation and screenshot tools. Screenshots should be saved to the spec-tasks folder and referenced in final specs and pull requests.

## User Stories

### US-1: Agent Self-Testing
As an AI agent, I want to test my UI/frontend changes visually so that I can verify they work before requesting human review.

**Acceptance Criteria:**
- Agent knows it has access to Chrome DevTools MCP for browser automation
- Agent knows it has access to helix-desktop MCP for screenshots
- Agent understands when visual testing is appropriate (UI changes, frontend work)

### US-2: Screenshot Documentation
As a reviewer, I want screenshots of the agent's work included in the spec folder so that I can see visual proof of what was implemented.

**Acceptance Criteria:**
- Screenshots saved to `/home/retro/work/helix-specs/design/tasks/{task-dir}/screenshots/`
- Filenames are descriptive (e.g., `01-login-form-before.png`, `02-login-form-after.png`)
- Screenshots referenced in design.md or a dedicated `screenshots.md` file
- PR description includes references to screenshots

### US-3: Window Visibility for Screenshots
As an agent, I need to ensure the browser window is visible before taking screenshots so that the screenshots capture actual content.

**Acceptance Criteria:**
- Agent uses `list_windows` to find browser window
- Agent uses `focus_window` to bring browser to front before screenshot
- Agent understands potential visibility issues and workarounds

## Out of Scope

- Automated visual regression testing
- Screenshot comparison tools
- Video recording of testing sessions

## Technical Notes

- Chrome DevTools MCP: `chrome-devtools` context server (26 browser automation tools)
- Desktop MCP: `helix-desktop` context server (take_screenshot, save_screenshot, list_windows, focus_window)
- Screenshots should be PNG format for quality
- Agent should test in both planning (for discovery) and implementation (for verification) phases