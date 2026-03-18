# Implementation Tasks

## Go Backend (`for-mac/app.go`)

- [ ] Add `DiagnosticReport` struct with fields: `SystemInfo`, `AppVersion`, `VMVersion`, `VMState`, `ConsoleLogs`, `SSHLogs`, `ContainerLogs`
- [ ] Implement `collectSystemInfo() string` helper using `sw_vers` and `sysctl` shell commands
- [ ] Implement `CollectDiagnostics() (DiagnosticReport, error)` method on `App` struct
- [ ] In `CollectDiagnostics`, fetch container logs via SSH (`docker logs helix-api-1 --tail 100` and `helix-worker-1`) with 10s timeout, gracefully handle VM-not-running case
- [ ] Truncate console and SSH log buffers to last 200 lines before including in report
- [ ] Run `wails generate bindings` to regenerate `frontend/wailsjs/go/main/App.d.ts` and `models.ts`

## React Frontend (`for-mac/frontend/src/components/SettingsPanel.tsx`)

- [ ] Add state variables: `reportDialogOpen`, `diagnosticsLoading`, `diagnosticsReport`, `userDescription`
- [ ] Add `handleReportIssue` handler that opens dialog and calls `CollectDiagnostics()`
- [ ] Add `handleSubmitReport` handler that formats report, copies to clipboard via `ClipboardSetText`, opens `https://github.com/helixml/helix/issues/new` via `BrowserOpenURL`, shows toast, and closes dialog
- [ ] Add "Support" section above the Danger Zone section with a "Report Issue" button (styled as `btn btn-secondary`)
- [ ] Add Report Issue modal dialog with: loading spinner while collecting, read-only scrollable diagnostics text area, optional user description text area, "Submit on GitHub" and "Cancel" buttons
