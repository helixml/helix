# Requirements: Chrome MCP Default Viewport Width

## User Story

As a user of Helix Desktop, when the chrome-devtools MCP launches Chrome, I want it to start with a wide enough viewport to display desktop-view websites, so I don't have to manually resize the window or deal with mobile-view layouts.

## Problem

Chrome launched by the chrome-devtools MCP starts with too narrow a default window (~800x600). Most websites require at least 1280px width to show desktop view. Despite an existing intent to set the viewport to 1920x1080, it is not being applied correctly (see design.md).

## Acceptance Criteria

- When chrome-devtools MCP launches Chrome, the initial viewport is at least 1920x1080
- Desktop-view websites (e.g. GitHub, Google) render in desktop mode by default
- Both headless and non-headless Chrome launches use the wider viewport
