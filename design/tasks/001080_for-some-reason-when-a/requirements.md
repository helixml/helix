# Requirements: Cloned Task Spec Review Button Not Showing

## Problem Statement

When a task is cloned to other projects and goes into `spec_review` status, the "Review Spec" button does not appear in the UI. This prevents users from reviewing and approving cloned tasks.

## User Story

As a user who clones tasks to multiple projects, I want to see the "Review Spec" button on cloned tasks so that I can review and approve the specs for each target project.

## Root Cause Analysis

The "Review Spec" button visibility depends on TWO conditions (from `SpecTaskActionButtons.tsx`):
1. `task.status === 'spec_review'`
2. `task.design_docs_pushed_at` is set (truthy)

When cloning a task in `spec_task_clone_handlers.go`, the `cloneTaskToProject` function:
- Copies specs (`RequirementsSpec`, `TechnicalDesign`, `ImplementationPlan`)
- Sets status to `queued_spec_generation` (if autoStart && !JustDoItMode)
- **Does NOT copy or set `DesignDocsPushedAt`**

The cloned task eventually reaches `spec_review` status, but `DesignDocsPushedAt` remains `nil`, so the button never shows.

## Acceptance Criteria

1. When a task with existing specs is cloned, the cloned task's `DesignDocsPushedAt` field should be set
2. The "Review Spec" button should appear for cloned tasks in `spec_review` status
3. Design reviews should be properly created for cloned tasks so users can actually review specs
4. The fix should work for both auto-start and manual-start cloned tasks

## Out of Scope

- Changes to the clone UI/dialog
- Changes to how original tasks transition to spec_review
- Performance optimizations