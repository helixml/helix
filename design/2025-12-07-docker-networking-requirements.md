# Docker Networking Requirements

## Summary

This document tracks the networking requirements for Docker containers started inside Helix sandboxes (Docker-in-Docker), covering both Hydra mode (isolated dockerd per session) and Privileged mode (host Docker access).

## Requirements Matrix

| # | Requirement | Hydra Mode | Privileged Mode |
|---|-------------|------------|-----------------|
| 1 | `localhost:8080` for `docker run -p 8080:8080` | ✅ Yes | ✅ Yes |
| 2 | Container names resolve from desktop | ✅ Yes | ✅ Yes |
| 3 | Intranet DNS names resolve | ✅ Yes | ✅ Yes |
| 4 | External internet DNS resolves | ✅ Yes | ✅ Yes |

## Detailed Analysis

### 1. localhost:8080 Access ✅ IMPLEMENTED

**Solution:** iptables DNAT rules in desktop container's network namespace forward `localhost:PORT` to `gateway:PORT`.

**Hydra Mode:**
- `docker run -p 8080:8080` binds to Hydra dockerd's bridge (10.200.X.1)
- iptables DNAT rule: `127.0.0.1:* → 10.200.X.1:*`
- User accesses `localhost:8080` → forwarded to `10.200.X.1:8080` ✅

**Privileged Mode:**
- `docker run -p 8080:8080` binds to host Docker's network (172.17.0.1)
- iptables DNAT rule: `127.0.0.1:* → 172.17.0.1:*`
- User accesses `localhost:8080` → forwarded to `172.17.0.1:8080` ✅

**Implementation:** `configureLocalhostForwarding()` in `manager.go`:
- Adds iptables DNAT rules for ports 1-5999 and 6064-65535
- Excludes X11 ports 6000-6063 to avoid breaking display
- Rules are idempotent (deletes before adding)

### 2. Container Name Resolution ✅ IMPLEMENTED

**Hydra Mode:** ✅ Works
- Hydra DNS proxy runs on bridge gateway (10.200.X.1:53)
- Added to desktop's `/etc/resolv.conf` by `configureDNS()`
- DNS chain: Desktop → Hydra DNS (10.200.X.1) → Docker DNS → Host DNS
- Container names resolve to their 10.200.X.Y IPs

**Privileged Mode:** ✅ Now Works
- `BridgeDesktopPrivileged()` now calls `configureDNS(containerPID, sandboxIP)`
- Desktop's resolv.conf updated to use sandbox IP (172.17.255.253) as DNS
- DNS chain: Desktop → Sandbox DNS proxy → Docker DNS → Host DNS
- Container names resolve to their 172.17.X.Y IPs

### 3. Intranet DNS Names (Enterprise Internal) ✅ IMPLEMENTED

**Hydra Mode:** ✅ Works
- DNS chain ends at host's DNS (inherited from `/etc/resolv.conf`)
- Enterprise DNS configured on host → intranet names resolve
- Chain: Inner container → Hydra DNS → Docker DNS → Host DNS

**Privileged Mode:** ✅ Now Works
- Desktop now uses sandbox's DNS (which reads from `/etc/resolv.conf`)
- In Kubernetes: uses CoreDNS → forwards to cluster DNS
- In Docker: uses Docker DNS → forwards to host DNS

### 4. External Internet DNS ✅ IMPLEMENTED

**Both Modes:** ✅ Works
- Same DNS chain as intranet, resolves through host/cluster DNS
- `getUpstreamDNS()` reads `/etc/resolv.conf` for environment-agnostic DNS
- Works in Docker (127.0.0.11) and Kubernetes (CoreDNS IP)

## Network Architecture

### Hydra Mode

```
┌─────────────────────────────────────────────────────────────────┐
│                         Host Machine                            │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                    Outer Sandbox                          │   │
│  │  ┌──────────────┐    ┌──────────────────────────────┐    │   │
│  │  │   Hydra      │    │     Hydra Bridge             │    │   │
│  │  │   Daemon     │    │     (hydra3)                 │    │   │
│  │  │              │    │     10.200.3.1/24            │    │   │
│  │  │  DNS Proxy   │◄───┤                              │    │   │
│  │  │  :53         │    │  ┌────────┐  ┌────────┐     │    │   │
│  │  └──────┬───────┘    │  │Container│  │Container│    │    │   │
│  │         │            │  │10.200.  │  │10.200.  │    │    │   │
│  │         ▼            │  │3.2      │  │3.3      │    │    │   │
│  │  ┌──────────────┐    │  └────────┘  └────────┘     │    │   │
│  │  │Docker DNS    │    └──────────────────────────────┘    │   │
│  │  │127.0.0.11    │                                        │   │
│  │  └──────┬───────┘    ┌──────────────────────────────┐    │   │
│  │         │            │  Desktop Container (Wolf)     │    │   │
│  │         ▼            │  eth0: Wolf network           │    │   │
│  │  ┌──────────────┐    │  eth1: 10.200.3.254          │    │   │
│  │  │Host DNS      │    │  resolv.conf: 10.200.3.1     │    │   │
│  │  │(Enterprise)  │    └──────────────────────────────┘    │   │
│  │  └──────────────┘                                        │   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

### Privileged Mode

```
┌─────────────────────────────────────────────────────────────────┐
│                         Host Machine                            │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                    Outer Sandbox                          │   │
│  │  ┌──────────────────────────────────────────────────────┐│   │
│  │  │  Host Docker Network (172.17.0.0/16)                 ││   │
│  │  │                                                      ││   │
│  │  │  ┌────────┐  ┌────────┐  ┌──────────────────────┐   ││   │
│  │  │  │Dev     │  │Dev     │  │Sandbox               │   ││   │
│  │  │  │Container│  │Container│  │172.17.255.253       │   ││   │
│  │  │  │172.17. │  │172.17. │  │(veth peer)           │   ││   │
│  │  │  │0.X     │  │0.Y     │  └───────────┬──────────┘   ││   │
│  │  │  └────────┘  └────────┘              │              ││   │
│  │  └──────────────────────────────────────│──────────────┘│   │
│  │                                         │                │   │
│  │  ┌──────────────────────────────────────│──────────────┐│   │
│  │  │  Desktop Container (Wolf)            │              ││   │
│  │  │  eth0: Wolf network                  │              ││   │
│  │  │  eth1: 172.17.255.254 ◄──────────────┘              ││   │
│  │  │  resolv.conf: ??? (NOT CONFIGURED)                  ││   │
│  │  └──────────────────────────────────────────────────────┘│   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

## Action Items

### Completed ✅

1. **~~Add DNS configuration to Privileged Mode~~** ✅
   - Added `configureDNS(containerPID, sandboxIP)` to `BridgeDesktopPrivileged()`
   - Desktop now uses sandbox DNS → Docker DNS → Host DNS

2. **~~Implement localhost port forwarding~~** ✅
   - Added `configureLocalhostForwarding()` function
   - iptables DNAT redirects `localhost:PORT` → `gateway:PORT`
   - Works for both Hydra and Privileged modes

3. **~~Read DNS from /etc/resolv.conf for Kubernetes compatibility~~** ✅
   - Added `getUpstreamDNS()` function in `server.go`
   - Works in Docker (127.0.0.11) and Kubernetes (CoreDNS)

### Remaining

- Test all scenarios after sandbox rebuild
- Verify X11 still works (ports 6000-6063 excluded from DNAT)

## Testing Validation

After rebuilding sandbox:

```bash
# === HYDRA MODE ===

# 1. Start a test container with port mapping
docker -H unix:///var/run/hydra/active/session-xxx/docker.sock \
  run -d --name testapp -p 8080:80 nginx

# 2. Test localhost:8080 (CRITICAL - user expectation)
curl localhost:8080
# Should return nginx welcome page ✅

# 3. Test container name resolution
curl http://testapp/
# Should return nginx welcome page ✅

# 4. Check DNS configuration
cat /etc/resolv.conf
# Should show 10.200.X.1 as first nameserver

# 5. Test external DNS
nslookup google.com
# Should resolve

# 6. Verify X11 still works (ports 6000-6063 excluded)
echo $DISPLAY
# Should still be able to launch GUI apps

# === PRIVILEGED MODE ===

# 1. Start container on host Docker
docker run -d --name testapp2 -p 9090:80 nginx

# 2. Test localhost:9090
curl localhost:9090
# Should return nginx welcome page ✅

# 3. Test DNS
nslookup google.com
# Should resolve through sandbox DNS
```

## Kubernetes Deployment Considerations

### Critical Issue: 127.0.0.11 Does Not Exist in Kubernetes

**Problem:**
The current implementation hardcodes `127.0.0.11:53` as the upstream DNS server. This is Docker's internal DNS resolver that ONLY exists:
- When running inside a Docker container
- When the container is on a user-defined Docker network

**In Kubernetes:**
- Pods use CoreDNS (typically at `10.96.0.10` or cluster DNS IP)
- 127.0.0.11 does NOT exist
- The sandbox pod's `/etc/resolv.conf` points to CoreDNS

**Impact:**
- DNS resolution from inner containers will fail completely
- Container names won't resolve
- External DNS won't resolve
- Intranet DNS won't resolve

### Fix Implemented ✅

Added `getUpstreamDNS()` function in `server.go` that reads `/etc/resolv.conf`:

```go
// getUpstreamDNS reads /etc/resolv.conf and returns the nameservers configured there.
// This ensures DNS works in both Docker and Kubernetes environments:
// - Docker: returns 127.0.0.11 (Docker's internal DNS) or host DNS
// - Kubernetes: returns CoreDNS IP (e.g., 10.96.0.10)
func getUpstreamDNS() []string {
    // Reads /etc/resolv.conf, extracts nameserver lines
    // Skips 127.0.0.53 (systemd-resolved stub - doesn't work in containers)
    // Falls back to 8.8.8.8 if no nameservers found
}
```

This fix was applied to `api/pkg/hydra/server.go` and replaces the hardcoded `127.0.0.11:53`.

### Kubernetes Network Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    Kubernetes Cluster                           │
│                                                                 │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │                    Sandbox Pod                              │ │
│  │  /etc/resolv.conf: nameserver 10.96.0.10 (CoreDNS)         │ │
│  │                                                            │ │
│  │  ┌──────────────────────────────────────────────────────┐  │ │
│  │  │   Hydra (runs inside pod)                            │  │ │
│  │  │                                                      │  │ │
│  │  │   DNS Proxy: 10.200.X.1:53                           │  │ │
│  │  │        │                                             │  │ │
│  │  │        ▼                                             │  │ │
│  │  │   Upstream: 10.96.0.10:53 (from /etc/resolv.conf)   │  │ │
│  │  │        │                                             │  │ │
│  │  └────────│─────────────────────────────────────────────┘  │ │
│  │           │                                                │ │
│  │           ▼                                                │ │
│  │  ┌──────────────────────────────────────────────────────┐  │ │
│  │  │   CoreDNS (cluster DNS)                              │  │ │
│  │  │   10.96.0.10:53                                      │  │ │
│  │  │        │                                             │  │ │
│  │  │        ├── Cluster service names (*.svc.cluster.local) │ │
│  │  │        └── External DNS (forwarded to upstream)      │  │ │
│  │  └──────────────────────────────────────────────────────┘  │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### Kubernetes-Specific Testing

```bash
# 1. Verify DNS works from inner container
kubectl exec -it sandbox-pod -- docker exec inner-container \
  nslookup kubernetes.default.svc.cluster.local
# Should resolve to cluster API IP

# 2. Verify external DNS works
kubectl exec -it sandbox-pod -- docker exec inner-container \
  nslookup google.com
# Should resolve

# 3. Check what DNS the sandbox pod uses
kubectl exec -it sandbox-pod -- cat /etc/resolv.conf
# Should show CoreDNS IP (e.g., 10.96.0.10)
```

### Additional Kubernetes Considerations

1. **Network Policies**: If NetworkPolicies restrict egress, ensure DNS (port 53) is allowed
2. **DinD Requirements**: Sandbox needs privileged mode or sysbox runtime for Docker-in-Docker
3. **Host Docker Socket**: If using host Docker socket, need to ensure host Docker has proper DNS config
4. **Service Mesh**: If using Istio/Linkerd, DNS interception may require additional configuration

## Wolf Container Naming Convention

Wolf adds a UUID suffix to container names, which affects how Hydra looks up containers for bridging.

**Container name format:**
- Helix passes: `zed-external-{session_id}`
- Wolf creates: `zed-external-{session_id}_{uuid}`

Example:
- Requested: `zed-external-01kbyzs1ttmtmtwf19xeh51ed0`
- Actual: `zed-external-01kbyzs1ttmtmtwf19xeh51ed0_edf14aa9-9027-492a-a275-d7216b92d7a4`

**Fix implemented (2025-12-08):**

`getContainerPID()` in `manager.go` now:
1. First tries exact name match via `docker inspect`
2. If that fails, uses `docker ps --filter "name={prefix}"` to find containers by prefix
3. Inspects the matched container

This ensures `BridgeDesktop` can find containers regardless of Wolf's UUID suffix.

## References

- `api/pkg/hydra/manager.go` - Hydra manager and bridge creation
- `api/pkg/hydra/server.go` - Hydra API server and DNS setup
- `api/pkg/hydra/dns.go` - DNS proxy implementation
