# Implementation Tasks

- [ ] In `api/pkg/external-agent/zed_config.go`, add `"--viewport", "1920x1080"` to the `Args` slice for the `chrome-devtools` context server
- [ ] Remove `CHROME_DEVTOOLS_MCP_HEADLESS` and `CHROME_DEVTOOLS_MCP_VIEWPORT` from the `Env` map (keep `CHROME_PATH`)
