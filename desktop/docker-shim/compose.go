package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/rs/zerolog/log"
)

// runCompose handles docker-compose commands with BuildKit cache injection
func runCompose(args []string) int {
	// Handle docker-cli-plugin-metadata - pass through directly
	if len(args) > 0 && args[0] == "docker-cli-plugin-metadata" {
		return execReal(ComposeRealPath, args)
	}

	// Docker CLI plugin protocol: first arg is the plugin name ("compose")
	// Strip it - the real plugin doesn't expect it as an argument
	if len(args) > 0 && args[0] == "compose" {
		args = args[1:]
	}

	// Inject BuildKit cache flags for build commands
	var err error
	args, err = injectComposeCacheFlags(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "docker-shim: compose build failed: %v\n", err)
		return 1
	}

	log.Debug().
		Strs("processed", args).
		Msg("Executing docker-compose")

	return execReal(ComposeRealPath, args)
}

// isComposeBuildCommand checks if args contain a build command
func isComposeBuildCommand(args []string) bool {
	for i, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "build" {
			return true
		}
		if arg == "up" {
			// Check if --build is present
			for j := i + 1; j < len(args); j++ {
				if args[j] == "--build" {
					return true
				}
				if args[j] == "--" {
					break
				}
			}
		}
	}
	return false
}

// hasComposeCacheFlags checks if compose cache flags are already present
func hasComposeCacheFlags(args []string) bool {
	for _, arg := range args {
		if strings.Contains(arg, "cache_from") || strings.Contains(arg, "cache_to") {
			return true
		}
	}
	return false
}

// getComposeVersion returns the docker compose version
func getComposeVersion() string {
	cmd := exec.Command(ComposeRealPath, "compose", "version", "--short")
	out, err := cmd.Output()
	if err != nil {
		// Try without compose arg (direct invocation)
		cmd = exec.Command(ComposeRealPath, "version", "--short")
		out, err = cmd.Output()
		if err != nil {
			return ""
		}
	}
	return strings.TrimSpace(string(out))
}

// compareVersions returns true if v1 >= v2
func compareVersions(v1, v2 string) bool {
	parse := func(v string) (int, int, int) {
		v = strings.TrimPrefix(v, "v")
		parts := strings.Split(v, ".")
		major, minor, patch := 0, 0, 0
		if len(parts) > 0 {
			fmt.Sscanf(parts[0], "%d", &major)
		}
		if len(parts) > 1 {
			fmt.Sscanf(parts[1], "%d", &minor)
		}
		if len(parts) > 2 {
			fmt.Sscanf(parts[2], "%d", &patch)
		}
		return major, minor, patch
	}

	maj1, min1, pat1 := parse(v1)
	maj2, min2, pat2 := parse(v2)

	if maj1 != maj2 {
		return maj1 > maj2
	}
	if min1 != min2 {
		return min1 > min2
	}
	return pat1 >= pat2
}

// injectComposeCacheFlags adds BuildKit cache flags to compose build commands.
// For Compose 5.0+, cache injection is skipped (builds delegate to buildx which handles caching).
// Returns error if the cache directory exists but compose version is too old (< v2.24).
func injectComposeCacheFlags(args []string) ([]string, error) {
	if !isComposeBuildCommand(args) {
		return args, nil
	}

	if _, err := os.Stat(BuildKitCacheDir); os.IsNotExist(err) {
		log.Debug().Msg("BuildKit cache directory not found, skipping compose cache injection")
		return args, nil
	}

	if hasComposeCacheFlags(args) {
		log.Debug().Msg("Compose cache flags already present, skipping injection")
		return args, nil
	}

	version := getComposeVersion()
	if version == "" {
		return nil, fmt.Errorf("could not determine Docker Compose version. "+
			"BuildKit cache directory exists at %s, which requires Compose v2.24+", BuildKitCacheDir)
	}

	log.Debug().Str("version", version).Msg("Detected compose version")

	if !compareVersions(version, "2.24.0") {
		return nil, fmt.Errorf("Docker Compose %s is too old for cache injection (requires v2.24+). "+
			"BuildKit cache directory exists at %s", version, BuildKitCacheDir)
	}

	if compareVersions(version, "5.0.0") {
		// Compose 5.0+ delegates to buildx, so inject --builder instead of --set
		if hasBuilderFlag(args) {
			log.Debug().Msg("Builder flag already present, skipping injection")
			return args, nil
		}

		// Try to ensure the shared builder is available, but don't fail if it's not
		if err := ensureSharedBuilder(); err != nil {
			log.Warn().
				Err(err).
				Str("cache_dir", BuildKitCacheDir).
				Msg("Shared BuildKit builder unavailable, builds will proceed without shared cache. " +
					"This is expected during initial helix-in-helix setup.")
			return args, nil
		}

		insertIdx := -1
		for i, arg := range args {
			if arg == "build" || arg == "up" {
				insertIdx = i + 1
				break
			}
		}

		if insertIdx == -1 || insertIdx > len(args) {
			return args, nil
		}

		result := make([]string, 0, len(args)+1)
		result = append(result, args[:insertIdx]...)
		result = append(result, "--builder="+SharedBuilderName)
		result = append(result, args[insertIdx:]...)

		log.Debug().
			Str("builder", SharedBuilderName).
			Str("cache_dir", BuildKitCacheDir).
			Msg("Injected compose builder flag for Compose 5.0+")

		return result, nil
	}

	insertIdx := -1
	for i, arg := range args {
		if arg == "build" || arg == "up" {
			insertIdx = i + 1
			break
		}
	}

	if insertIdx == -1 || insertIdx > len(args) {
		return args, nil
	}

	cacheFrom := `--set=*.build.cache_from=["type=local,src=` + BuildKitCacheDir + `"]`
	cacheTo := `--set=*.build.cache_to=["type=local,dest=` + BuildKitCacheDir + `,mode=max"]`

	result := make([]string, 0, len(args)+2)
	result = append(result, args[:insertIdx]...)
	result = append(result, cacheFrom, cacheTo)
	result = append(result, args[insertIdx:]...)

	log.Debug().
		Str("cache_dir", BuildKitCacheDir).
		Msg("Injected compose BuildKit cache flags")

	return result, nil
}
