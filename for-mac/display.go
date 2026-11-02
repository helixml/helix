package main

import (
	"fmt"
	"log"
	"math"
)

// Bounds for the desktop size derived from the Mac's display. 5120x2880 is
// the largest mode the VM's virtio-gpu EDID offers (vm.go: xres/yres).
const (
	nativeDisplayMaxWidth = 5120
	nativeDisplayMinWidth = 1280
)

// nativeDesktopDisplay turns the laptop's display into the size desktops in
// the VM default to: as wide in pixels as macOS renders the screen (points x
// scale, so "More Space" 1800pt at 2x gives 3600), 16:9 rather than the
// screen's own aspect ratio, which suits window chrome, and the screen's
// scale as GNOME zoom. GNOME scales in whole steps, so the zoom is the scale
// rounded, and both sides are multiples of twice the zoom factor.
func nativeDesktopDisplay(pointsWide int, scale float64) (width, height, zoom int, ok bool) {
	if pointsWide <= 0 || scale <= 0 {
		return 0, 0, 0, false
	}
	factor := int(math.Round(scale))
	if factor < 1 {
		factor = 1
	}
	if factor > 3 {
		factor = 3
	}
	step := 2 * factor

	width = pointsWide * factor
	if width > nativeDisplayMaxWidth {
		width = nativeDisplayMaxWidth
	}
	width -= width % step
	if width < nativeDisplayMinWidth {
		return 0, 0, 0, false
	}
	height = int(math.Round(float64(width)*9/16/float64(step))) * step
	return width, height, factor * 100, true
}

// nativeDisplayEnvScript is the shell fragment injectDesktopSecret runs to
// keep HELIX_NATIVE_DISPLAY_* in the VM's .env in step with the laptop's
// display. The API reads them at startup (types.SetNativeDisplay); a change
// sets CHANGED so the API is restarted. With no display measured the
// variables are removed and desktops fall back to 1080p.
func nativeDisplayEnvScript(width, height, zoom int, ok bool) string {
	if !ok {
		return `
# Laptop display not measured: no native desktop size
if grep -q '^HELIX_NATIVE_DISPLAY_' "$ENV_FILE" 2>/dev/null; then
    sed -i '/^HELIX_NATIVE_DISPLAY_/d' "$ENV_FILE"
    CHANGED=1
fi
`
	}
	script := `
# Laptop display: desktops default to its width and zoom, at 16:9
set_native_display_env() {
    if grep -q "^$1=" "$ENV_FILE" 2>/dev/null; then
        if [ "$(grep "^$1=" "$ENV_FILE" | cut -d= -f2-)" != "$2" ]; then
            sed -i "s|^$1=.*|$1=$2|" "$ENV_FILE"
            CHANGED=1
        fi
    else
        echo "$1=$2" >> "$ENV_FILE"
        CHANGED=1
    fi
}
`
	return script + fmt.Sprintf(`set_native_display_env HELIX_NATIVE_DISPLAY_WIDTH %d
set_native_display_env HELIX_NATIVE_DISPLAY_HEIGHT %d
set_native_display_env HELIX_NATIVE_DISPLAY_ZOOM %d
`, width, height, zoom)
}

// laptopNativeDisplayEnvScript measures the laptop's display now and returns
// the .env fragment for it.
func laptopNativeDisplayEnvScript() string {
	points, scale, ok := laptopDisplay()
	width, height, zoom, ok := nativeDesktopDisplay(points, scale)
	if ok {
		log.Printf("Laptop display %dpt at %.1fx: desktops default to %dx%d at %d%%", points, scale, width, height, zoom)
	} else {
		log.Printf("Laptop display not measured (%dpt at %.1fx): desktops default to 1080p", points, scale)
	}
	return nativeDisplayEnvScript(width, height, zoom, ok)
}
