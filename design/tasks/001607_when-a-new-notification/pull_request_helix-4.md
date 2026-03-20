# Fix notification badge: make red state persistent until acknowledged

## Summary

The notification bell badge and panel header count pill were not reliably turning red when new notifications arrived. The badge would flash red briefly (explaining why it "sometimes worked") but immediately reset.

## Root Cause

`useAttentionEvents` computed `hasNew` by diffing event IDs against a ref that was overwritten on **every render**. This meant `hasNew` was only `true` for the single render cycle where a new event first appeared — then reset to `false` on any subsequent re-render.

## Changes

- **`frontend/src/hooks/useAttentionEvents.ts`**: Changed `hasNew` from transient render-cycle detection (`newEvents.length > 0`) to stable server-side state (`(query.data ?? []).some(e => !e.acknowledged_at)`). Red state now persists until the user opens the drawer (which acknowledges all events).

- **`frontend/src/components/system/GlobalNotifications.tsx`**: Wired the panel header count pill to use `hasNew`-conditional colors (`#ef4444` / `#fff` when new, muted gray otherwise). Previously it always showed gray regardless.
