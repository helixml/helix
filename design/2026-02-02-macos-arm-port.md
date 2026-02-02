# Helix macOS ARM Port Design Document

**Date:** 2026-02-02
**Author:** Claude (with Luke)
**Status:** Draft

## Executive Summary

This document outlines the architecture for running Helix on macOS ARM (Apple Silicon). The main challenges are:
1. Building all container images for ARM64
2. GPU-accelerated desktop environments inside Linux VMs
3. Hardware video encoding (avoiding slow software encoding)

We propose **"Helix Desktop for Mac"** - a native macOS app built with **Wails (Go + WebView)** that:
- Manages a single Linux VM (UTM/QEMU) containing the entire Helix control plane
- Provides virtio-gpu 3D acceleration for desktop environments
- Extracts frames via virtio-vsock from the VM
- Encodes video using GStreamer + VideoToolbox (hardware H.264)
- Embeds the Helix web UI in a native WebView
- Lives in the menu bar with VM status and session management
- Auto-updates via get.helix.ml infrastructure

**Key architecture:** The Helix API, PostgreSQL, and sandbox containers run **inside the Linux VM** (exactly like production), while only video encoding runs natively on macOS. This minimizes platform-specific code while maximizing video performance.

**Vision (March 2026):** With M5 Macs featuring 128GB unified memory, users will be able to run a **fully local AI development environment** - 70B+ parameter LLM + agent swarm + GPU-accelerated desktops, all offline with no cloud dependencies.

## Current Architecture (x86_64 Linux)

```
┌─────────────────────────────────────────────────────────────────┐
│ Host (Linux x86_64 + NVIDIA/AMD/Intel GPU)                      │
├─────────────────────────────────────────────────────────────────┤
│ Docker + Docker Compose                                         │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ sandbox-nvidia container (Docker-in-Docker)                 │ │
│ │ ┌─────────────────────────────────────────────────────────┐ │ │
│ │ │ Hydra (manages isolated dockerd instances)              │ │ │
│ │ │ ┌─────────────────────────────────────────────────────┐ │ │ │
│ │ │ │ helix-ubuntu container (GNOME + Zed + streaming)    │ │ │ │
│ │ │ │  - PipeWire ScreenCast → pipewiresrc/zerocopy       │ │ │ │
│ │ │ │  - nvh264enc / vah264enc (GPU encoding)             │ │ │ │
│ │ │ │  - WebSocket H.264 stream to browser                │ │ │ │
│ │ │ └─────────────────────────────────────────────────────┘ │ │ │
│ │ └─────────────────────────────────────────────────────────┘ │ │
│ └─────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

**Key dependencies on Linux/NVIDIA:**
- `/dev/dri` device passthrough for GPU
- NVIDIA Container Toolkit for CUDA
- nvh264enc (NVIDIA) or vah264enc (AMD/Intel VA-API) for hardware encoding
- PipeWire + Wayland (GNOME/Sway) for screen capture

## Proposed macOS ARM Architecture

### Option A: Single VM + Native Encoding (Recommended)

Run the **entire Helix stack inside one Linux VM**, with native macOS encoding:

```
┌─────────────────────────────────────────────────────────────────┐
│ macOS Host (Apple Silicon M1/M2/M3/M4)                          │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ Helix Desktop.app (Wails - Go + WebView)                   │ │
│  │  • Native macOS app wrapping Helix web UI                  │ │
│  │  • Video encoding via GStreamer + vtenc_h264_hw            │ │
│  │  • VM lifecycle management (start/stop/snapshot)           │ │
│  │  • WebSocket server for video streams                      │ │
│  └────────────────────────────────────────────────────────────┘ │
│         ↑ frames (vsock)              ↓ control (ssh/API)       │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ UTM/QEMU VM (ARM64 Linux) - virtio-gpu-gl-pci for 3D       │ │
│  │ ┌────────────────────────────────────────────────────────┐ │ │
│  │ │ Docker (inside VM) - THE HELIX CONTROL PLANE           │ │ │
│  │ │  • Helix API server          ← same as production      │ │ │
│  │ │  • PostgreSQL                                          │ │ │
│  │ │  • Vectorchord                                         │ │ │
│  │ │  • Sandbox container(s) ──┐                            │ │ │
│  │ │                           ▼                            │ │ │
│  │ │  ┌──────────────────────────────────────────────────┐  │ │ │
│  │ │  │ helix-ubuntu (GNOME + Zed + Qwen Code)          │  │ │ │
│  │ │  │  - virtio-gpu (virgl3d) for 3D acceleration     │  │ │ │
│  │ │  │  - PipeWire ScreenCast → raw frames → vsock     │  │ │ │
│  │ │  └──────────────────────────────────────────────────┘  │ │ │
│  │ │  ┌──────────────────────────────────────────────────┐  │ │ │
│  │ │  │ helix-ubuntu (another session)                  │  │ │ │
│  │ │  └──────────────────────────────────────────────────┘  │ │ │
│  │ └────────────────────────────────────────────────────────┘ │ │
│  └────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

**Key insight:** The Helix API, PostgreSQL, and all services run **inside the VM**, exactly like production Linux. The only thing on macOS is:
1. The Mac app (Wails) for user interface
2. Video encoding (GStreamer + VideoToolbox)
3. VM management

**Pros:**
- Single VM to manage (simpler operations)
- Helix runs exactly like production Linux deployment
- Multiple sandbox sessions inside one VM (existing Docker-in-Docker architecture)
- Hardware video encoding via VideoToolbox (~60 FPS)
- virtio-gpu provides 3D acceleration for desktops

**Cons:**
- VM needs more resources (8-16GB RAM for control plane + sandboxes)
- Frame transfer from VM to host still needed (vsock)

### Option B: All-in-VM with Software Encoding

```
┌─────────────────────────────────────────────────────────────────┐
│ macOS Host (Apple Silicon)                                      │
├─────────────────────────────────────────────────────────────────┤
│  ┌──────────────────────────────────────────────────────────────┤
│  │ UTM/QEMU VM (ARM64 Linux)                                   │ │
│  │                                                             │ │
│  │  ┌────────────────────────────────────────────────────────┐ │ │
│  │  │ Docker (inside VM)                                     │ │ │
│  │  │  - helix-ubuntu (GNOME + Zed + streaming)             │ │ │
│  │  │  - API server, PostgreSQL, etc.                       │ │ │
│  │  │  - Software encoding (openh264enc/x264enc)            │ │ │
│  │  └────────────────────────────────────────────────────────┘ │ │
│  └──────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

**Pros:**
- Simpler architecture (everything in one VM)
- Closest to current Linux architecture

**Cons:**
- Software encoding is CPU-intensive and slow (~15-20 FPS max at 1080p)
- Poor user experience with laggy video
- Nested Docker-in-Docker-in-VM adds complexity

### Option C: Native macOS Desktop (Most Invasive)

Run the desktop natively on macOS with screen sharing:

```
┌─────────────────────────────────────────────────────────────────┐
│ macOS Host (Apple Silicon)                                      │
├─────────────────────────────────────────────────────────────────┤
│  ┌────────────────────┐  ┌────────────────────────────────────┐ │
│  │ macOS Zed IDE      │  │ Docker/OrbStack (ARM64)            │ │
│  │ (native app)       │  │  - API server                      │ │
│  │                    │  │  - PostgreSQL                      │ │
│  │ ScreenCaptureKit   │  │  - Vectorchord                     │ │
│  │ → VideoToolbox     │  │                                    │ │
│  │ → WebSocket        │  │ Linux container for Qwen Code      │ │
│  └────────────────────┘  └────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

**Pros:**
- Best performance (native everything)
- Simplest video pipeline (ScreenCaptureKit → VideoToolbox)

**Cons:**
- Requires macOS-native Zed build (already exists)
- Loses Linux container isolation for user workspaces
- Security model changes significantly
- Would need ScreenCaptureKit permissions from user

---

## Recommendation: Option A (Hybrid VM + Native Encoding)

This provides the best balance of performance and compatibility.

## Detailed Design

### Component 1: ARM64 Container Builds

All Dockerfiles need multi-arch support:

```dockerfile
# Example: Dockerfile.ubuntu-helix changes
FROM --platform=$TARGETPLATFORM ubuntu:25.10

# Conditional GPU packages
RUN if [ "$(uname -m)" = "aarch64" ]; then \
      # No NVIDIA CUDA on ARM, use software GL
      apt-get install -y mesa-utils libgl1-mesa-dri; \
    else \
      # x86_64: keep existing NVIDIA/VA-API setup
      apt-get install -y nvidia-cuda-toolkit libva-dev; \
    fi
```

**Files to modify:**
- `Dockerfile.ubuntu-helix` - ARM64 build, remove NVIDIA deps
- `Dockerfile.sway-helix` - ARM64 build, remove NVIDIA deps
- `Dockerfile.sandbox` - ARM64 build (or eliminate for macOS)
- `docker-compose.dev.yaml` - Add ARM64 profiles

**Build commands:**
```bash
# Build ARM64 images on macOS
docker buildx build --platform linux/arm64 -t helix-ubuntu:arm64 -f Dockerfile.ubuntu-helix .
docker buildx build --platform linux/arm64 -t helix-sway:arm64 -f Dockerfile.sway-helix .
```

### Component 2: UTM/QEMU Linux VM for Desktop

Use UTM with QEMU backend (not Apple Virtualization Framework) for 3D acceleration.

**VM Configuration:**
```
- CPU: 4-8 cores (host passthrough)
- RAM: 8-16 GB
- Disk: 64 GB qcow2
- Display: virtio-gpu-gl-pci (virgl3d acceleration)
- Network: virtio-net (bridged or NAT)
- virtio-vsock: enabled (for host↔guest communication)
```

**Guest OS:** Ubuntu 24.04+ ARM64 with:
- GNOME on Wayland (uses virgl3d)
- PipeWire for screen capture
- Docker for running helix-ubuntu container
- virtio-vsock tools for frame transfer

**Performance expectations with virtio-gpu:**
- OpenGL 3.3-4.5 via virgl3d
- Sufficient for GNOME desktop and Zed IDE
- Not suitable for heavy 3D games but fine for dev tools

### Component 3: Frame Transfer (VM → Host)

This is the key innovation needed. Options:

#### Option 3A: virtio-vsock + Raw Frames

```
VM (helix-ubuntu):
  PipeWire ScreenCast → raw NV12/I420 frames → vsock connection

macOS Host (helix-encoder):
  vsock listener → raw frames → vtenc_h264_hw → H.264 → WebSocket
```

**Implementation:**
```go
// VM side: desktop-bridge modifications
func streamToVsock(frames <-chan *gst.Buffer) {
    conn, _ := vsock.Dial(vsock.Host, 5000) // CID 2 = host
    for frame := range frames {
        // Send frame header (timestamp, size, format)
        // Send raw pixel data
    }
}

// Host side: helix-encoder (new binary)
func receiveFromVsock() {
    listener, _ := vsock.Listen(5000)
    for {
        conn, _ := listener.Accept()
        go encodeAndStream(conn)
    }
}
```

**Bandwidth calculation:**
- 1920x1080 @ 60fps NV12 = 1920 * 1080 * 1.5 * 60 = ~186 MB/s
- virtio-vsock can handle this (typically 1+ GB/s)

#### Option 3B: Shared Memory via virtio-fs

Map a shared memory region between VM and host:

```
VM: mmap /mnt/shared/framebuffer (ring buffer)
Host: mmap same file, poll for new frames
```

Lower latency than vsock but more complex synchronization.

#### Option 3C: Compress in VM, Re-encode on Host

```
VM: PipeWire → MJPEG (fast, low CPU) → vsock
Host: MJPEG decode → vtenc_h264_hw → H.264 → WebSocket
```

Lower bandwidth (~20 MB/s) but extra encode/decode cycle.

**Recommendation:** Start with Option 3A (vsock + raw frames) for simplicity.

### Component 4: Native macOS Encoder (`helix-encoder`)

New Go binary that runs on macOS host:

```go
// cmd/helix-encoder/main.go
package main

import (
    "github.com/go-gst/go-gst/gst"
)

func main() {
    gst.Init(nil)

    // GStreamer pipeline for macOS
    pipeline := `
        appsrc name=framesrc format=time
        ! video/x-raw,format=NV12,width=1920,height=1080,framerate=60/1
        ! vtenc_h264_hw
            allow-frame-reordering=false
            realtime=true
            max-keyframe-interval=120
        ! video/x-h264,stream-format=byte-stream
        ! appsink name=h264sink
    `

    // Accept frames from vsock, push to appsrc
    // Pull H.264 from appsink, send to WebSocket clients
}
```

**Build requirements:**
```bash
# Install GStreamer on macOS
brew install gstreamer gst-plugins-base gst-plugins-good gst-plugins-bad

# Build helix-encoder
CGO_ENABLED=1 go build -o helix-encoder ./cmd/helix-encoder
```

**GStreamer VideoToolbox verification:**
```bash
gst-inspect-1.0 vtenc_h264_hw
# Should show: Apple VideoToolbox H264 HW-only encoder
```

### Component 5: Modified Sandbox Architecture

On macOS, we don't use Docker-in-Docker. Instead:

```
macOS Host
├── OrbStack/Docker Desktop (ARM64 containers)
│   ├── API server
│   ├── PostgreSQL
│   └── Other services
├── UTM VM (managed by Helix)
│   └── helix-ubuntu container (GNOME + Zed)
└── helix-encoder (native binary)
```

**Hydra changes:**
- New `VMMDriver` interface alongside `DockerDriver`
- UTM/QEMU control via `qemu-system-aarch64` CLI or UTM's scripting
- VM lifecycle management (start, stop, snapshot)

### Component 6: Configuration

New environment variables:

```bash
# .env for macOS ARM
HELIX_PLATFORM=darwin-arm64
HELIX_VM_BACKEND=utm           # utm, qemu, or parallels
HELIX_ENCODER_MODE=native      # native (macOS binary) or vm (software)
HELIX_VSOCK_PORT=5000
HELIX_VM_CPUS=4
HELIX_VM_MEMORY=8192           # MB
```

---

## Mac App Technology Choices

### Why Go + Wails? (Recommended)

After researching macOS app development options, **[Wails](https://wails.io/)** is the best fit:

| Option | Language | Bundle Size | Browser | Pros | Cons |
|--------|----------|-------------|---------|------|------|
| **Wails** | Go | ~4 MB | Native WebView (WKWebView) | Reuse Go code, tiny binary | Less mature than Electron |
| Tauri | Rust | ~2-10 MB | Native WebView | Smallest, secure | Rust learning curve |
| Electron | JS | ~100+ MB | Bundled Chromium | Mature ecosystem | Huge, memory hungry |
| Fyne | Go | ~20 MB | None (native widgets) | Pure Go | Would need to rebuild UI |

**Wails advantages for Helix:**
1. **Go backend** - Reuse existing Helix Go code (GStreamer bindings, WebSocket server, API client)
2. **Web frontend** - Embed the existing Helix React UI in WKWebView
3. **Small binary** - ~4 MB vs Electron's 100+ MB
4. **Native WebView** - Uses macOS WKWebView (Safari engine), not bundled Chromium
5. **Easy interop** - Call Go functions directly from JavaScript

### GStreamer + VideoToolbox in Go

The [go-gst](https://github.com/go-gst/go-gst) bindings work on macOS with VideoToolbox:

```bash
# Install GStreamer on macOS
brew install gstreamer gst-plugins-base gst-plugins-good gst-plugins-bad

# Verify VideoToolbox encoder is available
gst-inspect-1.0 vtenc_h264_hw
# Should show: Apple VideoToolbox H264 HW-only encoder
```

**Example Go code using vtenc_h264_hw:**
```go
import "github.com/go-gst/go-gst/gst"

// Reuse existing ws_stream.go patterns, just change encoder
func buildMacOSPipeline() string {
    return `
        appsrc name=framesrc format=time
        ! video/x-raw,format=NV12,width=1920,height=1080,framerate=60/1
        ! vtenc_h264_hw
            allow-frame-reordering=false
            realtime=true
            max-keyframe-interval=120
        ! video/x-h264,stream-format=byte-stream
        ! appsink name=h264sink
    `
}
```

This is **very sympathetic with the existing stack** - same GStreamer patterns, just a different encoder element.

### Alternative: Pure VideoToolbox (No GStreamer)

For tighter macOS integration, could use VideoToolbox directly via cgo:

```go
// #cgo CFLAGS: -x objective-c
// #cgo LDFLAGS: -framework VideoToolbox -framework CoreVideo -framework CoreMedia
// #include <VideoToolbox/VideoToolbox.h>
import "C"

// Direct VTCompressionSession usage
// More work, but removes GStreamer dependency
```

**Recommendation:** Start with GStreamer (reuse existing patterns), consider pure VideoToolbox later if needed.

---

## "Helix Desktop for Mac" - Native App Vision

### Overview

A single macOS app built with **Wails (Go + WebView)** that packages everything:

```
┌─────────────────────────────────────────────────────────────────┐
│         Helix Desktop for Mac (native Swift/Go app)             │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │ Menu Bar Status                     [●] Running         │    │
│  │ • 2 active sessions                                     │    │
│  │ • CPU: 45%  GPU: 30%  Encoding: 60fps                  │    │
│  └─────────────────────────────────────────────────────────┘    │
│                                                                 │
│  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────┐  │
│  │   VM Manager     │  │  Video Pipeline  │  │   API Proxy  │  │
│  │                  │  │                  │  │              │  │
│  │ • Start/stop VMs │  │ • Frame capture  │  │ • Routes     │  │
│  │ • Snapshot mgmt  │  │ • VideoToolbox   │  │   /stream/*  │  │
│  │ • Resource alloc │  │ • WebSocket srv  │  │ • Coordinates│  │
│  │                  │  │                  │  │   with API   │  │
│  └────────┬─────────┘  └────────┬─────────┘  └──────┬───────┘  │
│           │                     │                    │          │
│           ▼                     ▼                    ▼          │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │              Embedded QEMU/UTM Runtime                   │   │
│  │  ┌─────────────────────────────────────────────────────┐│   │
│  │  │ VM: helix-desktop-1 (Ubuntu + GNOME + Zed)         ││   │
│  │  │ • virtio-gpu (virgl3d) for 3D acceleration         ││   │
│  │  │ • virtio-vsock for host↔guest communication        ││   │
│  │  │ • 4 CPU cores, 8GB RAM                             ││   │
│  │  └─────────────────────────────────────────────────────┘│   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
        ↑                         ↑                      ↑
        │ VM control              │ H.264 stream         │ API
        ▼                         ▼                      ▼
┌───────────────┐    ┌─────────────────────┐    ┌──────────────────┐
│ User's Docker │    │      Browser        │    │   Helix API      │
│ (OrbStack)    │    │ (video playback)    │    │ (in container)   │
│               │    │                     │    │                  │
│ • PostgreSQL  │    │ WebSocket client    │    │ Session mgmt     │
│ • Vectorchord │    │ receives H.264      │    │ Auth, billing    │
│ • Other svcs  │    │                     │    │ Agent coord      │
└───────────────┘    └─────────────────────┘    └──────────────────┘
```

### App Components

#### 1. VM Manager (Swift + Virtualization.framework or embedded QEMU)

```swift
class HelixVMManager {
    // Start a new desktop session
    func startSession(config: SessionConfig) async throws -> VMSession {
        // 1. Clone base VM snapshot (fast)
        // 2. Configure resources (CPU, RAM)
        // 3. Start QEMU with virtio-gpu-gl-pci
        // 4. Wait for guest agent ready
        // 5. Return session handle
    }

    // Get framebuffer access
    func getFramebuffer(session: VMSession) -> IOSurfaceRef {
        // Access QEMU's IOSurface for zero-copy frame access
    }
}
```

#### 2. Video Pipeline (Go + GStreamer or Swift + VideoToolbox)

**Option A: Go + GStreamer (reuse existing code)**
```go
// Reuse ws_stream.go patterns, just swap encoder
pipeline := `
    appsrc name=framesrc
    ! video/x-raw,format=NV12,width=1920,height=1080,framerate=60/1
    ! vtenc_h264_hw allow-frame-reordering=false realtime=true
    ! h264parse
    ! appsink name=h264sink
`
```

**Option B: Pure Swift + VideoToolbox (tighter integration)**
```swift
class VideoEncoder {
    private var compressionSession: VTCompressionSession?

    func encodeFrame(_ surface: IOSurfaceRef) {
        // Wrap IOSurface as CVPixelBuffer (zero-copy)
        var pixelBuffer: CVPixelBuffer?
        CVPixelBufferCreateWithIOSurface(nil, surface, nil, &pixelBuffer)

        // Encode with hardware
        VTCompressionSessionEncodeFrame(
            compressionSession!,
            pixelBuffer!,
            presentationTimestamp,
            duration,
            nil, nil, nil
        )
    }
}
```

#### 3. WebSocket Server

The native app serves WebSocket connections at `ws://localhost:8765/stream/{session_id}`:

```swift
func handleWebSocket(session: VMSession, ws: WebSocket) async {
    // Send stream init message (resolution, codec info)
    await ws.send(StreamInit(width: 1920, height: 1080, codec: "h264"))

    // Stream H.264 frames
    for await frame in encoder.frames(for: session) {
        await ws.send(frame.nalUnit)
    }
}
```

#### 4. API Proxy / Coordinator

The app intercepts certain Helix API calls:

| Route | Handled By | Notes |
|-------|------------|-------|
| `POST /api/v1/sessions` | API → App | App provisions VM, returns stream URL |
| `GET /api/v1/sessions/:id/stream` | App | WebSocket video stream |
| `POST /api/v1/sessions/:id/input` | App → VM | Keyboard/mouse to VM |
| Everything else | API | Normal Helix API |

### Distribution Options

#### Option A: Standalone App (Recommended for dev)

```
Helix Desktop.app/
├── Contents/
│   ├── MacOS/
│   │   └── Helix Desktop          # Main binary
│   ├── Frameworks/
│   │   ├── GStreamer.framework    # For video pipeline
│   │   └── QEMU.framework         # Embedded QEMU
│   ├── Resources/
│   │   ├── helix-ubuntu.qcow2     # Base VM image
│   │   └── config/                # Default configs
│   └── Info.plist
```

#### Option B: Homebrew Formula

```ruby
class HelixDesktop < Formula
  desc "GPU-accelerated remote desktops for AI agents"
  homepage "https://helix.ml"

  depends_on "qemu"
  depends_on "gstreamer"
  depends_on "orbstack" # or "docker"

  def install
    # Install helix-desktop binary
    # Download/create base VM image
  end
end
```

#### Option C: Docker Compose + Native Helper

For users who want containers everywhere:

```yaml
# docker-compose.macos.yaml
services:
  api:
    image: helix/api:arm64
    # ... normal API config

  # Native helper runs outside Docker
  # Started by: helix-desktop-helper
```

### Menu Bar Widget

The app lives primarily in the macOS menu bar with a status icon:

```
┌─────────────────────────────────────────────────────────────────┐
│ ◉ Helix                                              [▼]       │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Status: Running                                    🟢          │
│  ─────────────────────────────────────────────────────────      │
│  VM: helix-vm-1                                                 │
│  • CPU: 45%   Memory: 6.2 GB / 8 GB                            │
│  • Sessions: 2 active                                          │
│  • Encoding: 60 fps (VideoToolbox)                             │
│  ─────────────────────────────────────────────────────────      │
│  Sessions:                                                      │
│  • ses_abc123 - Project: my-app        [Open] [Stop]           │
│  • ses_def456 - Project: backend       [Open] [Stop]           │
│  ─────────────────────────────────────────────────────────      │
│  ⟳  Update available: v1.2.3 → v1.2.4  [Install]               │
│  ─────────────────────────────────────────────────────────      │
│  Open Helix UI...                                    ⌘O         │
│  New Session...                                      ⌘N         │
│  ─────────────────────────────────────────────────────────      │
│  Start VM                                                       │
│  Stop VM                                                        │
│  VM Settings...                                                 │
│  ─────────────────────────────────────────────────────────      │
│  Check for Updates...                                           │
│  About Helix Desktop                                            │
│  Quit                                                ⌘Q         │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

**Wails menu bar support:**
```go
// Using Wails v2 system tray
app := wails.CreateApp(&wails.AppConfig{
    Title:  "Helix Desktop",
    Width:  1200,
    Height: 800,
    Menu:   buildMenu(),
    SystemTray: &wails.SystemTray{
        Icon:    helixIcon,
        OnClick: showStatusMenu,
    },
})
```

### Auto-Updates via get.helix.ml

Leverage the existing Helix installer infrastructure for updates:

```go
const updateURL = "https://get.helix.ml/desktop/darwin-arm64/version.json"

type VersionInfo struct {
    Version     string `json:"version"`
    DownloadURL string `json:"download_url"`
    Checksum    string `json:"sha256"`
    ReleaseDate string `json:"release_date"`
    Changelog   string `json:"changelog_url"`
}

func checkForUpdates() (*VersionInfo, error) {
    resp, err := http.Get(updateURL)
    // Parse version.json
    // Compare with current version
    // Return update info if newer
}

func installUpdate(info *VersionInfo) error {
    // 1. Download new .app bundle to temp location
    // 2. Verify SHA256 checksum
    // 3. Stop VM gracefully
    // 4. Replace app bundle (or use Sparkle framework)
    // 5. Restart app
}
```

**Update check schedule:**
- On app launch
- Every 24 hours while running
- Manual check via menu

**get.helix.ml integration:**
```
https://get.helix.ml/
├── install.sh              # Existing Linux/macOS CLI installer
├── desktop/
│   ├── darwin-arm64/
│   │   ├── version.json    # Current version metadata
│   │   ├── Helix-Desktop-1.2.4.dmg
│   │   └── Helix-Desktop-1.2.4.dmg.sha256
│   ├── darwin-amd64/       # Intel Mac support (future)
│   └── windows-amd64/      # Windows support (future)
└── vm-images/
    ├── helix-vm-arm64-1.2.4.qcow2.zst  # Compressed VM image
    └── helix-vm-arm64-1.2.4.sha256
```

### User Experience

**First run:**
```
┌─────────────────────────────────────────────────────────────────┐
│                    Welcome to Helix Desktop                     │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Setting up your local AI development environment...            │
│                                                                 │
│  [████████████████░░░░░░░░░░░░░░░░] 45%                        │
│                                                                 │
│  ✓ Downloading VM image (2.1 GB)                               │
│  ✓ Verifying checksum                                          │
│  → Configuring VM...                                           │
│  ○ Starting services                                           │
│  ○ Ready!                                                      │
│                                                                 │
│  This will take a few minutes on first run.                    │
│  Subsequent starts will be much faster.                        │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

**Daily use:**
```bash
# App auto-starts VM on launch (optional setting)
# Or manual start via menu bar

# Open Helix UI (embedded WebView or browser)
# Click "New Session" → VM provisions sandbox container
# Video streams through native encoder to browser
```

### Frontend Options

The video stream ultimately needs to reach the user. Three options:

#### Option 1: External Browser (Default, Existing UI)

```
┌─────────────────────────────────────────────────────────────────┐
│ Safari / Chrome / Firefox                                       │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ https://localhost:3000 (Helix Web UI)                       │ │
│ │                                                             │ │
│ │  ┌─────────────────────────────────────────────────────┐   │ │
│ │  │ <video> element                                     │   │ │
│ │  │ WebSocket: ws://localhost:8765/stream/ses_xxx       │   │ │
│ │  │ H.264 → WebCodecs/MSE → Canvas                      │   │ │
│ │  └─────────────────────────────────────────────────────┘   │ │
│ └─────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

**Pros:** No changes to existing frontend, works on any device
**Cons:** Extra WebSocket hop, potential latency

#### Option 2: Embedded WKWebView (Native Mac App Experience)

```swift
// In Helix Desktop.app
class HelixWindow: NSWindow {
    let webView: WKWebView

    func showSession(_ session: VMSession) {
        // Load Helix web UI in embedded browser
        webView.load(URLRequest(url: URL(string: "http://localhost:3000/session/\(session.id)")!))

        // WebSocket connects to our local video server
        // Same flow as external browser, but contained in app
    }
}
```

**Pros:** Single app experience, can add native controls
**Cons:** Still using web rendering for video

#### Option 3: Native Metal View (Best Performance, Most Work)

```swift
// Direct Metal rendering of video frames
class HelixVideoView: MTKView {
    var videoTexture: MTLTexture?

    func displayFrame(_ surface: IOSurfaceRef) {
        // Create Metal texture from IOSurface (zero-copy!)
        let descriptor = MTLTextureDescriptor.texture2DDescriptor(...)
        videoTexture = device?.makeTexture(descriptor: descriptor, iosurface: surface, plane: 0)

        // Render to view
        setNeedsDisplay()
    }

    override func draw(_ rect: CGRect) {
        // Draw videoTexture directly
    }
}
```

**Pros:** Absolute lowest latency, skip all web rendering
**Cons:** Need to reimplement chat UI, controls, etc. in native

#### Recommendation

| User Type | Recommended Frontend |
|-----------|---------------------|
| Developers (local) | **Option 2**: Embedded WKWebView in Mac app |
| Remote/team access | **Option 1**: External browser to hosted Helix |
| Performance-critical | **Option 3**: Native Metal view (future) |

**Suggested approach:**
1. Start with **Option 1** (external browser) - zero frontend changes
2. Add **Option 2** (embedded WKWebView) for polished Mac experience
3. Consider **Option 3** only if latency is critical

The Mac app can support all three - it just changes where the WebSocket connects:
- External browser → `ws://localhost:8765/stream/...`
- Embedded browser → same URL, just in WKWebView
- Native view → skip WebSocket, render IOSurface directly

### Why This Architecture?

| Concern | Solution |
|---------|----------|
| Video encoding speed | VideoToolbox hardware (60fps easy) |
| GPU acceleration in VM | virtio-gpu with virgl3d |
| Container isolation | Linux VM provides full isolation |
| Developer experience | Single `helix-desktop start` command |
| Existing Helix compat | API unchanged, just video routing differs |
| Frontend flexibility | Browser, embedded WebView, or native Metal |

---

## Implementation Phases

### Phase 1: ARM64 Container Builds (Week 1-2)

1. [ ] Add `--platform linux/arm64` to all Dockerfiles
2. [ ] Remove/conditionalize NVIDIA-specific code
3. [ ] Build and test ARM64 images on OrbStack
4. [ ] Verify API server, PostgreSQL, Vectorchord work on ARM64
5. [ ] Create `docker-compose.macos.yaml` variant

### Phase 2: UTM VM Setup (Week 2-3)

1. [ ] Create UTM VM template with Ubuntu 24.04 ARM64
2. [ ] Configure virtio-gpu-gl-pci for 3D acceleration
3. [ ] Install GNOME, PipeWire, Docker in VM
4. [ ] Test Zed IDE runs acceptably in VM
5. [ ] Enable virtio-vsock

### Phase 3: Frame Transfer (Week 3-4)

1. [ ] Implement vsock frame sender in `desktop-bridge`
2. [ ] Build proof-of-concept macOS receiver
3. [ ] Measure latency and bandwidth
4. [ ] Optimize frame format (NV12 vs I420 vs RGB)

### Phase 4: Native Encoder (Week 4-5)

1. [ ] Create `cmd/helix-encoder` package
2. [ ] GStreamer pipeline with vtenc_h264_hw
3. [ ] WebSocket server integration
4. [ ] Test end-to-end streaming

### Phase 5: Integration (Week 5-6)

1. [ ] Modify Hydra for VM management
2. [ ] Update API to coordinate VM + encoder
3. [ ] Create `./stack` commands for macOS
4. [ ] Documentation and testing

---

## Technical Risks & Mitigations

### Risk 1: virtio-gpu Performance

**Risk:** virgl3d may be too slow for smooth GNOME desktop.

**Mitigation:**
- Test early with target applications (Zed, terminal, browser)
- Consider Sway instead of GNOME (lighter weight)
- Fall back to Option C (native macOS) if needed

### Risk 2: Frame Transfer Latency

**Risk:** vsock + re-encoding adds latency.

**Mitigation:**
- Profile and optimize frame pipeline
- Consider MJPEG intermediate format if raw frames too slow
- Target <100ms end-to-end latency

### Risk 3: UTM/QEMU Complexity

**Risk:** Users may struggle with VM setup.

**Mitigation:**
- Script VM creation/configuration
- Provide pre-built VM images
- Consider Tart (https://tart.run) for automated VM management on macOS

### Risk 4: VideoToolbox Encoding Issues

**Risk:** M1 Pro/Max/Ultra may have slower H.264 encoding (see [MacRumors thread](https://forums.macrumors.com/threads/so-whats-the-verdict-on-m2-pro-max-hw-accelerated-h-264-encoding-speed.2378503/)).

**Mitigation:**
- Test on multiple Apple Silicon variants
- Fall back to H.265 if H.264 is slow on Pro/Max chips
- Consider reducing resolution/framerate

---

## Alternative Technologies Considered

### Podman with libkrun

[Podman supports GPU acceleration on Apple Silicon](https://github.com/ggml-org/llama.cpp/discussions/8042) via libkrun with virtio-gpu (Venus/Vulkan). However:
- Vulkan, not OpenGL (GNOME needs GL)
- Primarily for compute, not desktop graphics
- May be viable in future

### Parallels Desktop

Commercial, but has excellent ARM Linux VM support with 3D acceleration. Could be an alternative to UTM for users who have it.

### Docker Model Runner

[DMR now supports Vulkan](https://www.docker.com/blog/docker-model-runner-vulkan-gpu-support/) but is focused on AI inference, not desktop streaming.

---

## Deep Dive: Frame Extraction from UTM

### How UTM Currently Renders (Key Discovery!)

According to [UTM's graphics documentation](https://github.com/utmapp/UTM/blob/main/Documentation/Graphics.md):

```
Guest (Linux VM)
    ↓ virtio-gpu commands
virglrenderer (host-side OpenGL translation)
    ↓ OpenGL
ANGLE (translates to Metal)
    ↓ renders to
IOSurface (shared GPU memory)
    ↓ passed via IOSurfaceID
CocoaSpice (Metal renderer)
    ↓
UTM Window (display)
```

**Critical insight:** The framebuffer is ALREADY on the macOS host as an **IOSurface**!
This is Apple's zero-copy GPU memory sharing primitive.

### Frame Extraction Options

| Method | How it Works | Latency | Bandwidth | Effort |
|--------|--------------|---------|-----------|--------|
| **IOSurface tap** | Access UTM's existing IOSurface | ~0ms | Zero-copy | Fork UTM |
| **SPICE client** | Connect via libspice-client | 5-20ms | Raw pixels | Use existing code |
| **virtio-vsock** | Stream from VM to host | 1-5ms | ~186 MB/s | Custom protocol |
| **SPICE streaming** | Built-in MJPEG mode | 20-50ms | Compressed | Config only |

### Option A: IOSurface Tap (Best Performance, Requires UTM Changes)

The most elegant solution is to tap into UTM's existing IOSurface:

```
┌─────────────────────────────────────────────────────────────────┐
│ UTM Process (QEMULauncher)                                      │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ QEMU + virglrenderer                                        │ │
│ │         ↓                                                   │ │
│ │ ANGLE → IOSurface ─────────┬───────────────────────────────┼─┤
│ │         ↓                  │                               │ │
│ │ CocoaSpice → UTM Window    │ Export via Mach port          │ │
│ └─────────────────────────────────────────────────────────────┘ │
└──────────────────────────────┼──────────────────────────────────┘
                               │
                               ▼ IOSurfaceLookupFromMachPort()
┌─────────────────────────────────────────────────────────────────┐
│ helix-encoder (native macOS)                                    │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ IOSurface → CVPixelBufferCreateWithIOSurface()              │ │
│ │         ↓                                                   │ │
│ │ vtenc_h264_hw (VideoToolbox - hardware H.264)               │ │
│ │         ↓                                                   │ │
│ │ WebSocket server → Browser                                  │ │
│ └─────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

**Implementation:**
1. Fork UTM or contribute upstream
2. Add XPC method: `- (mach_port_t)getFramebufferPort`
3. In helix-encoder:
   ```objc
   // Get IOSurface from UTM
   IOSurfaceRef surface = IOSurfaceLookupFromMachPort(utmFramebufferPort);

   // Wrap as CVPixelBuffer (zero-copy!)
   CVPixelBufferRef pixelBuffer;
   CVPixelBufferCreateWithIOSurface(NULL, surface, NULL, &pixelBuffer);

   // Feed directly to VideoToolbox
   VTCompressionSessionEncodeFrame(session, pixelBuffer, ...);
   ```

**Pros:** Zero-copy, lowest latency, elegant
**Cons:** Requires UTM modifications

### Option B: SPICE Client (Easiest, Good Enough?)

UTM uses SPICE protocol internally. We can connect as a SPICE client:

```
UTM VM
    ↓ SPICE protocol
┌─────────────────────────────────────────────────────────────────┐
│ helix-encoder (using libspice-client-glib)                      │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ SpiceDisplay → raw pixel callback                           │ │
│ │         ↓                                                   │ │
│ │ CVPixelBuffer (copy from SPICE buffer)                      │ │
│ │         ↓                                                   │ │
│ │ vtenc_h264_hw → WebSocket                                   │ │
│ └─────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

Reference implementation: [spice-record](https://github.com/JonathonReinhart/spice-record)

**Limitation:** "The spice server only supports a single client connection"
- UTM window would go blank while we're connected
- Or run UTM headless (no window)

**Pros:** No UTM modifications, existing libraries
**Cons:** Single client, extra memory copy

### Option C: virtio-vsock (My Original Proposal)

Run screen capture inside the VM, stream raw frames over virtio-vsock:

```
┌─────────────────────────────────────────────────────────────────┐
│ UTM VM (Linux)                                                  │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ helix-ubuntu container                                      │ │
│ │ PipeWire ScreenCast → raw NV12 frames                       │ │
│ │         ↓                                                   │ │
│ │ vsock connection (CID 2 = host, port 5000)                  │ │
│ └─────────────────────────────────────────────────────────────┘ │
└───────────────────────────┼─────────────────────────────────────┘
                            │ virtio-vsock (~1+ GB/s)
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│ helix-encoder (native macOS)                                    │
│ ┌─────────────────────────────────────────────────────────────┐ │
│ │ vsock listener → raw NV12 frames                            │ │
│ │         ↓                                                   │ │
│ │ CVPixelBufferCreateWithBytes() (1 copy)                     │ │
│ │         ↓                                                   │ │
│ │ vtenc_h264_hw → WebSocket                                   │ │
│ └─────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

**Bandwidth:** 1920×1080 @ 60fps NV12 = ~186 MB/s (vsock handles 1+ GB/s)

**Pros:** No UTM modifications, works with any VM tool
**Cons:** Extra capture + copy in VM, slightly higher latency

### Recommendation

| Scenario | Recommended Approach |
|----------|---------------------|
| Quick prototype | Option B (SPICE) or C (vsock) |
| Production quality | Option A (IOSurface tap) |
| UTM won't accept patches | Option C (vsock) |

**Suggested path:**
1. Start with **Option C (vsock)** - works today, no external dependencies
2. Prototype **Option B (SPICE)** - see if latency is acceptable
3. Long-term: Contribute **Option A** to UTM upstream

---

## Open Questions

1. **VM management tool:** UTM, QEMU directly, or Tart?
2. **Frame format:** Raw NV12, MJPEG, or something else?
3. **UTM upstream:** Would they accept an IOSurface export API?
4. **Licensing:** Any issues with VideoToolbox for commercial use?
5. **User experience:** How to make VM setup seamless?

---

## Vision: Fully Local AI Development (M5 Macs, March 2026)

### The Opportunity

With the upcoming **M5 Macs (expected March 2026)** featuring up to **128GB of unified memory**, it becomes viable to run a **fully local AI development environment**:

```
┌─────────────────────────────────────────────────────────────────┐
│ MacBook Pro M5 Max (128GB Unified Memory)                       │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ Local LLM (70B+ parameters)                     ~80GB RAM  │ │
│  │  • Llama 3.3 70B Q4_K_M or similar                        │ │
│  │  • Running via llama.cpp with Metal acceleration          │ │
│  │  • OpenAI-compatible API on localhost:11434               │ │
│  └────────────────────────────────────────────────────────────┘ │
│                           ↑ API calls                           │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ Helix VM (16-32GB RAM)                                     │ │
│  │  • Qwen Code agent swarm (multiple instances)              │ │
│  │  • Points to local LLM instead of cloud API               │ │
│  │  • GPU-accelerated desktops via virtio-gpu                │ │
│  │  • Full Linux dev environment                             │ │
│  └────────────────────────────────────────────────────────────┘ │
│                           ↓ video frames                        │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ Helix Desktop.app                               ~1GB RAM   │ │
│  │  • VideoToolbox encoding                                   │ │
│  │  • Browser UI                                              │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                 │
│  Total: ~100GB RAM used, leaving headroom for other apps       │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### What This Enables

**Fully offline AI development:**
- No cloud API calls needed
- No API costs
- No latency to cloud providers
- Works on airplanes, in secure environments, anywhere
- Complete privacy - your code never leaves your machine

**Agent swarm performance:**
- Multiple Qwen Code agents running simultaneously
- Local LLM responds in seconds, not network-dependent
- Unified memory = LLM weights stay resident, no swapping

**Configuration for local LLM:**
```yaml
# .env in Helix VM
HELIX_LLM_PROVIDER=openai-compatible
HELIX_LLM_BASE_URL=http://host.docker.internal:11434/v1
HELIX_LLM_MODEL=llama-3.3-70b

# Or run Ollama on the host
# ollama run llama3.3:70b
```

### Memory Budget (128GB M5 Max)

| Component | RAM | Notes |
|-----------|-----|-------|
| Local LLM (70B Q4) | ~80 GB | Unified memory, Metal-accelerated |
| Helix VM | 24 GB | API + 2-3 sandbox sessions |
| macOS + Helix Desktop | 8 GB | OS, app, video encoding |
| Headroom | 16 GB | Swap buffer, other apps |
| **Total** | **128 GB** | |

### Memory Configurations

| Mac | RAM | Local LLM | Agent Sessions | Use Case |
|-----|-----|-----------|----------------|----------|
| MacBook Air M5 | 24 GB | 7B (Qwen 2.5 Coder) | 1 | Light local dev |
| MacBook Pro M5 | 48 GB | 32B (DeepSeek Coder) | 2 | Medium projects |
| MacBook Pro M5 Max | 64 GB | 70B Q4 | 2 | Serious local AI |
| MacBook Pro M5 Max | 128 GB | 70B Q6 + 2×7B | 4+ | Full agent swarm |
| **Mac Studio M5 Ultra** | **256 GB** | **405B Q4** or 2×70B | 8+ | Enterprise-grade local |
| **Mac Studio M5 Ultra** | **512 GB** | **405B Q8** + 70B + tools | 16+ | **Full datacenter on desk** |

### Mac Studio M5 Ultra (512GB): The Local AI Datacenter

With 512GB unified memory, a single Mac Studio becomes a **self-contained AI development datacenter**:

```
┌─────────────────────────────────────────────────────────────────┐
│ Mac Studio M5 Ultra (512GB Unified Memory)                      │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ Primary LLM: Llama 4 405B Q8                   ~420 GB     │ │
│  │  • Full quality weights, no aggressive quantization        │ │
│  │  • Frontier-class reasoning on your desk                  │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                 │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ Secondary models:                              ~40 GB      │ │
│  │  • 70B coding specialist (always loaded)                  │ │
│  │  • 7B fast model for quick tasks                          │ │
│  │  • Embedding model for RAG                                │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                 │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ Helix VM                                       ~40 GB      │ │
│  │  • 16+ concurrent agent sessions                          │ │
│  │  • Full Kubernetes-style orchestration                    │ │
│  │  • Multiple GPU-accelerated desktops                      │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                 │
│  Headroom for macOS + apps                        ~12 GB      │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

**What 512GB enables:**
- **405B models at Q8 quality** - Near-lossless weights, GPT-4 class performance locally
- **Multi-model architectures** - Router + specialists + embeddings all in memory
- **Massive agent swarms** - 16+ parallel Zed instances with individual sandboxes
- **No model swapping** - Everything stays resident, instant switching
- **Team server** - One Mac Studio serving a small dev team

**Enterprise use cases:**
- Air-gapped development (defense, finance, healthcare)
- On-premise AI for compliance requirements
- Development server for small teams (5-10 developers)
- CI/CD with local AI code review
- Training environment with immediate feedback

**Cost comparison:**
| Option | Upfront | Monthly | Privacy |
|--------|---------|---------|---------|
| Mac Studio 512GB | ~$12,000 | $0 | Complete |
| Cloud GPU cluster | $0 | $5,000+ | None |
| On-prem NVIDIA DGX | $200,000+ | Power/cooling | Complete |

The Mac Studio becomes the **most cost-effective way to run frontier AI locally**.

### Implementation Notes

1. **LLM hosting:** Use Ollama or llama.cpp on macOS host (not in VM)
   - Direct Metal GPU access for inference
   - No virtualization overhead for LLM

2. **Network bridge:** VM accesses host LLM via `host.docker.internal`
   - Or dedicated virtio-net bridge to host

3. **Model management:** Helix Desktop could include Ollama integration
   - Download/manage models from the menu bar
   - Show model status (loaded, memory usage)

4. **Graceful fallback:** If local LLM is slow or unavailable, option to fall back to cloud API

### Timeline

| Date | Milestone |
|------|-----------|
| March 2026 | M5 Macs announced/released |
| Q2 2026 | Helix Desktop 1.0 for macOS |
| Q2 2026 | Local LLM integration (Ollama) |
| Q3 2026 | Optimized for M5 unified memory |

This positions Helix as the **premiere local AI development platform for Mac** - no cloud required.

---

## Windows Support (Future)

### Key Difference: NVENC Inside VM

Unlike macOS where video encoding must happen on the host (no VideoToolbox access from VM), **Windows with WSL2 supports GPU passthrough**. This means:

```
┌─────────────────────────────────────────────────────────────────┐
│ Windows Host (NVIDIA GPU)                                        │
├─────────────────────────────────────────────────────────────────┤
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ Helix for Windows (Wails - Go + WebView2)                  │ │
│  │  • VM lifecycle management                                  │ │
│  │  • WebSocket proxy to VM                                   │ │
│  │  • No encoding needed on host!                             │ │
│  └────────────────────────────────────────────────────────────┘ │
│         ↓ H.264 stream (already encoded)                        │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │ WSL2 / Hyper-V VM (Linux)                                  │ │
│  │  • CUDA available via GPU passthrough                      │ │
│  │  • nvh264enc / nvenc works directly!                       │ │
│  │  • Same architecture as Linux production                   │ │
│  │  • H.264 encoding inside VM, stream out via WebSocket      │ │
│  └────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

**Windows advantages:**
- **Same encoding path as Linux** - Use existing nvh264enc/NVENC code unchanged
- **Simpler architecture** - No host-side encoding, just proxy the WebSocket stream
- **Better GPU support** - Full CUDA + NVENC available in VM via WSL2 GPU passthrough

**Implementation notes:**
- Wails supports Windows (uses WebView2 instead of WKWebView)
- WSL2 GPU support: `wsl --install` + NVIDIA drivers automatically enable CUDA in WSL
- Hyper-V with GPU-PV also works for full VM isolation
- QSV (Intel Quick Sync) also available for Intel integrated GPUs

**Timeline:** After macOS release (Q3 2026)

---

## References

### VM & GPU Virtualization
- [UTM Documentation - virtio-gpu-gl](https://docs.getutm.app/updates/v4.1/)
- [UTM Graphics Architecture](https://github.com/utmapp/UTM/blob/main/Documentation/Graphics.md)
- [QEMU virtio-gpu documentation](https://qemu.readthedocs.io/en/v8.2.10/system/devices/virtio-gpu.html)
- [virtio-gpu and QEMU graphics (Kraxel)](https://www.kraxel.org/blog/2021/05/virtio-gpu-qemu-graphics-update/)
- [OrbStack GPU acceleration discussion](https://github.com/orbstack/orbstack/issues/1818)
- [Docker GPU on macOS](https://forums.docker.com/t/apple-silicon-gpu-support/139968)

### Video Encoding
- [GStreamer vtenc_h264_hw](https://gstreamer.freedesktop.org/documentation/applemedia/vtenc_h264_hw.html)
- [GStreamer applemedia plugin](https://gstreamer.freedesktop.org/documentation/applemedia/index.html)
- [Apple VideoToolbox](https://developer.apple.com/documentation/videotoolbox)
- [WWDC 2021: Low-latency video encoding with VideoToolbox](https://developer.apple.com/videos/play/wwdc2021/10158/)
- [VTCompressionSession](https://developer.apple.com/documentation/VideoToolbox/VTCompressionSession)
- [VideoToolbox best practices (objc.io)](https://www.objc.io/issues/23-video/videotoolbox/)

### Frame Sharing
- [IOSurface documentation](https://developer.apple.com/documentation/iosurface)
- [CVPixelBufferCreateWithIOSurface](https://developer.apple.com/documentation/corevideo/1456968-cvpixelbuffercreatewithiosurface)
- [IOSurface Mach port sharing example](https://fdiv.net/2011/01/27/example-iosurfacecreatemachport-and-iosurfacelookupfrommachport)
- [virtio-vsock documentation](https://man7.org/linux/man-pages/man7/vsock.7.html)
- [spice-record (frame extraction reference)](https://github.com/JonathonReinhart/spice-record)

### Mac App Development
- [Wails - Go desktop apps](https://wails.io/)
- [go-gst - GStreamer Go bindings](https://github.com/go-gst/go-gst)
- [Tauri vs Electron comparison](https://www.gethopp.app/blog/tauri-vs-electron)

### GPU Containers on macOS
- [GPU-accelerated containers for M-series Macs](https://medium.com/@andreask_75652/gpu-accelerated-containers-for-m1-m2-m3-macs-237556e5fe0b)
- [Enabling containers GPU access on macOS](https://sinrega.org/2024-03-06-enabling-containers-gpu-macos/)
- [Podman GPU acceleration via libkrun](https://github.com/ggml-org/llama.cpp/discussions/8042)
