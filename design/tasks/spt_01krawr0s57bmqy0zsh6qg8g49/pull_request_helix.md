# fix(frontend): mobile chat overflow & spec-tasks search bar padding

## Summary

Fixes two mobile-only layout issues on the spec task pages:

1. **Spec task detail → Chat view (mobile):** the `RobustPromptInput` wrapper and the queued-messages panel that sits above it overflowed the right edge of the viewport and clipped (no horizontal scroll). Root cause: the `<Box sx={{ flex: 1 }}>` wrapping `RobustPromptInput` in the mobile chat view inherited the flexbox default `min-width: auto`, so the inner content's min-content width pushed past the available space. `RobustPromptInput`'s own outer Box also had no `width: 100%` / `minWidth: 0`, and the action-buttons row did not declare `flexWrap: 'wrap'`.
2. **Spec tasks list (mobile):** the mobile-only search bar inside `SpecTaskKanbanBoard` was flush against the top of the kanban container (no `pt`) and had only 8px horizontal padding, making it look cropped against the screen edges.

## Changes

- `frontend/src/components/tasks/SpecTaskDetailContent.tsx` — add `minWidth: 0` to the `<Box sx={{ flex: 1 }}>` that wraps `RobustPromptInput` in the mobile chat view.
- `frontend/src/components/common/RobustPromptInput.tsx` — add `width: '100%', minWidth: 0` to the outer container; add `flexWrap: 'wrap'` to the action-buttons row so a long row of buttons can wrap on very narrow viewports instead of contributing a large fixed min-content width.
- `frontend/src/components/tasks/SpecTaskKanbanBoard.tsx` — add `pt: 2` and bump `px: 1 → 2` on the mobile-only search bar container.

Desktop (≥ 900px) layouts (split-view chat panel, kanban header) are untouched.

## Test plan

- [x] `yarn tsc` — clean
- [x] Inner-Helix mobile (~375px): kanban search bar has visible top + side padding (screenshot `01`)
- [x] Inner-Helix mobile: chat view with empty queue — input fits viewport (screenshot `02`)
- [x] Inner-Helix mobile: chat view with 2 queued messages — queue header, items (drag handle / text / buttons), and input box all fit inside viewport with no clipping (screenshot `03`)
- [x] Inner-Helix desktop (1440px): split view renders unchanged (screenshot `04`)
- [x] Inner-Helix desktop: kanban header with search input renders unchanged (screenshot `05`)

## Screenshots

Mobile spec-tasks list — search bar now has top + side spacing:
![Mobile search bar](https://github.com/helixml/helix/raw/helix-specs/design/tasks/spt_01krawr0s57bmqy0zsh6qg8g49/screenshots/01-mobile-search-bar-after.png)

Mobile chat input (empty queue) — input fits viewport:
![Mobile chat input](https://github.com/helixml/helix/raw/helix-specs/design/tasks/spt_01krawr0s57bmqy0zsh6qg8g49/screenshots/02-mobile-chat-input-after.png)

Mobile chat with 2 queued messages — queue + input both fit:
![Mobile chat with queue](https://github.com/helixml/helix/raw/helix-specs/design/tasks/spt_01krawr0s57bmqy0zsh6qg8g49/screenshots/03-mobile-chat-with-queue-after.png)

Desktop split view — unchanged:
![Desktop split view](https://github.com/helixml/helix/raw/helix-specs/design/tasks/spt_01krawr0s57bmqy0zsh6qg8g49/screenshots/04-desktop-split-view-after.png)

Desktop kanban header — unchanged:
![Desktop kanban](https://github.com/helixml/helix/raw/helix-specs/design/tasks/spt_01krawr0s57bmqy0zsh6qg8g49/screenshots/05-desktop-kanban-after.png)
