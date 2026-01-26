#!/bin/bash
# Generate transparent cursor theme with unique hotspots for fingerprinting
#
# Each cursor type has a unique (hotspot_x, hotspot_y) pair so the GNOME Shell
# extension can identify the cursor shape without needing pixel data.
#
# Cursor size: 48x48 (larger size for better hotspot spacing at HiDPI scales)
# All cursors are 100% transparent (invisible) so composited cursor is hidden
#
# IMPORTANT: Hotspots must be multiples of 6 to survive Mutter's rounding at
# 200% and 300% display scaling. See meta-cursor-sprite-xcursor.c:385-390.
# At scale N, hotspots are rounded: roundf(hotspot/N)*N
# Multiples of 6 survive both scale 2 and scale 3.

set -e

# Support both local development and Docker build contexts
if [ -d "/tmp" ] && [ -w "/tmp" ]; then
    THEME_DIR="/tmp/Helix-Invisible"
else
    THEME_DIR="$(dirname "$0")/Helix-Invisible"
fi
CURSORS_DIR="$THEME_DIR/cursors"

mkdir -p "$CURSORS_DIR"

# Create transparent 48x48 PNG
create_transparent_png() {
    local output="$1"
    # 48x48 fully transparent RGBA PNG
    convert -size 48x48 xc:transparent "$output"
}

# Generate cursor from PNG using xcursorgen
# Format: size hotspot_x hotspot_y filename frame_delay(optional)
generate_cursor() {
    local name="$1"
    local hotspot_x="$2"
    local hotspot_y="$3"

    local config_file=$(mktemp)
    echo "48 $hotspot_x $hotspot_y transparent.png" > "$config_file"

    xcursorgen "$config_file" "$CURSORS_DIR/$name"
    rm "$config_file"
}

# Create symlink for cursor alias
link_cursor() {
    local target="$1"
    local link="$2"
    ln -sf "$target" "$CURSORS_DIR/$link"
}

echo "Creating transparent cursor theme..."

# Create the transparent PNG source
TMPDIR=$(mktemp -d)
create_transparent_png "$TMPDIR/transparent.png"
cd "$TMPDIR"

# ============================================================================
# CURSOR DEFINITIONS - Unique hotspots for fingerprinting
# Format: cursor_name hotspot_x hotspot_y
#
# The fingerprint map in extension.js must match these EXACTLY
#
# ALL hotspots are multiples of 6 to survive Mutter's rounding at 2x and 3x scale
# Available values in 48x48: 0, 6, 12, 18, 24, 30, 36, 42
# ============================================================================

# Arrow/pointer cursors
generate_cursor "default"         0  0
generate_cursor "left_ptr"        0  0
generate_cursor "arrow"           0  0
generate_cursor "top_left_arrow"  0  0

# Link/hand pointer
generate_cursor "pointer"         6  0
generate_cursor "hand1"           6  0
generate_cursor "hand2"           6  0
generate_cursor "pointing_hand"   6  0

# Context menu (arrow with menu)
generate_cursor "context-menu"    12 0

# Help cursor (arrow with ?)
generate_cursor "help"            18 0
generate_cursor "question_arrow"  18 0
generate_cursor "whats_this"      18 0

# Progress cursors (arrow with spinner)
generate_cursor "progress"        24 0
generate_cursor "left_ptr_watch"  24 0

# Wait/busy cursor (spinner only)
generate_cursor "wait"            0  6
generate_cursor "watch"           0  6

# Text cursor / I-beam
generate_cursor "text"            0  12
generate_cursor "xterm"           0  12
generate_cursor "ibeam"           0  12
generate_cursor "vertical-text"   0  12

# Crosshair
generate_cursor "crosshair"       6  6
generate_cursor "cross"           6  6
generate_cursor "tcross"          6  6

# Move cursor (4-way arrows)
generate_cursor "move"            12 6
generate_cursor "fleur"           12 6
generate_cursor "all-scroll"      12 6
generate_cursor "size_all"        12 6

# Grab cursors
generate_cursor "grab"            18 6
generate_cursor "openhand"        18 6
generate_cursor "grabbing"        24 6
generate_cursor "closedhand"      24 6
generate_cursor "dnd-move"        24 6
generate_cursor "dnd-none"        24 6

# Not-allowed / forbidden
generate_cursor "not-allowed"     30 6
generate_cursor "no-drop"         30 6
generate_cursor "circle"          30 6
generate_cursor "forbidden"       30 6

# Copy/alias (drag cursors)
generate_cursor "copy"            30 0
generate_cursor "dnd-copy"        30 0
generate_cursor "alias"           36 0
generate_cursor "dnd-link"        36 0

# Cell selection (spreadsheet)
generate_cursor "cell"            36 6
generate_cursor "plus"            36 6

# Zoom cursors
generate_cursor "zoom-in"         42 0
generate_cursor "zoom_in"         42 0
generate_cursor "zoom-out"        42 6
generate_cursor "zoom_out"        42 6

# ============================================================================
# RESIZE CURSORS - Each direction has unique hotspot (multiples of 6)
# ============================================================================

# North-South resize (vertical)
generate_cursor "ns-resize"       6  12
generate_cursor "ns_resize"       6  12
generate_cursor "n-resize"        6  12
generate_cursor "s-resize"        6  12
generate_cursor "size_ver"        6  12
generate_cursor "sb_v_double_arrow" 6 12
generate_cursor "v_double_arrow"  6  12
generate_cursor "top_side"        6  12
generate_cursor "bottom_side"     6  12
generate_cursor "row-resize"      6  12
generate_cursor "row_resize"      6  12

# East-West resize (horizontal)
generate_cursor "ew-resize"       12 12
generate_cursor "ew_resize"       12 12
generate_cursor "e-resize"        12 12
generate_cursor "w-resize"        12 12
generate_cursor "size_hor"        12 12
generate_cursor "sb_h_double_arrow" 12 12
generate_cursor "h_double_arrow"  12 12
generate_cursor "left_side"       12 12
generate_cursor "right_side"      12 12
generate_cursor "col-resize"      12 12
generate_cursor "col_resize"      12 12

# NW-SE resize (diagonal backslash) - top_left_corner, bottom_right_corner
generate_cursor "nwse-resize"     18 12
generate_cursor "nwse_resize"     18 12
generate_cursor "nw-resize"       18 12
generate_cursor "se-resize"       18 12
generate_cursor "size_fdiag"      18 12
generate_cursor "bd_double_arrow" 18 12
generate_cursor "top_left_corner" 18 12
generate_cursor "bottom_right_corner" 18 12

# NE-SW resize (diagonal slash) - top_right_corner, bottom_left_corner
generate_cursor "nesw-resize"     24 12
generate_cursor "nesw_resize"     24 12
generate_cursor "ne-resize"       24 12
generate_cursor "sw-resize"       24 12
generate_cursor "size_bdiag"      24 12
generate_cursor "fd_double_arrow" 24 12
generate_cursor "top_right_corner" 24 12
generate_cursor "bottom_left_corner" 24 12

# ============================================================================
# Additional cursor aliases via symlinks
# ============================================================================

link_cursor "default" "left_ptr"
link_cursor "pointer" "hand"
link_cursor "text" "xterm"
link_cursor "wait" "watch"
link_cursor "move" "fleur"
link_cursor "not-allowed" "crossed_circle"
link_cursor "ns-resize" "split_v"
link_cursor "ew-resize" "split_h"

# Cleanup
cd -
rm -rf "$TMPDIR"

# Create index.theme
# NOTE: No Inherits= line - we don't want to fall back to visible cursors
# If a cursor type isn't defined, X/Wayland will use a blank cursor
cat > "$THEME_DIR/index.theme" << 'EOF'
[Icon Theme]
Name=Helix-Invisible
Comment=Invisible cursor theme for Helix client-side cursor rendering
EOF

echo "Cursor theme created at: $THEME_DIR"
echo "Cursors generated: $(ls -1 "$CURSORS_DIR" | wc -l)"
echo ""
echo "To install: copy $THEME_DIR to /usr/share/icons/ or ~/.icons/"
echo "To activate: gsettings set org.gnome.desktop.interface cursor-theme 'Helix-Invisible'"
