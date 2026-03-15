# Design: Chrome MCP Default Viewport Width

## Root Cause

In `api/pkg/external-agent/zed_config.go`, the chrome-devtools MCP server is configured with viewport and headless settings as **environment variables**:

```go
config.ContextServers["chrome-devtools"] = ContextServerConfig{
    Command: "npx",
    Args:    []string{"chrome-devtools-mcp@latest"},
    Env: map[string]string{
        "CHROME_DEVTOOLS_MCP_HEADLESS": "true",
        "CHROME_DEVTOOLS_MCP_VIEWPORT": "1920x1080",
        "CHROME_PATH": "/usr/bin/google-chrome-stable",
    },
}
```

However, `chrome-devtools-mcp` only reads **two** env vars (`CHROME_DEVTOOLS_MCP_NO_USAGE_STATISTICS`, `CHROME_DEVTOOLS_MCP_CRASH_ON_UNCAUGHT`). `VIEWPORT` and `HEADLESS` must be **CLI arguments**.

In `browser.js` (chrome-devtools-mcp source), the viewport is only applied if `serverArgs.viewport` is set, which comes from the `--viewport` CLI arg. Without it, `defaultViewport: null` is set and no resize occurs, leaving Chrome at its ~800x600 default.

## Fix

Move `--viewport` and `--headless` from `Env` to `Args`:

```go
config.ContextServers["chrome-devtools"] = ContextServerConfig{
    Command: "npx",
    Args: []string{
        "chrome-devtools-mcp@latest",
        "--viewport", "1920x1080",
        "--headless",
    },
    Env: map[string]string{
        "CHROME_PATH": "/usr/bin/google-chrome-stable",
    },
}
```

Note: `CHROME_PATH` stays as an env var — Puppeteer reads it to locate the Chrome binary.

## Key Findings

- `chrome-devtools-mcp` uses yargs for CLI args and does NOT call `.env()` to auto-map env vars to options
- `--viewport 1920x1080` calls `page.resize({ contentWidth: 1920, contentHeight: 1080 })` on the first Chrome page
- `--headless` adds `--screen-info={3840x2160}` to Chrome's args (for large headless canvas)
- `CHROME_PATH` IS read by Puppeteer internally, so it remains as an env var
- The `CHROME_DEVTOOLS_MCP_VIEWPORT` and `CHROME_DEVTOOLS_MCP_HEADLESS` env vars are currently set system-wide but have no effect
