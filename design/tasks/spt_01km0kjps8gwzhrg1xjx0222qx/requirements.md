# Requirements: Report Issue Feature (Helix Mac App)

## Background

Users of the Helix Mac app sometimes encounter bugs they can't easily diagnose. We need a way for users to send us diagnostic information alongside a bug report, similar to how macOS apps like Docker Desktop, OrbStack, and Feedback Assistant work.

---

## User Stories

### US-1: Report Issue Button in Settings
**As a** Helix Mac app user
**I want to** click a "Report Issue" button in the Settings panel
**So that** I can easily report bugs without hunting for a GitHub link

**Acceptance Criteria:**
- A "Report Issue" button appears in the Settings panel, near the Danger Zone section
- The button is visually distinct from destructive actions (not red)
- Clicking the button opens a Report Issue dialog/modal

---

### US-2: Automatic Diagnostic Collection
**As a** Helix Mac app user reporting a bug
**I want** the app to automatically collect relevant system and app logs
**So that** I don't have to manually find and copy logs

**Acceptance Criteria:**
- The report includes Mac hardware info: macOS version, CPU type (Apple Silicon vs Intel), CPU count, RAM
- The report includes the Helix app version and VM version
- The report includes recent VM console logs (kernel/boot output)
- The report includes recent SSH/command logs from within the VM
- The report includes recent container logs fetched from the running VM via SSH (helix API container, helix worker)
- All logs are truncated to a sensible length (last ~200 lines per source) to keep reports manageable

---

### US-3: User Review Before Submission
**As a** Helix Mac app user
**I want to** see what diagnostic data will be included before I submit
**So that** I can review it for sensitive information

**Acceptance Criteria:**
- The dialog shows all collected diagnostics in a scrollable text area before submission
- The user can add a description of the issue
- The user can choose to cancel and not submit anything

---

### US-4: Submission via GitHub Issues
**As a** Helix Mac app user
**I want to** submit my report to GitHub Issues
**So that** the Helix team can track and respond to it

**Acceptance Criteria:**
- Clicking "Submit" copies the formatted diagnostic report to clipboard
- The app opens `https://github.com/helixml/helix/issues/new` in the default browser
- A toast message tells the user "Diagnostics copied to clipboard — paste them into the issue form"
- This avoids needing any backend submission endpoint

---

## Out of Scope

- Automatic crash reporting (no Sentry integration for the Mac app)
- Email-based submission
- Attaching screenshots or video
- Sending logs to a Helix-owned backend endpoint (future work)
