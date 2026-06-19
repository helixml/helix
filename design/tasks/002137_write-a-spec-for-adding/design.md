# Design: Add Export-to-CSV on the App Usage (Reports) Tab

## Context

The feature lives entirely in `frontend/src/components/app/AppUsage.tsx`. No API changes are needed — the interactions data is already fetched via `useListAppInteractions` and held in `interactionsData.interactions`.

The org-level usage page (`frontend/src/components/orgs/OrgUsage.tsx`) already contains a working `csvEscape()` helper and an `exportRows()` function (lines 123–188). The pattern here mirrors that implementation.

## Key Decision

**Client-side generation only.** The full (post-filter) interaction list is already in memory after the `useListAppInteractions` call. Serialising it to CSV in the browser avoids a new backend endpoint and matches existing practice in the codebase.

## Implementation

### 1. CSV helper (add near top of `AppUsage.tsx`)

```ts
const csvEscape = (value: string | number | undefined) =>
  `"${String(value ?? '').replace(/"/g, '""')}"`

const exportInteractionsCSV = (appId: string, interactions: TypesInteraction[]) => {
  const headers = [
    'interaction_id', 'session_id', 'created', 'completed',
    'state', 'feedback', 'prompt', 'duration_ms', 'total_tokens', 'total_cost',
  ]
  const lines = [
    headers.join(','),
    ...interactions.map(i => [
      i.id,
      i.session_id,
      i.created,
      i.completed,
      i.state,
      i.feedback ?? '',
      i.display_message ?? i.prompt_message ?? '',
      i.duration_ms,
      // total_tokens and total_cost come from the LLM calls summary if available
      '',   // total_tokens – populated from llm_call aggregates if present
      '',   // total_cost   – populated from llm_call aggregates if present
    ].map(csvEscape).join(',')),
  ]
  const blob = new Blob([lines.join('\n')], { type: 'text/csv;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `app-${appId}-interactions-${new Date().toISOString().slice(0, 10)}.csv`
  a.click()
  URL.revokeObjectURL(url)
}
```

### 2. Button in the header row

In the `<Box>` at line ~321 that contains the period toggle and Refresh button, add:

```tsx
<Button
  size="small"
  variant="outlined"
  startIcon={<Download size={16} />}
  disabled={!filteredInteractions.length}
  onClick={() => exportInteractionsCSV(appId, filteredInteractions)}
>
  Export CSV
</Button>
```

`Download` is already imported from `lucide-react` (used elsewhere in the file).

## Data Notes

- `TypesInteraction` is defined in `frontend/src/api/api.ts` at line 3486.
- `display_message` overrides `prompt_message` for display; use the same fallback logic.
- `total_tokens` / `total_cost` are not direct fields on `TypesInteraction`; if the LLM calls data is already loaded via `useListAppLLMCalls`, sum per-interaction costs there. Otherwise, leave the columns blank for the initial implementation and note them as a follow-up.

## What Does Not Change

- No new routes, no new API endpoints, no new services.
- The existing `useListAppInteractions` pagination is respected — the export covers only the currently loaded page (same behaviour as OrgUsage breakdown tables before the `export_*` full-set fields were added).
