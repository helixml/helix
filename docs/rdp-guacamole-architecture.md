# RDP over Reverse Dial with Guacamole Architecture

## Overview

This document describes the architecture for providing RDP access to external agent runners through a Guacamole-based remote desktop solution that uses reverse dial connections for NAT traversal.

Goal: Provide a top-of-the-range AI coding experience with GPU accelerated remote desktop as well as secure agent sandboxes.

In the future, we may need to use Zed SSH remote support (once AI agent support works there) to properly isolate the agents from the GUI environments (which are probably ok to run in containers, rather than trying to get OpenGL working in VMs!).

## Architecture Flow

```
Frontend Guacamole Client (JavaScript) <-(Guacamole Protocol over WebSocket)->
    ↓
API Guacamole Proxy <-(REST API + WebSocket)->
    ↓
guacamole-client:8080 (Guacamole Web App) <-(Guacamole Protocol)->
    ↓
guacamole:4822 (guacd daemon) <-(RDP)->
    ↓
API RDP TCP Proxy <-(TCP over Reverse Dial)->
    ↓
Agent Runner Reverse Dial Listener <-(TCP)->
    ↓
XRDP Server (localhost:3389)
```

## Components

### 1. Frontend Guacamole Client (JavaScript)
- **Location**: `frontend/assets/guacamole-client.html`
- **Purpose**: Browser-side JavaScript client using guacamole-common-js library
- **Function**: Renders remote desktop in browser, handles user input
- **Protocol**: Guacamole protocol over WebSocket
- **Connects to**: API Guacamole Proxy

### 2. API Guacamole Proxy (MISSING - TO IMPLEMENT)
- **Location**: `api/pkg/server/guacamole_proxy.go` (to be created)
- **Purpose**: WebSocket proxy between frontend and Guacamole server
- **Protocol**: Guacamole protocol (passthrough)
- **Endpoints**:
  - `/api/v1/sessions/{sessionID}/guac/proxy` - Session-specific RDP
  - `/api/v1/external-agents/runners/{runnerID}/guac/proxy` - Runner-specific RDP
- **Connects to**: Guacamole Server

### 3. Guacamole Web Application (guacamole-client)
- **Location**: Docker container (`guacamole/guacamole:1.5.5`)
- **Port**: 8080 (exposed as ${GUACAMOLE_PORT:-8090}:8080)
- **Purpose**: Provides REST API, WebSocket tunnels, and web interface
- **Features**:
  - Dynamic connection creation via REST API
  - Authentication and session management
  - Connection management interface (useful for debugging)
  - WebSocket tunnel endpoints for frontend clients
- **Database**: PostgreSQL (guacamole_db)
- **Connects to**: guacd daemon

### 4. Guacamole Daemon (guacd)
- **Location**: Docker container (`guacamole/guacd:1.5.4`)
- **Port**: 4822
- **Purpose**: Protocol conversion daemon (Guacamole ↔ RDP/VNC/SSH)
- **Function**: Handles actual remote desktop protocol communication
- **Connects to**: API RDP TCP Proxy

### 5. API RDP TCP Proxy (UPDATED)
- **Location**: `api/pkg/server/rdp_proxy_handlers.go` (to be simplified)
- **Purpose**: TCP proxy that forwards RDP traffic via reverse dial connections
- **Protocol**: Raw TCP over reverse dial
- **Ports**: Dynamic allocation (15900+)
- **Connects to**: Agent Runner via reverse dial connection from connman
- **Key Change**: Uses `connman.Dial()` instead of NATS messaging

### 6. Agent Runner Reverse Dial Listener (NEW)
- **Location**: `api/pkg/external-agent/runner.go` (to be updated)
- **Purpose**: Creates reverse dial listener and forwards TCP to local XRDP
- **Protocol**: TCP forwarding (no protocol conversion)
- **Connects to**: Local XRDP server via standard TCP

### 7. XRDP Server
- **Location**: Agent runner container
- **Purpose**: RDP server providing desktop access
- **Port**: 3389 (local)
- **Authentication**: Per-session rotated passwords

## Connection Types

### Session-specific RDP
- **Use Case**: User accesses their specific session workspace
- **URL**: `/api/v1/sessions/{sessionID}/rdp-connection`
- **Authentication**: User must own the session
- **Password**: Runner's current RDP password
- **Isolation**: Session ID used to isolate desktop context

### Runner-specific RDP
- **Use Case**: Admin/debugging direct runner access
- **URL**: `/api/v1/external-agents/runners/{runnerID}/rdp`
- **Authentication**: Admin privileges required
- **Password**: Runner's stored RDP password
- **Scope**: Full runner desktop access

## Security

### Password Management
- **Initial Password**: **REQUIRED** static password configured via `RDP_PASSWORD` environment variable for container startup
  - **Development**: Uses insecure default if not set (with warnings)
  - **Production**: Must be unique per deployment - never use default values
  - **Purpose**: Prevents authentication gaps during container initialization
- **Generation**: Control plane generates cryptographically secure passwords for sessions
- **Rotation**: New password per session for enhanced security
- **Storage**: Passwords stored in `agent_runners` table
- **Transmission**: Passwords sent securely to runner via NATS
- **Application**: Runner receives password and sets it via `chpasswd` command

### Authentication Flow
1. Container starts with static `RDP_PASSWORD` to avoid initial authentication gap
2. **Runner connects** to control plane via WebSocket
3. **Control plane generates secure password** for the runner and stores in database
4. **Control plane sends password to runner** via ZedAgent WebSocket message
5. **Runner receives password and configures XRDP** using `chpasswd` (replaces initial password)
6. **Guacamole connection created** with the database password for admin access
7. **Session creation**: Control plane generates new session-specific password
8. **Session password sent to runner** via ZedAgent mechanism (same as step 4-5)
9. **Frontend authenticates** with API and validates permissions
10. **Connection established** with current password (either runner or session password)

## Reverse Dial Architecture Details

### Key Components Integration

1. **Existing revdial/connman** (NO CHANGES)
   - `api/pkg/revdial/revdial.go` - Handles reverse dial connections
   - `api/pkg/connman/connman.go` - Manages connection mapping
   - These components are proven and will be used exactly as-is

2. **Connection Establishment Flow**
   - Agent runner establishes WebSocket connection (existing)
   - Agent runner establishes additional reverse dial connection to control plane
   - Control plane registers reverse dial connection in connman: `connman.Set(runnerID, conn)`
   - When RDP access needed: `deviceConn, err := connman.Dial(ctx, runnerID)`

3. **RDP Traffic Flow**
   - guacd connects to API RDP proxy on localhost:15900+
   - API RDP proxy calls `connman.Dial(ctx, runnerID)` to get connection to runner
   - TCP traffic flows directly through reverse dial connection (no protocol conversion)
   - Agent runner forwards TCP traffic to localhost:3389 (XRDP)

## Implementation Status

### ✅ Completed
- Agent Runner database table and password management
- **Runner password synchronization** - Control plane sends passwords to runners via ZedAgent WebSocket
- Frontend Guacamole JavaScript client (with URL parameter approach)
- Session and runner API endpoints
- Guacamole server containers (guacd + guacamole-client)
- **Simplified XRDP configuration** - Direct X11 sessions without session manager
- **Existing revdial/connman infrastructure** - Ready to use without modification
- **✅ NEW: API RDP TCP Proxy** - COMPLETED reverse dial implementation using connman.Dial()
- **✅ NEW: Reverse Dial Connection Management** - COMPLETED `/revdial` endpoint and connection registration
- **✅ NEW: Simplified TCP Forwarding** - COMPLETED direct TCP proxy using io.Copy() instead of NATS

### 🔄 Remaining Tasks (Priority Order)
1. **Agent Runner RDP Handler** - Update runner to establish reverse dial connections + TCP forwarding
2. **API Guacamole Proxy** - WebSocket proxy to guacamole-client container (unchanged)
3. **Guacamole REST API Integration** - Dynamic connection creation/management via API
4. **End-to-End Testing** - Test complete flow with new reverse dial architecture

## Implementation Steps (Reverse Dial Approach)

### ✅ Phase 1: Simplify API RDP TCP Proxy (COMPLETED)
**File**: `api/pkg/server/rdp_proxy_handlers.go`
**Changes COMPLETED**:
1. ✅ Removed all NATS-related code (`pubsub`, `types.ZedAgentRDPData`, etc.)
2. ✅ Replaced complex TCP proxy with simple `connman.Dial()` approach
3. ✅ Implemented direct TCP forwarding when guacd connects to localhost:15900+:
   ```go
   // Get reverse dial connection to runner
   deviceConn, err := rpm.connman.Dial(ctx, proxy.RunnerID)
   if err != nil { return err }

   // Simple bidirectional TCP proxy (no protocol conversion)
   go io.Copy(guacdConn, deviceConn)
   go io.Copy(deviceConn, guacdConn)
   ```
4. ✅ Removed complex `handleTCPConnection*`, `runTCPProxy*` functions (600+ lines → 50 lines)
5. ✅ Added connman integration and `/revdial` endpoint
6. ✅ Maintained backward compatibility with legacy method signatures

### Phase 2: Simplify Agent Runner RDP Handler
**File**: `api/pkg/external-agent/runner.go`
**Changes**:
1. Remove all NATS RDP message handling
2. Add reverse dial connection establishment:
   ```go
   // Establish reverse dial connection in addition to WebSocket
   revDialConn, err := establishReverseDialConnection(controlPlaneURL)
   listener := revdial.NewListener(revDialConn, dialServer)
   ```
3. Add simple TCP forwarding to localhost:3389:
   ```go
   for {
       conn, err := listener.Accept()
       if err != nil { break }
       go func() {
           rdpConn, err := net.Dial("tcp", "localhost:3389")
           if err != nil { return }
           defer rdpConn.Close()
           defer conn.Close()

           go io.Copy(conn, rdpConn)
           go io.Copy(rdpConn, conn)
       }()
   }
   ```

### ✅ Phase 3: Update Connection Management (COMPLETED)
**File**: `api/pkg/server/server.go`
**Changes COMPLETED**:
1. ✅ Initialized connman: `connectionManager := connman.New()`
2. ✅ Added `/revdial` endpoint with connection registration:
   ```go
   // Hijack HTTP connection to get raw TCP
   conn, _, err := hijacker.Hijack()
   // Register reverse dial connection in connman
   apiServer.connman.Set(runnerID, conn)
   ```
3. ✅ Integrated connman with RDPProxyManager
4. ✅ Added proper error handling and logging

### Phase 4: API Guacamole Proxy (UNCHANGED)
Create WebSocket proxy in API server that:
- Accepts Guacamole protocol WebSocket connections from frontend
- Forwards to guacamole-client:8080 WebSocket tunnel
- Handles bidirectional Guacamole protocol traffic
- Manages connection lifecycle and authentication

### Phase 5: Guacamole REST API Integration (UNCHANGED)
Implement dynamic connection management:
- Authenticate with guacamole-client REST API
- Create RDP connections programmatically via `/api/session/data/postgresql/connections`
- Update connection parameters when passwords rotate
- Clean up connections when sessions end

### Phase 6: End-to-End Testing
- Test complete flow from frontend to XRDP with new reverse dial approach
- Verify password rotation and security still work
- Test both session and runner connection types
- Debug via Guacamole web interface
- Performance testing (should be faster with direct TCP)

## Configuration

### Environment Variables (SIMPLIFIED)
```bash
# Guacamole Configuration (UNCHANGED)
GUACAMOLE_SERVER_URL=http://guacamole-client:8080  # Internal container URL
GUACAMOLE_USERNAME=guacadmin                       # Admin username for API access
GUACAMOLE_PASSWORD=your-secure-password-here       # Admin password - CHANGE IN PRODUCTION!
GUACAMOLE_PORT=8090                                # External port for web UI debugging

# RDP Proxy (SIMPLIFIED)
RDP_PROXY_START_PORT=15900                         # Start port for local TCP proxies
# Removed: RDP_PROXY_MAX_CONNECTIONS (managed by connman instead)
# Removed: NATS configuration variables

# Agent Runner (SIMPLIFIED)
RDP_USER=zed                                       # XRDP username
RDP_PASSWORD=YOUR_SECURE_INITIAL_RDP_PASSWORD_HERE  # REQUIRED - Generate a unique password
# Removed: RDP_START_PORT (always 3389 locally)
# Removed: NATS configuration variables

# Database (for Guacamole connections)
POSTGRES_ADMIN_PASSWORD=your-postgres-password-here

# Reverse Dial (NEW)
REVDIAL_PATH=/revdial                              # Path for reverse dial endpoint
```

### Required .env Configuration
Add these to your `.env` file:
```bash
# Guacamole credentials (used by both containers and API)
GUACAMOLE_USERNAME=guacadmin
GUACAMOLE_PASSWORD=your-secure-guacamole-password

# Initial RDP password (for container startup - will be rotated dynamically)
# DEVELOPMENT: Uses insecure default if not set (docker-compose.dev.yaml only)
# PRODUCTION: Generate a unique password - DO NOT use default values!
# Generate with: openssl rand -base64 32
RDP_PASSWORD=YOUR_SECURE_INITIAL_RDP_PASSWORD_HERE

# Postgres password (used by Guacamole database)
POSTGRES_ADMIN_PASSWORD=your-secure-postgres-password
```

### Password Generation Commands
Generate secure passwords using these commands:
```bash
# Generate RDP password (32 characters)
openssl rand -base64 32

# Alternative using /dev/urandom (16 characters)
head -c 12 /dev/urandom | base64

# Generate all passwords at once
echo "RDP_PASSWORD=$(openssl rand -base64 32)"
echo "GUACAMOLE_PASSWORD=$(openssl rand -base64 32)"
echo "POSTGRES_ADMIN_PASSWORD=$(openssl rand -base64 32)"
```

### Docker Compose Configuration
The `docker-compose.dev.yaml` automatically configures:
- **guacamole-client** container with admin credentials
- **API server** with Guacamole connection info
- **PostgreSQL** database for connection storage
- **Port exposure** for debugging web interface

## Key Architecture Insights

### Reverse Dial Advantages
- **NAT Traversal**: Agents behind NAT/firewall can accept connections from control plane
- **Proven Technology**: revdial/connman code is well-tested and battle-proven
- **Simplified Protocol**: Direct TCP forwarding eliminates NATS message overhead
- **Lower Latency**: Fewer network hops and protocol conversions
- **Easier Debugging**: Standard TCP connections instead of custom NATS messaging

### Guacamole Component Separation (UNCHANGED)
- **Frontend JavaScript**: Browser rendering and user interaction
- **guacamole-client**: Server-side web app providing REST API and WebSocket tunnels
- **guacd**: Protocol conversion daemon (Guacamole ↔ RDP/VNC/SSH)

### Dynamic Connection Management
- **Admin API Access**: Helix API authenticates as `guacadmin` to access Guacamole REST API
- **On-Demand Creation**: Connections created programmatically per session/runner
- **Embedded Credentials**: RDP username/password stored in Guacamole connection definition
- **PostgreSQL Storage**: Connection metadata stored in Guacamole's database
- **WebSocket Tunneling**: Frontend connects via admin token + connection ID
- **Automatic Cleanup**: Connections removed when sessions end

### Session vs Runner Connection Handling (UNCHANGED)
- **Session RDP**: `/api/v1/sessions/{sessionID}/guac/proxy` - User workspace access
- **Runner RDP**: `/api/v1/external-agents/runners/{runnerID}/guac/proxy` - Admin debugging
- Both map to the same runner but with different connection contexts

### Security Configuration
- **Configurable Credentials**: No hardcoded passwords - all via environment variables
- **Runner Authentication**: Static runner token authentication via existing auth middleware
- **Admin Access**: Guacamole web interface requires authentication
- **Password Rotation**: RDP passwords change per session automatically
- **Database Security**: PostgreSQL credentials configurable via environment
- **Reverse Dial Security**: Authenticated endpoint prevents unauthorized connection establishment

### Reverse Dial Connection Flow
1. **Agent Runner Startup**: Establishes WebSocket + reverse dial connections to control plane
2. **Authentication**: Runner authenticates using static runner token via Authorization header or Bearer token
3. **Control Plane Registration**: Registers reverse dial connection in connman with runnerID
4. **RDP Request**: guacd connects to API TCP proxy on localhost:15900+
5. **Connection Lookup**: API proxy calls `connman.Dial(ctx, runnerID)` to get runner connection
6. **TCP Forwarding**: Direct bidirectional TCP copy between guacd and agent runner
7. **XRDP Forward**: Agent runner forwards TCP to localhost:3389 (XRDP server)

## Authentication & Security

### Guacamole Authentication Strategy

Guacamole authentication follows a **privileged proxy pattern** where the Helix API server acts as an administrative client:

**Architecture**: Helix API ↔ Guacamole Admin ↔ Dynamic RDP Connections

**Flow**:
1. **API Admin Login**: Helix API authenticates once as Guacamole admin (`guacadmin`)
2. **Token Acquisition**: Gets admin `authToken` for programmatic access
3. **Dynamic Connection Creation**: Uses admin token to create RDP connections per session
4. **Frontend Proxy**: Frontend connects via admin token + specific connection ID
5. **RDP Credentials**: Actual RDP username/password embedded in connection definition

**Benefits**:
- **No per-user Guacamole accounts** - Helix users never see Guacamole login
- **Centralized management** - API controls all Guacamole interactions
- **Session isolation** - Each session gets unique RDP connection
- **Dynamic provisioning** - Connections created/destroyed on demand

**Security Model** (RBAC Compliant):
```
Helix User → Helix Auth → API Server (as guacadmin) → Guacamole → RDP Server
     ↑              ↑                    ↑                ↑
  User Auth    Admin Token + Scoped     Connection ID    RDP Creds
               Connection ID            (Session Specific)
```

**RBAC Security Guarantees**:
1. **Frontend Never Gets Admin Token** - API server proxies all Guacamole connections
2. **Session Authorization** - `session.Owner != user.ID` check before connection creation
3. **Connection Scoping** - Each Guacamole connection tied to specific session via `GUAC_ID`
4. **Token + Connection ID** - Admin token provides access capability, connection ID provides scope
5. **No Cross-Session Access** - Users cannot access other users' connection IDs

**Critical Security Properties**:
- Admin token never exposed to frontend
- Connection IDs are session-specific and non-guessable
- Helix RBAC enforced at API layer before Guacamole interaction
- Each user can only access connections they own

### Addressing RBAC Security Concerns

**Q: Does the admin token allow access to all sessions?**

**A: No, due to connection scoping and API-level authorization:**

1. **Frontend Isolation**: Frontend never receives the admin token - only connects to Helix API
2. **API Authorization**: Helix API verifies `session.Owner == user.ID` before creating connections
3. **Connection Scoping**: Guacamole WebSocket URL includes specific `GUAC_ID` parameter
4. **Non-Guessable IDs**: Connection IDs are generated by Guacamole and non-predictable
5. **Proxy Pattern**: API server acts as trusted proxy, not token delegator

**Security Verification**:
```go
// 1. User must be authenticated
user := getRequestUser(r)
if user == nil { return unauthorized }

// 2. User must own the session
if session.Owner != user.ID { return forbidden }

// 3. Connection created with session-specific credentials
connectionID := createConnection(sessionID, rdpUsername, rdpPassword)

// 4. WebSocket proxied with scoped connection
ws://guacamole/websocket-tunnel?token=ADMIN&GUAC_ID=SESSION_SPECIFIC_ID
```

**Result**: Users can only access their own sessions, even though API uses admin token internally.

### Runner Authentication Strategy

The reverse dial endpoint uses the existing Helix authentication middleware with the static runner token:

**Endpoint**: `GET /api/v1/revdial?runnerid=<runner-id>`

**Authentication Methods**:
1. **Authorization Header**: `Authorization: Bearer <runner-token>`
2. **Query Parameter**: `?access_token=<runner-token>` (fallback for compatibility)

**Authentication Flow**:
1. Runner sends request to `/revdial` endpoint with runner token
2. Auth middleware validates token against `cfg.WebServer.RunnerToken`
3. If valid, creates user object with `TokenTypeRunner`
4. Handler verifies user has runner token type
5. Connection is established and registered in connman

**Security Benefits**:
- **Reuses existing auth infrastructure** - No duplicate authentication logic
- **Centralized token management** - Same runner token used for WebSocket and reverse dial
- **Proper authorization** - Only authenticated runners can establish reverse dial connections
- **Audit logging** - All connection attempts are logged with authentication status
- **Token rotation support** - Can be updated via configuration without code changes

**Configuration**:
```bash
# Set the runner token in environment or config
WEBSERVER_RUNNER_TOKEN=your-secure-runner-token-here
```

**Runner Usage Example**:
```bash
# Establish reverse dial connection
curl -X GET "https://api.helix.ml/api/v1/revdial?runnerid=runner-123" \
     -H "Authorization: Bearer your-secure-runner-token-here" \
     --http1.1 --no-buffer
```

## Error Handling

### Connection Failures (SIMPLIFIED)
- Graceful degradation when guacamole-client unavailable
- **Reverse Dial Timeout**: Handle `connman.Dial()` timeouts gracefully
- Timeout handling at each layer (shorter timeouts with direct TCP)
- User-friendly error messages
- **No Complex Fallbacks**: Direct TCP connection or failure (no NATS fallback needed)
- **WebSocket Proxy Failures**: Automatic failover if WebSocket upgrade fails

### Security Failures (UNCHANGED)
- Immediate connection termination on authentication failure
- Password rotation on security events
- Audit logging of access attempts
- Rate limiting for connection attempts
- Secure cleanup of Guacamole connections
- **Credential Security**: Environment-based configuration prevents hardcoded secrets

## Quick Setup Guide

1. **Development (Quick Start)**:
   ```bash
   # For local development only - uses insecure defaults with warnings
   docker-compose up -d
   # Container will warn about insecure development password
   # Control plane will automatically generate secure passwords and send to runners
   ```

2. **Production (Secure Setup)**:
   ```bash
   # Generate all required passwords
   echo "RDP_PASSWORD=$(openssl rand -base64 32)" >> .env
   echo "GUACAMOLE_PASSWORD=$(openssl rand -base64 32)" >> .env
   echo "POSTGRES_ADMIN_PASSWORD=$(openssl rand -base64 32)" >> .env
   echo "GUACAMOLE_USERNAME=guacadmin" >> .env
   ```

3. **Configure Environment** (Production Only):
   ```bash
   # Verify your .env file contains:
   cat .env
   # Should show:
   # RDP_PASSWORD=<generated-secure-password>
   # GUACAMOLE_PASSWORD=<generated-secure-password>
   # POSTGRES_ADMIN_PASSWORD=<generated-secure-password>
   # GUACAMOLE_USERNAME=guacadmin
   ```

4. **Start Services**:
   ```bash
   docker-compose up -d
   ```

5. **Access Debugging Interface**:
   - Guacamole Web UI: `http://localhost:8090/guacamole/`
   - Login with configured credentials
   - View dynamic connections created by Helix

6. **Test RDP Access**:
   - Runner RDP: API endpoint `/api/v1/external-agents/runners/{runnerID}/rdp-connection`
   - Session RDP: API endpoint `/api/v1/sessions/{sessionID}/rdp-connection`
   - Frontend automatically uses Guacamole proxy for connections

## Future Roadmap: Moonlight Protocol Integration

### Overview

As an enhancement to the current RDP/Guacamole architecture, we plan to integrate **Moonlight** protocol support for high-performance, GPU-accelerated remote desktop streaming. Moonlight provides superior performance for graphical workloads and gaming compared to traditional RDP.

### Moonlight Architecture Vision

```
Frontend Moonlight Client (WebRTC/WebCodecs) <-(H.264/H.265 Stream over WebSocket)->
    ↓
API Moonlight Proxy <-(WebSocket/WebRTC Signaling)->
    ↓
Moonlight Embedded Server <-(GameStream Protocol)->
    ↓
Agent Runner Reverse Dial Listener <-(TCP/UDP)->
    ↓
Sunshine Server (GPU-Accelerated Streaming)
```

### Key Advantages of Moonlight Integration

1. **GPU Hardware Acceleration**: Native NVENC/AMD VCE encoding for efficient streaming
2. **Low Latency**: Optimized for real-time interaction with sub-20ms latency
3. **High Quality**: Support for 4K@60fps streaming with adaptive bitrate
4. **Gaming Performance**: Designed specifically for interactive graphical applications
5. **HDR Support**: Wide color gamut and HDR streaming capabilities

### Implementation Components

#### 1. Sunshine Server (Agent Runner)
- **Purpose**: Replace XRDP with GPU-accelerated Sunshine streaming server
- **Protocol**: GameStream-compatible protocol (Moonlight standard)
- **GPU Acceleration**: Direct NVENC/VCE hardware encoding
- **Configuration**: Auto-detect optimal settings based on GPU capabilities

#### 2. API Moonlight Proxy
- **Location**: `api/pkg/server/moonlight_proxy.go` (future implementation)
- **Purpose**: WebSocket/WebRTC proxy for Moonlight protocol
- **Features**:
  - Protocol translation between WebRTC and GameStream
  - Session management and authentication
  - Adaptive bitrate control based on network conditions

#### 3. Frontend Moonlight Client
- **Technology**: WebRTC/WebCodecs for browser-native H.264/H.265 decoding
- **Purpose**: High-performance video streaming in browser
- **Features**:
  - Hardware-accelerated video decoding
  - Low-latency input capture and transmission
  - Adaptive quality based on network/device capabilities

### Migration Strategy

#### Phase 1: Parallel Implementation
- Implement Moonlight alongside existing RDP infrastructure
- Allow users to choose between RDP (compatibility) and Moonlight (performance)
- Maintain Guacamole for legacy support and troubleshooting

#### Phase 2: Capability Detection
- Auto-detect GPU capabilities on agent runners
- Prefer Moonlight for GPU-enabled runners, fallback to RDP for CPU-only
- Implement client capability detection for optimal protocol selection

#### Phase 3: Full Migration (Long-term)
- Default to Moonlight for all graphical workloads
- Retain RDP for text-based/headless scenarios
- Sunset Guacamole infrastructure for graphical applications

### Technical Requirements

#### Agent Runner Prerequisites
- **GPU**: NVIDIA GTX 1000+ or AMD RX 400+ series for hardware encoding
- **Software**: Sunshine server installation and configuration
- **Network**: UDP hole-punching support for optimal performance

#### API Server Changes
- **WebRTC Signaling**: Implement WebRTC signaling server functionality
- **Reverse Dial Integration**: Adapt existing reverse dial for UDP streams
- **Authentication**: Extend current auth model to Moonlight sessions

#### Frontend Enhancements
- **WebCodecs Support**: Modern browser requirement for hardware decode
- **Input Optimization**: Low-latency keyboard/mouse input handling
- **Quality Controls**: User-accessible bitrate and quality settings

### Performance Expectations

| Protocol | Latency | Quality | GPU Usage | Bandwidth | Use Case |
|----------|---------|---------|-----------|-----------|----------|
| **RDP (Current)** | 50-150ms | Good | None | 5-20 Mbps | General desktop, text editing |
| **Moonlight (Future)** | 5-20ms | Excellent | High | 10-100 Mbps | Graphics, gaming, video editing |

### Security Considerations

#### Authentication Integration
- Reuse existing Helix authentication infrastructure
- Session-based access control consistent with current RDP implementation
- Encrypted streams with same security model as current architecture

#### Network Security
- Maintain reverse dial architecture for NAT traversal
- Encrypted GameStream protocol (AES-256)
- Certificate-based authentication between components

### Implementation Priority

**High Priority:**
- Research Sunshine integration patterns
- WebRTC signaling server implementation
- Protocol detection and fallback mechanisms

**Medium Priority:**
- Frontend WebCodecs client development
- Performance optimization and tuning
- Comprehensive testing across GPU types

**Low Priority:**
- HDR and advanced color support
- Multi-monitor streaming optimization
- Advanced quality control features

### Success Metrics

1. **Latency Reduction**: Target <20ms end-to-end latency for local network
2. **Quality Improvement**: Support for 1080p@60fps minimum, 4K@60fps target
3. **Resource Efficiency**: Reduce CPU usage by 70% through GPU offloading
4. **User Experience**: Seamless protocol selection based on capabilities
5. **Compatibility**: Maintain 100% backward compatibility with RDP fallback

This roadmap positions Helix to offer industry-leading remote desktop performance while maintaining the robust architecture and security model established with the current RDP/Guacamole implementation.
