# Open Source Notices

Helix for Mac bundles the following open-source software.

## QEMU (GPL v2)

QEMU is a generic machine emulator and virtualizer, used here for ARM64
virtual machine management with Apple Hypervisor.framework acceleration.

- License: GNU General Public License v2
- Source code: https://github.com/helixml/qemu-utm (branch: utm-edition-venus-helix)
- Upstream: https://www.qemu.org/
- Copyright: The QEMU contributors

As required by GPLv2, the complete corresponding source code for the
bundled QEMU binary is available at the repository linked above.

## virglrenderer (MIT)

GPU 3D rendering translation layer for virtual machines.

- License: MIT
- Source: https://gitlab.freedesktop.org/virgl/virglrenderer

## SPICE (LGPL v2.1)

Remote display protocol libraries.

- License: GNU Lesser General Public License v2.1
- Source: https://www.spice-space.org/

## GLib / GIO / GObject / GModule / GThread (LGPL v2.1)

General-purpose utility libraries.

- License: GNU Lesser General Public License v2.1
- Source: https://gitlab.gnome.org/GNOME/glib

## Mesa Vulkan Driver — KosmicKrisp (MIT)

Mesa-based Vulkan driver for virtio-gpu Venus protocol.

- License: MIT
- Source: https://gitlab.freedesktop.org/mesa/mesa

## pixman (MIT)

Low-level pixel manipulation library.

- License: MIT
- Source: https://pixman.org/

## libepoxy (MIT)

OpenGL function pointer management library.

- License: MIT
- Source: https://github.com/anholt/libepoxy

## libjpeg-turbo (BSD-3-Clause)

JPEG image codec.

- License: BSD-3-Clause / IJG
- Source: https://libjpeg-turbo.org/

## zstd (BSD-3-Clause)

Fast lossless compression algorithm.

- License: BSD-3-Clause
- Source: https://github.com/facebook/zstd

## libslirp (BSD-3-Clause)

User-mode networking for QEMU.

- License: BSD-3-Clause
- Source: https://gitlab.freedesktop.org/slirp/libslirp

## libusb (LGPL v2.1)

USB device access library.

- License: GNU Lesser General Public License v2.1
- Source: https://libusb.info/

## usbredir (LGPL v2.1)

USB redirection protocol.

- License: GNU Lesser General Public License v2.1
- Source: https://www.spice-space.org/usbredir.html

## GStreamer (LGPL v2.1)

Multimedia framework (gstreamer, gstbase, gstapp).

- License: GNU Lesser General Public License v2.1
- Source: https://gstreamer.freedesktop.org/

## Opus (BSD-3-Clause)

Audio codec used by SPICE.

- License: BSD-3-Clause
- Source: https://opus-codec.org/

## OpenSSL (Apache 2.0)

TLS/SSL library (libssl, libcrypto).

- License: Apache License 2.0
- Source: https://www.openssl.org/

## libffi (MIT)

Foreign function interface library.

- License: MIT
- Source: https://sourceware.org/libffi/

## GNU libiconv (LGPL v2.1)

Character encoding conversion.

- License: GNU Lesser General Public License v2.1
- Source: https://www.gnu.org/software/libiconv/

## GNU gettext / libintl (LGPL v2.1)

Internationalization library.

- License: GNU Lesser General Public License v2.1
- Source: https://www.gnu.org/software/gettext/

## libgpg-error / libgcrypt (LGPL v2.1)

Cryptographic library used by GStreamer.

- License: GNU Lesser General Public License v2.1
- Source: https://gnupg.org/software/libgcrypt/

## Vulkan-Loader (Apache 2.0)

Vulkan API loader library.

- License: Apache License 2.0
- Source: https://github.com/KhronosGroup/Vulkan-Loader

## EDK2 / UEFI Firmware (BSD-2-Clause)

UEFI firmware for ARM64 virtual machines.

- License: BSD-2-Clause
- Source: https://github.com/tianocore/edk2

---

All LGPL libraries are dynamically linked as macOS Framework bundles,
satisfying the LGPL requirement that users can replace them.

For the complete text of each license, see the respective project's
source repository.
