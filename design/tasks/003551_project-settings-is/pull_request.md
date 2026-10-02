# Add project settings shortcut to the chat sidebar

## Summary
Add a settings gear beside the New action on each project row in the chat sidebar. The shortcut opens the existing project settings dialog directly, making settings accessible without navigating through the Projects page. Keep the sidebar action labels concise and consistent as "+ New."

## Testing
- `corepack yarn test ProjectChatGroup.test.tsx ProjectChatSidebar.logic.test.ts` — 51 tests passed.
- `corepack yarn tsc` — passed.
- `docker compose -f docker-compose.dev.yaml exec -T frontend yarn build` — production frontend build passed.
- Verified in the live UI that the gear appears beside "+ New" and opens the project settings control without starting a task or collapsing the project row.
