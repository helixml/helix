# Helix Runs Helix: Why We Deleted 2000 Lines of Networking Code

**Date:** 2026-02-11

![Self-similarity all the way down](https://media4.giphy.com/media/v1.Y2lkPTc5MGI3NjExN3I3MzNhZ2I0N3F0MG1ocDU3eTZhNzJpMHZuNXdvM29tbnIyZW04cCZlcD12MV9pbnRlcm5hbF9naWZfYnlfaWQmY3Q9Zw/aBzjyuoYLnRM4uuU4P/giphy.gif)

We build [Helix](https://github.com/helixml/helix), a platform that gives AI agents full desktop environments with Docker, GPUs, and Kubernetes. Agents get a real Linux desktop with a code editor, a terminal, and unrestricted Docker access — they can `docker compose up`, `kind create cluster`, or run anything that a human developer would.

Last week, we deleted ~2000 lines of networking infrastructure from our codebase. This post explains how a single false assumption created all that complexity, and what we learned by questioning it.

## The problem: Docker-in-Docker "only nests 2 levels deep"

Our architecture has three layers:

```
Host → Sandbox container (manages sessions) → Desktop container (user's environment)
```

The desktop container needs Docker so agents can build and run software. The conventional wisdom is that Docker-in-Docker only works 2 levels deep because overlay2 (Docker's storage driver) can't stack more than 2 layers of overlay filesystems. This is true — the Linux kernel enforces a `FILESYSTEM_MAX_STACK_DEPTH` of 2 for overlayfs.

We believed this meant we couldn't run dockerd inside the desktop container (that would be level 3). So we ran each session's dockerd as a **sibling process** inside the sandbox, connected to the desktop via:

- Virtual ethernet (veth) pairs and Linux bridges
- A custom DNS proxy (so containers could resolve each other)
- iptables DNAT rules (so `localhost` ports worked across containers)
- Bridge index allocation and cleanup
- Orphaned veth reconnection on crash recovery
- Per-session subnet management to avoid IP conflicts

It worked. It was also a constant source of bugs — the docker0 bridge deletion race, orphaned veths accumulating after crashes, bridge index exhaustion, DNS resolution failures, subnet conflicts between sessions.

## The fix: one line of Docker knowledge

The overlay2 limitation applies to **filesystem stacking**, not to Docker itself. Docker uses overlay2 to layer images, but the actual container data lives in `/var/lib/docker`. If you back `/var/lib/docker` with a real filesystem — say, a Docker volume (which is ext4) — then there's nothing to stack. Each nested Docker daemon sees a real filesystem, not an overlay-on-overlay.

```yaml
# This is all you need. The volume is ext4, not overlayfs.
volumes:
  - docker-data-${SESSION_ID}:/var/lib/docker
```

So we moved dockerd inside the desktop container. No veth pairs. No bridges. No DNS proxy. No iptables DNAT. No subnet management. The desktop container runs its own dockerd, and user containers share its network namespace — `localhost` just works.

We've tested this to 4+ levels of nesting: host → sandbox → desktop → Docker-in-Docker container → hello-world. Kind (Kubernetes-in-Docker) works too — `kind create cluster` succeeds inside the desktop.

## The unexpected payoff: Helix runs Helix

With the sibling-dockerd architecture, running Helix inside Helix (for development and testing) required special-casing: host Docker socket exposure, dual-Docker aliases, veth reconnection for shared BuildKit, and an admin-only `UseHostDocker` flag. About 500 lines of H-in-H-specific code across the API, CLI, and frontend.

With docker-in-desktop, all of that is gone. Running Helix inside Helix is just... running Helix. The inner stack brings up its own compose stack (API, database, registry, sandbox) on the desktop's local dockerd. The only configuration is a two-line `.env` file that routes LLM inference to the outer Helix:

```
OPENAI_API_KEY=<agent's API key>
OPENAI_BASE_URL=http://outer-api:8080/v1
```

This isn't even in the Helix source code — it's in the project's startup script. The platform has zero awareness that it's running inside itself. Every nesting level runs identical code.

## Why this matters

If your platform can run itself — with all its Docker nesting, GPU passthrough, compose stacks, image registries, and Kubernetes — then it can run arbitrary complex development environments. There's no upper bound on what agents can build and deploy, because the platform doesn't impose artificial constraints on the tooling available to them.

The architecture is self-similar: each level has its own dockerd, its own registry, its own subnet range (`10.(212+depth).0.0/16`), and its own compose DNS. Resources don't conflict because they're scoped to their Docker daemon, and the depth-based addressing prevents network collisions.

## What we deleted

| Component | Lines | Purpose |
|-----------|-------|---------|
| DNS proxy | ~290 | Custom DNS server (miekg/dns) for cross-container resolution |
| Bridge management | ~400 | veth pair creation, bridge allocation, orphan cleanup |
| iptables NAT | ~200 | localhost port forwarding, DNAT rules, port scanning |
| Subnet allocation | ~150 | Per-session subnet tracking, conflict avoidance |
| H-in-H special cases | ~500 | UseHostDocker flag, dual-Docker setup, host socket exposure |
| Associated bug fixes | ~500 | docker0 race, veth reconnect, bridge index exhaustion |

Total: ~2000 lines removed, replaced by a 98-line shell script (`17-start-dockerd.sh`) that starts dockerd with a volume mount.

## The takeaway

When you find yourself building elaborate infrastructure to work around a limitation, check whether the limitation is real. Ours wasn't — we confused a filesystem constraint with a Docker constraint, and spent months building (and debugging) a complex networking layer that was never necessary.

The fix was a Docker volume mount and a `--privileged` flag. The 2000 lines of code we deleted were solving a problem that didn't exist.

---

*[Helix](https://github.com/helixml/helix) is source available. The changes described here are on the `feature/docker-in-desktop` branch.*
