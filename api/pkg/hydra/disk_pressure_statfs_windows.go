//go:build windows

package hydra

import "fmt"

// Windows stubs for the statfs disk-pressure backend. Hydra is not deployed
// on Windows; these exist solely so the CLI (and any other transitive
// importer) can cross-compile to windows/amd64. At runtime they always
// report "not supported", which causes measureDisk() to surface the
// existing "all backends unavailable, fail-open" path in
// checkDiskPressureForStart with a prominent warn log.

func statfsFreePercentMulti(_ []string) (float64, string, bool, error) {
	return 0, "", false, fmt.Errorf("statfs disk-pressure not supported on windows")
}

func statfsFreeBytesMulti(_ []string) (int64, string, bool, error) {
	return 0, "", false, fmt.Errorf("statfs disk-pressure not supported on windows")
}
