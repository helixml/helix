# Requirements: Display Breadcrumb in Page Title

## User Stories

**As a user**, I want the browser tab title to show which project and task I'm viewing, so I can:
- Quickly identify the correct tab when multiple Helix tabs are open
- See context at a glance without switching tabs
- Have meaningful entries in browser history

## Acceptance Criteria

1. **Page title reflects current location**
   - Task detail page: `Task Name - Project Name - Helix`
   - Project page: `Project Name - Helix`
   - Other pages: `Page Name - Helix` (or just `Helix` for home)

2. **Title updates dynamically**
   - Title updates when navigating between pages
   - Title updates when data loads (e.g., task name fetched from API)

3. **Truncation for long names**
   - Titles over ~60 characters should be truncated with ellipsis
   - Most specific item (task name) gets priority; project name truncates first

4. **Fallback behavior**
   - While loading: show "Loading... - Helix"
   - If name unavailable: use "Task" or "Project" as placeholder
