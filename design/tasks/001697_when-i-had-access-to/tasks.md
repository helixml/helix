# Implementation Tasks

- [x] Find where exploratory sessions are created (likely `startExploratorySession` or similar)
- [x] Set `session.ProjectID = session.Metadata.ProjectID` when creating exploratory sessions
- [x] Add database migration to backfill existing sessions where `ProjectID` is empty but `Metadata.ProjectID` is set (SKIPPED: sessions are ephemeral, new sessions will be created correctly)
- [x] Test: verify fix by checking authorization logic (traced through authz.go - confirmed fix works)
