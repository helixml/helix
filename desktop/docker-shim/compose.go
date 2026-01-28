package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

// runCompose handles docker-compose commands with path translation and cache injection
func runCompose(args []string) int {
	// Handle docker-cli-plugin-metadata - pass through directly
	if len(args) > 0 && args[0] == "docker-cli-plugin-metadata" {
		return execReal(ComposeRealPath, args)
	}

	// Docker CLI plugin protocol: first arg is the plugin name ("compose")
	// We need to preserve it
	pluginName := ""
	if len(args) > 0 && args[0] == "compose" {
		pluginName = args[0]
		args = args[1:]
	}

	// Process arguments
	newArgs, tmpFiles := processComposeArgs(args)
	defer cleanupTmpFiles(tmpFiles)

	// Add project name isolation per session if not already specified
	projectArgs := getProjectArgs(newArgs)

	// Build final arguments
	finalArgs := make([]string, 0, len(newArgs)+len(projectArgs)+1)
	if pluginName != "" {
		finalArgs = append(finalArgs, pluginName)
	}
	finalArgs = append(finalArgs, projectArgs...)
	finalArgs = append(finalArgs, newArgs...)

	log.Debug().
		Strs("original", args).
		Strs("processed", finalArgs).
		Msg("Executing docker-compose")

	return execReal(ComposeRealPath, finalArgs)
}

// processComposeArgs processes docker-compose arguments
// Returns the processed args and a list of temp files to clean up
func processComposeArgs(args []string) ([]string, []string) {
	var tmpFiles []string

	// Check if this is a command that needs compose file processing
	if !needsComposeProcessing(args) {
		// Still try to inject cache flags for build commands
		return injectComposeCacheFlags(args), tmpFiles
	}

	// Find compose files from arguments
	composeFiles := findComposeFiles(args)

	// If no -f specified, check for default files
	if len(composeFiles) == 0 {
		defaults := []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"}
		for _, def := range defaults {
			if _, err := os.Stat(def); err == nil {
				composeFiles = append(composeFiles, def)
				break
			}
		}
	}

	if len(composeFiles) == 0 {
		return injectComposeCacheFlags(args), tmpFiles
	}

	// Process each compose file - create temp file in same directory
	fileMap := make(map[string]string)
	for _, file := range composeFiles {
		if _, err := os.Stat(file); err != nil {
			continue
		}

		fileDir := filepath.Dir(file)
		fileBase := filepath.Base(file)
		tmpFile := filepath.Join(fileDir, ".hydra-resolved."+fileBase)

		if err := processComposeFile(file, tmpFile); err != nil {
			log.Warn().Err(err).Str("file", file).Msg("Failed to process compose file")
			continue
		}

		fileMap[file] = tmpFile
		tmpFiles = append(tmpFiles, tmpFile)
	}

	// Rebuild args with modified file paths
	newArgs := make([]string, 0, len(args)+2)
	skipNext := false
	foundFileArg := false

	for i, arg := range args {
		if skipNext {
			skipNext = false
			// This is a file path after -f
			if mapped, ok := fileMap[arg]; ok {
				newArgs = append(newArgs, mapped)
			} else {
				newArgs = append(newArgs, arg)
			}
			continue
		}

		switch {
		case arg == "-f" || arg == "--file":
			newArgs = append(newArgs, arg)
			skipNext = true
			foundFileArg = true

		case strings.HasPrefix(arg, "-f=") || strings.HasPrefix(arg, "--file="):
			prefix := arg[:strings.Index(arg, "=")+1]
			orig := arg[strings.Index(arg, "=")+1:]
			if mapped, ok := fileMap[orig]; ok {
				newArgs = append(newArgs, prefix+mapped)
			} else {
				newArgs = append(newArgs, arg)
			}
			foundFileArg = true

		default:
			// Skip the index check since we handle i properly
			_ = i
			newArgs = append(newArgs, arg)
		}
	}

	// If no -f was specified but we found a default file, add it
	if !foundFileArg && len(fileMap) > 0 {
		for orig, mapped := range fileMap {
			log.Debug().
				Str("original", orig).
				Str("mapped", mapped).
				Msg("Adding default compose file")
			newArgs = append([]string{"-f", mapped}, newArgs...)
			break
		}
	}

	return injectComposeCacheFlags(newArgs), tmpFiles
}

// needsComposeProcessing checks if this is a command that uses compose files
func needsComposeProcessing(args []string) bool {
	processingCommands := map[string]bool{
		"up": true, "down": true, "start": true, "stop": true, "restart": true,
		"run": true, "exec": true, "build": true, "pull": true, "push": true,
		"logs": true, "ps": true, "create": true, "convert": true,
	}

	for _, arg := range args {
		if arg == "--" {
			break
		}
		if processingCommands[arg] {
			return true
		}
	}
	return false
}

// findComposeFiles extracts compose file paths from arguments
func findComposeFiles(args []string) []string {
	var files []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-f" || arg == "--file":
			if i+1 < len(args) {
				i++
				files = append(files, args[i])
			}
		case strings.HasPrefix(arg, "-f="):
			files = append(files, strings.TrimPrefix(arg, "-f="))
		case strings.HasPrefix(arg, "--file="):
			files = append(files, strings.TrimPrefix(arg, "--file="))
		}
	}
	return files
}

// processComposeFile reads a compose file, resolves volume paths, and writes to output
func processComposeFile(input, output string) error {
	data, err := os.ReadFile(input)
	if err != nil {
		return fmt.Errorf("failed to read compose file: %w", err)
	}

	// Parse YAML
	var compose map[string]interface{}
	if err := yaml.Unmarshal(data, &compose); err != nil {
		return fmt.Errorf("failed to parse compose YAML: %w", err)
	}

	// Get directory of compose file for relative path resolution
	baseDir := filepath.Dir(input)
	if absPath, err := filepath.Abs(baseDir); err == nil {
		baseDir = absPath
	}

	// Process services
	if services, ok := compose["services"].(map[string]interface{}); ok {
		for _, svc := range services {
			if service, ok := svc.(map[string]interface{}); ok {
				processServiceVolumes(service, baseDir)
			}
		}
	}

	// Write modified file
	outData, err := yaml.Marshal(compose)
	if err != nil {
		return fmt.Errorf("failed to marshal compose YAML: %w", err)
	}

	if err := os.WriteFile(output, outData, 0644); err != nil {
		return fmt.Errorf("failed to write processed compose file: %w", err)
	}

	log.Debug().
		Str("input", input).
		Str("output", output).
		Msg("Processed compose file")

	return nil
}

// processServiceVolumes resolves volume paths in a service definition
func processServiceVolumes(service map[string]interface{}, baseDir string) {
	volumes, ok := service["volumes"]
	if !ok {
		return
	}

	switch v := volumes.(type) {
	case []interface{}:
		for i, vol := range v {
			if volStr, ok := vol.(string); ok {
				v[i] = processComposeVolume(volStr, baseDir)
			} else if volMap, ok := vol.(map[string]interface{}); ok {
				processComposeVolumeMap(volMap, baseDir)
			}
		}
	}
}

// processComposeVolume processes a string volume definition (short syntax)
// Format: /src:/dst or /src:/dst:ro
func processComposeVolume(vol, baseDir string) string {
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

	// Handle relative paths
	var resolved string
	if strings.HasPrefix(src, "./") || strings.HasPrefix(src, "../") {
		// Relative path - resolve from compose file directory
		absPath := filepath.Join(baseDir, src)
		resolved = resolvePath(absPath)
	} else if strings.HasPrefix(src, "~") || strings.HasPrefix(src, "/") {
		// Absolute or home path
		resolved = resolvePath(src)
	} else {
		// Named volume, leave unchanged
		return vol
	}

	if resolved != src {
		log.Debug().
			Str("original", src).
			Str("resolved", resolved).
			Msg("Resolved compose volume path")
		return resolved + ":" + rest
	}

	return vol
}

// processComposeVolumeMap processes a map volume definition (long syntax)
func processComposeVolumeMap(volMap map[string]interface{}, baseDir string) {
	source, ok := volMap["source"].(string)
	if !ok {
		return
	}

	// Skip named volumes
	if isNamedVolume(source) {
		return
	}

	var resolved string
	if strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../") {
		absPath := filepath.Join(baseDir, source)
		resolved = resolvePath(absPath)
	} else if strings.HasPrefix(source, "~") || strings.HasPrefix(source, "/") {
		resolved = resolvePath(source)
	} else {
		return
	}

	if resolved != source {
		volMap["source"] = resolved
		log.Debug().
			Str("original", source).
			Str("resolved", resolved).
			Msg("Resolved compose volume map path")
	}
}

// hasProjectFlag checks if project name is already specified in args
func hasProjectFlag(args []string) bool {
	for _, arg := range args {
		if arg == "-p" || arg == "--project-name" ||
			strings.HasPrefix(arg, "-p=") || strings.HasPrefix(arg, "--project-name=") {
			return true
		}
	}
	return false
}

// getProjectArgs returns project name arguments for session isolation
func getProjectArgs(args []string) []string {
	if hasProjectFlag(args) {
		return nil
	}

	// Prefer HELIX_TASK_NUMBER for human-readable names, fallback to HELIX_SESSION_ID
	if taskNum := os.Getenv("HELIX_TASK_NUMBER"); taskNum != "" {
		return []string{"-p", "helix-task-" + taskNum}
	}
	if sessionID := os.Getenv("HELIX_SESSION_ID"); sessionID != "" {
		return []string{"-p", "helix-" + sessionID}
	}

	return nil
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
	// Simple version comparison - parse major.minor.patch
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

// injectComposeCacheFlags adds BuildKit cache flags to compose build commands
func injectComposeCacheFlags(args []string) []string {
	// Only process build commands
	if !isComposeBuildCommand(args) {
		return args
	}

	// Check if cache directory exists
	if _, err := os.Stat(BuildKitCacheDir); os.IsNotExist(err) {
		log.Debug().Msg("BuildKit cache directory not found, skipping compose cache injection")
		return args
	}

	// Don't inject if user already specified cache flags
	if hasComposeCacheFlags(args) {
		log.Debug().Msg("Compose cache flags already present, skipping injection")
		return args
	}

	// Check compose version for --set support (v2.24+)
	version := getComposeVersion()
	if version == "" {
		log.Warn().Msg("Could not determine compose version, skipping cache injection")
		return args
	}

	log.Debug().Str("version", version).Msg("Detected compose version")

	if !compareVersions(version, "2.24.0") {
		log.Debug().Msg("Compose version < 2.24, --set not supported, skipping cache injection")
		// For older versions, we'd need to modify the compose file directly
		// which we already do in processComposeFile for path translation
		// Adding cache config there would require more complex YAML manipulation
		return args
	}

	// Find where to insert flags (after "build" or after "up")
	insertIdx := -1
	for i, arg := range args {
		if arg == "build" || arg == "up" {
			insertIdx = i + 1
			break
		}
	}

	if insertIdx == -1 || insertIdx > len(args) {
		return args
	}

	// Build cache flags using --set
	// The wildcard *.build targets all services with build configs
	cacheFrom := `--set=*.build.cache_from=["type=local,src=` + BuildKitCacheDir + `"]`
	cacheTo := `--set=*.build.cache_to=["type=local,dest=` + BuildKitCacheDir + `,mode=max"]`

	result := make([]string, 0, len(args)+2)
	result = append(result, args[:insertIdx]...)
	result = append(result, cacheFrom, cacheTo)
	result = append(result, args[insertIdx:]...)

	log.Debug().
		Str("cache_dir", BuildKitCacheDir).
		Msg("Injected compose BuildKit cache flags")

	return result
}

// cleanupTmpFiles removes temporary files
func cleanupTmpFiles(files []string) {
	for _, f := range files {
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			log.Warn().Err(err).Str("file", f).Msg("Failed to remove temp file")
		}
	}
}
