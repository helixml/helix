# Sandbox Kubernetes Deployment

**Date:** 2026-02-05
**Status:** Proposed
**Author:** Claude (with user)

## Overview

This document outlines the plan for deploying the Helix sandbox on Kubernetes (GKE, EKS, bare metal). The sandbox is more complex than the runner because it runs containers inside containers (Docker-in-Docker) for multi-tenant GPU-accelerated cloud desktops.

## Current Architecture Summary

The sandbox consists of:

```
HOST MACHINE (or Kubernetes Node)
│
└── SANDBOX CONTAINER (helix-sandbox)
    ├── dockerd (sandbox's isolated Docker daemon)
    ├── Hydra (multi-Docker isolation manager)
    ├── DNS Proxy (DNS forwarding for enterprise environments)
    ├── Sandbox Heartbeat (health monitoring)
    └── Desktop Containers (helix-sway, helix-ubuntu)
        ├── GNOME/Sway desktop
        ├── Zed IDE
        ├── Video streaming (GStreamer + NVENC)
        └── MCP server
```

### Key Challenges for Kubernetes

1. **Docker-in-Docker**: The sandbox runs Docker inside the container to launch desktop containers
2. **Nested Isolation**: Hydra creates per-session Docker daemons for tenant isolation
3. **GPU Passthrough**: Desktop containers need GPU access for video encoding
4. **Privileged Mode**: DinD requires privileged containers or specific capabilities
5. **Storage**: Both main dockerd and Hydra need persistent volumes
6. **Registry Access**: Desktop images are pulled from a registry at runtime

## Existing Helm Chart Assessment

There's already a `charts/helix-sandbox/` helm chart with:

- ✅ Basic deployment structure
- ✅ GPU support (NVIDIA, AMD, Intel, software rendering)
- ✅ Persistent volumes for Docker and Hydra storage
- ✅ Service exposure for WebSocket streaming
- ✅ Resource limits and GPU resource requests
- ✅ OpenShift SCC support (placeholder)

### Missing/Needs Work

1. **Desktop Image Distribution**
   - Current: References `ZED_IMAGE` but no mechanism for desktop images
   - Need: Support for registry-based desktop image pulling
   - Need: Enterprise registry override configuration

2. **Hydra Configuration**
   - Current: Basic volume mounts
   - Need: `HYDRA_ENABLED` and related environment variables
   - Need: Decide on privileged vs non-privileged mode for K8s

3. **Networking**
   - Current: Basic service exposure
   - Need: Consider how RevDial works in K8s context
   - Need: TURN server configuration for NAT traversal

4. **Multi-Replica Support**
   - Current: Single replica assumption
   - Need: Consider StatefulSet for stable identities
   - Need: Dedicated PVC per replica

5. **Health Checks**
   - Current: Basic TCP socket probe
   - Need: Proper liveness/readiness based on Hydra status

6. **Security**
   - Current: Privileged=true required
   - Need: Document minimum capabilities required
   - Need: Pod Security Standards consideration

## Implementation Plan

### Phase 1: Core Functionality

#### 1.1 Desktop Image Distribution

Add support for registry-based desktop images:

```yaml
# values.yaml additions
desktopImages:
  # Registry to pull desktop images from
  # Default: registry.helixml.tech
  # Enterprise: set to internal registry
  registry: "registry.helixml.tech"

  # Image versions (tags)
  # These should match the sandbox image version
  sway:
    enabled: true
    repository: "helix/helix-sway"
    tag: "latest"  # Override with specific version

  ubuntu:
    enabled: true
    repository: "helix/helix-ubuntu"
    tag: "latest"

  # Pull policy for desktop images inside sandbox
  # "always" - Always pull (ensures latest)
  # "if-not-present" - Only pull if not cached
  pullPolicy: "if-not-present"

  # Image pull secrets for private registries
  # These are passed to the sandbox's dockerd, not K8s
  pullSecrets: []
```

Environment variables to add to deployment:

```yaml
- name: HELIX_SANDBOX_REGISTRY
  value: {{ .Values.desktopImages.registry | quote }}
- name: HELIX_SWAY_IMAGE
  value: "{{ .Values.desktopImages.registry }}/{{ .Values.desktopImages.sway.repository }}:{{ .Values.desktopImages.sway.tag }}"
- name: HELIX_UBUNTU_IMAGE
  value: "{{ .Values.desktopImages.registry }}/{{ .Values.desktopImages.ubuntu.repository }}:{{ .Values.desktopImages.ubuntu.tag }}"
```

#### 1.2 Hydra Configuration

Add Hydra-specific settings:

```yaml
# values.yaml additions
hydra:
  # Enable Hydra for multi-tenant isolation
  enabled: true

  # Privileged mode (uses host Docker socket)
  # true: simpler networking, no tenant isolation (dev only)
  # false: full isolation with per-session dockerd (production)
  privilegedMode: false

  # Maximum concurrent sessions per sandbox pod
  maxSessions: 10
```

Environment variables:

```yaml
- name: HYDRA_ENABLED
  value: {{ .Values.hydra.enabled | quote }}
- name: HYDRA_PRIVILEGED_MODE_ENABLED
  value: {{ .Values.hydra.privilegedMode | quote }}
- name: MAX_SANDBOXES
  value: {{ .Values.hydra.maxSessions | quote }}
```

#### 1.3 Network Configuration

The sandbox uses RevDial to connect back to the control plane (no inbound connectivity required). Add configuration:

```yaml
# values.yaml additions
network:
  # RevDial configuration
  revdial:
    # Enable RevDial for control plane connectivity
    enabled: true
    # Reconnect interval on connection failure
    reconnectInterval: "5s"

  # TURN server for WebRTC NAT traversal
  turn:
    # Public IP/hostname for TURN server
    publicIp: ""
    # Password for TURN authentication
    password: ""
    # Read from existing secret
    existingSecret: ""
    existingSecretKey: "password"
```

### Phase 2: Production Hardening

#### 2.1 StatefulSet for Stable Identity

For multi-replica deployments, use StatefulSet:

```yaml
# values.yaml additions
deployment:
  # "Deployment" or "StatefulSet"
  # StatefulSet recommended for:
  # - Stable network identity (for RevDial)
  # - Per-pod persistent volumes
  type: "StatefulSet"
```

Benefits:
- Stable pod names (`helix-sandbox-0`, `helix-sandbox-1`)
- Per-pod PVCs automatically created
- Ordered scaling (important for GPU allocation)

#### 2.2 Pod Disruption Budget

Prevent all sandbox pods from being evicted simultaneously:

```yaml
# values.yaml additions
podDisruptionBudget:
  enabled: true
  minAvailable: 1
  # or: maxUnavailable: 1
```

#### 2.3 Priority Class

Ensure sandbox pods aren't preempted:

```yaml
# values.yaml additions
priorityClassName: ""
# Example: "system-cluster-critical" for critical workloads
```

#### 2.4 Resource Quotas

Add memory/CPU requests that reflect actual usage:

```yaml
# values.yaml - updated defaults
resources:
  requests:
    memory: "4Gi"
    cpu: "2"
  limits:
    memory: "32Gi"
    cpu: "8"
```

### Phase 3: Security Hardening

#### 3.1 Minimum Capabilities Analysis

The sandbox requires privileged mode for DinD, but we should document why:

```yaml
# Required capabilities for DinD:
# - SYS_ADMIN: Mount namespaces, cgroups
# - NET_ADMIN: Network namespace management
# - MKNOD: Create device nodes
# - SYS_PTRACE: Process inspection (debug)
# - DAC_OVERRIDE: File permission override

# Volume mounts required:
# - /dev: GPU and input devices
# - /run/udev: Device events
# - cgroup filesystem: Container resource limits
```

#### 3.2 Pod Security Standards

Document exemptions needed:

```yaml
# Pod Security Admission labels
# namespace labels required:
pod-security.kubernetes.io/enforce: privileged
pod-security.kubernetes.io/audit: privileged
pod-security.kubernetes.io/warn: privileged
```

#### 3.3 Network Policies

Restrict sandbox network access:

```yaml
# values.yaml additions
networkPolicy:
  enabled: false  # Disabled by default (may break DNS)

  # If enabled, allow:
  # - Egress to control plane (RevDial)
  # - Egress to registry (image pulls)
  # - Egress to DNS (53/UDP, 53/TCP)
  # - Ingress from control plane (streaming)
```

### Phase 4: Cloud Provider Specifics

#### 4.1 GKE (Google Kubernetes Engine)

```yaml
# values-gke.yaml
gpu:
  vendor: "nvidia"
  nvidia:
    enabled: true
    # GKE uses nvidia.com/gpu automatically
    runtimeClassName: ""

# GKE Autopilot doesn't support privileged pods
# Must use GKE Standard with node pools

nodeSelector:
  cloud.google.com/gke-accelerator: "nvidia-tesla-t4"

tolerations:
  - key: "nvidia.com/gpu"
    operator: "Exists"
    effect: "NoSchedule"
```

#### 4.2 EKS (Amazon Elastic Kubernetes Service)

```yaml
# values-eks.yaml
gpu:
  vendor: "nvidia"
  nvidia:
    enabled: true
    # EKS with NVIDIA device plugin
    runtimeClassName: ""

nodeSelector:
  # EKS GPU instance types
  node.kubernetes.io/instance-type: "g4dn.xlarge"

tolerations:
  - key: "nvidia.com/gpu"
    operator: "Exists"
    effect: "NoSchedule"
```

#### 4.3 Bare Metal / On-Premises

```yaml
# values-bare-metal.yaml
gpu:
  vendor: "nvidia"
  nvidia:
    enabled: true
    # May need custom runtime class
    runtimeClassName: "nvidia"

# Enterprise registry for air-gapped environments
desktopImages:
  registry: "internal-registry.company.com"
  pullSecrets:
    - name: internal-registry-creds
```

### Phase 5: Monitoring & Observability

#### 5.1 Prometheus Metrics

Add metrics endpoint:

```yaml
# values.yaml additions
metrics:
  enabled: true
  port: 9090
  path: "/metrics"

  serviceMonitor:
    enabled: false
    interval: "30s"
    labels: {}
```

Metrics to expose:
- `helix_sandbox_sessions_active` - Current active sessions
- `helix_sandbox_sessions_total` - Total sessions created
- `helix_sandbox_gpu_utilization` - GPU usage percentage
- `helix_sandbox_disk_usage_bytes` - Docker storage usage

#### 5.2 Logging

Structured logging configuration:

```yaml
# values.yaml additions
logging:
  level: "info"  # debug, info, warn, error
  format: "json"  # json or text
```

## File Changes Required

### 1. `charts/helix-sandbox/values.yaml`

Add sections for:
- `desktopImages` - Registry and image configuration
- `hydra` - Hydra isolation settings
- `network` - RevDial and TURN configuration
- `deployment.type` - Deployment vs StatefulSet
- `podDisruptionBudget` - PDB settings
- `metrics` - Prometheus integration
- `logging` - Log configuration

### 2. `charts/helix-sandbox/templates/deployment.yaml`

- Add all new environment variables
- Update volume mounts if needed
- Add init containers for pre-pulling images (optional)

### 3. `charts/helix-sandbox/templates/statefulset.yaml` (new)

- Create StatefulSet alternative with volumeClaimTemplates

### 4. `charts/helix-sandbox/templates/pdb.yaml` (new)

- Pod Disruption Budget

### 5. `charts/helix-sandbox/templates/servicemonitor.yaml` (new)

- Prometheus ServiceMonitor

### 6. `charts/helix-sandbox/templates/networkpolicy.yaml` (new)

- Network policies (optional)

### 7. `charts/helix-sandbox/README.md`

- Installation instructions
- Configuration reference
- Cloud provider examples
- Troubleshooting guide

## Testing Plan

### 1. Local Testing (kind/minikube)

```bash
# Create cluster with GPU support (if available)
kind create cluster --config kind-gpu-config.yaml

# Install NVIDIA device plugin
kubectl apply -f https://raw.githubusercontent.com/NVIDIA/k8s-device-plugin/main/nvidia-device-plugin.yml

# Install sandbox
helm install helix-sandbox ./charts/helix-sandbox \
  --set sandbox.apiUrl=https://api.helix.test \
  --set sandbox.runnerToken=test-token \
  --set gpu.vendor=none  # For testing without GPU
```

### 2. GKE Testing

```bash
# Create GKE cluster with GPU node pool
gcloud container clusters create helix-test \
  --accelerator type=nvidia-tesla-t4,count=1 \
  --machine-type n1-standard-4

# Install NVIDIA drivers
kubectl apply -f https://raw.githubusercontent.com/GoogleCloudPlatform/container-engine-accelerators/master/nvidia-driver-installer/cos/daemonset-preloaded.yaml

# Install sandbox
helm install helix-sandbox ./charts/helix-sandbox \
  -f values-gke.yaml
```

### 3. Integration Testing

- Verify RevDial connection to control plane
- Create session and verify desktop container starts
- Test video streaming
- Test input handling
- Verify GPU encoding works

## Open Questions

1. **Sidecar vs DinD**: Should we explore using a sidecar container for Docker daemon instead of DinD?
   - Pro: Cleaner separation of concerns
   - Con: More complex volume sharing

2. **Kata Containers**: For stricter isolation, should we support Kata Containers runtime?
   - Pro: Hardware virtualization isolation
   - Con: Performance overhead, complexity

3. **containerd + nerdctl**: Should we support containerd directly instead of Docker?
   - Pro: Lighter weight, native K8s integration
   - Con: Hydra is built around Docker API

4. **Image Caching**: Should sandbox pods share a common image cache?
   - Option A: PVC per pod (current)
   - Option B: ReadWriteMany PVC shared across pods
   - Option C: Node-level caching with DaemonSet

5. **Autoscaling**: How should sandbox pods scale?
   - Based on active sessions?
   - Based on GPU availability?
   - Manual scaling only?

## Implementation Order

1. **Week 1**: Core functionality
   - Update values.yaml with new configuration sections
   - Add desktop image environment variables
   - Add Hydra configuration
   - Test basic deployment on GKE

2. **Week 2**: Production hardening
   - Add StatefulSet support
   - Add PDB
   - Add network policies (optional)
   - Document security requirements

3. **Week 3**: Cloud provider support
   - Create values-gke.yaml
   - Create values-eks.yaml
   - Create values-bare-metal.yaml
   - Test on each platform

4. **Week 4**: Documentation & observability
   - Write comprehensive README
   - Add Prometheus metrics
   - Add troubleshooting guide
   - Create example configurations

## Success Criteria

1. Sandbox can be deployed on GKE with `helm install`
2. Sessions can be created and desktop containers launch
3. Video streaming works through the control plane
4. GPU encoding works (NVENC)
5. Multiple sandbox replicas can run simultaneously
6. Rolling updates work without session disruption
7. Documentation covers common deployment scenarios
