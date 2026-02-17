package main

import (
	"fmt"
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
	newArgs, err := processDockerArgs(args)
	if err != nil {
		// Fail fast: print error and exit with non-zero code
		fmt.Fprintf(os.Stderr, "docker-shim: build failed: %v\n", err)
		return 1
	}

	log.Debug().
		Strs("original", args).
		Strs("processed", newArgs).
		Msg("Executing docker")

	// Execute the real docker binary
	return execReal(DockerRealPath, newArgs)
}

// processDockerArgs processes docker arguments for cache injection.
// Returns error if a required component (like shared builder) is unavailable.
func processDockerArgs(args []string) ([]string, error) {
	return injectBuildCacheFlags(args)
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

// SharedBuilderName is the name of the shared buildx builder with cache mount
const SharedBuilderName = "helix-shared"

// SharedBuildKitContainerName is the name of the BuildKit container
const SharedBuildKitContainerName = "helix-buildkit"

// ensureSharedBuilder checks if the helix-shared builder exists, creates it if not
// Returns nil if the builder is available, error with details if it couldn't be set up
func ensureSharedBuilder() error {
	// Check if builder already exists
	checkCmd := exec.Command(DockerRealPath, "buildx", "inspect", SharedBuilderName)
	if err := checkCmd.Run(); err == nil {
		log.Debug().Str("builder", SharedBuilderName).Msg("Shared builder already exists")
		return nil
	}

	// Get BuildKit endpoint - prefer BUILDKIT_HOST env var (set by Hydra for dev containers),
	// fall back to looking up helix-buildkit container directly.
	// BUILDKIT_HOST is needed because dev containers mount a per-session Docker socket
	// but helix-buildkit runs on the sandbox's main dockerd.
	buildkitEndpoint := os.Getenv("BUILDKIT_HOST")
	if buildkitEndpoint == "" {
		// Fall back to container lookup (works when Docker socket points to main dockerd)
		checkContainerCmd := exec.Command(DockerRealPath, "inspect", SharedBuildKitContainerName)
		if err := checkContainerCmd.Run(); err != nil {
			return fmt.Errorf("shared BuildKit container '%s' not found and BUILDKIT_HOST not set. "+
				"This container should be started by Hydra. Check that Hydra is running and has set up the BuildKit container. "+
				"Error: %w", SharedBuildKitContainerName, err)
		}

		// Get buildkit container IP
		ipCmd := exec.Command(DockerRealPath, "inspect", "-f",
			"{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}",
			SharedBuildKitContainerName)
		ipOutput, err := ipCmd.Output()
		if err != nil {
			return fmt.Errorf("could not get IP for BuildKit container '%s': %w",
				SharedBuildKitContainerName, err)
		}
		buildkitIP := strings.TrimSpace(string(ipOutput))
		if buildkitIP == "" {
			return fmt.Errorf("BuildKit container '%s' has no IP address. "+
				"The container may not be connected to a network properly",
				SharedBuildKitContainerName)
		}
		buildkitEndpoint = "tcp://" + buildkitIP + ":1234"
	}

	// Create the builder
	log.Info().
		Str("builder", SharedBuilderName).
		Str("endpoint", buildkitEndpoint).
		Msg("Creating shared buildx builder")

	createCmd := exec.Command(DockerRealPath, "buildx", "create",
		"--name", SharedBuilderName,
		"--driver", "remote",
		buildkitEndpoint,
	)
	if output, err := createCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create shared builder '%s': %s: %w",
			SharedBuilderName, strings.TrimSpace(string(output)), err)
	}

	return nil
}

// hasBuilderFlag checks if --builder flag is already present
func hasBuilderFlag(args []string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, "--builder") {
			return true
		}
	}
	return false
}

// injectBuildCacheFlags adds BuildKit cache flags and builder to build commands
// Returns the modified args and an error if the shared builder can't be set up
// Fail-fast: if cache directory exists, builder MUST be available
func injectBuildCacheFlags(args []string) ([]string, error) {
	// Only process build commands
	if !isBuildCommand(args) {
		return args, nil
	}

	// Check if cache directory exists - if it does, we REQUIRE the builder
	if _, err := os.Stat(BuildKitCacheDir); os.IsNotExist(err) {
		log.Debug().Msg("BuildKit cache directory not found, skipping cache injection")
		return args, nil
	}

	// Don't inject if user already specified cache flags
	if hasCacheFlags(args) {
		log.Debug().Msg("Cache flags already present, skipping injection")
		return args, nil
	}

	// Extract image name for cache key
	imageName := extractImageTag(args)
	cacheKey := sanitizeForPath(imageName)
	if cacheKey == "" {
		cacheKey = "default"
	}

	cacheDir := filepath.Join(BuildKitCacheDir, cacheKey)

	// Ensure cache subdirectory exists with world-writable permissions.
	// BuildKit remote builders export type=local cache via the client process,
	// which may run as a non-root user (e.g. retro/uid 1000 in dev containers).
	// The directory might have been previously created by root (e.g. by BuildKit
	// itself or a sudo'd process), so we also chmod existing directories.
	if err := os.MkdirAll(cacheDir, 0777); err != nil {
		return nil, fmt.Errorf("failed to create cache directory '%s': %w", cacheDir, err)
	}
	// Fix permissions on existing directories that may have been created with
	// restrictive permissions (e.g. root:root 0755 from a previous BuildKit run)
	if err := os.Chmod(cacheDir, 0777); err != nil {
		log.Warn().Err(err).Str("dir", cacheDir).Msg("Failed to chmod cache directory, cache export may fail if owned by another user")
	}

	// Find where to insert flags (after "build" or "buildx build")
	insertIdx := -1
	for i, arg := range args {
		if arg == "build" {
			insertIdx = i + 1
			break
		}
	}

	if insertIdx == -1 || insertIdx > len(args) {
		return args, nil
	}

	// FAIL FAST: Shared builder is REQUIRED when cache directory exists
	// This ensures builds always use the shared cache for consistency
	if !hasBuilderFlag(args) {
		if err := ensureSharedBuilder(); err != nil {
			return nil, fmt.Errorf("shared BuildKit builder required for cached builds: %w\n\n"+
				"The /buildkit-cache directory exists, which means cached builds are expected.\n"+
				"Ensure Hydra has started the helix-buildkit container.", err)
		}
	}

	// Build new args with builder and cache flags
	cacheFrom := "--cache-from=type=local,src=" + cacheDir
	cacheTo := "--cache-to=type=local,dest=" + cacheDir + ",mode=max"

	result := make([]string, 0, len(args)+4)
	result = append(result, args[:insertIdx]...)

	// Add --builder flag if not already specified
	if !hasBuilderFlag(args) {
		result = append(result, "--builder="+SharedBuilderName)
		// With remote builder, we need --load to get image into local docker
		// Check if --load or --push already specified
		hasOutput := false
		for _, arg := range args {
			if arg == "--load" || arg == "--push" || strings.HasPrefix(arg, "-o") || strings.HasPrefix(arg, "--output") {
				hasOutput = true
				break
			}
		}
		if !hasOutput {
			result = append(result, "--load")
		}
	}

	result = append(result, cacheFrom, cacheTo)
	result = append(result, args[insertIdx:]...)

	log.Debug().
		Str("cache_dir", cacheDir).
		Str("image", imageName).
		Msg("Injected BuildKit cache flags")

	return result, nil
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
