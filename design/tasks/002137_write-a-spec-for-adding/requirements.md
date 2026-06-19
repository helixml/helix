# Requirements: Add Export-to-CSV on the App Usage (Reports) Tab

## Background

The **Usage** tab on each agent/app detail page (`AppUsage.tsx`) shows interactions, token counts, and cost metrics for a selected time period. Users have no way to take that data offline. The org-level Usage page already has CSV/JSON export; this spec brings the same capability to the per-app level.

## User Stories

**US-1 — Export interactions as CSV**
As an app owner or org admin viewing an agent's Usage tab, I want to download the current interactions table as a CSV file so I can analyse or share the data outside Helix.

**US-2 — Export respects active filters**
As a user who has filtered the interactions list (by period, search query, or feedback), I want the exported CSV to contain only the rows currently shown, so the download matches what I see on screen.

## Acceptance Criteria

| # | Criterion |
|---|-----------|
| AC-1 | An **Export CSV** button appears in the Usage tab header, next to the existing Refresh button. |
| AC-2 | Clicking the button triggers a browser file download. |
| AC-3 | The downloaded file is named `app-<appId>-interactions-<YYYY-MM-DD>.csv`. |
| AC-4 | The CSV includes a header row and one data row per displayed interaction. |
| AC-5 | Columns: `interaction_id`, `session_id`, `created`, `completed`, `state`, `feedback`, `prompt`, `duration_ms`, `total_tokens`, `total_cost`. |
| AC-6 | The button is disabled (greyed out) when there are no interactions loaded. |
| AC-7 | String values containing commas or quotes are properly escaped per RFC 4180. |
| AC-8 | No backend changes are required; the export is generated entirely client-side. |
