# Implementation Tasks

- [ ] In `frontend/src/components/system/GlobalNotifications.tsx`, update the count pill `<Box>` inside the panel header (around line 378) to use `hasNew`-conditional colors: `backgroundColor: hasNew ? '#ef4444' : 'rgba(255,255,255,0.06)'` and `color: hasNew ? '#fff' : 'rgba(255,255,255,0.5)'`
- [ ] Verify in the browser: trigger a new notification and confirm the panel header count pill turns red; open the drawer and confirm it reverts to gray after acknowledgement
