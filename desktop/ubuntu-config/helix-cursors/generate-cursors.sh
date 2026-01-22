#!/bin/bash
# Generate transparent cursor theme with unique hotspots for fingerprinting
#
# Each cursor type has a unique (hotspot_x, hotspot_y) pair so the GNOME Shell
# extension can identify the cursor shape without needing pixel data.
#
# Cursor size: 24x24 (standard size for X11 cursors)
# All cursors are 100% transparent (invisible) so composited cursor is hidden

set -e

# Support both local development and Docker build contexts
if [ -d "/tmp" ] && [ -w "/tmp" ]; then
    THEME_DIR="/tmp/Helix-Invisible"
else
    THEME_DIR="$(dirname "$0")/Helix-Invisible"
fi
CURSORS_DIR="$THEME_DIR/cursors"

mkdir -p "$CURSORS_DIR"

# Create transparent 24x24 PNG
create_transparent_png() {
    local output="$1"
    # 24x24 fully transparent RGBA PNG
    convert -size 24x24 xc:transparent "$output"
}

# Generate cursor from PNG using xcursorgen
# Format: size hotspot_x hotspot_y filename frame_delay(optional)
generate_cursor() {
    local name="$1"
    local hotspot_x="$2"
    local hotspot_y="$3"

    local config_file=$(mktemp)
    echo "24 $hotspot_x $hotspot_y transparent.png" > "$config_file"

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
# ============================================================================

# Arrow/pointer cursors (hotspot at top-left area)
generate_cursor "default"         0  0
generate_cursor "left_ptr"        0  0
generate_cursor "arrow"           0  0
generate_cursor "top_left_arrow"  0  0

# Link/hand pointer (hotspot at fingertip - different from default)
generate_cursor "pointer"         6  0
generate_cursor "hand1"           6  0
generate_cursor "hand2"           6  0
generate_cursor "pointing_hand"   6  0

# Context menu (arrow with menu)
generate_cursor "context-menu"    0  1

# Help cursor (arrow with ?)
generate_cursor "help"            0  2
generate_cursor "question_arrow"  0  2
generate_cursor "whats_this"      0  2

# Progress cursors (arrow with spinner)
generate_cursor "progress"        0  3
generate_cursor "left_ptr_watch"  0  3

# Wait/busy cursor (spinner only)
generate_cursor "wait"            12 0
generate_cursor "watch"           12 0

# Text cursor / I-beam (at 12,14 to avoid collision with resize cursor slots)
generate_cursor "text"            12 14
generate_cursor "xterm"           12 14
generate_cursor "ibeam"           12 14
generate_cursor "vertical-text"   12 14

# Crosshair (centered)
generate_cursor "crosshair"       12 1
generate_cursor "cross"           12 1
generate_cursor "tcross"          12 1

# Move cursor (4-way arrows)
generate_cursor "move"            12 2
generate_cursor "fleur"           12 2
generate_cursor "all-scroll"      12 2
generate_cursor "size_all"        12 2

# Grab cursors
generate_cursor "grab"            12 3
generate_cursor "openhand"        12 3
generate_cursor "grabbing"        12 4
generate_cursor "closedhand"      12 4
generate_cursor "dnd-move"        12 4
generate_cursor "dnd-none"        12 4

# Not-allowed / forbidden
generate_cursor "not-allowed"     12 5
generate_cursor "no-drop"         12 5
generate_cursor "circle"          12 5
generate_cursor "forbidden"       12 5

# Copy/alias (drag cursors)
generate_cursor "copy"            0  4
generate_cursor "dnd-copy"        0  4
generate_cursor "alias"           0  5
generate_cursor "dnd-link"        0  5

# Cell selection (spreadsheet)
generate_cursor "cell"            12 6
generate_cursor "plus"            12 6

# Zoom cursors
generate_cursor "zoom-in"         12 7
generate_cursor "zoom_in"         12 7
generate_cursor "zoom-out"        12 8
generate_cursor "zoom_out"        12 8

# ============================================================================
# RESIZE CURSORS - Each direction has unique hotspot
# ============================================================================

# North-South resize (vertical)
generate_cursor "ns-resize"       12 9
generate_cursor "ns_resize"       12 9
generate_cursor "n-resize"        12 9
generate_cursor "s-resize"        12 9
generate_cursor "size_ver"        12 9
generate_cursor "sb_v_double_arrow" 12 9
generate_cursor "v_double_arrow"  12 9
generate_cursor "top_side"        12 9
generate_cursor "bottom_side"     12 9
generate_cursor "row-resize"      12 9
generate_cursor "row_resize"      12 9

# East-West resize (horizontal)
generate_cursor "ew-resize"       12 10
generate_cursor "ew_resize"       12 10
generate_cursor "e-resize"        12 10
generate_cursor "w-resize"        12 10
generate_cursor "size_hor"        12 10
generate_cursor "sb_h_double_arrow" 12 10
generate_cursor "h_double_arrow"  12 10
generate_cursor "left_side"       12 10
generate_cursor "right_side"      12 10
generate_cursor "col-resize"      12 10
generate_cursor "col_resize"      12 10

# NW-SE resize (diagonal backslash)
generate_cursor "nwse-resize"     12 11
generate_cursor "nwse_resize"     12 11
generate_cursor "nw-resize"       12 11
generate_cursor "se-resize"       12 11
generate_cursor "size_fdiag"      12 11
generate_cursor "bd_double_arrow" 12 11
generate_cursor "top_left_corner" 12 11
generate_cursor "bottom_right_corner" 12 11

# NE-SW resize (diagonal slash) - at 12,12 (moved from 12,13 to keep resize cursors consecutive)
generate_cursor "nesw-resize"     12 12
generate_cursor "nesw_resize"     12 12
generate_cursor "ne-resize"       12 12
generate_cursor "sw-resize"       12 12
generate_cursor "size_bdiag"      12 12
generate_cursor "fd_double_arrow" 12 12
generate_cursor "top_right_corner" 12 12
generate_cursor "bottom_left_corner" 12 12

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
cat > "$THEME_DIR/index.theme" << 'EOF'
[Icon Theme]
Name=Helix-Invisible
Comment=Invisible cursor theme for Helix client-side cursor rendering
Inherits=Adwaita
EOF

echo "Cursor theme created at: $THEME_DIR"
echo "Cursors generated: $(ls -1 "$CURSORS_DIR" | wc -l)"
echo ""
echo "To install: copy $THEME_DIR to /usr/share/icons/ or ~/.icons/"
echo "To activate: gsettings set org.gnome.desktop.interface cursor-theme 'Helix-Invisible'"
