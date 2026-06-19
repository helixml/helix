# Implementation Tasks: Add Export-to-CSV on the App Usage (Reports) Tab

- [ ] Add `csvEscape` and `exportInteractionsCSV` helper functions to `frontend/src/components/app/AppUsage.tsx`
- [ ] Add an **Export CSV** `<Button>` to the Usage tab header row (next to the Refresh button), disabled when `filteredInteractions` is empty
- [ ] Verify the downloaded file has correct headers and one row per interaction
- [ ] Verify the button is disabled when the interactions list is empty
