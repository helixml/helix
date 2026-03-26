Add "Report Issue" button to Mac app Settings panel

## Summary
Adds a "Report Issue" button to the Settings panel in the Helix Mac app that collects diagnostic information and opens a pre-filled GitHub issue. Users can review the collected data before submitting.

## Changes
- `for-mac/app.go`: Added `DiagnosticReport` struct, `collectSystemInfo()` helper (macOS version, arch, CPU, RAM via `sw_vers`/`sysctl`), and `CollectDiagnostics()` method that gathers system info, app/VM versions, VM console/SSH logs (last 200 lines each), and container logs fetched via SSH (10s timeout)
- `for-mac/frontend/src/components/SettingsPanel.tsx`: Added "Support" section above Danger Zone with a "Report Issue" button; clicking it opens a modal that shows a loading spinner while diagnostics are collected, then displays the report in a read-only text area alongside an optional description field; "Submit on GitHub" copies everything to clipboard and opens `github.com/helixml/helix/issues/new`
- `for-mac/frontend/wailsjs/go/main/App.d.ts`, `App.js`, `models.ts`: Manually added `CollectDiagnostics` binding and `DiagnosticReport` TypeScript type

## Testing
Build and run `wails dev` in `for-mac/`, open Settings, scroll to the new "Support" section, and click "Report Issue".
