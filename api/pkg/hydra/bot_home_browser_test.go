package hydra

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeclaredBrowserProfile(t *testing.T) {
	const decl = `{"profile":"chrome-profile","profile_id":"id-1"}`
	profile := func(home string) {
		writeFile(t, filepath.Join(home, "chrome-profile", "Local State"), `{"profile":{}}`, 0o600)
		writeFile(t, filepath.Join(home, "chrome-profile", ".helix-profile.json"), `{"profile_id":"id-1"}`, 0o644)
		writeFile(t, filepath.Join(home, "chrome-profile", "Default", "Cookies"), "SECRET-COOKIE-BYTES", 0o600)
	}
	tests := map[string]struct {
		setup func(t *testing.T, home string)
		want  *DeclaredBrowserProfile
	}{
		"no declaration": {
			setup: func(t *testing.T, home string) { profile(home) },
			want:  nil,
		},
		"declared and present": {
			setup: func(t *testing.T, home string) {
				profile(home)
				writeFile(t, filepath.Join(home, ".helix", "browser.json"), decl, 0o644)
			},
			want: &DeclaredBrowserProfile{Path: "chrome-profile", State: DeclaredBrowserProfileOK},
		},
		"declared profile missing": {
			setup: func(t *testing.T, home string) {
				writeFile(t, filepath.Join(home, ".helix", "browser.json"), decl, 0o644)
			},
			want: &DeclaredBrowserProfile{Path: "chrome-profile", State: DeclaredBrowserProfileMissing},
		},
		"another profile in its place": {
			setup: func(t *testing.T, home string) {
				profile(home)
				writeFile(t, filepath.Join(home, "chrome-profile", ".helix-profile.json"), `{"profile_id":"other"}`, 0o644)
				writeFile(t, filepath.Join(home, ".helix", "browser.json"), decl, 0o644)
			},
			want: &DeclaredBrowserProfile{Path: "chrome-profile", State: DeclaredBrowserProfileWrongIdentity},
		},
		"damaged Local State": {
			setup: func(t *testing.T, home string) {
				profile(home)
				writeFile(t, filepath.Join(home, "chrome-profile", "Local State"), `{"profile":`, 0o600)
				writeFile(t, filepath.Join(home, ".helix", "browser.json"), decl, 0o644)
			},
			want: &DeclaredBrowserProfile{Path: "chrome-profile", State: DeclaredBrowserProfileInconsistent},
		},
		"path outside the home": {
			setup: func(t *testing.T, home string) {
				writeFile(t, filepath.Join(home, ".helix", "browser.json"), `{"profile":"../other","profile_id":"id-1"}`, 0o644)
			},
			want: &DeclaredBrowserProfile{Path: "../other", State: DeclaredBrowserProfileInvalidDeclaration},
		},
		"symlink out of the home": {
			setup: func(t *testing.T, home string) {
				outside := t.TempDir()
				profile(outside)
				require.NoError(t, os.Symlink(filepath.Join(outside, "chrome-profile"), filepath.Join(home, "chrome-profile")))
				writeFile(t, filepath.Join(home, ".helix", "browser.json"), decl, 0o644)
			},
			want: &DeclaredBrowserProfile{Path: "chrome-profile", State: DeclaredBrowserProfileOutsideWorkspace},
		},
		"unparseable declaration": {
			setup: func(t *testing.T, home string) {
				profile(home)
				writeFile(t, filepath.Join(home, ".helix", "browser.json"), `{`, 0o644)
			},
			want: &DeclaredBrowserProfile{State: DeclaredBrowserProfileInvalidDeclaration},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			tc.setup(t, home)
			inv := inventoryBotHome(home)
			require.Equal(t, tc.want, inv.BrowserProfile)
			encoded, err := json.Marshal(inv)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), "SECRET-COOKIE-BYTES")
		})
	}
}
