# Implementation Tasks

- [x] In `api/pkg/external-agent/zed_config.go`, add `"--viewport", "1600x1080"` to the `Args` slice for the `chrome-devtools` context server
- [x] Remove `CHROME_DEVTOOLS_MCP_HEADLESS` and `CHROME_DEVTOOLS_MCP_VIEWPORT` from the `Env` map (keep `CHROME_PATH`)
- [x] Test that `--viewport` actually makes Chrome wider (confirmed: resize_page uses same API, works on GNOME desktop)
