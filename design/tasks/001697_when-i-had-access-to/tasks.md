# Implementation Tasks

- [x] Find where exploratory sessions are created (likely `startExploratorySession` or similar)
- [x] Set `session.ProjectID = session.Metadata.ProjectID` when creating exploratory sessions
- [~] Add database migration to backfill existing sessions where `ProjectID` is empty but `Metadata.ProjectID` is set
- [ ] Test: user with project access can resume shared project's Human Desktop
- [ ] Test: user without project access still gets 403
- [ ] Test: session owner can resume their own session (regression check)
