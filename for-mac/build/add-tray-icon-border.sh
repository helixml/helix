#!/usr/bin/env bash
# Add an anti-aliased white border around the tray icon.
# Requires ImageMagick (brew install imagemagick).
#
# Usage: ./add-tray-icon-border.sh [input.png] [output.png]
# Defaults to reading/writing tray-icon.png in the same directory.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
INPUT="${1:-$SCRIPT_DIR/tray-icon.png}"
OUTPUT="${2:-$INPUT}"

magick "$INPUT" \
  \( +clone -alpha extract -morphology Dilate Disk:2 \
     -background white -alpha shape \) \
  -compose DstOver -composite \
  "$OUTPUT"

echo "Saved $OUTPUT with anti-aliased white border"
