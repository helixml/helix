# RDP over NATS with Guacamole Architecture

## Overview

This document describes the architecture for providing RDP access to external agent runners through a Guacamole-based remote desktop solution that uses NATS for communication.

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
API RDP TCP Proxy <-(RDP over NATS)->
    ↓
Agent Runner Server <-(RDP)->
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

### 5. API RDP TCP Proxy
- **Location**: `api/pkg/server/rdp_proxy_handlers.go`
- **Purpose**: TCP proxy that forwards RDP traffic via NATS
- **Protocol**: RDP over NATS
- **Ports**: Dynamic allocation (15900+)
- **Connects to**: Agent Runner via NATS

### 6. Agent Runner Server
- **Location**: `api/pkg/external-agent/runner.go`
- **Purpose**: NATS ↔ local XRDP bridge
- **Protocol**: RDP
- **Connects to**: Local XRDP server

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
- **Generation**: Control plane generates cryptographically secure passwords
- **Rotation**: New password per session for enhanced security
- **Storage**: Passwords stored in `agent_runners` table
- **Transmission**: Passwords sent securely to runner via NATS

### Authentication Flow
1. Frontend authenticates with API
2. API validates user permissions (session ownership/admin)
3. API creates proxy connections with secure passwords
4. Runner receives password and configures XRDP
5. Connection established with per-session credentials

## Implementation Status

### ✅ Completed
- API RDP TCP Proxy with NATS communication
- Agent Runner RDP forwarding to XRDP
- Agent Runner database table and password management
- Frontend Guacamole JavaScript client
- Session and runner API endpoints
- Guacamole server containers (guacd + guacamole-client)

### ❌ Missing (Priority Order)
1. **API Guacamole Proxy** - WebSocket proxy to guacamole-client container
2. **Guacamole REST API Integration** - Dynamic connection creation/management via API
3. **Guacamole Web Interface Proxy** - Expose guacamole-client web UI for debugging
4. **Connection Lifecycle Management** - Create/update/delete connections when runners start/stop
5. **Password Synchronization** - Update Guacamole connections when passwords rotate

## Next Steps

### Phase 1: API Guacamole Proxy
Create WebSocket proxy in API server that:
- Accepts Guacamole protocol WebSocket connections from frontend
- Forwards to guacamole-client:8080 WebSocket tunnel
- Handles bidirectional Guacamole protocol traffic
- Manages connection lifecycle and authentication

### Phase 2: Guacamole REST API Integration
Implement dynamic connection management:
- Authenticate with guacamole-client REST API
- Create RDP connections programmatically via `/api/session/data/postgresql/connections`
- Update connection parameters when passwords rotate
- Clean up connections when sessions end

### Phase 3: Guacamole Web Interface Proxy
Expose guacamole-client web interface:
- Proxy `/guacamole/*` routes to guacamole-client:8080
- Enable debugging and connection management via web UI
- Maintain authentication and access control

### Phase 4: Connection Lifecycle Management
Integrate with session/runner lifecycle:
- Create Guacamole connections when runners come online
- Update passwords when sessions start (password rotation)
- Clean up stale connections
- Handle runner reconnection scenarios

### Phase 5: End-to-End Testing
- Test complete flow from frontend to XRDP
- Verify password rotation and security
- Test both session and runner connection types
- Debug via Guacamole web interface
- Performance and reliability testing

## Configuration

### Environment Variables
```bash
# Guacamole Configuration
GUACAMOLE_CLIENT_URL=http://guacamole-client:8080
GUACAMOLE_USERNAME=guacadmin  
GUACAMOLE_PASSWORD=guacadmin
GUACAMOLE_DATA_SOURCE=postgresql

# RDP Proxy
RDP_PROXY_START_PORT=15900
RDP_PROXY_MAX_CONNECTIONS=100

# Agent Runner
RDP_START_PORT=3389
RDP_USER=zed

# Development - Expose Guacamole Web UI
GUACAMOLE_PORT=8090  # External port for debugging
```

## Key Architecture Insights

### Guacamole Component Separation
- **Frontend JavaScript**: Browser rendering and user interaction
- **guacamole-client**: Server-side web app providing REST API and WebSocket tunnels  
- **guacd**: Protocol conversion daemon (Guacamole ↔ RDP/VNC/SSH)

### Dynamic Connection Management
- Guacamole supports REST API for creating connections programmatically
- No need to pre-configure connections - create them on-demand
- Connection parameters include hostname, port, credentials
- Connections stored in PostgreSQL database

### Session vs Runner Connection Handling
- **Session RDP**: `/api/v1/sessions/{sessionID}/guac/proxy` - User workspace access
- **Runner RDP**: `/api/v1/external-agents/runners/{runnerID}/guac/proxy` - Admin debugging
- Both map to the same runner but with different connection contexts

## Error Handling

### Connection Failures
- Graceful degradation when guacamole-client unavailable
- Retry logic for NATS communication  
- Timeout handling at each layer
- User-friendly error messages
- Fallback to direct RDP proxy if needed

### Security Failures
- Immediate connection termination on authentication failure
- Password rotation on security events
- Audit logging of access attempts
- Rate limiting for connection attempts
- Secure cleanup of Guacamole connections