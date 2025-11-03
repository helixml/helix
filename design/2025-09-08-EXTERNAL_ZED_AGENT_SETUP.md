# External Zed Agent Runner Setup

This guide explains how to easily attach an external machine (like your development machine) as a Zed agent runner to your Helix control plane.

## Quick Start

The simplest way to run an external Zed agent is with a single Docker command:

```bash
# Basic usage - replace with your actual API host and token
./scripts/run-external-zed-agent.sh \
  --api-host http://your-helix-server.com \
  --api-token your-runner-token
```

## Prerequisites

1. **Docker**: Make sure Docker is installed and running on your machine
2. **Zed Agent Image**: Build the Zed agent Docker image (see [Building the Image](#building-the-image))
3. **Runner Token**: Get the runner token from your Helix control plane configuration

## Building the Image

Before running the external agent, you need to build the Zed agent Docker image:

```bash
# First, build the Zed binary with external sync support
./stack build-zed

# Then build the Docker image
docker build -f Dockerfile.zed-agent -t helix-zed-agent:latest .
```

## Configuration Options

### Method 1: Command Line Arguments

```bash
./scripts/run-external-zed-agent.sh \
  --api-host http://your-helix-server.com \
  --api-token your-runner-token \
  --runner-id my-dev-machine \
  --concurrency 2
```

### Method 2: Environment Variables

```bash
# Copy and customize the environment file
cp scripts/external-zed-agent.env.example scripts/external-zed-agent.env

# Edit the file with your settings
nano scripts/external-zed-agent.env

# Source the environment file and run
source scripts/external-zed-agent.env
./scripts/run-external-zed-agent.sh
```

### Method 3: Direct Docker Run

If you prefer to run Docker directly:

```bash
docker run -d \
  --name helix-external-zed-agent \
  --restart unless-stopped \
  -e API_HOST=http://your-helix-server.com \
  -e API_TOKEN=your-runner-token \
  -e RUNNER_ID=external-zed-$(hostname) \
  -e CONCURRENCY=1 \
  -e MAX_TASKS=0 \
  -p 3389-3409:3389-3409 \
  -p 5900-5920:5900-5920 \
  -p 3030:3030 \
  helix-zed-agent:latest
```

## Configuration Parameters

| Parameter | Default | Description |
|-----------|---------|-------------|
| `API_HOST` | `http://localhost:80` | URL of your Helix control plane |
| `API_TOKEN` | `oh-hallo-insecure-token` | Authentication token (must match control plane) |
| `RUNNER_ID` | `external-zed-{hostname}` | Unique identifier for this runner |
| `CONCURRENCY` | `1` | Number of concurrent Zed sessions |
| `MAX_TASKS` | `0` | Max tasks before restart (0=unlimited) |
| `SESSION_TIMEOUT` | `3600` | Session timeout in seconds |
| `WORKSPACE_DIR` | `/tmp/zed-workspaces` | Container workspace directory |

## Remote Access

The Zed agent runner provides RDP access for remote desktop functionality:

- **Host**: Your machine's IP address or `localhost`
- **Port**: 3389 (or check `docker ps` for actual port mapping)
- **Username**: `zed`
- **Password**: Managed by the Helix control plane

You can connect using any RDP client:
- Windows: Built-in Remote Desktop Connection
- macOS: Microsoft Remote Desktop
- Linux: Remmina, xfreerdp, etc.

## Managing the Runner

### Check Status
```bash
# View running containers
docker ps --filter "name=helix-external-zed-agent"

# View logs
docker logs -f helix-external-zed-agent

# Check resource usage
docker stats helix-external-zed-agent
```

### Control the Runner
```bash
# Stop the runner
docker stop helix-external-zed-agent

# Start the runner
docker start helix-external-zed-agent

# Restart the runner
docker restart helix-external-zed-agent

# Remove the runner
docker rm helix-external-zed-agent
```

### Access Container Shell
```bash
# Get a shell inside the container
docker exec -it helix-external-zed-agent /bin/bash

# Run commands as the zed user
docker exec -it -u zed helix-external-zed-agent /bin/bash
```

## Troubleshooting

### Common Issues

1. **Container fails to start**
   ```bash
   # Check Docker logs for errors
   docker logs helix-external-zed-agent
   
   # Verify the image exists
   docker images | grep helix-zed-agent
   ```

2. **Cannot connect to control plane**
   - Verify `API_HOST` is correct and accessible
   - Check that `API_TOKEN` matches your control plane configuration
   - Ensure network connectivity between runner and control plane

3. **RDP connection fails**
   - Check port mappings: `docker port helix-external-zed-agent`
   - Verify RDP credentials are managed by the control plane
   - Ensure firewall allows RDP connections

4. **Zed binary issues**
   - Make sure you ran `./stack build-zed` before building the Docker image
   - Check that the Zed binary has external sync support

### Debug Mode

Enable debug logging for more detailed output:

```bash
./scripts/run-external-zed-agent.sh \
  --api-host http://your-helix-server.com \
  --api-token your-runner-token \
  -e LOG_LEVEL=debug
```

### Health Checks

The runner provides several health check endpoints:

```bash
# Check if external sync is running (inside container)
curl http://localhost:3030/health

# Check running processes
docker exec helix-external-zed-agent pgrep -af "helix\|zed"

# Check network connectivity
docker exec helix-external-zed-agent curl -I http://your-helix-server.com
```

## Security Considerations

1. **Secure tokens**: Use a strong, unique `API_TOKEN`
2. **RDP access**: RDP credentials are managed by the control plane
3. **Network isolation**: Consider running on a private network
4. **Firewall rules**: Limit access to RDP ports if not needed
5. **Regular updates**: Keep the Docker image updated

## Scaling

You can run multiple external agents on the same machine:

```bash
# Run multiple agents with different runner IDs
./scripts/run-external-zed-agent.sh \
  --runner-id external-zed-1 \
  --container-name helix-zed-agent-1 \
  --api-token your-token

./scripts/run-external-zed-agent.sh \
  --runner-id external-zed-2 \
  --container-name helix-zed-agent-2 \
  --api-token your-token
```

Note: Each container will need a unique runner ID and container name.

## Integration with docker-compose.dev.yaml

The external runner is based on the same configuration as the `zed-runner` service in `docker-compose.dev.yaml`:

```yaml
zed-runner:
  build:
    context: .
    dockerfile: Dockerfile.zed-agent
  environment:
    - API_HOST=http://api:8080
    - API_TOKEN=${RUNNER_TOKEN-oh-hallo-insecure-token}
    - CONCURRENCY=1
    - MAX_TASKS=0
    - WORKSPACE_DIR=/tmp/workspace
  # ... other configuration
```

The external runner script essentially replicates this setup but allows you to:
- Connect to a remote control plane
- Run on any machine with Docker
- Customize configuration easily
- Scale independently of the main deployment

## Next Steps

1. Monitor the runner in your Helix control plane dashboard
2. Test Zed agent functionality through the web interface
3. Scale up by adding more external runners as needed
4. Consider setting up monitoring and alerting for production use
