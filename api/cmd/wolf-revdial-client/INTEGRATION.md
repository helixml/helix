# Wolf RevDial Client Integration Guide

This guide shows how to integrate the Wolf RevDial client into your Wolf deployment for distributed, multi-Wolf architecture.

## Prerequisites

- Control plane API running with RevDial support
- Wolf instance (local or remote)
- Runner token from control plane
- Docker or Kubernetes environment

## Integration Options

### Option 1: Docker Compose (Recommended for Single Instance)

Use the provided example docker-compose file:

```bash
# 1. Copy example configuration
cp api/cmd/wolf-revdial-client/docker-compose.example.yaml docker-compose.yaml

# 2. Create .env file
cat > .env <<EOF
HELIX_API_URL=https://api.example.com
WOLF_ID=wolf-1
RUNNER_TOKEN=your-runner-token-here
EOF

# 3. Start Wolf + RevDial client
docker compose up -d

# 4. Verify connection
docker compose logs wolf-revdial-client
# Should see: "✅ Connected to control plane via RevDial"
```

### Option 2: Systemd Service (Bare Metal)

For bare metal deployments without Docker Compose:

```bash
# 1. Build and install binary
go build -o /usr/local/bin/wolf-revdial-client ./api/cmd/wolf-revdial-client/
chmod +x /usr/local/bin/wolf-revdial-client

# 2. Create systemd service
cat > /etc/systemd/system/wolf-revdial-client.service <<EOF
[Unit]
Description=Wolf RevDial Client
After=network.target wolf.service
Requires=wolf.service

[Service]
Type=simple
Environment="HELIX_API_URL=https://api.example.com"
Environment="WOLF_ID=wolf-1"
Environment="RUNNER_TOKEN=your-runner-token-here"
ExecStart=/usr/local/bin/wolf-revdial-client
Restart=always
RestartSec=5
User=wolf
Group=wolf

[Install]
WantedBy=multi-user.target
EOF

# 3. Enable and start service
systemctl daemon-reload
systemctl enable wolf-revdial-client
systemctl start wolf-revdial-client

# 4. Check status
systemctl status wolf-revdial-client
journalctl -u wolf-revdial-client -f
```

### Option 3: Kubernetes DaemonSet (Multi-Node Deployment)

Deploy Wolf + RevDial client to all GPU nodes:

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: wolf-instance
  namespace: helix
spec:
  selector:
    matchLabels:
      app: wolf
  template:
    metadata:
      labels:
        app: wolf
    spec:
      # Only schedule on nodes with GPUs
      nodeSelector:
        nvidia.com/gpu: "true"

      runtimeClassName: sysbox-runc  # Or standard runtime for privileged containers

      containers:
      # Wolf container
      - name: wolf
        image: ghcr.io/helixml/wolf:latest
        securityContext:
          privileged: true  # Required for Docker-in-Docker
        env:
        - name: WOLF_ID
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName  # Use node name as Wolf ID
        volumeMounts:
        - name: docker-storage
          mountPath: /var/lib/docker
        resources:
          limits:
            nvidia.com/gpu: 1

      # RevDial client container
      - name: revdial-client
        image: ghcr.io/helixml/helix/wolf-revdial-client:latest
        env:
        - name: HELIX_API_URL
          value: "https://api.helix.example.com"
        - name: WOLF_ID
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName
        - name: RUNNER_TOKEN
          valueFrom:
            secretKeyRef:
              name: wolf-runner-token
              key: token

      volumes:
      - name: docker-storage
        emptyDir: {}

---
apiVersion: v1
kind: Secret
metadata:
  name: wolf-runner-token
  namespace: helix
type: Opaque
stringData:
  token: "your-runner-token-here"
```

Deploy:
```bash
kubectl apply -f wolf-daemonset.yaml
kubectl -n helix get pods -l app=wolf
kubectl -n helix logs -l app=wolf -c revdial-client
```

### Option 4: Embedded in Wolf Container

Add RevDial client directly to Wolf image:

```dockerfile
# In Wolf Dockerfile
FROM ghcr.io/helixml/wolf:base

# Install Wolf RevDial client
COPY --from=helix-builder /build/wolf-revdial-client /usr/local/bin/

# Startup script runs both Wolf and RevDial client
COPY wolf-entrypoint.sh /usr/local/bin/
ENTRYPOINT ["/usr/local/bin/wolf-entrypoint.sh"]
```

**wolf-entrypoint.sh**:
```bash
#!/bin/bash
set -e

# Start RevDial client in background
/usr/local/bin/wolf-revdial-client &
REVDIAL_PID=$!

# Start Wolf in foreground
/usr/local/bin/wolf

# Cleanup on exit
kill $REVDIAL_PID 2>/dev/null || true
```

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `HELIX_API_URL` | Yes | - | Control plane API URL |
| `WOLF_ID` | Yes | - | Unique Wolf instance ID |
| `RUNNER_TOKEN` | Yes | - | Runner authentication token |

### Runner Token Generation

Generate a runner token from the control plane:

```bash
# Using Helix CLI
helix runner create-token --name wolf-1

# Or via API
curl -X POST https://api.example.com/api/v1/runner-tokens \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  -d '{"name": "wolf-1", "type": "wolf"}'
```

### Wolf ID Naming Convention

Choose descriptive Wolf IDs:
- Geographic location: `wolf-us-west-2`, `wolf-eu-central-1`
- GPU type: `wolf-nvidia-rtx-4090`, `wolf-amd-mi250`
- Purpose: `wolf-production-1`, `wolf-dev`

**Requirements**:
- Must be unique across all Wolf instances
- Alphanumeric + hyphens only
- Max 255 characters

## Verification

### Check Connection Status

**Docker Compose**:
```bash
docker compose logs wolf-revdial-client | grep "Connected"
# Should see: ✅ Connected to control plane via RevDial
```

**Systemd**:
```bash
journalctl -u wolf-revdial-client | grep "Connected"
```

**Kubernetes**:
```bash
kubectl logs -l app=wolf -c revdial-client | grep "Connected"
```

### Test Wolf API via RevDial

From the control plane, test routing to Wolf:

```bash
# List Wolf instances (should show your new Wolf)
curl https://api.example.com/api/v1/wolf-instances \
  -H "Authorization: Bearer ${USER_TOKEN}"

# Test Wolf API via RevDial tunnel
curl https://api.example.com/api/v1/wolf/apps?wolf_id=wolf-1 \
  -H "Authorization: Bearer ${USER_TOKEN}"
```

### Common Issues

**Connection Refused**:
- Verify `HELIX_API_URL` is correct
- Check firewall rules allow outbound WebSocket connections
- Ensure control plane `/api/v1/revdial` endpoint is accessible

**Authentication Failed**:
- Verify `RUNNER_TOKEN` is valid
- Check token hasn't expired
- Ensure token has `wolf` type permissions

**Local Wolf API Unreachable**:
- Verify Wolf is running: `curl http://localhost:8080/api/v1/apps`
- Check Wolf API port (default 8080)
- Ensure RevDial client can reach Wolf on same network

## Monitoring

### Metrics

The RevDial client logs important events:

```
[timestamp] Wolf RevDial client starting...
[timestamp] Connecting to control plane via RevDial: ws://api.example.com:8080/api/v1/revdial?runnerid=wolf-wolf-1
[timestamp] ✅ Connected to control plane via RevDial
[timestamp] ✅ RevDial listener ready, proxying connections to local Wolf API at localhost:8080
[timestamp] Accepted RevDial connection, proxying to local Wolf API at localhost:8080
[timestamp] Established proxy connection: RevDial ↔ Wolf API
```

### Health Checks

**Docker Compose**:
```yaml
wolf-revdial-client:
  # ... other config
  healthcheck:
    test: ["CMD", "pgrep", "-f", "wolf-revdial-client"]
    interval: 30s
    timeout: 10s
    retries: 3
```

**Kubernetes**:
```yaml
livenessProbe:
  exec:
    command:
    - pgrep
    - -f
    - wolf-revdial-client
  initialDelaySeconds: 10
  periodSeconds: 30
```

### Logging

**Increase verbosity** (if needed):
```bash
# Add to environment
LOG_LEVEL=debug

# Or modify main.go to enable debug logging
```

## Production Deployment Checklist

- [ ] Runner token generated and stored securely
- [ ] `HELIX_API_URL` points to production control plane
- [ ] Wolf ID is unique and descriptive
- [ ] RevDial client auto-restarts on failure
- [ ] Logs are collected (Docker logs, journald, or K8s logging)
- [ ] Monitoring alerts configured for connection drops
- [ ] TLS enabled (wss:// instead of ws://)
- [ ] Firewall allows outbound WebSocket connections
- [ ] Wolf API only listens on localhost (not exposed)
- [ ] Docker-in-Docker configured (privileged mode enabled)

## Scaling

### Multiple Wolf Instances

Deploy multiple Wolf instances with different IDs:

```yaml
# docker-compose.yaml
services:
  wolf-1:
    # ... Wolf config
    environment:
      WOLF_ID: wolf-1

  wolf-revdial-client-1:
    environment:
      WOLF_ID: wolf-1
      # ... other config

  wolf-2:
    # ... Wolf config
    environment:
      WOLF_ID: wolf-2

  wolf-revdial-client-2:
    environment:
      WOLF_ID: wolf-2
      # ... other config
```

### Load Balancing

The control plane handles load balancing across Wolf instances:
- Round-robin scheduling
- Least-loaded scheduling
- Geographic affinity

No client-side configuration needed.

## Migration from Local Wolf

Migrating from local Wolf (same machine as control plane) to remote Wolf:

1. **Deploy remote Wolf** with RevDial client
2. **Verify connection** to control plane
3. **Test sandbox creation** routes to remote Wolf
4. **Update DNS/firewall** if needed
5. **Decommission local Wolf** once stable

**Rollback plan**: Keep local Wolf running during migration, update control plane config to prefer remote Wolf.

## Troubleshooting

### Enable Debug Logging

**Docker Compose**:
```yaml
wolf-revdial-client:
  environment:
    DEBUG: "true"  # Add this line
```

**Systemd**:
```bash
# Edit service file
Environment="DEBUG=true"
systemctl daemon-reload
systemctl restart wolf-revdial-client
```

### Network Debugging

**Test WebSocket connectivity**:
```bash
# Install websocat
curl -o /usr/local/bin/websocat \
  https://github.com/vi/websocat/releases/download/v1.12.0/websocat.x86_64-unknown-linux-musl
chmod +x /usr/local/bin/websocat

# Test WebSocket connection
websocat ws://api.example.com:8080/api/v1/revdial?runnerid=wolf-test \
  -H "Authorization: Bearer ${RUNNER_TOKEN}"
```

**Check firewall rules**:
```bash
# Verify outbound WebSocket (port 80/443)
telnet api.example.com 80
telnet api.example.com 443
```

### Performance Tuning

**Increase reconnect interval** (reduce log spam):
```yaml
command: ["-reconnect", "30"]  # 30 seconds instead of default 5
```

**Connection pooling** (not yet implemented):
Future versions may support connection pooling for high-throughput scenarios.

## Support

For issues or questions:
- Check logs first (see Monitoring section)
- Review [Wolf RevDial Architecture](../../../design/2025-11-22-distributed-wolf-revdial-architecture.md)
- Review [DinD + RevDial Status](../../../design/2025-11-23-wolf-dind-revdial-implementation-status.md)
- File issue: https://github.com/helixml/helix/issues
