# Requirements: Project Secret Files

## Summary
Add support for storing secret files (like `.env`, certificates, config files) in project settings. Each secret file is associated with a specific repository within the project. These files are injected into the workspace at specified paths when sessions start, and are automatically added to `.gitignore` to prevent accidental commits.

## User Stories

### US1: Add secret file to project
As a developer, I want to add a secret file (e.g., `.env`) to my project settings so that agents have access to API keys and configuration without committing them to git.

**Acceptance Criteria:**
- Can select which repository the file belongs to (from project's attached repos)
- Can specify file path relative to repo root (e.g., `.env`, `config/secrets.json`)
- Can enter file content (multi-line text editor)
- Content is encrypted at rest (same as existing project secrets)
- File appears in project settings list with repository name and path displayed

### US2: Secret files injected at session start
As a developer, I want secret files automatically written to the workspace when a session starts so agents can use them immediately.

**Acceptance Criteria:**
- Files written to specified paths within their associated repository's checkout directory
- Files created before startup script runs
- Parent directories created if needed
- Existing files are overwritten (secret file takes precedence)

### US3: Secret files auto-gitignored
As a developer, I want secret file paths automatically added to `.gitignore` so they can't be accidentally committed.

**Acceptance Criteria:**
- Each secret file path is added to `.gitignore` if not already present
- Works with existing `.gitignore` files (appends)
- Creates `.gitignore` if it doesn't exist
- Handles paths with leading `/` or without

### US4: Manage secret files
As a developer, I want to view, edit, and delete secret files in project settings.

**Acceptance Criteria:**
- List shows file path (content hidden for security)
- Can delete individual files
- Can edit file content (shows current content when editing)

## Non-Requirements
- File upload from disk (content entered directly in UI)
- Binary file support (text files only)
- Versioning of secret file content