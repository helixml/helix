# Implementation Tasks

- [ ] Create `helix/api/pkg/cli/spectask/revdial_cmd.go` with `newRevDialCommand()` function
- [ ] Implement session lookup (GET `/api/v1/sessions/{id}`) to verify session exists and is running
- [ ] Implement screenshot test with timing (reuse pattern from `testScreenshot()` in `test_cmd.go`)
- [ ] Add `--json` flag support with structured output matching existing `TestResult` pattern
- [ ] Add `--timeout` flag (default 15s) for configuring request timeout
- [ ] Register command in `spectask.go` via `cmd.AddCommand(newRevDialCommand())`
- [ ] Test manually with a running session: `helix spectask revdial ses_xxx`
- [ ] Verify JSON output: `helix spectask revdial ses_xxx --json`
