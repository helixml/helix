# TUI E2E test image
# Pre-built binaries: Zed, tui-test-server, helix (TUI)
#
# Usage:
#   ./run_docker_e2e.sh

FROM ubuntu:25.10

ENV DEBIAN_FRONTEND=noninteractive

# Runtime dependencies for headless Zed + test tooling
RUN apt-get update && apt-get install -y --no-install-recommends \
    # Zed runtime
    libxkbcommon0 libxkbcommon-x11-0 libwayland-client0 libvulkan1 \
    libx11-xcb1 libxcb-shape0 libxcb-xfixes0 \
    libxcb-randr0 libxcb-render0 \
    libxcb-shm0 libxcb-glx0 libxcb-xkb1 \
    libxcb1 libx11-6 \
    libfontconfig1 libfreetype6 \
    libssl3t64 libsqlite3-0 libzstd1 ca-certificates \
    libasound2t64 \
    libatk1.0-0t64 libgtk-3-0t64 \
    # Headless display
    xvfb x11-utils \
    # Software Vulkan
    mesa-vulkan-drivers \
    # D-Bus
    dbus \
    # Test tools
    curl python3 \
    && rm -rf /var/lib/apt/lists/*

# Copy pre-built binaries
COPY zed-binary /usr/local/bin/zed
COPY tui-test-server/tui-test-server /usr/local/bin/tui-test-server
COPY helix-binary /usr/local/bin/helix
COPY tui-e2e-test /usr/local/bin/tui-e2e-test
RUN chmod +x /usr/local/bin/zed /usr/local/bin/tui-test-server /usr/local/bin/helix /usr/local/bin/tui-e2e-test

# Copy test script
COPY run_e2e.sh /test/run_e2e.sh
RUN chmod +x /test/run_e2e.sh

# Headless display
ENV DISPLAY=:99
ENV VK_ICD_FILENAMES=/usr/share/vulkan/icd.d/lvp_icd.json
ENV LIBGL_ALWAYS_SOFTWARE=1
ENV ZED_ALLOW_EMULATED_GPU=1
ENV ZED_ALLOW_ROOT=true

WORKDIR /test
RUN mkdir -p /test/project && echo "# TUI E2E Test" > /test/project/README.md

ENTRYPOINT ["/test/run_e2e.sh"]
