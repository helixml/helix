# Docker-in-Docker Networking Investigation

## Date: 2025-12-08

## Problem Statement

`localhost:PORT` and `http://nginx/` (container name DNS) not working from desktop containers in Helix sandboxes.

## Network Architecture (Multi-Layer)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              HOST MACHINE                                    │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │           OUTER SANDBOX (helix-sandbox-nvidia-1)                       │ │
│  │           Docker network: 172.19.0.0/16                                │ │
│  │                                                                        │ │
│  │  ┌─────────────────────────────────────────────────────────────────┐   │ │
│  │  │  WOLF (streaming platform)                                      │   │ │
│  │  │  Manages desktop containers via its own dockerd                 │   │ │
│  │  │  Docker network: 172.20.0.0/16 (wolf_default)                   │   │ │
│  │  │                                                                 │   │ │
│  │  │  ┌─────────────────────────────────────────────────────────┐   │   │ │
│  │  │  │  DESKTOP CONTAINER (zed-external-xxx)                   │   │   │ │
│  │  │  │  eth0: 172.20.0.2 (Wolf network)                        │   │   │ │
│  │  │  │  eth1: 10.200.1.254 (Hydra bridge - INJECTED)           │   │   │ │
│  │  │  │                                                         │   │   │ │
│  │  │  │  User runs: docker run -p 8082:80 nginx                 │   │   │ │
│  │  │  │  User expects: curl localhost:8082 → works              │   │   │ │
│  │  │  └─────────────────────────────────────────────────────────┘   │   │ │
│  │  └─────────────────────────────────────────────────────────────────┘   │ │
│  │                                                                        │ │
│  │  ┌─────────────────────────────────────────────────────────────────┐   │ │
│  │  │  HYDRA (multi-Docker isolation daemon)                         │   │ │
│  │  │  Creates per-session/scope isolated dockerd instances          │   │ │
│  │  │                                                                 │   │ │
│  │  │  Bridge: hydra1 (10.200.1.1/24)                                │   │ │
│  │  │  DNS proxy: 10.200.1.1:53                                      │   │ │
│  │  │                                                                 │   │ │
│  │  │  ┌───────────────────────────────────────────────────────┐     │   │ │
│  │  │  │  PER-SESSION DOCKERD                                  │     │   │ │
│  │  │  │  Socket: /var/run/hydra/active/spectask-xxx/docker.sock    │ │   │ │
│  │  │  │  Bridge: hydra1 (shared with desktop)                 │     │   │ │
│  │  │  │                                                       │     │   │ │
│  │  │  │  ┌─────────────────┐  ┌─────────────────┐            │     │   │ │
│  │  │  │  │ nginx container │  │ other container │            │     │   │ │
│  │  │  │  │ 10.200.1.X      │  │ 10.200.1.Y      │            │     │   │ │
│  │  │  │  │ -p 8082:80      │  │                 │            │     │   │ │
│  │  │  │  └─────────────────┘  └─────────────────┘            │     │   │ │
│  │  │  │                                                       │     │   │ │
│  │  │  │  Port 8082 binds to: 10.200.1.1:8082 (bridge gateway) │     │   │ │
│  │  │  └───────────────────────────────────────────────────────┘     │   │ │
│  │  └─────────────────────────────────────────────────────────────────┘   │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────────┘
```

## The Problem: Multiple Isolated Docker Daemons

1. **Wolf's dockerd** manages desktop containers (172.20.0.0/16)
2. **Hydra's per-session dockerd** manages dev containers (10.200.1.0/24)

These are **completely separate Docker daemons** with **separate networks**.

When user runs `docker run -p 8082:80 nginx` in the desktop:
- The command uses Hydra's dockerd (via `DOCKER_HOST` env var pointing to Hydra socket)
- nginx binds to **Hydra's bridge gateway** (10.200.1.1:8082)
- NOT to the desktop container's localhost!

## Attempted Solution: Bridge + DNAT + DNS

### 1. Veth Bridge Injection ✅ WORKS
- Inject eth1 into desktop container connecting to Hydra bridge
- Desktop gets 10.200.1.254, gateway is 10.200.1.1
- Direct access works: `curl http://10.200.1.1:8082` ✅

### 2. DNS Configuration ✅ WORKS
- Prepend `nameserver 10.200.1.1` to resolv.conf
- Hydra DNS proxy resolves container names to their IPs
- `nslookup nginx 10.200.1.1` should return 10.200.1.X ✅

### 3. Localhost DNAT ❌ NOT WORKING
- iptables rule: `DNAT tcp 127.0.0.1 → 10.200.1.1`
- Goal: `curl localhost:8082` → `curl 10.200.1.1:8082`

**Why it fails:**

When we DNAT:
- Source: 127.0.0.1 (loopback address)
- Dest: 127.0.0.1:8082 → DNAT → 10.200.1.1:8082

The packet now has:
- Source: 127.0.0.1 (still!)
- Dest: 10.200.1.1:8082

The kernel sees a packet with **loopback source address (127.0.0.1)** trying to go out via eth1. By default, `net.ipv4.conf.*.route_localnet = 0`, which means the kernel **drops** packets with 127.x.x.x addresses on non-loopback routes.

Even if we enable `route_localnet=1`, the return packet would have:
- Source: 10.200.1.1:8082
- Dest: 127.0.0.1

This return path might also be blocked or confused.

## Root Cause Analysis

The fundamental issue: **localhost DNAT across network namespaces is inherently problematic**.

The loopback address (127.0.0.1) is designed to stay within a single network namespace. Trying to DNAT it to an external IP breaks networking assumptions.

## Alternative Approaches

### Option 1: DNAT + SNAT (Source NAT / Masquerade)

Instead of just changing destination, also change source:

```bash
# DNAT in OUTPUT chain
iptables -t nat -A OUTPUT -d 127.0.0.1 -p tcp -j DNAT --to-destination 10.200.1.1

# MASQUERADE in POSTROUTING to fix source address
iptables -t nat -A POSTROUTING -d 10.200.1.0/24 -j MASQUERADE
```

Now:
- Outgoing: src=10.200.1.254 (desktop's eth1), dst=10.200.1.1:8082
- Return: src=10.200.1.1:8082, dst=10.200.1.254
- Conntrack reverses both NATs for the application

### Option 2: Bind Docker Ports to 0.0.0.0 on Desktop's IP

Configure Hydra's dockerd to bind published ports to an IP accessible from desktop:
- Instead of binding to 10.200.1.1 (Hydra bridge)
- Bind to 0.0.0.0 inside a container that's also on desktop's network

This is complex because Hydra's dockerd is separate from Wolf's.

### Option 3: Use a TCP Proxy Instead of DNAT

Run a simple TCP proxy (socat, nginx stream, haproxy) on localhost that forwards to 10.200.1.1:

```bash
# For each exposed port, run a proxy
socat TCP-LISTEN:8082,bind=127.0.0.1,fork TCP:10.200.1.1:8082
```

Pros: No iptables complexity, no kernel routing issues
Cons: Need to manage proxy processes, discover ports dynamically

### Option 4: Transparent Proxy with TPROXY

Use TPROXY instead of DNAT for more control over packet handling. More complex but more powerful.

### Option 5: Rethink the Architecture

Instead of trying to make `localhost:PORT` work, accept that dev containers are on a different network:
- User runs `docker run -p 8082:80 nginx`
- User accesses `http://10.200.1.1:8082` or `http://nginx:80`
- Document this as the expected behavior

## Recommended Fix: Option 1 (DNAT + MASQUERADE)

```go
func (m *Manager) configureLocalhostForwarding(containerPID int, gateway string) error {
    // Enable route_localnet to allow 127.0.0.1 on non-loopback routes
    m.runNsenter(containerPID, "sysctl", "-w", "net.ipv4.conf.all.route_localnet=1")
    m.runNsenter(containerPID, "sysctl", "-w", "net.ipv4.conf.eth1.route_localnet=1")

    // DNAT localhost to gateway
    m.runNsenter(containerPID, "iptables", "-t", "nat", "-A", "OUTPUT",
        "-d", "127.0.0.1", "-p", "tcp",
        "-j", "DNAT", "--to-destination", gateway)

    // MASQUERADE to fix source address for packets going to Hydra network
    m.runNsenter(containerPID, "iptables", "-t", "nat", "-A", "POSTROUTING",
        "-d", fmt.Sprintf("%s/24", gateway), // e.g., 10.200.1.0/24
        "-j", "MASQUERADE")

    return nil
}
```

## Root Cause

The localhost DNAT was failing due to two issues:

### Issue 1: route_localnet disabled

When we DNAT `127.0.0.1:PORT → 10.200.1.1:PORT`, the packet has:
- Source: 127.0.0.1 (loopback address)
- Dest: 10.200.1.1 (Hydra gateway)

By default, `net.ipv4.conf.*.route_localnet = 0`, which means the kernel **drops** packets with 127.x.x.x addresses on non-loopback interfaces. The packet never reaches POSTROUTING.

**Fix:** Enable route_localnet via `/proc`:
```bash
echo 1 > /proc/sys/net/ipv4/conf/all/route_localnet
echo 1 > /proc/sys/net/ipv4/conf/eth1/route_localnet
```

### Issue 2: Missing MASQUERADE

Even with route_localnet, the source address is still 127.0.0.1. Return packets from nginx would have `dst=127.0.0.1`, but the Hydra network doesn't know how to route back to the desktop's loopback.

**Fix:** Add MASQUERADE rule to change source address:
```bash
iptables -t nat -A POSTROUTING -d 10.200.1.0/24 -j MASQUERADE
```

This changes the source from 127.0.0.1 to 10.200.1.254 (desktop's eth1 IP), allowing proper return routing.

## Complete Fix (Implemented)

Updated `configureLocalhostForwarding()` in `manager.go`:

```go
func (m *Manager) configureLocalhostForwarding(containerPID int, gateway string) error {
    // 1. Enable route_localnet
    m.runNsenterSh(containerPID, "echo 1 > /proc/sys/net/ipv4/conf/all/route_localnet")
    m.runNsenterSh(containerPID, "echo 1 > /proc/sys/net/ipv4/conf/eth1/route_localnet")

    // 2. DNAT localhost -> gateway (no -o lo flag!)
    m.runNsenter(containerPID, "iptables", "-t", "nat", "-A", "OUTPUT",
        "-d", "127.0.0.1", "-p", "tcp",
        "-j", "DNAT", "--to-destination", gateway)

    // 3. MASQUERADE to fix source address
    m.runNsenter(containerPID, "iptables", "-t", "nat", "-A", "POSTROUTING",
        "-d", "10.200.1.0/24", "-j", "MASQUERADE")

    return nil
}
```

## Verified Working

After manual fix, tested from desktop container:
- `curl localhost:8082` → nginx welcome page ✅
- `curl http://dreamy_euler/` (container name) → nginx welcome page ✅
- `curl http://10.200.1.1:8082` (direct) → nginx welcome page ✅

## Final Status

- Veth injection: ✅ Working
- DNS configuration: ✅ Working (10.200.1.1 prepended)
- Container name DNS: ✅ Working (`http://container_name/`)
- Direct access: ✅ Working (`curl http://10.200.1.1:8082`)
- Localhost forwarding: ✅ Working (DNAT + route_localnet + MASQUERADE)

## Deployment

Requires sandbox rebuild to pick up the Hydra code changes:
```bash
./stack build-sandbox
```

## Host Configuration

The following host-level settings are now configured automatically:

### Development (`./stack start`)
- Calls `setup_dev_networking()` which sets:
  - `net.ipv4.conf.all.route_localnet=1`
  - `net.ipv4.conf.default.route_localnet=1`
  - `net.ipv4.ip_forward=1`
  - inotify limits for Zed file watching

### Production (`install.sh`)
- Persists settings to `/etc/sysctl.d/99-helix-networking.conf`
- Survives reboots

### Why Host Settings?
The host-level `route_localnet` serves as a belt-and-suspenders approach. The actual fix is in Hydra which sets `route_localnet` inside each desktop container's network namespace via nsenter. But having it on the host:
1. Ensures the sandbox container itself can forward traffic
2. May help with edge cases where namespace inheritance matters

## Files Changed

- `api/pkg/hydra/manager.go` - configureLocalhostForwarding() with route_localnet + MASQUERADE
- `install.sh` - Host sysctl configuration for production
- `stack` - setup_dev_networking() for development

## References

- `api/pkg/hydra/manager.go` - configureLocalhostForwarding()
- `design/2025-12-07-docker-networking-requirements.md` - original requirements
