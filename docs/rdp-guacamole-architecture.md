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

## Implementation Status

### ✅ Completed
- API RDP TCP Proxy with NATS communication
- Agent Runner RDP forwarding to XRDP
- Agent Runner database table and password management
- **Runner password synchronization** - Control plane sends passwords to runners via ZedAgent WebSocket
- **Connection lifecycle management** - Create/update Guacamole connections when runners connect
- Frontend Guacamole JavaScript client
- Session and runner API endpoints
- Guacamole server containers (guacd + guacamole-client)

### ❌ Missing (Priority Order)
1. **API Guacamole Proxy** - WebSocket proxy to guacamole-client container
2. **Guacamole REST API Integration** - Dynamic connection creation/management via API  
3. **Guacamole Web Interface Proxy** - Expose guacamole-client web UI for debugging

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
GUACAMOLE_SERVER_URL=http://guacamole-client:8080  # Internal container URL
GUACAMOLE_USERNAME=guacadmin                       # Admin username for API access
GUACAMOLE_PASSWORD=your-secure-password-here       # Admin password - CHANGE IN PRODUCTION!
GUACAMOLE_PORT=8090                                # External port for web UI debugging

# RDP Proxy
RDP_PROXY_START_PORT=15900
RDP_PROXY_MAX_CONNECTIONS=100

# Agent Runner
RDP_START_PORT=3389
RDP_USER=zed
RDP_PASSWORD=YOUR_SECURE_INITIAL_RDP_PASSWORD_HERE  # REQUIRED - Generate a unique password

# Database (for Guacamole connections)
POSTGRES_ADMIN_PASSWORD=your-postgres-password-here
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

### Guacamole Component Separation
- **Frontend JavaScript**: Browser rendering and user interaction
- **guacamole-client**: Server-side web app providing REST API and WebSocket tunnels  
- **guacd**: Protocol conversion daemon (Guacamole ↔ RDP/VNC/SSH)

### Dynamic Connection Management
- Guacamole supports REST API for creating connections programmatically
- No need to pre-configure connections - create them on-demand
- Connection parameters include hostname, port, credentials
- Connections stored in PostgreSQL database
- **WebSocket Support**: API proxy handles WebSocket upgrades for tunnel connections

### Session vs Runner Connection Handling
- **Session RDP**: `/api/v1/sessions/{sessionID}/guac/proxy` - User workspace access
- **Runner RDP**: `/api/v1/external-agents/runners/{runnerID}/guac/proxy` - Admin debugging
- Both map to the same runner but with different connection contexts

### Security Configuration
- **Configurable Credentials**: No hardcoded passwords - all via environment variables
- **Admin Access**: Guacamole web interface requires authentication
- **Password Rotation**: RDP passwords change per session automatically
- **Database Security**: PostgreSQL credentials configurable via environment

## Error Handling

### Connection Failures
- Graceful degradation when guacamole-client unavailable
- Retry logic for NATS communication  
- Timeout handling at each layer
- User-friendly error messages
- Fallback to direct RDP proxy if needed
- **WebSocket Proxy Failures**: Automatic failover if WebSocket upgrade fails

### Security Failures
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