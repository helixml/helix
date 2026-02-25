# Requirements: Primary Repository Ordering Bug

## Problem Statement

When a Helix project has multiple repositories attached, the primary repository should:
1. Be displayed first in the project settings UI
2. Be used as the working directory when running the startup script

Currently, neither of these behaviors work correctly:
- The UI displays repos in `created_at DESC` order (most recently added first)
- The startup script runs in the **first repo in `HELIX_REPOSITORIES`** env var, which follows database ordering—not the designated primary repo

## User Stories

### US-1: Primary repo appears first in settings
**As a** project owner  
**I want** my primary repository to appear at the top of the repo list  
**So that** I can easily identify which repo is primary and the UI reflects my configuration

### US-2: Startup script runs in primary repo directory
**As a** developer using Helix  
**I want** my startup script to execute with the primary repo as the working directory  
**So that** my build/install commands work correctly regardless of repo attachment order

## Acceptance Criteria

### AC-1: UI Ordering
- [ ] Primary repository always appears first in the project settings repo list
- [ ] Other repos maintain consistent ordering (alphabetical or by created_at)

### AC-2: Startup Script Working Directory
- [ ] `helix-workspace-setup.sh` changes to `$HELIX_PRIMARY_REPO_NAME` directory before running startup script
- [ ] Verify this works for both regular sessions and golden builds

### AC-3: No Regressions
- [ ] Repos still display correctly when no primary is set (edge case during migration)
- [ ] Detaching/attaching repos doesn't break ordering