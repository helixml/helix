# Requirements: Show Sandbox Version in Admin UI

## User Stories

### US-1: View sandbox versions
As an admin, I want to see what version each sandbox is running so I can identify outdated sandboxes.

### US-2: Version mismatch alert
As an admin, I want to be alerted when a sandbox version doesn't match the control plane version so I can take action to update it.

## Acceptance Criteria

### AC-1: Sandbox reports version in heartbeat
- [ ] `sandbox-heartbeat` daemon reports its Helix version (built-in at compile time via `data.GetHelixVersion()`)
- [ ] Version stored in `sandbox_instances` table alongside existing `desktop_versions`

### AC-2: Version displayed in sandbox dropdown
- [ ] Sandbox selector in admin UI shows version for each sandbox
- [ ] Format: `{hostname} (v{short_version}) - {sessions}`
- [ ] Version truncated to first 7 chars if it's a git hash

### AC-3: Version mismatch alert
- [ ] Global alert banner shown when any online sandbox has a different version than control plane
- [ ] Alert lists affected sandbox(es) by hostname/ID
- [ ] Alert severity: warning (not error)
- [ ] Alert dismissible but reappears on page refresh if mismatch persists

### AC-4: API exposes version
- [ ] `GET /api/v1/sandboxes` returns `helix_version` field for each sandbox
- [ ] `GET /api/v1/config` already returns control plane version (existing)

## Out of Scope
- Auto-updating sandboxes
- Version compatibility matrix
- Multiple control plane version support