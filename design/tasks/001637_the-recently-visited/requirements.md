# Requirements: Notifications Panel Polish

## User Stories

### 1. Recently Visited — Deduplication
**As a user**, I want the "Recently visited" list to not show duplicate entries for the same task, so I get a clean, useful history without noise.

**Acceptance Criteria:**
- If a user visited both the task-detail and task-review pages for the same task, only the most-recently-visited one appears in the list.
- Deduplication is by `taskId` (params.taskId), matching the same logic used for filtering out active alert tasks.

### 2. Recently Visited — Time Ago
**As a user**, I want to see how long ago I visited each page in the "Recently visited" list, so I can tell what's recent vs. stale.

**Acceptance Criteria:**
- Each entry shows a relative time label (e.g. "3m ago", "2h ago", "1d ago") using the existing `timeAgo()` helper.
- The timestamp comes from `NavHistoryEntry.timestamp` (already stored as `Date.now()` in ms).
- Time label is dim/secondary, similar to how it appears on notification events.

### 3. Notification Items — Swap Title & Subtitle for Specs
**As a user**, I want to see the spec prompt as the primary text in notification items, so I can immediately identify which task the notification is about.

**Acceptance Criteria:**
- Primary line (bold): `event.spec_task_name || event.spec_task_id` — the prompt/task name.
- Primary line wraps up to 2 lines (not truncated to 1 line).
- Secondary line (dim): `event.title` (e.g. "Spec ready", "Agent finished") + `event.project_name || event.project_id`.
- Grouped items ("Spec ready & agent finished") also show the task name as primary.

### 4. Icons — Replace Emojis with Proper Icons
**As a user**, I want the notification event icons to use the same icon style as the rest of the app (lucide-react), so the panel looks polished and consistent.

**Acceptance Criteria:**
- `agent_interaction_completed` (currently 🛑): replaced with a friendly lucide icon, e.g. `Hand` (wave gesture).
- `specs_pushed` (currently 📋): replaced with a lucide sparkle icon, e.g. `Sparkles`.
- `spec_failed` / `implementation_failed` (currently ❌): replaced with a lucide error icon, e.g. `AlertCircle`.
- `pr_ready` (currently 🔀): replaced with a lucide icon, e.g. `GitMerge`.
- Default (currently 🔔): replaced with `Bell` (already imported).
- Grouped event icon (currently 📋): use `Sparkles`.
- Icons are rendered as lucide SVG components with a size appropriate for the context (e.g. `size={14}`), colored to match the existing accent colors.
