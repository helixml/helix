# Implementation Tasks

- [ ] In `useBrowserNotifications.ts`: on mount, load previously-shown notification IDs from `sessionStorage` into `shownRef`; after each successful `fireNotification` call, persist the new ID to sessionStorage.
- [ ] In `GlobalNotifications.tsx`: replace the raw `newEvents` loop in the browser-notification `useEffect` with a grouped version — run `newEvents` through `groupEvents()` and fire one notification per group, using a merged title for grouped pairs (e.g., "Spec ready & agent finished").
- [ ] Verify in the browser: complete a spec task and confirm exactly one OS notification appears, not two.
- [ ] Verify in the browser: navigate away and back while unacknowledged events exist; confirm no duplicate OS notifications fire.
