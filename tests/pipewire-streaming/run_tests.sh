#!/bin/bash
# Run PipeWire streaming tests
#
# Usage:
#   ./run_tests.sh          # Run Python tests in Docker (fast)
#   ./run_tests.sh --local  # Run all tests locally (requires dependencies)
#   ./run_tests.sh --rust   # Run Rust tests via Wolf build
#   ./run_tests.sh --python # Run only Python tests locally
#
# Test Architecture:
#   - Python tests: Run in lightweight Docker container (ubuntu:24.04)
#   - Rust tests: Run during Wolf build (requires GStreamer CUDA from GOW image)
#
# Note: helix and wolf are sibling directories in development.
# Docker builds from the parent directory (pm/) to access both.

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
HELIX_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
# Wolf is a sibling directory to helix
WOLF_ROOT="$(cd "$HELIX_ROOT/../wolf" && pwd)"
# Parent dir for Docker context (contains both helix/ and wolf/)
PARENT_DIR="$(cd "$HELIX_ROOT/.." && pwd)"

run_docker_tests() {
    echo "Building and running tests in Docker..."
    echo "Build context: $PARENT_DIR"
    echo "Dockerfile: $HELIX_ROOT/tests/pipewire-streaming/Dockerfile"
    cd "$PARENT_DIR"
    docker build -t pipewire-streaming-tests -f helix/tests/pipewire-streaming/Dockerfile .
    docker run --rm pipewire-streaming-tests
}

run_rust_tests() {
    echo "Running Rust unit tests via Wolf build..."
    echo "Rust tests are integrated into the Wolf Dockerfile."
    echo "Running: ./stack build-wolf"
    cd "$HELIX_ROOT"
    ./stack build-wolf
    echo ""
    echo "Rust tests passed as part of Wolf build (see 'cargo test' output above)."
}

run_python_tests() {
    echo "Running Python unit tests..."
    cd "$SCRIPT_DIR"

    # Create virtual environment if it doesn't exist
    if [ ! -d ".venv" ]; then
        python3 -m venv .venv
        .venv/bin/pip install -r requirements.txt
    fi

    .venv/bin/python -m pytest test_remotedesktop_session.py -v
}

case "${1:-}" in
    --local)
        run_rust_tests
        run_python_tests
        ;;
    --rust)
        run_rust_tests
        ;;
    --python)
        run_python_tests
        ;;
    *)
        run_docker_tests
        ;;
esac

echo "Tests completed successfully!"
