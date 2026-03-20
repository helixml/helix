# Design: Notification Bar Count Badge Red on New Notifications

## Root Cause

`GlobalNotifications.tsx` (lines 377-392) renders the count pill in the panel header with hard-coded muted colors:

```tsx
<Box
  sx={{
    color: 'rgba(255,255,255,0.5)',
    backgroundColor: 'rgba(255,255,255,0.06)',
    ...
  }}
>
  {totalCount}
</Box>
```

The `hasNew` boolean (already available in scope from `useAttentionEvents()`) is never consulted here, even though the same flag drives the red color on the bell icon badge at line 316.

## Fix

Apply conditional styles to the count pill based on `hasNew`, mirroring the bell badge pattern:

```tsx
<Box
  sx={{
    color: hasNew ? '#fff' : 'rgba(255,255,255,0.5)',
    backgroundColor: hasNew ? '#ef4444' : 'rgba(255,255,255,0.06)',
    ...
  }}
>
  {totalCount}
</Box>
```

- `#ef4444` is the Tailwind `red-500` / MUI `error.main` equivalent already used for failure event accents in `eventAccentColor()`.
- No new state, hooks, or API calls needed — `hasNew` is already in scope.

## Pattern Notes

- `hasNew` is computed in `useAttentionEvents` by detecting events not in the previous render's ID set (truly new arrivals).
- When the drawer opens, `handleDrawerOpen` calls `acknowledge()` for all visible events, which clears `hasNew` and reverts both the bell badge and the count pill back to muted.
- The fix is a one-file, two-line change confined to `frontend/src/components/system/GlobalNotifications.tsx`.
