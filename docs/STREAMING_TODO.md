# Moonlight Streaming Integration - Todo List

## Progress Tracker

- [x] Remove incomplete WebSocket streaming implementation
- [x] Build moonlight-web Docker image
- [x] Start moonlight-web service in docker-compose
- [x] Add reverse proxy in Helix API
- [x] Copy moonlight-web TypeScript modules to assets
- [x] Create MoonlightStreamViewer React component
- [x] Add streaming RBAC types to types.go
- [x] Add AutoMigrate for streaming tables
- [x] Create store methods for streaming access
- [x] Implement streaming token generation API
- [ ] **IN PROGRESS**: Register streaming API endpoints in server.go
- [ ] Add frontend streaming token fetch
- [ ] Test moonlight-web UI loads correctly
- [ ] Test WebRTC streaming works end-to-end
- [ ] Commit streaming implementation
- [ ] Push changes to remote

## Current Task

Registering streaming access API endpoints in server.go and verifying API compiles.

## Next Steps

1. Register endpoints in server.go
2. Check API hot reload logs
3. Test frontend can fetch streaming tokens
4. Verify moonlight-web integr ation works
5. Commit and push

## Blockers

None currently.
