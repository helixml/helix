# Requirements: Agent Editor Tab Switching Loses Settings

## Problem

When a user changes a setting on one tab of the agent editor (e.g., resolution on Settings tab), then switches to another tab (e.g., Appearance) and makes a change there, the first change is silently lost. There is no warning and no auto-save protection.

## User Stories

1. **As a user**, when I change resolution on the Settings tab and then change the agent name on the Appearance tab, I expect both changes to be saved — not just the last one.

2. **As a user**, if switching tabs would lose my unsaved changes, I expect to either be warned or have my changes auto-saved before the switch.

## Acceptance Criteria

- [ ] Changing resolution on Settings → switching to Appearance → changing name → switching back to Settings: resolution change is preserved
- [ ] This applies to ALL settings across ALL tabs, not just resolution
- [ ] No silent data loss when switching between any combination of tabs
- [ ] Existing save-on-change behavior continues to work (settings that already auto-save should keep working)
