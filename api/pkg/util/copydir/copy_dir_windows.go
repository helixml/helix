//go:build windows

package copydir

func sameFilesystem(path1, path2 string) bool {
	// Windows build - just return false to force copy mode
	return false
}
