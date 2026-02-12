# Building Helix Desktop for Windows

## Prerequisites

1. **Windows 10 21H2+ or Windows 11** with WSL2 support
2. **Go 1.24+**: https://go.dev/dl/
3. **Node.js 20+**: https://nodejs.org/
4. **Wails CLI**: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
5. **WebView2**: Comes pre-installed on Windows 10/11. If not: https://developer.microsoft.com/en-us/microsoft-edge/webview2/
6. **GCC (MinGW-w64)**: Required for CGo (WebView2 bindings)
   - Install via MSYS2: https://www.msys2.org/
   - Or: `winget install --id GnuWin32.Gcc`
   - Or: `choco install mingw`

## Build Steps

```powershell
# 1. Clone the repo
git clone https://github.com/helixml/helix.git
cd helix
git checkout feature/windows-desktop-port

# 2. Install frontend dependencies
cd for-mac/frontend
npm install
cd ..

# 3. Build the Wails app
wails build

# Output: build/bin/Helix Desktop.exe
```

## WSL2 Setup (Required for Running)

The Helix Desktop app uses WSL2 to run its Linux-based services. Users need:

1. **WSL2 enabled**: `wsl --install` (from elevated PowerShell)
2. **Helix rootfs tarball**: Downloaded by the app on first launch

The app automatically:
- Checks for WSL2 availability
- Imports the Helix distro from the downloaded rootfs
- Starts Docker and the Helix stack inside WSL2

## Creating the WSL2 Rootfs Tarball

See `scripts/build-wsl-rootfs.sh` for creating the rootfs tarball that gets
downloaded on first launch. This is an Ubuntu 24.04 base with:
- Docker CE pre-installed
- User `ubuntu` configured as default
- systemd enabled for Docker service management

## GPU Acceleration

WSL2 provides hardware GPU acceleration automatically:
- **OpenGL/Vulkan**: Via GPU-PV (GPU paravirtualization)
- **Hardware H.264 encoding**: Via VAAPI over D3D12 (`vaapih264enc`)
- Works with NVIDIA, AMD, and Intel GPUs

No additional GPU driver installation is needed — WSL2 uses the host GPU driver.

## Architecture

```
┌─────────────────────────────────────────┐
│  Helix Desktop.exe (Wails + WebView2)   │
│  ├── Frontend: React + TypeScript       │
│  └── Backend: Go (vm_windows.go)        │
│       └── runInVM: wsl.exe -d Helix     │
├─────────────────────────────────────────┤
│  WSL2 (Helix distro)                    │
│  ├── Docker                             │
│  │   ├── Helix API                      │
│  │   ├── Helix Sandbox                  │
│  │   └── helix-ubuntu containers        │
│  │       ├── GNOME + Zed IDE            │
│  │       ├── PipeWire → GStreamer       │
│  │       └── vaapih264enc (D3D12)       │
│  └── overlay2 storage (no ZFS)          │
├─────────────────────────────────────────┤
│  Windows Host                           │
│  ├── GPU Driver (NVIDIA/AMD/Intel)      │
│  └── GPU-PV → WSL2 GPU access          │
└─────────────────────────────────────────┘
```
