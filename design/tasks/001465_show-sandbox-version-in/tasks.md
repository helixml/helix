# Implementation Tasks

## Backend

- [ ] Add `HelixVersion string` field to `SandboxHeartbeatRequest` in `api/pkg/types/types.go`
- [ ] Add `HelixVersion string` field to `SandboxInstance` struct in `api/pkg/types/types.go` with gorm tag
- [ ] Update `HeartbeatRequest` struct in `api/cmd/sandbox-heartbeat/main.go` to include `HelixVersion`
- [ ] Import `data` package in sandbox-heartbeat and call `GetHelixVersion()` when building heartbeat payload
- [ ] Update `UpdateSandboxHeartbeat` in `api/pkg/store/store_sandbox.go` to persist `helix_version`

## API & OpenAPI

- [ ] Run `./stack update_openapi` to regenerate swagger docs and frontend types

## Frontend

- [ ] Update sandbox dropdown in `frontend/src/pages/Dashboard.tsx` to display version
- [ ] Add version mismatch detection comparing sandbox versions to `account.serverConfig.version`
- [ ] Add `<Alert severity="warning">` banner in agent_sandboxes tab when version mismatch detected
- [ ] Format version display (truncate git hashes to 7 chars)

## Testing

- [ ] Build sandbox image: `./stack build-sandbox`
- [ ] Start fresh sandbox and verify `helix_version` appears in `GET /api/v1/sandboxes` response
- [ ] Verify version displays correctly in admin UI sandbox dropdown
- [ ] Test mismatch alert by manually setting different version in DB
- [ ] Build frontend: `cd frontend && yarn build`
