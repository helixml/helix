# Wolf DinD + RevDial Implementation Status

**Date**: 2025-11-23
**Status**: DinD Complete ✅ | RevDial Code Complete ✅ | Testing Blocked (need fresh sandbox) ⏳
**Branch**: `feature/wolf-dind` (helix), `feature/wolf-dind` (wolf)
**Commits**: 6 commits (e965afac5...6876b8e11)

---

## ✅ COMPLETED: Docker-in-Docker

### What Works

**Wolf runs isolated dockerd** - No host Docker socket dependency:
- Wolf container runs privileged with dockerd installed
- nvidia-container-toolkit installed for GPU support
- Auto-creates helix_default network (172.20.0.0/16) on startup
- Persistent docker storage: `wolf-docker-storage` volume

**Sandboxes run in Wolf's dockerd** - Verified working:
- Created sandbox containers successfully
- Desktop streaming works (Sway + GPU + Moonlight)
- Docker commands work inside sandboxes (`docker ps`, `docker run hello-world`)
- Devcontainers created as **siblings** to sandboxes (only 2 nesting levels!)

**Network routing** - Critical fix for DinD:
- Wolf's internal network: **172.20.0.0/16** (different from host's 172.19.0.0/16)
- Prevents subnet conflict, enables routing to host network
- Sandboxes can reach internet + API container
- Git cloning works, settings-sync-daemon works
- ExtraHosts: `api:172.19.0.20` for DNS resolution

**Development workflow**:
- `./stack build-sway` auto-transfers image to Wolf's dockerd
- `/helix-dev/` bind-mounts enable hot-reloading in DinD
- No special configuration needed

### Implementation Details

**Wolf Dockerfile changes** (`wolf/docker/wolf.Dockerfile`):
```dockerfile
# Install Docker + nvidia-container-toolkit
RUN apt-get install docker-ce docker-ce-cli containerd.io
RUN apt-get install nvidia-container-toolkit nvidia-container-runtime

# Startup script
COPY docker/start-dockerd.sh /etc/cont-init.d/04-start-dockerd.sh
```

**Wolf startup script** (`wolf/docker/start-dockerd.sh`):
- Uses iptables-legacy for DinD compatibility
- Sets `iptables -P FORWARD ACCEPT` for outbound routing
- Creates helix_default network with 172.20.0.0/16 subnet
- Configures nvidia runtime in dockerd

**Sandbox configuration** (API changes):
- Mount Wolf's docker socket: `/var/run/docker.sock:/var/run/docker.sock`
- Use `/helix-dev/` paths for dev mode (not host paths)
- ExtraHosts: `["api:172.19.0.20"]` for routing

**docker-compose changes**:
- Wolf: `privileged: true`, removed host socket mount
- Both `wolf` and `wolf-amd` profiles updated
- Added `wolf-docker-storage` volume

### Commits (Helix Repo)
1. Add Docker socket bind-mount for sandboxes and enable Wolf DinD
2. Add Wolf DinD support to production docker-compose (both NVIDIA and AMD profiles)
3. Add helix-sway image transfer to Wolf's dockerd
4. Fix duplicate docker socket mount in sandboxes
5. Fix DinD dev mode paths - use /helix-dev inside Wolf container
6. Remove design/ from gitignore and add DinD+RevDial architecture doc
7. Update design doc: Network routing fixed with different subnet

### Commits (Wolf Repo)
1. Add Docker-in-Docker support to Wolf (dockerd + nvidia-container-toolkit)
2. Add start-dockerd.sh to dockerignore whitelist
3. Create helix_default network in Wolf's dockerd for sandbox networking
4. Add iptables FORWARD ACCEPT for DinD outbound networking
5. Use iptables-legacy for DinD nested container networking
6. Use different subnet for Wolf's internal network (172.20.0.0/16)

---

## ⏳ IN PROGRESS: RevDial Implementation

### What's Implemented (Code Complete)

**RevDial client binary** (`api/cmd/revdial-client/main.go`):
- Connects to API's `/revdial` endpoint via WebSocket
- Proxies localhost:9876 (screenshot/clipboard server)
- Auto-reconnects on connection drop
- Uses USER_API_TOKEN for authentication (user-scoped, not system token)
- Sends `Authorization: Bearer {USER_API_TOKEN}` header on WebSocket upgrade

**Sandbox integration**:
- revdial-client built into helix-sway image
- Auto-starts on sandbox boot (`wolf/sway-config/startup-app.sh`)
- Runner ID: `sandbox-{HELIX_SESSION_ID}`
- Connects to: `ws://api:8080/revdial?runnerid=sandbox-{session_id}`
- **AUTHENTICATION**: Uses USER_API_TOKEN with session ownership validation
  - `/revdial` endpoint moved to authRouter (accepts user tokens)
  - Server extracts session ID from runner ID format: `sandbox-{session_id}`
  - Validates `session.Owner == user.ID` before accepting connection
  - Prevents privilege escalation (users can only connect to their own sandboxes)
  - SECURITY: Does NOT use system RUNNER_TOKEN (users can read env vars!)

**API routing** (`api/pkg/server/external_agent_handlers.go`):
- Screenshot requests: Try RevDial first, fallback to direct HTTP
- Clipboard GET: Try RevDial first, fallback to direct HTTP
- Clipboard SET: Try RevDial first, fallback to direct HTTP
- Uses connman to manage RevDial connections

**Server-side** (already existed):
- `/revdial` endpoint in API server (`api/pkg/server/server.go:668`)
- Connection manager (`api/pkg/connman/connman.go`)
- RevDial package (`api/pkg/revdial/revdial.go`)

### What Needs Testing (Ready but Blocked on Fresh Sandbox)

**Latest helix-sway image** (f123bd4e3635) **has**:
- ✅ RevDial client with WebSocket URL fix (ws:// not http://)
- ✅ USER_API_TOKEN authentication
- ✅ Auto-starts on sandbox boot
- ✅ Session ownership validation on server

**To verify RevDial works** (requires fresh sandbox):
1. **Via Web UI** (RECOMMENDED):
   - Navigate to Projects → Select project → Spec Tasks
   - Click "Start Planning" on a new task
   - Opens sandbox with latest helix-sway image
   - Check `/tmp/revdial-client.log`: should show `✅ Connected to RevDial server`
   - Screenshot should work (tests RevDial tunnel)

2. **Via CLI** (BLOCKED - needs fixes):
   - **Issue**: Tasks created by runner-system have no API keys
   - **Fix needed**: Auto-create API key for users without one
   - **Or**: Use web UI to create task as real user first

**Testing commands**:
```bash
# Check RevDial client in sandbox
SANDBOX_ID=$(docker exec helix-wolf-1 docker ps -q --filter "name=zed-external" | head -1)
docker exec helix-wolf-1 docker logs $SANDBOX_ID 2>&1 | grep "RevDial"
docker exec helix-wolf-1 docker exec $SANDBOX_ID cat /tmp/revdial-client.log

# Check API logs for connection registration
docker compose -f docker-compose.dev.yaml logs api | grep "Registered reverse dial"

# Test screenshot via curl
curl -v http://localhost:8080/api/v1/external-agents/{session_id}/screenshot \
  -H "Authorization: Bearer {user_token}"
```

### Commits (RevDial)
1. Add RevDial client for sandbox ↔ API communication
2. Route screenshot requests through RevDial with direct HTTP fallback
3. Route clipboard GET/SET requests through RevDial with fallback
4. Fix RevDial client WebSocket URL (convert http:// to ws://)
5. **Fix RevDial authentication: Use user API tokens with session validation**
   - Move /revdial from runnerRouter to authRouter (accept user tokens)
   - Validate session ownership (session.Owner == user.ID)
   - Extract session ID from runner ID format: sandbox-{session_id}
   - SECURITY: Prevent passing system RUNNER_TOKEN to user sandboxes

---

## 🚧 TODO: Remaining RevDial Work

### Phase 1: Verify Basic RevDial Works
- [x] RevDial client code complete
- [x] API routing code complete
- [ ] Test with fresh sandbox (needs user to create new one)
- [ ] Verify screenshot works via RevDial
- [ ] Verify clipboard works via RevDial

### Phase 2: Multi-Sandbox Support
- [ ] Update connman to handle multiple sandbox connections concurrently
- [ ] Add sandbox list UI (dashboard showing all connected sandboxes)
- [ ] Tab/dropdown selector for switching between sandboxes
- [ ] Test multiple sandboxes simultaneously

### Phase 3: Wolf Instance Registry (For Remote Wolf)
- [ ] Database schema: `WolfInstance` table
- [ ] Wolf CRUD endpoints (register, heartbeat, deregister)
- [ ] Wolf RevDial client (connects Wolf to API)
- [ ] Scheduling algorithm (round-robin or least-loaded)

### Phase 4: Moonlight Web RevDial
- [ ] Moonlight Web RevDial client
- [ ] WebSocket proxy for browser streaming
- [ ] Test browser → API → RevDial → Moonlight Web → WebRTC

### Phase 5: Production Deployment
- [ ] Update install.sh to handle Wolf DinD setup
- [ ] K8s deployment manifests (Wolf pod with privileged mode)
- [ ] Registry-based image distribution (no manual transfers)
- [ ] Monitoring, health checks, failure handling

---

## Summary

**What's Working Right Now**:
✅ Wolf runs isolated dockerd (K8s-ready, no host socket)
✅ Sandboxes work with full GPU, desktop streaming, Docker access
✅ Network routing works (different subnets prevent conflicts)
✅ DevOps workflow complete (builds, transfers, hot-reload)

**What's Code-Complete But Untested**:
⏳ RevDial client connects to API
⏳ Screenshot/clipboard requests route through RevDial
⏳ Fallback to direct HTTP works

**What's Next**:
1. **User creates new sandbox** with updated image
2. Verify RevDial connection succeeds
3. Implement multi-sandbox support (current limitation: single sandbox)
4. Implement Wolf instance registry for remote deployment

**Estimated Time to Production-Ready**:
- Phase 1 (verify basic RevDial): 1-2 hours testing
- Phase 2 (multi-sandbox): 1 day (UI + backend)
- Phase 3 (remote Wolf): 2-3 days
- Phase 4 (Moonlight RevDial): 1-2 days
- **Total**: ~1 week for full distributed Wolf architecture
