# Helix-in-Helix Development Guidance

This project sets up a development environment for working on Helix itself inside a Helix cloud desktop.

## Prerequisites

Your session must be running with **privileged mode enabled** (`HYDRA_PRIVILEGED_MODE_ENABLED=true` on the production sandbox). This provides access to the host Docker socket at `/var/run/host-docker.sock`.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ Production Helix                                                             │
│   └── Sandbox (HYDRA_PRIVILEGED_MODE_ENABLED=true)                          │
│         │                                                                    │
│         ├── /var/run/docker.sock (Inner Docker - Hydra's DinD)              │
│         └── /var/run/host-docker.sock (Outer Docker - Host)                 │
│                                                                              │
│         ┌────────────────────────────────────────────────────────────────┐  │
│         │ Developer Desktop (this environment)                           │  │
│         │                                                                 │  │
│         │  Inner Docker (/var/run/docker.sock)                           │  │
│         │  ├── helix-api (dev control plane on port 8080)                │  │
│         │  ├── helix-postgres                                            │  │
│         │  ├── helix-frontend                                            │  │
│         │  └── other services                                            │  │
│         │                                                                 │  │
│         │  ↓ Expose port 8080 via API's proxy endpoint                   │  │
│         │                                                                 │  │
│         └────────────────────────────────────────────────────────────────┘  │
│                                                                              │
│         Exposed URL: https://helix.example.com/api/v1/sessions/{id}/proxy/8080/
│                        ↓                                                     │
└────────────────────────│─────────────────────────────────────────────────────┘
                         │
                         ↓ RevDial
┌────────────────────────────────────────────────────────────────────────────┐
│ Host Docker (via /var/run/host-docker.sock)                                 │
│   └── helix-sandbox-dev-* (RevDials to exposed inner API)                  │
│         └── DinD with desktop containers for testing                       │
└────────────────────────────────────────────────────────────────────────────┘
```

## Key Insight: Service Exposure

The **Service Exposure** feature allows you to expose ports from your dev container to the outside world via the Helix API. This is how the sandbox on host Docker reaches your inner control plane:

1. Inner control plane runs on inner Docker (port 8080)
2. You call `POST /api/v1/sessions/{id}/expose` to expose port 8080
3. Production Helix API proxies requests to your desktop via RevDial
4. Sandbox on host Docker sets `HELIX_API_URL` to the exposed URL
5. Sandbox RevDials to your inner API through the proxy

## Two Docker Endpoints

| Endpoint | Socket Path | Purpose |
|----------|-------------|---------|
| Inner Docker | `/var/run/docker.sock` | Run Helix control plane |
| Outer Docker | `/var/run/host-docker.sock` | Run test sandboxes |

## Setup Steps

1. **Run the setup script** (first time only):
   ```bash
   ~/helix-dev-setup.sh
   ```

2. **Source the environment**:
   ```bash
   source ~/.helix-dev-env
   ```

3. **Start the inner control plane**:
   ```bash
   cd ~/helix-workspace/helix
   ./start-inner-stack.sh
   ```

4. **Expose the inner API**:
   ```bash
   export SESSION_ID=ses_xxx  # Your session ID from the URL
   ./expose-inner-api.sh
   ```

5. **Start a test sandbox on host Docker**:
   ```bash
   ./start-outer-sandbox.sh
   ```

## Helper Scripts

| Script | Purpose |
|--------|---------|
| `start-inner-stack.sh` | Start Helix control plane on inner Docker |
| `expose-inner-api.sh` | Expose port 8080 via the API proxy |
| `start-outer-sandbox.sh` | Start a sandbox on host Docker |

## Docker Commands

```bash
# Inner Docker (control plane)
docker-inner ps
docker-inner logs api -f
compose-inner logs -f

# Outer Docker (host)
docker-outer ps
docker-outer logs helix-sandbox-dev-* -f
```

## Building Components

```bash
cd ~/helix-workspace/helix

# Build API
./stack build

# Build Zed IDE
./stack build-zed

# Build desktop images
./stack build-sway
./stack build-ubuntu
```

## Troubleshooting

### Host Docker socket not available

Privileged mode must be enabled on the production sandbox:
```bash
# Check for socket
ls -la /var/run/host-docker.sock
```

If missing, the session was started without `HYDRA_PRIVILEGED_MODE_ENABLED=true`.

### Cannot expose port

Ensure you have the correct session ID and API credentials:
```bash
echo $SESSION_ID
echo $HELIX_API_URL
echo $HELIX_API_KEY
```

### Sandbox not connecting to inner API

1. Verify inner API is running: `curl http://localhost:8080/health`
2. Verify port is exposed: Check the response from `expose-inner-api.sh`
3. Check sandbox logs: `docker-outer logs helix-sandbox-dev-*`

## Repository Structure

- `~/helix-workspace/helix/` - Main Helix repository
- `~/helix-workspace/zed/` - Zed IDE fork
- `~/helix-workspace/qwen-code/` - Qwen Code agent fork

## Important Notes

1. **Use unique sandbox names** - The scripts auto-generate unique names
2. **Clean up test sandboxes** when done: `docker-outer stop helix-sandbox-dev-* && docker-outer rm helix-sandbox-dev-*`
3. **Changes to desktop images** require rebuild + new session
4. **The exposed URL routes through production Helix** - this adds latency but provides secure access
