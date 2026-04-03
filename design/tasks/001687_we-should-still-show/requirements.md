# Requirements: Show Pull Requests on Finished Tasks

## Problem

When a spec task is marked "done" (finished/merged), the pull request links disappear from the task detail page. Users cannot find which PR was associated with a finished task.

## User Story

As a user, when I view a finished spec task, I want to see the pull request(s) that were created for it, so I can navigate to the PR in my Git provider to review history, comments, or code.

## Acceptance Criteria

1. When a task has `status === "done"` and has one or more entries in `repo_pull_requests`, the PR link(s) are visible on the task detail page.
2. The PR button(s) appear in the same location as they do during the `pull_request` status phase.
3. Clicking a PR link opens the PR URL in a new tab — same behavior as the active phase.
4. The behavior works for both single-PR and multi-PR cases.
5. The display works in both inline (task list card) and full detail page views.
