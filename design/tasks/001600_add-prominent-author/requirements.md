# Requirements: Add Prominent Author Field in SpecTask Details

## User Stories

**As a project member**, I want to see who created a spectask when viewing its details, so I can know who to contact about the task's origin or intent.

**As a team lead**, I want the author to be visually prominent in the task detail panel, so I can quickly attribute tasks without digging through audit logs.

## Acceptance Criteria

1. The `created_by` field (already present in the API response) is displayed in the spectask detail side panel.
2. The author is shown prominently — not buried in a debug section — alongside the creation timestamp.
3. If `created_by` is empty or missing, the field is omitted gracefully (no "N/A" or empty row).
4. The display is consistent with the existing MUI `Typography` caption style used for timestamps.
5. No backend changes are required — the field is already returned by the API.
