# Docker Networking Requirements

## Summary

This document tracks the networking requirements for Docker containers started inside Helix sandboxes (Docker-in-Docker), covering both Hydra mode (isolated dockerd per session) and Privileged mode (host Docker access).

## Requirements Matrix

| # | Requirement | Hydra Mode | Privileged Mode |
|---|-------------|------------|-----------------|
| 1 | `localhost:8080` for `docker run -p 8080:8080` | ❌ No | ❌ No |
| 2 | Container names resolve from desktop | ✅ Yes | ❌ No |
| 3 | Intranet DNS names resolve | ✅ Should work | ❌ No |
| 4 | External internet DNS resolves | ✅ Should work | ⚠️ Partial |

## Detailed Analysis

### 1. localhost:8080 Access

**Hydra Mode:**
- `docker run -p 8080:8080` binds to Hydra dockerd's bridge (10.200.X.1)
- Desktop container sees `localhost` as its own loopback (127.0.0.1)
- Port is NOT exposed on desktop's localhost
- **Workaround:** Access via container name (e.g., `http://myapp:8080`) or gateway IP (`http://10.200.X.1:8080`)

**Privileged Mode:**
- `docker run -p 8080:8080` binds to host Docker's network
- Port exposed on host's 0.0.0.0:8080
- Desktop routes through sandbox (172.17.255.253) but localhost still means desktop's loopback
- **Workaround:** Need to determine host Docker gateway IP

**Potential Fix:** Add iptables DNAT rule to forward desktop's localhost:8080 to Docker gateway's :8080. This is complex because we'd need to know which ports to forward.

### 2. Container Name Resolution

**Hydra Mode:** ✅ Works
- Hydra DNS proxy runs on bridge gateway (10.200.X.1:53)
- Added to desktop's `/etc/resolv.conf` by `configureDNS()`
- DNS chain: Desktop → Hydra DNS (10.200.X.1) → Docker DNS (127.0.0.11) → Host DNS
- Container names resolve to their 10.200.X.Y IPs

**Privileged Mode:** ❌ Not Implemented
- `BridgeDesktopPrivileged()` does NOT call `configureDNS()`
- Desktop has no route to Docker's internal DNS (127.0.0.11)
- Container names don't resolve

**Fix Required:** Add DNS configuration to `BridgeDesktopPrivileged()`:
```go
// Need to either:
// a) Run a DNS proxy in the sandbox that forwards to 127.0.0.11
// b) Configure desktop to use sandbox IP as DNS, with forwarding
```

### 3. Intranet DNS Names (Enterprise Internal)

**Hydra Mode:** ✅ Should Work
- DNS chain ends at host's DNS (inherited from `/etc/resolv.conf`)
- Enterprise DNS configured on host → intranet names resolve
- Tested chain: Inner container → Hydra DNS → Docker DNS → Host DNS

**Privileged Mode:** ❌ No
- Without DNS configuration, desktop uses its original resolv.conf
- Wolf desktop container may have different DNS settings
- No bridge to enterprise DNS through Docker

### 4. External Internet DNS

**Hydra Mode:** ✅ Works
- Same DNS chain as intranet, resolves through host DNS

**Privileged Mode:** ⚠️ Partial
- Desktop container's original DNS config determines behavior
- May work if Wolf container has proper DNS
- Not guaranteed to follow host DNS configuration

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

### High Priority

1. **Add DNS configuration to Privileged Mode**
   - Run DNS proxy on sandbox's veth endpoint (172.17.255.253:53)
   - Forward to Docker's internal DNS (127.0.0.11)
   - Add to desktop's resolv.conf

### Medium Priority

2. **Document workarounds for localhost:8080**
   - In Zed/IDE, configure dev server to advertise correct URL
   - Use container names instead of localhost in browser
   - Consider adding helper scripts to show accessible URLs

### Low Priority / Future

3. **Investigate localhost port forwarding**
   - Complex: requires knowing which ports to forward
   - May not be worth the complexity
   - Container names + DNS resolution is a better pattern

## Testing Validation

After rebuilding sandbox with DNS proxy changes:

```bash
# 1. Hydra Mode - Container Name Resolution
docker -H unix:///var/run/hydra/active/session-xxx/docker.sock \
  run -d --name testapp nginx
# From desktop browser: http://testapp/ should work

# 2. Hydra Mode - Intranet DNS
docker -H unix:///var/run/hydra/active/session-xxx/docker.sock \
  exec testapp nslookup internal.corp.example.com
# Should resolve to internal IP

# 3. Hydra Mode - External DNS
docker -H unix:///var/run/hydra/active/session-xxx/docker.sock \
  exec testapp nslookup google.com
# Should resolve

# 4. Check DNS chain from desktop
cat /etc/resolv.conf
# Should show 10.200.X.1 as first nameserver
nslookup testapp
# Should resolve to 10.200.X.Y
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

## References

- `api/pkg/hydra/manager.go` - Hydra manager and bridge creation
- `api/pkg/hydra/server.go` - Hydra API server and DNS setup
- `api/pkg/hydra/dns.go` - DNS proxy implementation
