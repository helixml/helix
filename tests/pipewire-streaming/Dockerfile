# Test container for PipeWire streaming Python tests
#
# Build from parent of helix directory (where both helix/ and wolf/ exist):
#   cd /path/to/pm && docker build -t pipewire-streaming-tests -f helix/tests/pipewire-streaming/Dockerfile .
# Run:   docker run --rm pipewire-streaming-tests
#
# NOTE: Rust tests run during Wolf builds (./stack build-wolf).
# The GStreamer plugin tests require CUDA libraries from the GOW base image.
# See wolf/docker/wolf.Dockerfile line 72: `cargo test --lib`

FROM ubuntu:24.04 AS base

ENV DEBIAN_FRONTEND=noninteractive

# Install dependencies for Python tests
RUN apt-get update && apt-get install -y \
    python3 \
    python3-pip \
    python3-venv \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build/tests

# Copy Python test files
COPY helix/wolf/ubuntu-config/remotedesktop-session.py /build/ubuntu-config/remotedesktop-session.py
COPY helix/tests/pipewire-streaming/ /build/tests/

# Set up Python venv and install dependencies
RUN python3 -m venv /opt/venv
ENV PATH="/opt/venv/bin:${PATH}"
RUN pip install --no-cache-dir -r requirements.txt

# Run Python tests during build
RUN python -m pytest test_remotedesktop_session.py -v --tb=short

# Default command for interactive use
CMD ["python", "-m", "pytest", "-v"]
