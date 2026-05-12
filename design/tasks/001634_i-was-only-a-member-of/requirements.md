# Requirements

## Problem

An org member who belongs to a team that holds an access grant on a project cannot see the project in the org project list. Promoting the user to org owner makes it visible (because owners bypass per-project checks).

Reported case: org `the-linux-foundation`, team granted "admin" on a project, user is org member only.

## User Stories

- As an org member in a team with any access grant (`read`, `write`, or `admin`) on a project, I see that project when I open the org page.

## Acceptance Criteria

1. Org member in a team with `admin` grant on a project sees it in `GET /api/v1/projects?organization_id=...`.
2. Org member in a team with `read` grant sees it.
3. Org member in a team with `write` grant sees it.
4. Org owners and project owners still see all projects they currently see (no regression).
5. A user with no team grant and no direct grant still does NOT see the project.
