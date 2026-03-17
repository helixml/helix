# Implementation Tasks

## Backend

- [x] Add `HelixVersion string` field to `SandboxHeartbeatRequest` in `api/pkg/types/types.go`
- [x] Add `HelixVersion string` field to `SandboxInstance` struct in `api/pkg/types/types.go` with gorm tag
- [x] Update `HeartbeatRequest` struct in `api/cmd/sandbox-heartbeat/main.go` to include `HelixVersion`
- [x] Import `data` package in sandbox-heartbeat and call `GetHelixVersion()` when building heartbeat payload
- [x] Update `UpdateSandboxHeartbeat` in `api/pkg/store/store_sandbox.go` to persist `helix_version`

## API & OpenAPI

- [x] Run `./stack update_openapi` to regenerate swagger docs and frontend types

## Frontend

- [x] Update sandbox dropdown in `frontend/src/pages/Dashboard.tsx` to display version
- [x] Add version mismatch detection comparing sandbox versions to `account.serverConfig.version`
- [x] Add `<Alert severity="warning">` banner in agent_sandboxes tab when version mismatch detected
- [x] Format version display (truncate git hashes to 7 chars)
- [x] Build frontend: `cd frontend && yarn build`

## Testing (Manual - requires deployed environment)

- [ ] Build sandbox image: `./stack build-sandbox`
- [ ] Start fresh sandbox and verify `helix_version` appears in `GET /api/v1/sandboxes` response
- [ ] Verify version displays correctly in admin UI sandbox dropdown
- [ ] Test mismatch alert by manually setting different version in DB