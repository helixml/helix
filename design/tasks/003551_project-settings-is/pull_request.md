# Add project settings shortcut to the chat sidebar

## Summary
Add a settings gear beside the New action on each desktop project row in the chat sidebar. On mobile, expose the existing project actions menu through a three-dots button, matching the Chief of Staff row, with Project settings available in that menu. Both paths open the existing project settings dialog without navigating through the Projects page. Keep the sidebar action labels concise and consistent as "+ New."

## Testing
- `corepack yarn test ProjectChatGroup.test.tsx` — 19 tests passed.
- `corepack yarn tsc` — passed.
- `docker compose -f docker-compose.dev.yaml exec -T frontend yarn build` — production frontend build passed.
- Verified in the live desktop UI that the gear appears beside "+ New" and opens project settings without starting a task or collapsing the project row.
- Verified at a phone viewport that the three-dots button opens the project menu and its Project settings action opens the correct project dialog.
