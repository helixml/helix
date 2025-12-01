# TURN-over-TLS Fallback for Restrictive Networks

**Date:** 2025-12-01
**Status:** Implemented

## Problem

Enterprise customers behind restrictive firewalls cannot use WebRTC streaming because:
1. Only HTTPS connections on port 443 are allowed
2. UDP traffic is blocked entirely
3. Non-standard TCP ports (3478, 40000-40100) are blocked

Current Helix TURN configuration only supports:
- `turn:host:3478?transport=udp` - blocked
- `turn:host:3478?transport=tcp` - blocked (non-443 port)

## Solution

Add TURNS (TURN over TLS) on port 443 as a last-resort fallback.

### Connection Priority (WebRTC ICE)

1. **Direct UDP** (40000-40100) - Best latency, works on open networks
2. **TURN/UDP** (3478) - Relay when direct fails, still low latency
3. **TURN/TCP** (3478) - TCP relay, higher latency
4. **TURNS/TLS** (443) - TLS-wrapped TCP relay, works through enterprise firewalls

### Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Enterprise Network                          │
│   ┌──────────┐                                                      │
│   │ Browser  │ ─── HTTPS only, port 443 ───┐                        │
│   └──────────┘                             │                        │
└────────────────────────────────────────────│────────────────────────┘
                                             │
                                             ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    Helix Control Plane                              │
│                                                                     │
│   ┌─────────────────────────────────────────────────────────────┐   │
│   │            TLS Termination (Caddy / External Proxy)          │   │
│   │                                                             │   │
│   │   HTTPS:443  ──────────────────────────▶  API:8080 (HTTP)   │   │
│   │                                                             │   │
│   │   TURNS:443 (TLS) ─────────────────────▶  TURN:5349 (TCP)   │   │
│   │   (path-based or port-based routing)                        │   │
│   └─────────────────────────────────────────────────────────────┘   │
│                                                                     │
│   ┌───────────────────────────────┐                                 │
│   │        TURN Server            │                                 │
│   │   (pion/turn)                 │                                 │
│   │                               │                                 │
│   │   0.0.0.0:3478 UDP  ◄── Standard TURN/UDP                       │
│   │   0.0.0.0:3478 TCP  ◄── Standard TURN/TCP                       │
│   │   0.0.0.0:5349 TCP  ◄── Internal TCP (behind TLS proxy)         │
│   └───────────────────────────────┘                                 │
│                │                                                    │
│                │ UDP relay                                          │
│                ▼                                                    │
│   ┌───────────────────────────────┐                                 │
│   │   Moonlight Web Stream        │                                 │
│   └───────────────────────────────┘                                 │
└─────────────────────────────────────────────────────────────────────┘
```

### Why This Approach?

1. **TLS termination at proxy level** - Helix already runs behind Caddy/TLS proxy
2. **No TLS code in TURN server** - Simpler, leverages existing infrastructure
3. **pion/turn ListenerConfig** - Supports TCP via `net.Listener` interface
4. **Auto-detection of API scheme** - If `API_HOST` starts with `https://`, enable TURNS

## Implementation

### 1. API Configuration Changes

New environment variables:
```bash
TURN_TCP_PORT=5349              # Internal TCP port for TURNS (behind TLS proxy)
TURN_TLS_ENABLED=auto           # auto|true|false - auto enables if API_HOST is https
```

### 2. TURN Server Changes (`api/pkg/turn/server.go`)

Add TCP listener alongside UDP:
```go
// Create TCP listener for TURNS (TLS terminated by proxy)
tcpListener, err := net.Listen("tcp", "0.0.0.0:"+strconv.Itoa(cfg.TCPPort))

turn.NewServer(turn.ServerConfig{
    // Existing UDP config
    PacketConnConfigs: []turn.PacketConnConfig{...},
    // New TCP config for TURNS
    ListenerConfigs: []turn.ListenerConfig{
        {
            Listener: tcpListener,
            RelayAddressGenerator: &turn.RelayAddressGeneratorStatic{...},
        },
    },
})
```

### 3. Moonlight Web Config (`moonlight-web-config/config.json.template`)

Add TURNS URL:
```json
{
  "webrtc_ice_servers": [
    {
      "urls": ["stun:stun.l.google.com:19302", ...]
    },
    {
      "urls": [
        "turn:{{TURN_PUBLIC_IP}}:3478?transport=udp",
        "turn:{{TURN_PUBLIC_IP}}:3478?transport=tcp",
        "turns:{{TURN_PUBLIC_IP}}:443?transport=tcp"
      ],
      "username": "helix",
      "credential": "{{TURN_PASSWORD}}"
    }
  ]
}
```

### 4. Install Script Changes

Auto-detect HTTPS and configure TURNS:
```bash
# Extract scheme from API_HOST
if [[ "$API_HOST" == https://* ]]; then
    TURN_TLS_ENABLED=true
    TURN_PUBLIC_HOST=$(echo "$API_HOST" | sed -E 's|^https?://||' | sed 's|:[0-9]+$||')
fi
```

### 5. Caddy Configuration (for control plane)

```caddyfile
helix.example.com {
    # Existing API routes
    reverse_proxy /api/* api:8080

    # TURNS proxy to internal TCP port
    # Note: Caddy doesn't support raw TCP proxying, need alternative
}
```

**Alternative for TURNS:** Since Caddy doesn't support raw TCP/TLS passthrough for TURN:

**Option A: Separate port for TURNS**
- Use port 5349 directly (standard TURNS port)
- Configure firewall to allow 5349

**Option B: HAProxy/Nginx for TLS passthrough**
- Front Helix with HAProxy/Nginx for TURNS
- Caddy handles HTTP(S), HAProxy handles TURNS

**Option C: TURN server handles TLS directly**
- pion/turn can wrap listener with `tls.NewListener()`
- Requires TLS certificates in TURN server config

## Proxy Compatibility

**IMPORTANT:** Standard HTTP reverse proxies (Nginx in HTTP mode, Caddy) **cannot**
forward TURN traffic because they parse HTTP protocol. TURN is a binary protocol.

### What Works

1. **TCP/TLS Passthrough Mode**
   - Proxy forwards raw TCP without HTTP parsing
   - HAProxy TCP mode, Nginx stream module, AWS NLB

2. **SNI-Based Routing** (Recommended for Enterprise)
   - Use separate subdomain: `turns.helix.example.com`
   - Proxy routes based on TLS SNI before decryption
   - `helix.example.com` → Caddy → API (HTTP)
   - `turns.helix.example.com` → TCP passthrough → TURN

3. **Same-Port Multiplexing** (Advanced)
   - Helix API's MuxListener detects STUN/TURN vs HTTP by first byte
   - Requires proxy to forward **all** TCP traffic, not just HTTP
   - Works when TLS is terminated at proxy, raw TCP forwarded to API

### Recommended Setup for Enterprise

```
                                   ┌─────────────────────────────────┐
                                   │  Enterprise Load Balancer       │
Internet ──── Port 443 (TLS) ─────▶│  (HAProxy/F5/AWS ALB+NLB)       │
                                   │                                 │
                                   │  SNI: helix.example.com         │
                                   │    → HTTP Mode → API:8080       │
                                   │                                 │
                                   │  SNI: turns.helix.example.com   │
                                   │    → TCP Passthrough → API:8080 │
                                   │    (TLS terminated, TCP forwarded)│
                                   └─────────────────────────────────┘
                                              │
                                              ▼
                                   ┌─────────────────────────────────┐
                                   │  Helix API (port 8080)          │
                                   │                                 │
                                   │  MuxListener detects protocol:  │
                                   │  - First byte 0-3 → TURN        │
                                   │  - First byte 'G','P' → HTTP    │
                                   └─────────────────────────────────┘
```

### HAProxy Example Configuration

```haproxy
frontend https_in
    bind *:443 ssl crt /etc/ssl/helix.pem

    # Route based on SNI
    use_backend turn_backend if { ssl_fc_sni turns.helix.example.com }
    default_backend api_backend

backend api_backend
    mode http
    server api helix-api:8080

backend turn_backend
    mode tcp
    server turn helix-api:8080
```

### Nginx Stream Example (for TURNS)

```nginx
stream {
    upstream turn_backend {
        server helix-api:8080;
    }

    server {
        listen 5349 ssl;
        ssl_certificate /etc/ssl/helix.pem;
        ssl_certificate_key /etc/ssl/helix.key;

        proxy_pass turn_backend;
    }
}
```

## Recommendation

For enterprise customers behind restrictive firewalls:

1. **Use SNI-based routing** with separate subdomain `turns.helix.example.com`
2. Configure load balancer to TLS-terminate and TCP-forward to API:8080
3. Helix API's MuxListener handles protocol detection internally
4. Moonlight config advertises both:
   - `turn:helix.example.com:3478` (for UDP/TCP when not blocked)
   - `turns:turns.helix.example.com:443` (for TLS fallback)

## Testing

1. Block UDP at firewall level
2. Block TCP ports except 443
3. Verify WebRTC connects via TURNS
4. Check latency impact of TLS overhead

## Implementation Summary

### Files Modified

1. **`api/pkg/turn/server.go`**
   - Added `TCPListener`, `TURNSHost`, `TURNSPort` to Config
   - Added `ListenerConfigs` for TCP-based TURN
   - Updated `GetURLs()` to include TURNS URLs

2. **`api/pkg/turn/mux.go`** (NEW)
   - Protocol multiplexer that detects STUN/TURN vs HTTP by first byte
   - Routes TURN traffic to pion/turn, HTTP to standard handler
   - Enables TURNS on same port as API

3. **`api/pkg/config/config.go`**
   - Added `TURNSEnabled`, `TURNSHost`, `TURNSPort` to TURN config

4. **`api/cmd/helix/serve.go`**
   - Integrated MuxListener when TURNS is enabled
   - Passes HTTP listener to server, TURN listener to pion/turn

5. **`api/pkg/server/server.go`**
   - Added `SetListener()` method for custom listener support

6. **`moonlight-web-config/config.json.template`**
   - Changed from `{{TURN_PUBLIC_IP}}` to `{{TURN_URLS}}` array

7. **`moonlight-web-config/init-moonlight-config.sh`**
   - Builds TURN URLs array dynamically
   - Adds TURNS URL when `TURNS_HOST` is set
   - Uses jq for JSON updates

8. **`Dockerfile.sandbox`**
   - Added jq package for JSON manipulation
   - Updated inline init script to build TURN URLs array

9. **`docker-compose.dev.yaml`**
   - Added `TURNS_ENABLED`, `TURNS_HOST`, `TURNS_PORT` env vars to API
   - Added `TURNS_HOST`, `TURNS_PORT` to sandbox containers

10. **`install.sh`**
    - Added TURNS config variables to .env template

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `TURNS_ENABLED` | `false` | Enable TURNS on API port |
| `TURNS_HOST` | (empty) | External hostname for TURNS |
| `TURNS_PORT` | `443` | External port for TURNS |

### Usage

For enterprise customers behind restrictive firewalls:

1. Configure load balancer with SNI-based routing:
   - `helix.example.com` → HTTP proxy → API:8080
   - `turns.helix.example.com` → TCP passthrough → API:8080

2. Set environment variables:
   ```bash
   TURNS_ENABLED=true
   TURNS_HOST=turns.helix.example.com
   TURNS_PORT=443
   ```

3. WebRTC clients will automatically try:
   - Direct UDP (fastest)
   - TURN/UDP on 3478
   - TURN/TCP on 3478
   - TURNS/TLS on 443 (fallback for restrictive networks)

## Rollout

1. ✅ Add TCP listener to TURN server (backward compatible)
2. ✅ Update Moonlight config template with TURNS URL
3. ✅ Document proxy configuration for TURNS
4. ⏳ Test with enterprise customer
