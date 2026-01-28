package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/rs/zerolog/log"
)

// runDocker handles docker CLI commands with path translation and cache injection
func runDocker(args []string) int {
	// Process arguments
	newArgs := processDockerArgs(args)

	log.Debug().
		Strs("original", args).
		Strs("processed", newArgs).
		Msg("Executing docker")

	// Execute the real docker binary
	return execReal(DockerRealPath, newArgs)
}

// processDockerArgs processes docker arguments for path translation and cache injection
func processDockerArgs(args []string) []string {
	result := make([]string, 0, len(args)+4) // Extra space for cache flags

	// First pass: translate paths in volume/mount arguments
	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case arg == "-v" || arg == "--volume":
			// Next argument is the volume spec
			result = append(result, arg)
			if i+1 < len(args) {
				i++
				result = append(result, processVolumeArg(args[i]))
			}

		case strings.HasPrefix(arg, "-v=") || strings.HasPrefix(arg, "--volume="):
			// Volume spec is part of the argument
			prefix := arg[:strings.Index(arg, "=")+1]
			vol := arg[strings.Index(arg, "=")+1:]
			result = append(result, prefix+processVolumeArg(vol))

		case arg == "--mount":
			// --mount uses key=value pairs
			result = append(result, arg)
			if i+1 < len(args) {
				i++
				result = append(result, processMountArg(args[i]))
			}

		case strings.HasPrefix(arg, "--mount="):
			// Mount spec is part of the argument
			mountSpec := strings.TrimPrefix(arg, "--mount=")
			result = append(result, "--mount="+processMountArg(mountSpec))

		default:
			result = append(result, arg)
		}
	}

	// Second pass: inject BuildKit cache flags for build commands
	result = injectBuildCacheFlags(result)

	return result
}

// isBuildCommand checks if the args represent a docker build command
func isBuildCommand(args []string) bool {
	for i, arg := range args {
		// Skip flags and their values
		if strings.HasPrefix(arg, "-") {
			continue
		}

		// Found a command
		if arg == "build" {
			return true
		}
		if arg == "buildx" && i+1 < len(args) {
			// Check if next non-flag arg is "build"
			for j := i + 1; j < len(args); j++ {
				if !strings.HasPrefix(args[j], "-") {
					return args[j] == "build"
				}
			}
		}
		// Once we hit a non-flag, non-build command, stop looking
		break
	}
	return false
}

// extractImageTag extracts the image name from -t flag for use as cache key
func extractImageTag(args []string) string {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-t" || arg == "--tag" {
			if i+1 < len(args) {
				return args[i+1]
			}
		}
		if strings.HasPrefix(arg, "-t=") {
			return strings.TrimPrefix(arg, "-t=")
		}
		if strings.HasPrefix(arg, "--tag=") {
			return strings.TrimPrefix(arg, "--tag=")
		}
	}
	return ""
}

// sanitizeForPath converts an image name to a safe directory name
func sanitizeForPath(imageName string) string {
	// Remove registry prefix and tag
	// e.g., "registry.example.com/foo/bar:latest" -> "foo_bar"
	name := imageName

	// Remove tag/digest
	if idx := strings.LastIndex(name, ":"); idx != -1 {
		name = name[:idx]
	}
	if idx := strings.LastIndex(name, "@"); idx != -1 {
		name = name[:idx]
	}

	// Remove registry (anything before first /)
	if idx := strings.Index(name, "/"); idx != -1 {
		// Check if it looks like a registry (contains . or :)
		prefix := name[:idx]
		if strings.Contains(prefix, ".") || strings.Contains(prefix, ":") {
			name = name[idx+1:]
		}
	}

	// Replace / with _
	name = strings.ReplaceAll(name, "/", "_")

	// Remove any remaining unsafe characters
	result := make([]byte, 0, len(name))
	for _, c := range []byte(name) {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' || c == '.' {
			result = append(result, c)
		}
	}

	if len(result) == 0 {
		return "default"
	}
	return string(result)
}

// hasCacheFlags checks if cache flags are already present
func hasCacheFlags(args []string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, "--cache-from") || strings.HasPrefix(arg, "--cache-to") {
			return true
		}
	}
	return false
}

// injectBuildCacheFlags adds BuildKit cache flags to build commands
func injectBuildCacheFlags(args []string) []string {
	// Only process build commands
	if !isBuildCommand(args) {
		return args
	}

	// Check if cache directory exists
	if _, err := os.Stat(BuildKitCacheDir); os.IsNotExist(err) {
		log.Debug().Msg("BuildKit cache directory not found, skipping cache injection")
		return args
	}

	// Don't inject if user already specified cache flags
	if hasCacheFlags(args) {
		log.Debug().Msg("Cache flags already present, skipping injection")
		return args
	}

	// Extract image name for cache key
	imageName := extractImageTag(args)
	cacheKey := sanitizeForPath(imageName)
	if cacheKey == "" {
		cacheKey = "default"
	}

	cacheDir := filepath.Join(BuildKitCacheDir, cacheKey)

	// Ensure cache subdirectory exists
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		log.Warn().Err(err).Str("dir", cacheDir).Msg("Failed to create cache directory")
		return args
	}

	// Find where to insert cache flags (after "build" or "buildx build")
	insertIdx := -1
	for i, arg := range args {
		if arg == "build" {
			insertIdx = i + 1
			break
		}
	}

	if insertIdx == -1 || insertIdx > len(args) {
		return args
	}

	// Build new args with cache flags
	cacheFrom := "--cache-from=type=local,src=" + cacheDir
	cacheTo := "--cache-to=type=local,dest=" + cacheDir + ",mode=max"

	result := make([]string, 0, len(args)+2)
	result = append(result, args[:insertIdx]...)
	result = append(result, cacheFrom, cacheTo)
	result = append(result, args[insertIdx:]...)

	log.Debug().
		Str("cache_dir", cacheDir).
		Str("image", imageName).
		Msg("Injected BuildKit cache flags")

	return result
}

// execReal executes the real binary, replacing the current process
func execReal(path string, args []string) int {
	// Build full argument list (path must be argv[0])
	argv := append([]string{path}, args...)

	// Try to exec (replaces current process)
	err := syscall.Exec(path, argv, os.Environ())
	if err != nil {
		// If exec fails, fall back to cmd.Run
		log.Warn().Err(err).Str("path", path).Msg("syscall.Exec failed, using exec.Command")
		cmd := exec.Command(path, args...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				return exitErr.ExitCode()
			}
			log.Error().Err(err).Msg("Failed to execute docker")
			return 1
		}
	}
	return 0
}
