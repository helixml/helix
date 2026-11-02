package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeDesktopDisplay(t *testing.T) {
	cases := []struct {
		name                string
		points              int
		scale               float64
		width, height, zoom int
		ok                  bool
	}{
		{"MBP 14 More Space", 1800, 2, 3600, 2024, 200, true},
		{"MBP 14 default", 1512, 2, 3024, 1700, 200, true},
		{"MBA 13 default", 1470, 2, 2940, 1652, 200, true},
		{"non-Retina 1080p monitor", 1920, 1, 1920, 1080, 100, true},
		{"6K XDR capped at the VM's 5120 EDID", 3008, 2, 5120, 2880, 200, true},
		{"too small", 1024, 1, 0, 0, 0, false},
		{"no display", 0, 0, 0, 0, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w, h, z, ok := nativeDesktopDisplay(tc.points, tc.scale)
			if ok != tc.ok || w != tc.width || h != tc.height || z != tc.zoom {
				t.Fatalf("got %dx%d zoom %d ok=%v, want %dx%d zoom %d ok=%v", w, h, z, ok, tc.width, tc.height, tc.zoom, tc.ok)
			}
			if ok && (w%(z/50) != 0 || h%(z/50) != 0) {
				t.Fatalf("%dx%d not a multiple of twice the zoom factor", w, h)
			}
		})
	}
}

// runEnvScript runs the fragment the way injectDesktopSecret does, against a
// scratch .env, and returns the file and whether CHANGED was set.
func runEnvScript(t *testing.T, env, fragment string) (string, bool) {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}
	script := "ENV_FILE=" + path + "\nCHANGED=0\n" + fragment + "\necho CHANGED=$CHANGED\n"
	out, err := exec.Command("bash", "-c", strings.ReplaceAll(script, "sed -i ", "sed -i.bak ")).CombinedOutput()
	if err != nil {
		t.Fatalf("script failed: %v\n%s", err, out)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(got), strings.Contains(string(out), "CHANGED=1")
}

func TestNativeDisplayEnvScript(t *testing.T) {
	set := nativeDisplayEnvScript(3600, 2024, 200, true)

	env, changed := runEnvScript(t, "GPU_VENDOR=virtio\n", set)
	if !changed || !strings.Contains(env, "HELIX_NATIVE_DISPLAY_WIDTH=3600\nHELIX_NATIVE_DISPLAY_HEIGHT=2024\nHELIX_NATIVE_DISPLAY_ZOOM=200\n") || !strings.HasPrefix(env, "GPU_VENDOR=virtio\n") {
		t.Fatalf("first run: changed=%v env=%q", changed, env)
	}

	if _, changed := runEnvScript(t, env, set); changed {
		t.Fatal("unchanged display must not restart the API")
	}

	env, changed = runEnvScript(t, env, nativeDisplayEnvScript(3024, 1700, 200, true))
	if !changed || !strings.Contains(env, "HELIX_NATIVE_DISPLAY_WIDTH=3024\n") || strings.Count(env, "HELIX_NATIVE_DISPLAY_WIDTH=") != 1 {
		t.Fatalf("display change: changed=%v env=%q", changed, env)
	}

	env, changed = runEnvScript(t, env, nativeDisplayEnvScript(0, 0, 0, false))
	if !changed || strings.Contains(env, "HELIX_NATIVE_DISPLAY_") || !strings.Contains(env, "GPU_VENDOR=virtio") {
		t.Fatalf("no display: changed=%v env=%q", changed, env)
	}
}

// The real display can be read on the machine running the tests.
func TestLaptopDisplayReadsAScreen(t *testing.T) {
	points, scale, ok := laptopDisplay()
	if !ok {
		t.Skip("no display attached")
	}
	w, h, z, ok := nativeDesktopDisplay(points, scale)
	t.Logf("laptop display %dpt at %.1fx -> desktop %dx%d at %d%%", points, scale, w, h, z)
	if !ok {
		t.Fatalf("%dpt at %.2fx gave no desktop size", points, scale)
	}
}
