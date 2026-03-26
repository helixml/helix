# Implementation Tasks

## Go Backend (`for-mac/app.go`)

- [x] Add `DiagnosticReport` struct with fields: `SystemInfo`, `AppVersion`, `VMVersion`, `VMState`, `ConsoleLogs`, `SSHLogs`, `ContainerLogs`
- [x] Implement `collectSystemInfo() string` helper using `sw_vers` and `sysctl` shell commands
- [x] Implement `CollectDiagnostics() (DiagnosticReport, error)` method on `App` struct
- [x] In `CollectDiagnostics`, fetch container logs via SSH (`docker compose logs api/worker --tail 100`) with 10s timeout, gracefully handle VM-not-running case
- [x] Truncate console and SSH log buffers to last 200 lines before including in report
- [x] Manually updated `App.d.ts`, `App.js`, and `models.ts` wailsjs bindings (wails CLI not available in build env)

## React Frontend (`for-mac/frontend/src/components/SettingsPanel.tsx`)

- [x] Add state variables: `reportDialogOpen`, `diagnosticsLoading`, `diagnosticsReport`, `userDescription`
- [x] Add `handleReportIssue` handler that opens dialog and calls `CollectDiagnostics()`
- [x] Add `handleSubmitReport` handler that formats report, copies to clipboard via `ClipboardSetText`, opens `https://github.com/helixml/helix/issues/new` via `BrowserOpenURL`, shows toast, and closes dialog
- [x] Add "Support" section above the Danger Zone section with a "Report Issue" button (styled as `btn btn-secondary`)
- [x] Add Report Issue modal dialog with: loading spinner while collecting, read-only scrollable diagnostics text area, optional user description text area, "Submit on GitHub" and "Cancel" buttons
