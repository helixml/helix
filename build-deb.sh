#!/bin/bash
set -euo pipefail

echo "🔨 Building HyprMoon Debian Package"

# Create output directory
OUTPUT_DIR="$(pwd)/deb-output"
mkdir -p "$OUTPUT_DIR"

echo "📁 Output directory: $OUTPUT_DIR"

# Build and run
docker build -f Dockerfile.deb-builder -t hyprmoon-deb-builder . && \
docker run --rm -v "$OUTPUT_DIR:/output-bind" hyprmoon-deb-builder

# Show results
if [ -d "$OUTPUT_DIR" ] && [ -n "$(ls -A "$OUTPUT_DIR" 2>/dev/null)" ]; then
    echo "📦 Built packages:"
    ls -la "$OUTPUT_DIR"
else
    echo "❌ No packages generated"
    exit 1
fi
