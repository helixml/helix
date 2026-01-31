package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
)

// resolvePath translates a path from user-friendly to actual workspace path.
// If WORKSPACE_DIR is set and path starts with /home/retro/work,
// it translates to the actual workspace path.
func resolvePath(path string) string {
	// Expand ~ to home directory
	if strings.HasPrefix(path, "~") {
		home := os.Getenv("HOME")
		if home != "" {
			path = home + path[1:]
		}
	}

	workspaceDir := os.Getenv("WORKSPACE_DIR")
	if workspaceDir != "" && strings.HasPrefix(path, UserPath) {
		// Replace /home/retro/work prefix with WORKSPACE_DIR
		relative := strings.TrimPrefix(path, UserPath)
		resolved := workspaceDir + relative
		log.Debug().
			Str("original", path).
			Str("resolved", resolved).
			Msg("Translated path via WORKSPACE_DIR")
		return resolved
	}

	// Fallback to symlink resolution for other paths or when WORKSPACE_DIR not set
	if pathExists(path) {
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil && resolved != path {
			log.Debug().
				Str("original", path).
				Str("resolved", resolved).
				Msg("Resolved symlink")
			return resolved
		}
		return path
	}

	// Path doesn't exist yet - resolve parent directory
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	if pathExists(dir) {
		resolvedDir, err := filepath.EvalSymlinks(dir)
		if err == nil && resolvedDir != dir {
			resolved := filepath.Join(resolvedDir, base)
			log.Debug().
				Str("original", path).
				Str("resolved", resolved).
				Msg("Resolved parent symlink")
			return resolved
		}
	}

	return path
}

// pathExists checks if a path exists (file, directory, or symlink)
func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// isNamedVolume checks if a volume source is a named volume (not a path).
// Named volumes don't start with / or . and don't exist on the filesystem.
func isNamedVolume(src string) bool {
	// Named volumes don't start with / or .
	if strings.HasPrefix(src, "/") || strings.HasPrefix(src, ".") {
		return false
	}
	// If it exists on the filesystem, treat as a path
	if pathExists(src) {
		return false
	}
	return true
}

// processVolumeArg processes a Docker volume argument (-v format).
// Formats: /src:/dst, /src:/dst:ro, /src:/dst:rw,z, etc.
func processVolumeArg(vol string) string {
	// Split on first colon to get source path
	parts := strings.SplitN(vol, ":", 2)
	if len(parts) < 2 {
		return vol
	}

	src := parts[0]
	rest := parts[1]

	// Skip named volumes
	if isNamedVolume(src) {
		return vol
	}

	// Resolve the source path
	resolvedSrc := resolvePath(src)
	if resolvedSrc != src {
		return resolvedSrc + ":" + rest
	}

	return vol
}

// processMountArg processes a Docker --mount argument.
// Mount uses key=value pairs, source= or src= contains the path.
func processMountArg(mountSpec string) string {
	parts := strings.Split(mountSpec, ",")
	modified := false
	result := make([]string, 0, len(parts))

	for _, part := range parts {
		if strings.HasPrefix(part, "source=") {
			src := strings.TrimPrefix(part, "source=")
			resolved := resolvePath(src)
			if resolved != src {
				part = "source=" + resolved
				modified = true
			}
		} else if strings.HasPrefix(part, "src=") {
			src := strings.TrimPrefix(part, "src=")
			resolved := resolvePath(src)
			if resolved != src {
				part = "src=" + resolved
				modified = true
			}
		}
		result = append(result, part)
	}

	if modified {
		return strings.Join(result, ",")
	}
	return mountSpec
}
