# Fix Chrome viewport width when launched by chrome-devtools MCP

## Summary
Chrome launched by the chrome-devtools MCP started at ~920px wide, too narrow for desktop-view websites. The existing viewport/headless settings were passed as environment variables (`CHROME_DEVTOOLS_MCP_VIEWPORT`, `CHROME_DEVTOOLS_MCP_HEADLESS`) which chrome-devtools-mcp doesn't read — they must be CLI arguments.

## Changes
- Added `--viewport 1600x1080` as a CLI argument to the chrome-devtools MCP server config in `zed_config.go`
- Removed non-functional `CHROME_DEVTOOLS_MCP_HEADLESS` and `CHROME_DEVTOOLS_MCP_VIEWPORT` env vars (kept `CHROME_PATH` which Puppeteer does read)
- Used 1600px width (not 1920) so Zed remains visible behind Chrome on a 1920x1080 screen

## Screenshots
Before (default ~920px viewport):
![Before](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001532_can-we-make-chrome-start/screenshots/02-before-default-800x600.png)

After (1600x1080 viewport):
![After](https://github.com/helixml/helix/raw/helix-specs/design/tasks/001532_can-we-make-chrome-start/screenshots/03-1600x1080.png)
