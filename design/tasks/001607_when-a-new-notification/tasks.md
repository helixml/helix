# Implementation Tasks

- [x] In `frontend/src/hooks/useAttentionEvents.ts`, change the `hasNew` return value from `newEvents.length > 0` to `(query.data ?? []).some(e => !e.acknowledged_at)` so it reflects stable server-side state instead of transient render-cycle detection
- [x] In `frontend/src/components/system/GlobalNotifications.tsx`, update the count pill `<Box>` in the panel header (around line 378) to use `hasNew`-conditional colors: `backgroundColor: hasNew ? '#ef4444' : 'rgba(255,255,255,0.06)'` and `color: hasNew ? '#fff' : 'rgba(255,255,255,0.5)'`
- [~] Verify in the browser: trigger a new notification and confirm both the bell badge and the panel header count pill stay red persistently (not just a brief flash); open the drawer and confirm both revert to gray
