# Add project settings shortcut to the chat sidebar

## Summary
Add a subtle three-dots action to each project row in the chat sidebar, matching the Chief of Staff row. The control appears on hover or keyboard focus on desktop and remains visible on mobile. Its existing project menu includes Project settings, making settings accessible without navigating through the Projects page while keeping the sidebar uncluttered. Keep the sidebar action labels concise and consistent as "+ New."

## Testing
- `corepack yarn test ProjectChatGroup.test.tsx` — 19 tests passed.
- `corepack yarn tsc` — passed.
- `docker compose -f docker-compose.dev.yaml exec -T frontend yarn build` — production frontend build passed.
- Verified in the live desktop UI that the three-dots control is hidden at rest, appears on project-row hover, and opens project actions without starting a task or collapsing the project row.
- Verified at a phone viewport that the three-dots button opens the project menu and its Project settings action opens the correct project dialog.
