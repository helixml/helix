# Design: Agent Screenshot Testing Capability

## Summary

Add instructions to planning and implementation prompts telling agents they can use Chrome DevTools MCP for browser automation and helix-desktop MCP for screenshots to self-test their work.

## Architecture

### Available MCP Tools

**helix-desktop** (already configured in `zed_config.go`):
- `take_screenshot` - Returns base64-encoded PNG
- `save_screenshot` - Saves to file, returns path
- `list_windows` - List all windows with IDs
- `focus_window` - Bring window to front

**chrome-devtools** (already configured in `zed_config.go`):
- Navigation: `navigate`, `goBack`, `goForward`, `reload`
- DOM inspection: `querySelector`, `getElement`, `getComputedStyle`
- Input: `click`, `type`, `scroll`
- Screenshots: `screenshot` (page-level)

### Prompt Changes

Two files need modification:

1. **`api/pkg/services/spec_task_prompts.go`** - Planning prompt
   - Add section explaining screenshot/testing tools
   - Instruct agent to take screenshots during discovery if relevant

2. **`api/pkg/services/agent_instruction_service.go`** - Implementation prompt
   - Add section on self-testing UI changes
   - Explain screenshot workflow and file naming
   - Instruct agent to reference screenshots in PR

### Screenshot Workflow

```
1. Agent makes UI change
2. Start dev server / open app in browser (if not running)
3. Use chrome-devtools to navigate to relevant page
4. Use helix-desktop list_windows to find browser
5. Use helix-desktop focus_window to bring browser to front
6. Use helix-desktop save_screenshot with descriptive path
7. Reference screenshot in design docs
```

### File Naming Convention

```
/home/retro/work/helix-specs/design/tasks/{task-dir}/screenshots/
├── 01-description.png
├── 02-description.png
└── 03-description.png
```

Examples:
- `01-homepage-before-changes.png`
- `02-homepage-after-dark-mode.png`
- `03-error-state-validation.png`

## Key Decisions

1. **Use save_screenshot over take_screenshot** - Returns file path, simpler for agent
2. **Screenshots in spec folder, not code repo** - Keeps evidence with design docs
3. **Numbered prefixes** - Shows chronological order
4. **PNG format** - Better quality for UI screenshots
5. **Focus window before screenshot** - Ensures content is visible (not minimized/behind)

## Open Question

The user noted uncertainty about whether window visibility is required. Based on code review:
- `helix-desktop` uses the desktop-bridge which captures the actual screen
- Window MUST be visible (not minimized, not behind other windows)
- Agent should use `focus_window` before `save_screenshot`

## Prompt Section to Add

```markdown
## Visual Testing (Optional - Use When Relevant)

You have tools to test UI changes visually:

**Browser automation:** `chrome-devtools` MCP
- Navigate: `navigate`, `reload`
- Interact: `click`, `type`, `scroll`

**Screenshots:** `helix-desktop` MCP
- `save_screenshot` - Save screenshot to file
- `list_windows` / `focus_window` - Ensure window is visible first

**When to use:** Frontend changes, UI bugs, visual features

**Screenshot workflow:**
1. Run the app (dev server, open browser)
2. Navigate to the relevant page
3. `list_windows` → find browser window ID
4. `focus_window` → bring browser to front
5. `save_screenshot` with path like:
   `/home/retro/work/helix-specs/design/tasks/{task-dir}/screenshots/01-description.png`
6. Reference screenshots in design.md or PR description

Screenshots are optional but valuable for UI work - they prove the change works.
```
