#!/bin/bash
set -e

echo "=== Building UTM v5.0.1 for macOS ARM ==="
echo ""

# Configuration
UTM_VERSION="v5.0.1"
UTM_DIR="${UTM_DIR:-$HOME/pm/helix/UTM}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Check if UTM directory exists
if [ ! -d "$UTM_DIR" ]; then
    echo "ERROR: UTM directory not found at: $UTM_DIR"
    echo "Clone it first: git clone https://github.com/utmapp/UTM.git $UTM_DIR"
    exit 1
fi

cd "$UTM_DIR"

# Ensure we're on the correct version
echo "Checking out UTM $UTM_VERSION..."
git fetch --tags
git checkout "$UTM_VERSION"

# Check if build already exists
if [ -d "build-macOS-arm64/UTM.app" ]; then
    read -p "Existing build found. Rebuild? (y/N) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Using existing build"
        exit 0
    fi
fi

# Follow UTM build instructions
echo ""
echo "Building UTM (this will take a while)..."
echo "See https://github.com/utmapp/UTM/blob/main/Documentation/Building.md"
echo ""

# Ensure Xcode is installed
if ! command -v xcodebuild &> /dev/null; then
    echo "ERROR: Xcode is required. Install from App Store."
    exit 1
fi

# Build dependencies (if not already built)
if [ ! -d "sysroot-macOS-arm64" ]; then
    echo "Building dependencies..."
    ./scripts/build_dependencies.sh -a arm64
fi

# Build UTM
echo "Building UTM.app..."
xcodebuild -project UTM.xcodeproj \
    -scheme UTM \
    -configuration Release \
    -derivedDataPath build \
    -arch arm64 \
    CODE_SIGN_IDENTITY="-" \
    CODE_SIGNING_ALLOWED=NO

# Copy MoltenVK dylib to correct location (fix for custom builds)
echo ""
echo "Fixing MoltenVK packaging..."
MOLTENVK_DYLIB="sysroot-macOS-arm64/lib/libMoltenVK.dylib"
UTM_APP="build-macOS-arm64/UTM.app"
VULKAN_DIR="$UTM_APP/Contents/Resources/vulkan"

if [ -f "$MOLTENVK_DYLIB" ] && [ -d "$VULKAN_DIR" ]; then
    cp "$MOLTENVK_DYLIB" "$VULKAN_DIR/"
    echo "✓ Copied libMoltenVK.dylib to $VULKAN_DIR/"
else
    echo "WARNING: Could not copy MoltenVK dylib (Venus/Vulkan may not work)"
fi

# Ad-hoc code sign
echo ""
echo "Code signing..."
codesign --force --deep --sign - "$UTM_APP"

echo ""
echo "=== Build complete! ==="
echo ""
echo "UTM.app location: $UTM_APP"
echo "To run: open $UTM_APP"
echo ""
echo "Note: This build uses ad-hoc signing and cannot use vmnet networking."
echo "Use 'Emulated' network mode in VM settings instead."
