# Requirements: PR Open Notification & Browser Notification Auto-Read

## User Stories

### 1. Pull Request Opened Notification
**As a** developer using Helix,
**I want** to see a notification when Helix opens a pull request on an external provider (GitHub, GitLab, etc.),
**So that** I can immediately jump to the PR to review or share it.

### 2. External PR Link Opens in New Tab
**As a** developer viewing the Helix notifications panel,
**I want** clicking a PR-opened notification to open the external pull request URL in a new browser tab,
**So that** I can view the PR on GitHub/GitLab without leaving my place in Helix.

### 3. Browser Notification Click Marks as Read
**As a** developer using desktop browser notifications,
**I want** clicking a browser notification (e.g. "Agent finished working", "Spec ready for review") to mark that notification as read,
**So that** the notification badge and unread state are cleared after I've acted on it.

---

## Acceptance Criteria

### PR Opened Notification
- [ ] A new attention event type `pr_opened` exists in the system
- [ ] The event is emitted when Helix successfully creates a PR on an external provider
- [ ] The notification title is "Pull request opened" with a description including the repo/task name
- [ ] The event metadata includes `pr_url` (the external PR URL) and `pr_id`
- [ ] The notification appears in the Helix notifications panel with a distinct icon (e.g. `GitPullRequest`)
- [ ] The notification has a distinct color in the panel (e.g. purple/indigo, distinct from the existing `pr_ready` type)
- [ ] Clicking the notification opens the external `pr_url` in a **new browser tab** (not navigating within Helix)
- [ ] The same external-link behavior applies when the PR URL is available in the `pr_ready` event

### Browser Notification Auto-Read
- [ ] When a user clicks a browser/desktop notification and is taken to a Helix page, that notification is automatically marked as acknowledged
- [ ] The notification badge count updates immediately after click
- [ ] This applies to all notification types that trigger browser notifications (agent completed, spec ready, etc.)
- [ ] Notifications dismissed via the X button in the panel are not affected (existing behavior unchanged)
