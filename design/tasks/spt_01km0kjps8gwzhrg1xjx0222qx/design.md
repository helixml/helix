# Design: Report Issue Feature (Helix Mac App)

## Architecture

This is a Wails v2 app (Go backend + React/TypeScript frontend). The existing pattern for new features is:
1. Add a Go method on `App` struct in `app.go`
2. Run `wails generate bindings` to regenerate `frontend/wailsjs/go/main/App.d.ts`
3. Add UI in the relevant React component (here: `SettingsPanel.tsx`)

---

## How Existing Mac Apps Handle This

- **Docker Desktop**: "Report Bug" in the troubleshoot menu — collects daemon logs and opens a pre-filled GitHub issue form
- **OrbStack**: "Send Feedback" — clipboard + browser open pattern
- **Apple Feedback Assistant**: Full structured form — overkill for us
- **Chosen approach**: Clipboard + GitHub Issues URL (simplest, no backend needed, consistent with existing `Report Issue` menu item in `main.go` line 118)

---

## Data to Collect

### Mac System Info (Go, using `os/exec` or `syscall`)
```
macOS version: 14.4.1 (Sonoma)
CPU: Apple M2 Pro (arm64), 12 cores
RAM: 32 GB
```
- `sw_vers -productVersion` → macOS version
- `sysctl -n machdep.cpu.brand_string` or check `runtime.GOARCH` for arm64
- `sysctl -n hw.memsize` → RAM bytes

### App Info (already available in Go)
```
Helix App Version: 0.9.3
VM Version: 0.9.1
VM State: running
```
- `version.Version` (already used in SettingsPanel)
- `AppSettings.InstalledVMVersion`
- `VMStatus.State`

### VM Console Logs (already available)
- `vm.GetConsoleOutput()` returns `vm.consoleBuf` (ring buffer ~200KB)
- Truncate to last 200 lines

### VM SSH/Command Logs (already available)
- `vm.GetLogsOutput()` returns `vm.logsBuf` (ring buffer ~200KB)
- Truncate to last 200 lines

### Container Logs (new — fetched via SSH into VM)
When VM is running, SSH in and run:
```bash
docker logs helix-api-1 --tail 100 2>&1
docker logs helix-worker-1 --tail 100 2>&1
```
- Use existing SSH infrastructure in `vm.go` (`runSSHCommand` pattern)
- If VM is not running or SSH fails, include an error note instead
- Timeout: 10 seconds

---

## Go Implementation

### New method in `app.go`:

```go
type DiagnosticReport struct {
    SystemInfo    string `json:"system_info"`
    AppVersion    string `json:"app_version"`
    VMVersion     string `json:"vm_version"`
    VMState       string `json:"vm_state"`
    ConsoleLogs   string `json:"console_logs"`
    SSHLogs       string `json:"ssh_logs"`
    ContainerLogs string `json:"container_logs"`
}

func (a *App) CollectDiagnostics() (DiagnosticReport, error)
```

The method gathers all sections, returns structured data. Container log fetching happens in a goroutine with a timeout.

### Formatted output (for clipboard):

```
=== Helix Diagnostic Report ===
Generated: 2025-01-15 14:32:00

=== System Info ===
macOS Version: 14.4.1
Architecture: arm64
CPU Cores: 12
RAM: 32 GB

=== App Info ===
App Version: 0.9.3
VM Version: 0.9.1
VM State: running

=== VM Console Logs (last 200 lines) ===
[... logs ...]

=== VM Command Logs (last 200 lines) ===
[... logs ...]

=== Container Logs: helix-api ===
[... logs ...]

=== Container Logs: helix-worker ===
[... logs ...]
```

---

## Frontend Implementation

### Button placement in `SettingsPanel.tsx`

Add a new **"Support"** section above the existing "Danger Zone" section:

```tsx
{/* Support Section */}
<div className="settings-section">
  <h3>Support</h3>
  <p>Collect diagnostic information and report a bug on GitHub.</p>
  <button className="btn btn-secondary" onClick={handleReportIssue}>
    Report Issue
  </button>
</div>
```

### Report Issue Dialog

A modal (using existing dialog pattern from factory reset confirmation) with:
1. Loading state while `CollectDiagnostics()` runs
2. Text area showing formatted diagnostics (read-only, scrollable)
3. Text area for user description (optional)
4. "Submit on GitHub" button → copies to clipboard + opens browser
5. "Cancel" button

```tsx
const handleReportIssue = async () => {
  setReportDialogOpen(true);
  setDiagnosticsLoading(true);
  try {
    const report = await CollectDiagnostics();
    setDiagnosticsReport(formatReport(report));
  } catch (err) {
    setDiagnosticsReport("Failed to collect diagnostics: " + err);
  } finally {
    setDiagnosticsLoading(false);
  }
};

const handleSubmitReport = async () => {
  const fullReport = `${userDescription}\n\n${diagnosticsReport}`;
  await ClipboardSetText(fullReport);  // Wails runtime clipboard
  await BrowserOpenURL("https://github.com/helixml/helix/issues/new");
  showToast("Diagnostics copied to clipboard — paste into the GitHub issue form");
  setReportDialogOpen(false);
};
```

---

## Key Files to Modify

| File | Change |
|------|--------|
| `for-mac/app.go` | Add `CollectDiagnostics()` method and `DiagnosticReport` struct |
| `for-mac/frontend/src/components/SettingsPanel.tsx` | Add Support section + Report Issue dialog |
| `for-mac/frontend/wailsjs/go/main/App.d.ts` | Auto-generated (run `wails generate bindings`) |
| `for-mac/frontend/wailsjs/go/models.ts` | Auto-generated (same command) |

---

## Codebase Patterns Discovered

- **Settings section pattern**: Each section in `SettingsPanel.tsx` is a `<div>` with a heading, description, and controls. Consistent padding/margin via CSS classes.
- **Confirmation dialogs**: `confirmReset` state pattern — show inline confirmation instead of using browser `confirm()`. Same pattern should be used for the report dialog (just open state).
- **Toast notifications**: `showToast(message)` helper already available in SettingsPanel.
- **Clipboard**: `ClipboardSetText` imported from `../wailsjs/runtime/runtime` — already used for copy-URL features.
- **Browser open**: `BrowserOpenURL` from Wails runtime — already used in Settings for "Open in browser".
- **SSH commands**: `vm.go` has `runSSHCommand(cmd string) (string, error)` pattern — reuse for fetching container logs.
- **System tray menu**: `main.go` line 118 already has a `Report Issue` menu item linking to GitHub. The new button is the in-app equivalent of this.
