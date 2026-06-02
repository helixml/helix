# Design: Show Session-Restart Notice in Skills/MCP Editor

## Where the editor lives

Both editors render the **same shared component**: `frontend/src/components/app/Skills.tsx`.

| Page | File | Render site |
| --- | --- | --- |
| Project Settings → Skills tab | `frontend/src/pages/ProjectSettings.tsx` (≈L1837-1857) | `<Skills … hideHeader compactGrid />` |
| Agent (App) Settings → Skills tab | `frontend/src/pages/App.tsx` (≈L125-136) | `<Skills … />` |

Because the same component renders in both, **one edit in `Skills.tsx` covers both pages.** No per-page duplication.

## Approach

Add a persistent informational alert at the top of the Skills component body, above the category/grid layout (around current line 1250, just before the grid render).

Use the existing MUI pattern already proven in this file:

```tsx
<Alert severity="info" sx={{ mb: 2 }}>
  Changes to MCP servers and API skills take effect in <strong>new sessions</strong>.
  Restart any active session to pick up updates.
</Alert>
```

Reference precedent: the OAuth Configuration Warning in `Skills.tsx` (≈L1507-1587) uses `<Alert severity="warning">` inside a `<Collapse>`. We omit the `<Collapse>` + dismiss button here because the notice should always be visible — it's a recurring constraint, not a one-time message.

## Key decisions

1. **Edit the shared `Skills.tsx` once, not each parent page.** Both project and agent settings reuse the same component, so a single change keeps them in sync and avoids drift.
2. **`severity="info"`, not `"warning"`.** This is normal expected behavior, not a problem the user has to fix. Reserve warnings for misconfiguration (matches existing OAuth warning usage).
3. **Always visible, not dismissible.** Users hit this constraint every time they edit MCP config, not just once. A dismissible banner would re-train them to ignore future warnings.
4. **Single notice covering both MCP and API skills.** The editor manages both in the same UI; one combined sentence avoids notice clutter.

## Files to modify

- `frontend/src/components/app/Skills.tsx` — add the `<Alert>` near the top of the rendered output, above the skills grid.

## Files NOT modified

- `ProjectSettings.tsx`, `App.tsx` — no change needed; they already render the shared component.
- Any backend file — this is a pure UI text addition.

## Testing

- Manual: open Project Settings → Skills tab, confirm the notice is shown above the skill cards.
- Manual: open an Agent (App) → Skills tab, confirm the same notice is shown.
- Manual: switch between category sub-tabs inside Skills (Core, MCP Servers, etc.) and confirm the notice remains visible.
- No new unit tests needed (cosmetic copy change).

## Notes for future agents

- The Skills component is large (~1900 lines) and is the **single source of truth** for both the project-level and agent-level skill editors. If you ever need to differentiate behavior between the two, the parent page passes different props (`hideHeader`, `compactGrid`, `defaultCategory`) — extend that prop set rather than forking the component.
- The OAuth warning block in this same file is a good template for any future MCP/API editor alerts.
