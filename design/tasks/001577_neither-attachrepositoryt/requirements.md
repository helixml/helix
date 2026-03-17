# Requirements: Fix default_repo_id Sync on Attach/Detach

## Problem

`default_repo_id` on a project is not kept in sync when repositories are attached or detached. This causes `hasExternalRepo` to return `false` in the frontend even when an external repo is attached, because the lookup `projectRepositories.find(r => r.id === defaultRepoId)` returns `undefined` when `default_repo_id` still points to a detached repo.

## Approach

TDD. For each user story: write a failing test first, confirm it is red, then implement until it is green.

## User Stories

**US-1:** As a user who detaches the current default repo and attaches a new one, I expect the pull_request column to appear in the Kanban board without needing to manually do anything else.
- Red test: `TestDetachRepo_UpdatesDefaultToRemainingRepo` — detach the default repo while another is attached, assert `SetProjectPrimaryRepository` is called with the remaining repo ID. Fails until handler is fixed.
- Red test: `TestAttachRepo_SetsDefaultWhenStale` — attach a new repo when `default_repo_id` references a now-detached repo, assert `SetProjectPrimaryRepository` is called with the new repo ID. Fails until handler is fixed.

**US-2:** As a user who attaches a repository to a project that has no valid default repo (or whose `default_repo_id` references a detached repo), the newly attached repo should automatically become the default.
- Red test: `TestAttachRepo_SetsDefaultWhenEmpty` — attach a repo to a project where `DefaultRepoID == ""`, assert `SetProjectPrimaryRepository` is called with the new repo ID. Fails until handler is fixed.

**US-3:** As a user who detaches a non-default repo, I expect the default repo to remain unchanged.
- Red test: `TestDetachRepo_KeepsDefaultWhenNotDefault` — detach a repo that is not the default, assert `SetProjectPrimaryRepository` is NOT called. Fails until handler is fixed.

## Acceptance Criteria

- **AC-1:** When `detachRepositoryFromProject` is called and the detached repo's ID equals `project.default_repo_id`, the project's `default_repo_id` is updated: set to another currently-attached repo if one exists, or cleared to `""` if none remain.
- **AC-2:** When `attachRepositoryToProject` is called and the project's `default_repo_id` is either empty (`""`) or no longer references an attached repo, `default_repo_id` is set to the newly attached repo's ID.
- **AC-3:** When `attachRepositoryToProject` is called and `default_repo_id` already references a valid attached repo, it is left unchanged.
- **AC-4:** The frontend `hasExternalRepo` computed value correctly reflects the attached external repo after a detach+attach cycle without requiring a page reload (dependent on AC-1 and AC-2 propagating via existing query invalidation).
