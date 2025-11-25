# Wolf RevDial Client Implementation

**Date**: 2025-11-23
**Status**: ✅ Complete - Ready for Testing
**Location**: `/home/luke/pm/helix/api/cmd/wolf-revdial-client/`

---

## Summary

Implemented a standalone RevDial client for Wolf instances, enabling remote Wolf deployment behind NAT/firewalls. This is a critical component for distributed multi-Wolf architecture.

## What Was Implemented

### 1. Wolf RevDial Client Binary (`main.go`)

**Purpose**: Connects Wolf to control plane API via outbound WebSocket, establishing a reverse tunnel for API → Wolf communication.

**Key Features**:
- ✅ Outbound WebSocket connection to control plane
- ✅ RevDial listener for accepting tunneled connections
- ✅ Bidirectional proxy between RevDial tunnel and local Wolf API
- ✅ Auto-reconnect on connection drop (configurable interval)
- ✅ Environment variable configuration support
- ✅ Command-line flag overrides
- ✅ Graceful shutdown handling

**Architecture**:
```
Control Plane API (/api/v1/revdial)
         ↑
         │ (outbound WebSocket)
         │
Wolf RevDial Client
         ↓
Local Wolf API (localhost:8080)
```

**Connection Flow**:
1. Client connects: `ws://api.example.com:8080/api/v1/revdial?runnerid=wolf-{wolf_id}`
2. Sends auth header: `Authorization: Bearer {RUNNER_TOKEN}`
3. WebSocket upgraded to RevDial tunnel
4. Creates `net.Listener` for API → Wolf requests
5. Proxies requests to local Wolf API on `localhost:8080`

### 2. Configuration Options

**Environment Variables**:
- `HELIX_API_URL` - Control plane API URL
- `WOLF_ID` - Unique Wolf instance ID
- `RUNNER_TOKEN` - Runner authentication token

**Command-Line Flags** (override env vars):
- `-api-url` - Control plane API URL
- `-wolf-id` - Wolf instance ID
- `-token` - Runner token
- `-local` - Local Wolf API address (default: `localhost:8080`)
- `-reconnect` - Reconnect interval in seconds (default: `5`)

### 3. Docker Support

**Dockerfile**: Multi-stage build for minimal image size
- Stage 1: Go builder (golang:1.22-alpine)
- Stage 2: Runtime (alpine:latest with CA certificates)
- Final size: ~10MB (plus binary ~8MB = ~18MB total)

**Docker Compose Example**: Complete deployment configuration
- Wolf container (privileged, GPU-enabled)
- RevDial client container (sidecar pattern)
- Shared network (client accesses Wolf via localhost)
- Example .env file configuration

### 4. Documentation

**README.md** (9.5 KB):
- Purpose and architecture overview
- Usage examples (env vars, flags)
- Docker Compose configuration
- Kubernetes deployment
- Building from source
- Security considerations
- Troubleshooting guide
- Testing instructions

**INTEGRATION.md** (10.7 KB):
- Four integration options:
  1. Docker Compose (single instance)
  2. Systemd service (bare metal)
  3. Kubernetes DaemonSet (multi-node)
  4. Embedded in Wolf container
- Complete configuration reference
- Runner token generation
- Wolf ID naming conventions
- Verification steps
- Monitoring and logging
- Production deployment checklist
- Migration guide
- Troubleshooting procedures

**docker-compose.example.yaml** (2.3 KB):
- Production-ready Docker Compose configuration
- Wolf + RevDial client as paired services
- GPU support (NVIDIA and AMD)
- Docker-in-Docker configuration
- Health checks and restart policies
- Example .env file template

### 5. Testing Tools

**test-local.sh** (5.2 KB):
- Automated local testing script
- Starts mock Wolf API server
- Builds and runs RevDial client
- Verifies connectivity
- Color-coded output for easy debugging
- Automatic cleanup on exit

## Implementation Details

### Code Structure

**Main Components**:
1. `main()` - Entry point, flag parsing, signal handling
2. `runRevDialClient()` - Connection establishment and proxy loop
3. `proxyConnection()` - Bidirectional TCP proxy (RevDial ↔ Wolf API)
4. `wsConnAdapter` - WebSocket to `net.Conn` adapter

**Key Design Decisions**:
- **No dependencies on Wolf codebase**: Standalone binary, doesn't require Wolf source
- **Simple proxy pattern**: No complex routing, just bidirectional TCP proxy
- **Auto-reconnect**: Essential for production reliability
- **Environment-first config**: Docker/K8s friendly, with CLI override support
- **Gorilla WebSocket**: Proven library, same as sandbox RevDial client
- **Minimal logging**: Only connection events and errors (not every request)

### Error Handling

**Connection Failures**:
- WebSocket dial errors → log + reconnect
- RevDial listener errors → log + reconnect
- Local Wolf API unreachable → log + close connection (don't kill client)

**Graceful Shutdown**:
- SIGINT/SIGTERM → close WebSocket → exit cleanly
- Cleanup goroutines properly
- No zombie processes

### Security

**Authentication**: Uses `RUNNER_TOKEN` for API authentication
- Sent as `Authorization: Bearer {token}` header
- Validated by control plane before accepting RevDial connection

**Network Isolation**:
- Wolf API only listens on `localhost:8080` (not exposed to network)
- Only accessible via RevDial tunnel or localhost
- Prevents direct network access to Wolf API

**TLS Support**: Automatic `wss://` for HTTPS API URLs
- `http://` → `ws://`
- `https://` → `wss://`

## Files Created

```
api/cmd/wolf-revdial-client/
├── main.go                        # 6.5 KB - Main client implementation
├── README.md                      # 9.5 KB - User documentation
├── INTEGRATION.md                 # 10.7 KB - Integration guide
├── Dockerfile                     # 506 B - Docker build configuration
├── docker-compose.example.yaml    # 2.3 KB - Example deployment
└── test-local.sh                  # 5.2 KB - Local testing script (executable)
```

**Total**: 6 files, ~35 KB of code and documentation

## Testing Status

### ✅ Verified Working

1. **Build**: Compiles successfully with no errors
2. **Flags**: Command-line help displays correctly
3. **Binary**: Runs and shows usage when missing required args
4. **Environment**: Reads `HELIX_API_URL`, `WOLF_ID`, `RUNNER_TOKEN`

### ⏳ Pending Testing (Requires Control Plane)

1. **RevDial Connection**: Connect to control plane `/api/v1/revdial`
2. **Authentication**: Verify `RUNNER_TOKEN` validation
3. **Proxying**: Test API → RevDial → Wolf API routing
4. **Reconnection**: Verify auto-reconnect on connection drop
5. **Docker Deployment**: Test docker-compose.example.yaml
6. **Kubernetes Deployment**: Test DaemonSet configuration

### Testing Prerequisites

**Control Plane Requirements**:
- `/api/v1/revdial` endpoint accepting Wolf connections
- Runner token validation
- Wolf instance registry (database table)
- Routing logic to send Wolf API requests via RevDial

**Wolf Requirements**:
- Wolf instance running with API on `localhost:8080`
- Valid runner token from control plane

### How to Test

**Local Development**:
```bash
# 1. Start control plane
docker compose -f docker-compose.dev.yaml up api

# 2. Get runner token (from API logs or database)
RUNNER_TOKEN="your-token-here"

# 3. Run test script
cd /home/luke/pm/helix/api/cmd/wolf-revdial-client
./test-local.sh
```

**Docker Compose**:
```bash
# 1. Copy example config
cp docker-compose.example.yaml docker-compose.yaml

# 2. Create .env file
cat > .env <<EOF
HELIX_API_URL=http://localhost:8080
WOLF_ID=wolf-test
RUNNER_TOKEN=your-token-here
EOF

# 3. Start services
docker compose up
```

## Integration Points

### Control Plane Changes Needed

**Already Implemented** (from sandbox RevDial):
- ✅ `/api/v1/revdial` endpoint
- ✅ Connection manager (`api/pkg/connman/connman.go`)
- ✅ RevDial package (`api/pkg/revdial/revdial.go`)

**Needs Implementation**:
- [ ] Wolf instance registry (database: `WolfInstance` table already exists)
- [ ] Wolf routing logic (route Wolf API requests via RevDial)
- [ ] Runner token validation for Wolf connections
- [ ] Wolf instance CRUD endpoints (register, heartbeat, deregister)

### Wolf Changes Needed

**Option 1: Sidecar Pattern** (Recommended):
- Deploy RevDial client as separate container
- Share network with Wolf container (`network_mode: "service:wolf"`)
- No changes to Wolf required ✅

**Option 2: Embedded Pattern**:
- Add RevDial client binary to Wolf image
- Modify Wolf entrypoint to start both Wolf and RevDial client
- Requires Wolf Dockerfile changes

**Recommendation**: Use sidecar pattern (simpler, no Wolf changes needed)

## Deployment Scenarios

### Scenario 1: Single Remote Wolf (Docker Compose)

**Use Case**: One Wolf instance on remote GPU server

**Deployment**:
```bash
# On remote GPU server
docker compose -f docker-compose.example.yaml up -d
```

**Control Plane**: Routes all sessions to this Wolf instance

### Scenario 2: Multiple Remote Wolves (Docker Compose)

**Use Case**: Multiple Wolf instances on different servers

**Deployment**: Same as Scenario 1, but with unique `WOLF_ID` per instance

**Control Plane**: Load balances sessions across Wolf instances (round-robin or least-loaded)

### Scenario 3: Kubernetes DaemonSet (Multi-Node)

**Use Case**: Wolf on every GPU node in K8s cluster

**Deployment**:
```bash
kubectl apply -f wolf-daemonset.yaml
```

**Control Plane**: Automatically discovers Wolf instances, schedules sessions

### Scenario 4: Hybrid (Local + Remote Wolves)

**Use Case**: Local Wolf for dev, remote Wolves for production

**Deployment**: Local Wolf without RevDial, remote Wolves with RevDial

**Control Plane**: Prefers local Wolf for dev sessions, remote Wolves for production

## Next Steps

### Phase 1: Control Plane Integration (1-2 days)

1. **Wolf Instance Registry**:
   - Database table already exists (`WolfInstance`)
   - Implement CRUD endpoints
   - Add heartbeat mechanism

2. **Wolf Routing**:
   - Modify Wolf client to use RevDial
   - Route `/api/v1/wolf/*` requests via RevDial
   - Implement scheduling algorithm (round-robin or least-loaded)

3. **Testing**:
   - Test single Wolf RevDial connection
   - Test multiple Wolf instances
   - Verify load balancing

### Phase 2: Production Deployment (1 week)

1. **Docker Images**:
   - Build and push wolf-revdial-client image to registry
   - Update Wolf image to include RevDial client (optional)

2. **Kubernetes Manifests**:
   - Create production DaemonSet YAML
   - Add RBAC, secrets, ConfigMaps
   - Document GPU node requirements

3. **Monitoring**:
   - Add Prometheus metrics for RevDial connections
   - Create Grafana dashboard
   - Configure alerts for connection drops

4. **Documentation**:
   - Update main README with distributed Wolf architecture
   - Add runbook for Wolf RevDial troubleshooting
   - Create migration guide for existing deployments

### Phase 3: Advanced Features (Future)

1. **Connection Pooling**: Reuse RevDial connections for multiple requests
2. **Geographic Routing**: Route sessions to nearest Wolf instance
3. **Auto-Scaling**: Scale Wolf instances based on load
4. **Health Checks**: Wolf reports GPU health, capacity metrics
5. **Session Migration**: Move active sessions between Wolf instances

## Known Limitations

1. **No Connection Pooling**: Each API request creates new proxy connection
   - Impact: Higher latency, more goroutines
   - Mitigation: Acceptable for low-traffic scenarios, can optimize later

2. **No Health Reporting**: Client doesn't report Wolf health to control plane
   - Impact: Control plane can't detect unhealthy Wolf instances
   - Mitigation: Add heartbeat mechanism in Phase 2

3. **No Metrics**: No Prometheus metrics exported
   - Impact: Limited observability
   - Mitigation: Add in Phase 2

4. **Single Control Plane**: No HA for control plane
   - Impact: Control plane failure breaks all Wolf connections
   - Mitigation: Not critical for initial deployment, add later

## Comparison with Sandbox RevDial Client

**Similarities**:
- Same RevDial package (`api/pkg/revdial`)
- Same WebSocket upgrade pattern
- Same proxy architecture (bidirectional TCP)
- Same auto-reconnect logic

**Differences**:

| Feature | Sandbox RevDial | Wolf RevDial |
|---------|----------------|--------------|
| **Runner ID Format** | `sandbox-{session_id}` | `wolf-{wolf_id}` |
| **Local Target** | `localhost:9876` (screenshot server) | `localhost:8080` (Wolf API) |
| **Authentication** | User API token (session ownership) | Runner token (system-level) |
| **Lifecycle** | Per-sandbox (ephemeral) | Per-Wolf instance (persistent) |
| **Endpoint** | `/api/v1/revdial` (authRouter) | `/api/v1/revdial` (runnerRouter) |
| **Purpose** | Sandbox ↔ API communication | Wolf ↔ API communication |

**Why Different?**
- **Sandboxes**: User-owned, ephemeral, need user token for security
- **Wolf**: Infrastructure component, persistent, needs system token

## Success Criteria

**✅ Implementation Complete** when:
- [x] Code compiles without errors
- [x] Binary runs and shows correct usage
- [x] Documentation complete (README, INTEGRATION, examples)
- [x] Docker build works
- [x] Test script provided

**✅ Integration Complete** when:
- [ ] Control plane accepts Wolf RevDial connections
- [ ] API routes Wolf requests via RevDial
- [ ] Multiple Wolf instances can connect simultaneously
- [ ] Load balancing works (round-robin or least-loaded)

**✅ Production Ready** when:
- [ ] Docker image published to registry
- [ ] Kubernetes manifests tested
- [ ] Monitoring and alerting configured
- [ ] Runbook and troubleshooting docs complete
- [ ] Migration guide for existing deployments

## References

- [Distributed Wolf Architecture](./2025-11-22-distributed-wolf-revdial-architecture.md)
- [DinD + RevDial Implementation Status](./2025-11-23-wolf-dind-revdial-implementation-status.md)
- [RevDial Package](../api/pkg/revdial/revdial.go)
- [Sandbox RevDial Client](../api/cmd/revdial-client/main.go)
- [Wolf Instance Types](../api/pkg/types/wolf_instance.go)

---

## Code Example

**Minimal usage**:
```bash
wolf-revdial-client \
  -api-url http://api.example.com:8080 \
  -wolf-id wolf-1 \
  -token abc123
```

**Docker Compose**:
```yaml
services:
  wolf:
    image: ghcr.io/helixml/wolf:latest
    # ... Wolf config

  wolf-revdial-client:
    image: ghcr.io/helixml/helix/wolf-revdial-client:latest
    environment:
      HELIX_API_URL: http://api.example.com:8080
      WOLF_ID: wolf-1
      RUNNER_TOKEN: abc123
    network_mode: "service:wolf"
```

**Kubernetes**:
```yaml
containers:
- name: wolf
  image: ghcr.io/helixml/wolf:latest
- name: revdial-client
  image: ghcr.io/helixml/helix/wolf-revdial-client:latest
  env:
  - name: HELIX_API_URL
    value: https://api.example.com
  - name: WOLF_ID
    valueFrom:
      fieldRef:
        fieldPath: metadata.name
  - name: RUNNER_TOKEN
    valueFrom:
      secretKeyRef:
        name: wolf-token
        key: token
```
