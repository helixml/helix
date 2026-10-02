# Hide projectless chat entry points from the UI

## Summary
Remove the top-level organization chat landing page and the "No project" option while keeping the Chat navigation available. Chat creation now requires a project, offers project creation directly from the picker, and opens newly created projects in their project chat view. Projectless chat APIs and direct session URLs remain intact; only their UI entry points and automatic redirects are removed.

## Testing
- Ran the focused frontend test suite for project selection, chat navigation, projectless session filtering, home routing, and org bot session resolution: 78 tests passed.
- Ran `yarn build`: production frontend build completed successfully.
- Started the local API, frontend, and Postgres services and verified `http://localhost:8080` returns HTTP 200.
