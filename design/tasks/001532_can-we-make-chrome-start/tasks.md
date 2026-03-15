# Implementation Tasks

- [ ] In `api/pkg/external-agent/zed_config.go`, move `--viewport 1920x1080` and `--headless` from `Env` map to `Args` slice for the `chrome-devtools` context server
- [ ] Remove `CHROME_DEVTOOLS_MCP_HEADLESS` and `CHROME_DEVTOOLS_MCP_VIEWPORT` from the `Env` map (keep `CHROME_PATH`)
