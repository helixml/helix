# Requirements: Chat/Spec Tab Toggle on Issue View

## Background

When a user clicks on a task in the Kanban board that is in the "Spec Review" column, they see a split view with chat on the left and task details/actions on the right. The "Review Spec" button is buried in the right panel. Users naturally expect to see the spec when clicking a task in the spec review column, but instead see the chat history.

## User Stories

**US-1: Spec tab in left panel**
As a user viewing an issue detail, I want "Chat" and "Spec" tabs at the top of the left panel, so I can quickly switch between the conversation and the spec document without hunting for the "Review Spec" button.

**US-2: Smart default for spec-review tasks**
As a user clicking on a task in the "Spec Review" (planning/review) column, I want the Spec tab to be selected by default, so the first thing I see is the spec rather than the chat.

**US-3: Return from spec view to normal view**
As a user reading the spec in full-screen review mode, I want a clear way to navigate back to the normal issue view (chat + details), so I'm not stranded in the review screen.

## Acceptance Criteria

- [ ] The left panel of `SpecTaskDetailContent` displays two tabs: **Chat** and **Spec**
- [ ] The **Chat** tab shows the existing `EmbeddedSessionView` (current default behaviour)
- [ ] Clicking the **Spec** tab opens the Review Spec view (same as clicking the "Review Spec" button today — either inline or navigating to the review tab/page)
- [ ] When a task's phase is `"planning"` or `"review"`, the **Spec** tab is selected by default on open
- [ ] For all other phases, **Chat** is selected by default (existing behaviour preserved)
- [ ] The Spec tab is only shown when the task has a design review available (i.e. `hasDesignReview` is true)
- [ ] The full-screen spec review view (DesignReviewContent / SpecTaskReviewPage) has a visible "Back" / "Close" affordance that returns the user to the normal issue view
- [ ] The "Review Spec" button in the right panel action buttons remains for discoverability but is now secondary (the tab is the primary entry point)
