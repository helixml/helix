# gnome-wolf-bridge

Bridge GNOME Shell's PipeWire screen-cast to Wolf's Wayland compositor.

## Overview

This bridge enables running GNOME Shell in headless mode and displaying it
inside Wolf's streaming infrastructure. It uses PipeWire for frame transfer,
supporting zero-copy GPU frames via DMA-BUF when available.

## Architecture

```
Wolf (wayland-1 compositor)
    ↑
    │ wl_surface + zwp_linux_dmabuf_v1
    │
gnome-wolf-bridge (Wayland client)
    ↑
    │ PipeWire stream (DMA-BUF or SHM)
    │
GNOME Shell (headless mode)
```

## Building

### Dependencies

```bash
# Ubuntu 24.04
sudo apt install \
    meson ninja-build \
    libwayland-dev wayland-protocols \
    libpipewire-0.3-dev \
    libgio-2.0-dev \
    libdrm-dev \
    libei-dev  # Optional, for input forwarding
```

### Build

```bash
meson setup build
meson compile -C build
```

### Install

```bash
sudo meson install -C build
```

## Usage

### 1. Start GNOME Shell in headless mode

```bash
gnome-shell --headless --wayland &
```

### 2. Wait for GNOME to be ready

```bash
gdbus wait --session --timeout 30 org.gnome.Mutter.ScreenCast
```

### 3. Start the bridge

```bash
WAYLAND_DISPLAY=wayland-1 gnome-wolf-bridge
```

### Options

```
-d, --display DISPLAY  Wayland display (default: wayland-1)
-w, --width WIDTH      Display width (default: 1920)
-h, --height HEIGHT    Display height (default: 1080)
```

## How It Works

1. **Connects to Wolf**: Creates a fullscreen Wayland surface on Wolf's display
2. **D-Bus Session**: Calls `org.gnome.Mutter.ScreenCast.CreateSession`
3. **PipeWire Stream**: Subscribes to the screen-cast PipeWire stream
4. **Frame Transfer**: For each frame:
   - DMA-BUF: Imports GPU buffer directly via `zwp_linux_dmabuf_v1`
   - SHM fallback: Copies frame data to `wl_shm` buffer
5. **Submits to Wolf**: Attaches buffer to surface, commits

## Performance

- **DMA-BUF mode**: Zero-copy GPU transfer, minimal CPU overhead
- **SHM mode**: One CPU copy per frame (fallback)
- **Latency**: ~1 frame (PipeWire queue)

## Troubleshooting

### "No wl_compositor found"

Wolf's Wayland compositor isn't running. Check that Wolf is started:
```bash
ls -la /tmp/sockets/wayland-1
```

### "CreateSession failed"

GNOME Shell isn't running or screen-cast service unavailable:
```bash
gdbus introspect --session --dest org.gnome.Mutter.ScreenCast \
    --object-path /org/gnome/Mutter/ScreenCast
```

### "Failed to connect to PipeWire node"

PipeWire server not running or node ID invalid:
```bash
pw-cli info
```

## License

MIT - Same as Helix project
