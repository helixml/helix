# Stash Recovery and Fixes - 2026-01-26

## Context

During helix-in-helix development, some changes were stashed and then the codebase was reverted. This caused regressions in normal dev Helix operation.

## Stash Contents (stash@{0})

Key changes that need to be recovered:

### Already Applied
- **api/pkg/external-agent/hydra_executor.go** (91 lines) - APPLIED
  - Fixed `waitForDesktopBridge` to use RevDial instead of direct IP access
  - Container IPs are inside sandbox's DinD network, not reachable from API container
  - Now uses `h.connman.Dial()` to reach containers via RevDial tunnel

### Backend - Still in Stash
- **api/pkg/desktop/diff.go** (565 lines) - Multi-workspace diff support
  - Likely adds ability to show diffs from multiple repositories/workspaces
  - Important for spec tasks that clone multiple repos

- **api/pkg/desktop/ws_stream.go** (75 lines) - WebSocket stream improvements

- **api/pkg/server/external_agent_handlers.go** (111 lines) - New external agent endpoints
  - Probably includes the diff endpoint changes for multi-workspace

### Frontend - Still in Stash
- **frontend/src/components/tasks/DiffViewer.tsx** (138 lines)
  - Tabs for different repos in diff view
  - Multi-workspace diff UI

- **frontend/src/hooks/useLiveFileDiff.ts** (67 lines)
  - Hook changes for multi-workspace diff

- **frontend/src/components/spec-tasks/DesignReviewContent.tsx** (138 lines)
  - Comment streaming fixes (WebSocket subscription)
  - May overlap with today's work

- **frontend/src/components/tasks/TabsView.tsx** (992 lines)
  - Major tab view improvements
  - Split screen fixes

- **frontend/src/components/external-agent/DesktopStreamViewer.tsx** (136 lines)
  - Stream viewer improvements
  - May include the `isStarting` fix we did today

### Docker/Desktop
- **Dockerfile.ubuntu-helix** (25 lines) - Ubuntu image changes
- **Dockerfile.sway-helix** (10 lines) - Sway image changes
- **desktop/*/zed.desktop** - Zed desktop entry changes

## Issues Fixed Today (Not in Stash)

1. **DesktopStreamViewer mount/unmount flicker**
   - File: `frontend/src/components/external-agent/ExternalAgentDesktopViewer.tsx`
   - Changed `isStarting` to include 'loading' state
   - Prevents component from mounting during 'loading' then unmounting on 'starting'

2. **Design review comment streaming**
   - File: `frontend/src/components/spec-tasks/DesignReviewContent.tsx`
   - WebSocket now subscribes immediately when viewing spec task
   - Uses `planningSessionId` directly from task, not waiting for queue status
   - Added [DRWS-DEBUG] logging for debugging

## Recommended Recovery Order

1. ~~Apply hydra_executor.go~~ DONE
2. Apply diff backend: `api/pkg/desktop/diff.go`, `external_agent_handlers.go`
3. Apply diff frontend: `DiffViewer.tsx`, `useLiveFileDiff.ts`
4. Review and selectively apply: `DesignReviewContent.tsx`, `DesktopStreamViewer.tsx`
   - Compare with today's changes to avoid conflicts
5. Apply TabsView.tsx if needed

## Commands to Apply

```bash
# Apply specific files from stash
git checkout stash@{0} -- api/pkg/desktop/diff.go
git checkout stash@{0} -- api/pkg/server/external_agent_handlers.go
git checkout stash@{0} -- frontend/src/components/tasks/DiffViewer.tsx
git checkout stash@{0} -- frontend/src/hooks/useLiveFileDiff.ts

# For files with potential conflicts, view diff first
git stash show -p stash@{0} -- frontend/src/components/spec-tasks/DesignReviewContent.tsx
```

## Current State

- API restarted with RevDial health check fix
- Frontend has today's changes (isStarting includes 'loading', comment streaming WebSocket)
- Multi-workspace diff UI still missing
- Need to test new spec task creation to verify health check fix works
