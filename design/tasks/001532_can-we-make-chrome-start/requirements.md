# Requirements: Chrome MCP Default Viewport Width

## User Story

As a user of Helix Desktop, when the chrome-devtools MCP launches Chrome, I want it to start with a wide enough viewport to display desktop-view websites, so I don't have to manually resize the window or deal with mobile-view layouts.

## Problem

Chrome launched by the chrome-devtools MCP starts with too narrow a default viewport (~800x600). Most websites require at least 1280px width to show desktop view. The current configuration in `zed_config.go` sets `CHROME_DEVTOOLS_MCP_VIEWPORT=1920x1080` as an environment variable, but chrome-devtools-mcp does NOT read this env var — it must be passed as a CLI argument `--viewport 1920x1080`.

## Acceptance Criteria

- When chrome-devtools MCP launches Chrome, the initial viewport is at least 1920x1080
- Desktop-view websites (e.g. GitHub, Google) render in desktop mode by default
- Both headless and non-headless Chrome launches use the wider viewport
