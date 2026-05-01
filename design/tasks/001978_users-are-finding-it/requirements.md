# Requirements

## Problem

Users have trouble finding the project settings on the project management page (`SpecTasksPage`). Today, settings are hidden behind a "three dots" (`MoreHorizontal`) icon button in the top-right of the page header. The icon has no tooltip and no visible label, so the entry point for Settings, Sharing, Files, and view toggles is not discoverable.

## User Stories

- As a **first-time project user**, I want to find project settings without having to discover a hidden menu, so that I can configure my project quickly.
- As a **returning user**, I want a fast, predictable way to open settings, so that I do not need to re-learn the UI.
- As any user, I want clear visual cues (icon + tooltip + optional label) for actions in the project header, so that nothing important is hidden behind ambiguous icons.

## Acceptance Criteria

1. Settings is reachable from the project header **without opening the kebab menu first** (one click instead of two).
2. The new entry point uses a recognisable icon (MUI `Settings` gear) and has a tooltip ("Project settings").
3. The change must work on desktop. Existing mobile behaviour (header hidden on `xs`) is preserved.
4. The kebab `MoreHorizontal` menu remains for the less-common items (Files, Sharing, view toggles), but its trigger gets a tooltip ("More options") so it is no longer mystery-meat.
5. No change to the underlying settings dialog itself — only its discoverability.
6. Existing right-click menu on project cards (`Projects.tsx`) is unchanged.

## Out of Scope

- Replacing the modal dialog with a `/project/:id/settings` route.
- Restructuring the settings tabs themselves.
- Mobile-specific redesign.
