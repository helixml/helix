# Wolf RevDial Client

The Wolf RevDial client enables remote Wolf instances to connect to the Helix control plane, establishing a reverse tunnel for multi-Wolf distributed deployment.

## Purpose

**Problem**: Wolf instances may be deployed behind NAT/firewalls where the control plane cannot make inbound connections.

**Solution**: Wolf makes an **outbound** WebSocket connection to the control plane, establishing a RevDial tunnel. The control plane can then make requests to Wolf's API through this tunnel, enabling:
- Remote Wolf deployment (separate hardware from control plane)
- NAT/firewall traversal (no inbound ports required)
- Multi-Wolf distributed architecture
- Kubernetes deployment without host Docker socket

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Helix Control Plane                      │
│  ┌────────────────────────────────────────────────────────┐  │
│  │ API Server                                             │  │
│  │  - RevDial listener at /api/v1/revdial                 │  │
│  │  - Routes requests over RevDial tunnels                │  │
│  └─────────────▲──────────────────────────────────────────┘  │
│                │                                              │
└────────────────┼──────────────────────────────────────────────┘
                 │
         (outbound RevDial WebSocket)
                 │
┌────────────────┼──────────────────────────────────────────────┐
│  Remote Wolf Instance (behind NAT)                            │
│  ┌─────────────┴──────────────────────────────────────────┐   │
│  │ wolf-revdial-client                                    │   │
│  │  - Connects to control plane                           │   │
│  │  - Proxies requests to local Wolf API                  │   │
│  └────────────────────────────────────────────────────────┘   │
│                                                                │
│  ┌────────────────────────────────────────────────────────┐   │
│  │ Wolf Container                                         │   │
│  │  - Local API: http://localhost:8080                    │   │
│  │  - Manages sandboxes, streaming                        │   │
│  └────────────────────────────────────────────────────────┘   │
└────────────────────────────────────────────────────────────────┘
```

## Usage

### Environment Variables

The client reads configuration from environment variables:

- `HELIX_API_URL` - Control plane API URL (e.g., `http://api.example.com:8080`)
- `WOLF_ID` - Unique Wolf instance ID (e.g., `wolf-1`, `wolf-us-west-2`)
- `RUNNER_TOKEN` - Runner authentication token (from control plane)

### Command Line Flags

Flags override environment variables:

```bash
wolf-revdial-client \
  -api-url http://api.example.com:8080 \
  -wolf-id wolf-1 \
  -token <runner-token> \
  -local localhost:8080 \
  -reconnect 5
```

**Flags**:
- `-api-url` - Control plane API URL
- `-wolf-id` - Unique Wolf instance ID
- `-token` - Runner authentication token
- `-local` - Local Wolf API address (default: `localhost:8080`)
- `-reconnect` - Reconnect interval in seconds (default: `5`)

### Docker Compose Example

Add the RevDial client as a sidecar to Wolf:

```yaml
services:
  wolf:
    image: ghcr.io/helixml/wolf:latest
    ports:
      - "8080:8080"  # Local only (not exposed to host network)
    # ... other Wolf configuration

  wolf-revdial-client:
    image: ghcr.io/helixml/helix/wolf-revdial-client:latest
    environment:
      HELIX_API_URL: "http://api.example.com:8080"
      WOLF_ID: "wolf-1"
      RUNNER_TOKEN: "${RUNNER_TOKEN}"  # Set via .env file
    depends_on:
      - wolf
    restart: unless-stopped
```

### Kubernetes Deployment

Deploy Wolf + RevDial client as a paired pod:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: wolf-instance-1
  labels:
    app: wolf
spec:
  containers:
  - name: wolf
    image: ghcr.io/helixml/wolf:latest
    ports:
    - containerPort: 8080
      name: api

  - name: revdial-client
    image: ghcr.io/helixml/helix/wolf-revdial-client:latest
    env:
    - name: HELIX_API_URL
      value: "https://api.example.com"
    - name: WOLF_ID
      valueFrom:
        fieldRef:
          fieldPath: metadata.name  # Use pod name as Wolf ID
    - name: RUNNER_TOKEN
      valueFrom:
        secretKeyRef:
          name: wolf-token
          key: token
```

## Building

### From Source

```bash
# Build binary
go build -o wolf-revdial-client ./api/cmd/wolf-revdial-client/

# Run locally
./wolf-revdial-client \
  -api-url http://localhost:8080 \
  -wolf-id wolf-dev \
  -token dev-token
```

### Docker Image

```dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o wolf-revdial-client ./api/cmd/wolf-revdial-client/

FROM alpine:latest
RUN apk --no-cache add ca-certificates
COPY --from=builder /build/wolf-revdial-client /usr/local/bin/
ENTRYPOINT ["/usr/local/bin/wolf-revdial-client"]
```

## How It Works

1. **Connection Establishment**:
   - Client connects to `ws://api.example.com:8080/api/v1/revdial?runnerid=wolf-{wolf_id}`
   - Sends `Authorization: Bearer {RUNNER_TOKEN}` header
   - WebSocket connection upgraded to RevDial tunnel

2. **RevDial Listener**:
   - Creates a `net.Listener` that accepts connections from the control plane
   - Each connection represents an HTTP request from API to Wolf

3. **Proxying**:
   - Accepts RevDial connection from API
   - Opens TCP connection to local Wolf API (`localhost:8080`)
   - Bidirectionally proxies data between RevDial tunnel and local Wolf API

4. **Auto-Reconnect**:
   - If connection drops, automatically reconnects after 5 seconds (configurable)
   - Continues retrying until successful or process terminated

## Security Considerations

**Authentication**: Uses `RUNNER_TOKEN` for authentication with control plane. This token:
- Should be kept secret (environment variable or Kubernetes secret)
- Grants permission to register as a Wolf instance
- Validated by control plane before accepting RevDial connection

**Network Isolation**: Wolf API only listens on `localhost:8080`:
- Not exposed to host network
- Only accessible via RevDial tunnel or localhost
- Prevents direct network access to Wolf API

**TLS**: For production deployments, use `wss://` (WebSocket over TLS):
```bash
HELIX_API_URL=https://api.example.com  # Automatically uses wss://
```

## Troubleshooting

### Connection Refused

**Symptom**: `failed to connect: connection refused`

**Cause**: Control plane not reachable or `/api/v1/revdial` endpoint not available

**Solution**: Verify control plane URL and network connectivity:
```bash
curl http://api.example.com:8080/healthz
```

### Authentication Failed

**Symptom**: `failed to connect: status: 401`

**Cause**: Invalid or missing `RUNNER_TOKEN`

**Solution**: Verify token is correct and has permission to register Wolf instances

### Local Wolf API Unreachable

**Symptom**: `Failed to connect to local Wolf API at localhost:8080`

**Cause**: Wolf container not running or API not listening on expected port

**Solution**: Verify Wolf is running:
```bash
curl http://localhost:8080/api/v1/apps
docker ps | grep wolf
```

## Testing

### Local Development

1. Start control plane API:
```bash
docker compose -f docker-compose.dev.yaml up api
```

2. Start Wolf (locally):
```bash
# In wolf directory
docker compose up
```

3. Run RevDial client:
```bash
go run ./api/cmd/wolf-revdial-client/ \
  -api-url http://localhost:8080 \
  -wolf-id wolf-dev \
  -token <runner-token>
```

4. Verify connection:
```bash
# Check API logs for "Registered reverse dial connection"
docker compose -f docker-compose.dev.yaml logs api | grep "reverse dial"

# Test Wolf API via RevDial
curl http://localhost:8080/api/v1/wolf/apps \
  -H "Authorization: Bearer <user-token>"
```

## Related Documentation

- [Distributed Wolf Architecture](../../../design/2025-11-22-distributed-wolf-revdial-architecture.md)
- [DinD + RevDial Implementation Status](../../../design/2025-11-23-wolf-dind-revdial-implementation-status.md)
- [RevDial Package](../../pkg/revdial/revdial.go)
- [Sandbox RevDial Client](../revdial-client/main.go)
