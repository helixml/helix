# Helix-in-Helix Development Guidance

This project sets up a development environment for working on Helix itself inside a Helix cloud desktop.

## Architecture Overview (docker-in-desktop mode)

With docker-in-desktop mode, each desktop container runs its own dockerd with a
volume-backed `/var/lib/docker`. Helix-in-Helix now works natively — just run
`./stack start` inside the desktop container.

```
Host Docker (Level 0)
└── Outer Sandbox (Level 1, /var/lib/docker = volume)
    └── Outer Desktop (Level 2, /var/lib/docker = volume)
        │  GNOME desktop, Zed IDE, video streaming
        │  Local dockerd (runs natively inside desktop)
        │
        │  User runs: ./stack start
        │
        ├── Inner Helix API, Postgres, Frontend (on desktop's dockerd)
        │
        └── Inner Sandbox (Level 3, /var/lib/docker = volume)
            └── Inner Desktop (Level 4, /var/lib/docker = volume)
                └── User's containers (Level 5)
```

No special configuration, no two Docker endpoints, no host socket exposure.
Each level is structurally identical and self-contained.

## Setup Steps

1. **Clone the Helix repository** inside the desktop:
   ```bash
   cd ~/work
   git clone https://github.com/helixml/helix.git
   cd helix
   ```

2. **Start the inner control plane**:
   ```bash
   ./stack start
   ```
   This starts Helix on the desktop's local dockerd. The inner sandbox runs
   inside the desktop's dockerd at Level 3.

3. **Access the inner Helix UI** at `http://localhost:8080`

## How It Works

- **Single Docker endpoint**: The desktop's own dockerd handles everything.
  `docker` commands work normally — no `docker-inner`/`docker-outer` split.
- **GPU passthrough**: NVIDIA runtime cascades through all levels via
  device mounts and cgroup rules.
- **Shared BuildKit cache**: Available at `/buildkit-cache` via the sandbox's
  shared BuildKit container (TCP endpoint via `BUILDKIT_HOST`).
- **Arbitrary nesting**: Each level's `/var/lib/docker` is a Docker volume
  backed by ext4, resetting overlay2 stacking depth to 1.

## Building Components

```bash
cd ~/work/helix

# Build API
./stack build

# Build desktop images
./stack build-ubuntu

# Build sandbox
./stack build-sandbox
```

## Troubleshooting

### Docker not working inside desktop

Check that dockerd is running:
```bash
docker info
```

If not, check the init script logs:
```bash
cat /config/logs/17-start-dockerd.sh.log
```

### Kind (Kubernetes) not working

Verify cgroup v2 controllers are delegated:
```bash
cat /sys/fs/cgroup/cgroup.subtree_control
# Should include: cpuset cpu io memory hugetlb pids
```

## Repository Structure

- `~/work/helix/` - Main Helix repository
