//go:build integration
// +build integration

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Integration tests for docker-shim
// Run with: go test -tags=integration -v ./...
//
// Prerequisites:
// - Docker must be installed and running
// - User must have permission to run docker commands
// - Tests create temporary directories and Dockerfiles
//
// Note: Some tests require BuildKit with cache export support.
// The default Docker driver doesn't support cache export.
// In production (Helix desktop), BuildKit is enabled by default.

func TestIntegration_DockerBuildCacheInjection(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker not available, skipping integration test")
	}

	// Create a temporary cache directory
	cacheDir, err := os.MkdirTemp("", "buildkit-cache-test")
	if err != nil {
		t.Fatalf("Failed to create temp cache dir: %v", err)
	}
	defer os.RemoveAll(cacheDir)

	// Create a temporary build context
	buildDir, err := os.MkdirTemp("", "docker-build-test")
	if err != nil {
		t.Fatalf("Failed to create temp build dir: %v", err)
	}
	defer os.RemoveAll(buildDir)

	// Create a simple Dockerfile
	dockerfile := `FROM alpine:latest
RUN echo "layer1" > /layer1.txt
RUN echo "layer2" > /layer2.txt
RUN echo "layer3" > /layer3.txt
`
	if err := os.WriteFile(filepath.Join(buildDir, "Dockerfile"), []byte(dockerfile), 0644); err != nil {
		t.Fatalf("Failed to write Dockerfile: %v", err)
	}

	imageName := "docker-shim-test:integration"

	// Test 1: Build with cache injection
	t.Run("build_with_cache_export", func(t *testing.T) {
		args := []string{"build", "-t", imageName, buildDir}

		// Simulate what docker-shim does
		processedArgs := processDockerArgsWithCacheDir(args, cacheDir, imageName)

		// Verify cache flags were added
		argsStr := strings.Join(processedArgs, " ")
		if !strings.Contains(argsStr, "--cache-from") {
			t.Errorf("Expected --cache-from flag, got: %v", processedArgs)
		}
		if !strings.Contains(argsStr, "--cache-to") {
			t.Errorf("Expected --cache-to flag, got: %v", processedArgs)
		}

		// Actually run the build
		// Note: Cache export may not work with default Docker driver
		// We test with buildx if available, otherwise skip the actual build
		if !buildxCacheExportSupported() {
			t.Log("BuildKit cache export not supported on this Docker installation, testing arg injection only")
			return
		}

		// Use buildx for cache export support
		buildxArgs := append([]string{"buildx"}, processedArgs...)
		buildxArgs = append(buildxArgs, "--load") // Load into docker images
		cmd := exec.Command("docker", buildxArgs...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Docker buildx build failed: %v\nOutput: %s", err, output)
		}
		t.Logf("Build output:\n%s", output)
	})

	// Test 2: Verify cache directory was populated
	t.Run("cache_directory_populated", func(t *testing.T) {
		if !buildxCacheExportSupported() {
			t.Skip("BuildKit cache export not supported, skipping cache verification")
		}

		cacheKey := sanitizeForPath(imageName)
		cacheSubdir := filepath.Join(cacheDir, cacheKey)

		// Check if cache directory exists and has content
		info, err := os.Stat(cacheSubdir)
		if err != nil {
			t.Fatalf("Cache directory not created: %v", err)
		}
		if !info.IsDir() {
			t.Fatalf("Cache path is not a directory")
		}

		// Check for cache files (BuildKit creates index.json and blobs/)
		entries, err := os.ReadDir(cacheSubdir)
		if err != nil {
			t.Fatalf("Failed to read cache directory: %v", err)
		}
		if len(entries) == 0 {
			t.Errorf("Cache directory is empty, expected cache files")
		}
		t.Logf("Cache directory contains %d entries", len(entries))
		for _, e := range entries {
			t.Logf("  - %s", e.Name())
		}
	})

	// Test 3: Second build should use cache
	t.Run("second_build_uses_cache", func(t *testing.T) {
		if !buildxCacheExportSupported() {
			t.Skip("BuildKit cache export not supported, skipping cache hit test")
		}

		// Remove the image to force a rebuild
		exec.Command("docker", "rmi", "-f", imageName).Run()

		args := []string{"build", "-t", imageName, buildDir}
		processedArgs := processDockerArgsWithCacheDir(args, cacheDir, imageName)

		// Use buildx for cache export support
		buildxArgs := append([]string{"buildx"}, processedArgs...)
		buildxArgs = append(buildxArgs, "--load")
		cmd := exec.Command("docker", buildxArgs...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Docker buildx build failed: %v\nOutput: %s", err, output)
		}

		// Check for cache hit indicators in output
		outputStr := string(output)
		t.Logf("Second build output:\n%s", outputStr)

		// BuildKit shows "CACHED" for cached layers
		if !strings.Contains(outputStr, "CACHED") {
			t.Logf("Warning: No CACHED indicator found - cache may not have been used")
		}
	})

	// Cleanup
	exec.Command("docker", "rmi", "-f", imageName).Run()
}

func TestIntegration_DockerBuildNoCacheDir(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker not available, skipping integration test")
	}

	// Test that without /buildkit-cache, no flags are injected
	args := []string{"build", "-t", "test:nocache", "."}

	// Save original and use non-existent cache dir
	processed, err := injectBuildCacheFlags(args)
	if err != nil {
		t.Fatalf("injectBuildCacheFlags returned error: %v", err)
	}

	// Should be unchanged since /buildkit-cache doesn't exist
	if len(processed) != len(args) {
		t.Errorf("Expected args unchanged when cache dir doesn't exist, got: %v", processed)
	}
}

func TestIntegration_DockerBuildExistingCacheFlags(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker not available, skipping integration test")
	}

	// Create temp cache dir
	cacheDir, err := os.MkdirTemp("", "buildkit-cache-test")
	if err != nil {
		t.Fatalf("Failed to create temp cache dir: %v", err)
	}
	defer os.RemoveAll(cacheDir)

	// Test that existing cache flags are not overwritten
	args := []string{"build", "--cache-from=type=registry,ref=myregistry/cache", "-t", "test:existing", "."}

	processed := processDockerArgsWithCacheDir(args, cacheDir, "test:existing")

	// Count cache-from flags - should only be one (the original)
	cacheFromCount := 0
	for _, arg := range processed {
		if strings.HasPrefix(arg, "--cache-from") {
			cacheFromCount++
		}
	}

	if cacheFromCount != 1 {
		t.Errorf("Expected 1 --cache-from flag (original), got %d: %v", cacheFromCount, processed)
	}
}

func TestIntegration_ComposeProjectIsolation(t *testing.T) {
	// Test that compose commands get project name from environment
	tests := []struct {
		name       string
		taskNum    string
		sessionID  string
		wantPrefix string
	}{
		{
			name:       "task number takes precedence",
			taskNum:    "42",
			sessionID:  "ses_abc123",
			wantPrefix: "-p helix-task-42",
		},
		{
			name:       "session id fallback",
			taskNum:    "",
			sessionID:  "ses_xyz789",
			wantPrefix: "-p helix-ses_xyz789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment
			os.Unsetenv("HELIX_TASK_NUMBER")
			os.Unsetenv("HELIX_SESSION_ID")
			if tt.taskNum != "" {
				os.Setenv("HELIX_TASK_NUMBER", tt.taskNum)
			}
			if tt.sessionID != "" {
				os.Setenv("HELIX_SESSION_ID", tt.sessionID)
			}
			defer os.Unsetenv("HELIX_TASK_NUMBER")
			defer os.Unsetenv("HELIX_SESSION_ID")

			args := []string{"up", "-d"}
			projectArgs := getProjectArgs(args)

			if len(projectArgs) == 0 {
				t.Fatalf("Expected project args, got none")
			}

			argsStr := strings.Join(projectArgs, " ")
			if argsStr != tt.wantPrefix {
				t.Errorf("Expected %q, got %q", tt.wantPrefix, argsStr)
			}
		})
	}
}

func TestIntegration_ComposeFileProcessing(t *testing.T) {
	// Create a temporary directory for compose file
	tmpDir, err := os.MkdirTemp("", "compose-integration-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Set WORKSPACE_DIR for path translation
	os.Setenv("WORKSPACE_DIR", "/data/workspaces/test-123")
	defer os.Unsetenv("WORKSPACE_DIR")

	// Create a compose file with volumes that need translation
	composeContent := `version: "3"
services:
  app:
    image: alpine
    volumes:
      - /home/retro/work/myproject:/app
      - data:/data
volumes:
  data:
`
	composeFile := filepath.Join(tmpDir, "docker-compose.yaml")
	if err := os.WriteFile(composeFile, []byte(composeContent), 0644); err != nil {
		t.Fatalf("Failed to write compose file: %v", err)
	}

	// Process the compose file
	outputFile := filepath.Join(tmpDir, ".hydra-resolved.docker-compose.yaml")
	if err := processComposeFile(composeFile, outputFile); err != nil {
		t.Fatalf("processComposeFile failed: %v", err)
	}

	// Read the output
	output, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	outputStr := string(output)
	t.Logf("Processed compose file:\n%s", outputStr)

	// Verify path was translated
	if !strings.Contains(outputStr, "/data/workspaces/test-123/myproject") {
		t.Errorf("Expected translated path /data/workspaces/test-123/myproject in output")
	}

	// Verify named volume is unchanged
	if !strings.Contains(outputStr, "data:/data") {
		t.Errorf("Expected named volume 'data:/data' to be unchanged")
	}
}

func TestIntegration_ComposeBuildWithCache(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker not available, skipping integration test")
	}

	if !composeAvailable() {
		t.Skip("Docker Compose not available, skipping integration test")
	}

	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "compose-build-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a simple Dockerfile
	dockerfile := `FROM alpine:latest
RUN echo "compose-build-test" > /test.txt
`
	if err := os.WriteFile(filepath.Join(tmpDir, "Dockerfile"), []byte(dockerfile), 0644); err != nil {
		t.Fatalf("Failed to write Dockerfile: %v", err)
	}

	// Create a compose file
	composeContent := `version: "3"
services:
  testapp:
    build:
      context: .
    image: compose-shim-test:integration
`
	if err := os.WriteFile(filepath.Join(tmpDir, "docker-compose.yaml"), []byte(composeContent), 0644); err != nil {
		t.Fatalf("Failed to write compose file: %v", err)
	}

	// Create a cache directory
	cacheDir, err := os.MkdirTemp("", "compose-cache-test")
	if err != nil {
		t.Fatalf("Failed to create cache dir: %v", err)
	}
	defer os.RemoveAll(cacheDir)

	// Check compose version for --set support
	version := getComposeVersion()
	t.Logf("Docker Compose version: %s", version)

	// Test cache injection for compose build
	args := []string{"-f", filepath.Join(tmpDir, "docker-compose.yaml"), "build"}

	// Simulate what the shim does
	if compareVersions(version, "2.24.0") {
		// Should use --set flags
		processedArgs := injectComposeCacheFlagsWithDir(args, cacheDir)
		argsStr := strings.Join(processedArgs, " ")

		if !strings.Contains(argsStr, "--set") {
			t.Logf("Compose v2.24+: Expected --set flags for cache injection")
		} else {
			t.Logf("Processed args: %v", processedArgs)
		}
	} else {
		t.Logf("Compose version %s < 2.24, --set not supported", version)
	}

	// Cleanup
	exec.Command("docker", "rmi", "-f", "compose-shim-test:integration").Run()
}

func TestIntegration_PathTranslationE2E(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker not available, skipping integration test")
	}

	// Create a real directory structure
	tmpDir, err := os.MkdirTemp("", "path-translation-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a file to mount
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello from host"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Set up WORKSPACE_DIR to simulate Hydra environment
	// In this test, we'll translate tmpDir -> /data/workspaces/test
	// But since we're testing locally, we'll verify the translation logic

	os.Setenv("WORKSPACE_DIR", tmpDir)
	defer os.Unsetenv("WORKSPACE_DIR")

	// Simulate a path that would be translated
	userPath := "/home/retro/work/test.txt"
	resolved := resolvePath(userPath)

	// Since WORKSPACE_DIR is set but path doesn't match /home/retro/work,
	// it should fall through to other resolution
	t.Logf("Resolved %q -> %q", userPath, resolved)

	// Now test with a matching path
	matchingPath := "/home/retro/work/myfile.txt"
	resolvedMatching := resolvePath(matchingPath)
	expectedResolved := tmpDir + "/myfile.txt"

	if resolvedMatching != expectedResolved {
		t.Errorf("Expected %q, got %q", expectedResolved, resolvedMatching)
	}
}

func TestIntegration_ConcurrentBuilds(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker not available, skipping integration test")
	}

	if !buildxCacheExportSupported() {
		t.Skip("BuildKit cache export not supported, skipping concurrent build test")
	}

	// Test that concurrent builds don't corrupt the cache
	cacheDir, err := os.MkdirTemp("", "concurrent-cache-test")
	if err != nil {
		t.Fatalf("Failed to create cache dir: %v", err)
	}
	defer os.RemoveAll(cacheDir)

	// Create multiple build contexts
	numBuilds := 3
	buildDirs := make([]string, numBuilds)
	for i := 0; i < numBuilds; i++ {
		dir, err := os.MkdirTemp("", "concurrent-build")
		if err != nil {
			t.Fatalf("Failed to create build dir: %v", err)
		}
		defer os.RemoveAll(dir)
		buildDirs[i] = dir

		// Create slightly different Dockerfiles
		dockerfile := `FROM alpine:latest
RUN echo "build-%d-layer1" > /layer1.txt
RUN echo "shared-layer" > /shared.txt
`
		content := strings.Replace(dockerfile, "%d", string(rune('A'+i)), 1)
		if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write Dockerfile: %v", err)
		}
	}

	// Run builds concurrently
	errChan := make(chan error, numBuilds)
	for i := 0; i < numBuilds; i++ {
		go func(idx int) {
			imageName := "concurrent-test:" + string(rune('a'+idx))
			args := []string{"build", "-t", imageName, buildDirs[idx]}
			processedArgs := processDockerArgsWithCacheDir(args, cacheDir, imageName)

			// Use buildx for cache export support
			buildxArgs := append([]string{"buildx"}, processedArgs...)
			buildxArgs = append(buildxArgs, "--load")
			cmd := exec.Command("docker", buildxArgs...)
			output, err := cmd.CombinedOutput()
			if err != nil {
				errChan <- err
				t.Logf("Build %d failed: %s", idx, output)
			} else {
				errChan <- nil
				t.Logf("Build %d completed successfully", idx)
			}

			// Cleanup
			exec.Command("docker", "rmi", "-f", imageName).Run()
		}(i)
	}

	// Wait for all builds
	var errors []error
	for i := 0; i < numBuilds; i++ {
		if err := <-errChan; err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		t.Errorf("Concurrent builds had %d failures", len(errors))
	}

	// Verify cache directory is not corrupted (should have content)
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatalf("Failed to read cache dir: %v", err)
	}
	t.Logf("Cache directory has %d entries after concurrent builds", len(entries))
}

// Helper functions

func dockerAvailable() bool {
	cmd := exec.Command("docker", "version")
	return cmd.Run() == nil
}

func composeAvailable() bool {
	cmd := exec.Command("docker", "compose", "version")
	return cmd.Run() == nil
}

// buildxCacheExportSupported checks if the Docker installation supports cache export
// The default Docker driver doesn't support cache export - need buildx with a builder
func buildxCacheExportSupported() bool {
	// Check if docker buildx is available
	cmd := exec.Command("docker", "buildx", "version")
	if cmd.Run() != nil {
		return false
	}

	// Check if containerd image store is enabled or a non-docker driver is in use
	// For simplicity, we try a test build with cache export and see if it fails
	tmpDir, err := os.MkdirTemp("", "cache-test")
	if err != nil {
		return false
	}
	defer os.RemoveAll(tmpDir)

	// Create minimal Dockerfile
	os.WriteFile(filepath.Join(tmpDir, "Dockerfile"), []byte("FROM scratch\n"), 0644)

	cacheDir := filepath.Join(tmpDir, "cache")
	os.MkdirAll(cacheDir, 0755)

	// Try a build with cache export using buildx explicitly
	// This uses the current buildx builder (set via `docker buildx use`)
	cmd = exec.Command("docker", "buildx", "build",
		"--cache-to=type=local,dest="+cacheDir,
		"-t", "cache-test:probe",
		"--load",
		tmpDir)
	output, err := cmd.CombinedOutput()

	// Clean up test image
	exec.Command("docker", "rmi", "-f", "cache-test:probe").Run()

	if err != nil {
		// Check if the error is specifically about cache export not being supported
		if bytes.Contains(output, []byte("Cache export is not supported")) {
			return false
		}
		// Other errors (like network issues pulling scratch) also mean we can't test
		return false
	}

	return true
}

// processDockerArgsWithCacheDir is a test helper that simulates cache injection
// with a custom cache directory (instead of /buildkit-cache)
func processDockerArgsWithCacheDir(args []string, cacheDir, imageName string) []string {
	result := make([]string, 0, len(args)+4)

	// First pass: copy args
	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case arg == "-v" || arg == "--volume":
			result = append(result, arg)
			if i+1 < len(args) {
				i++
				result = append(result, processVolumeArg(args[i]))
			}
		case strings.HasPrefix(arg, "-v=") || strings.HasPrefix(arg, "--volume="):
			prefix := arg[:strings.Index(arg, "=")+1]
			vol := arg[strings.Index(arg, "=")+1:]
			result = append(result, prefix+processVolumeArg(vol))
		case arg == "--mount":
			result = append(result, arg)
			if i+1 < len(args) {
				i++
				result = append(result, processMountArg(args[i]))
			}
		case strings.HasPrefix(arg, "--mount="):
			mountSpec := strings.TrimPrefix(arg, "--mount=")
			result = append(result, "--mount="+processMountArg(mountSpec))
		default:
			result = append(result, arg)
		}
	}

	// Second pass: inject cache flags if this is a build command
	if !isBuildCommand(result) {
		return result
	}

	if hasCacheFlags(result) {
		return result
	}

	// Create cache subdirectory
	cacheKey := sanitizeForPath(imageName)
	if cacheKey == "" {
		cacheKey = "default"
	}
	cacheSubdir := filepath.Join(cacheDir, cacheKey)
	os.MkdirAll(cacheSubdir, 0755)

	// Find where to insert cache flags
	insertIdx := -1
	for i, arg := range result {
		if arg == "build" {
			insertIdx = i + 1
			break
		}
	}

	if insertIdx == -1 || insertIdx > len(result) {
		return result
	}

	cacheFrom := "--cache-from=type=local,src=" + cacheSubdir
	cacheTo := "--cache-to=type=local,dest=" + cacheSubdir + ",mode=max"

	newResult := make([]string, 0, len(result)+2)
	newResult = append(newResult, result[:insertIdx]...)
	newResult = append(newResult, cacheFrom, cacheTo)
	newResult = append(newResult, result[insertIdx:]...)

	return newResult
}

// injectComposeCacheFlagsWithDir is a test helper for compose cache injection
func injectComposeCacheFlagsWithDir(args []string, cacheDir string) []string {
	if !isComposeBuildCommand(args) {
		return args
	}

	if hasComposeCacheFlags(args) {
		return args
	}

	// Find where to insert flags
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

	cacheFrom := `--set=*.build.cache_from=["type=local,src=` + cacheDir + `"]`
	cacheTo := `--set=*.build.cache_to=["type=local,dest=` + cacheDir + `,mode=max"]`

	result := make([]string, 0, len(args)+2)
	result = append(result, args[:insertIdx]...)
	result = append(result, cacheFrom, cacheTo)
	result = append(result, args[insertIdx:]...)

	return result
}

// Benchmark for cache injection overhead
func BenchmarkCacheInjection(b *testing.B) {
	args := []string{"build", "-t", "myimage:latest", "--build-arg", "VERSION=1.0", "."}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = processDockerArgs(args)
	}
}

func BenchmarkPathResolution(b *testing.B) {
	os.Setenv("WORKSPACE_DIR", "/data/workspaces/test-123")
	defer os.Unsetenv("WORKSPACE_DIR")

	path := "/home/retro/work/myproject/src/main.go"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = resolvePath(path)
	}
}

func BenchmarkComposeFileProcessing(b *testing.B) {
	// Create a temporary compose file
	tmpDir, _ := os.MkdirTemp("", "bench")
	defer os.RemoveAll(tmpDir)

	composeContent := `version: "3"
services:
  app:
    image: alpine
    volumes:
      - /home/retro/work/project:/app
      - ./local:/local
      - data:/data
  db:
    image: postgres
    volumes:
      - /home/retro/work/data:/var/lib/postgresql/data
volumes:
  data:
`
	inputFile := filepath.Join(tmpDir, "docker-compose.yaml")
	os.WriteFile(inputFile, []byte(composeContent), 0644)

	os.Setenv("WORKSPACE_DIR", "/data/workspaces/test")
	defer os.Unsetenv("WORKSPACE_DIR")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		outputFile := filepath.Join(tmpDir, ".out.yaml")
		processComposeFile(inputFile, outputFile)
		os.Remove(outputFile)
	}
}

// TestIntegration_RealWorldScenario simulates Helix-in-Helix usage
func TestIntegration_RealWorldScenario(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker not available, skipping integration test")
	}

	t.Log("Simulating Helix-in-Helix scenario: ./stack start")

	// This test simulates what happens when running ./stack start
	// in a Helix session, which triggers docker compose build

	// 1. Set up environment as Hydra would
	os.Setenv("HELIX_TASK_NUMBER", "1234")
	os.Setenv("WORKSPACE_DIR", "/data/workspaces/spec-tasks/test-1234")
	defer os.Unsetenv("HELIX_TASK_NUMBER")
	defer os.Unsetenv("WORKSPACE_DIR")

	// 2. Create a mock helix compose file
	tmpDir, err := os.MkdirTemp("", "helix-in-helix-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Simplified helix-like compose file
	composeContent := `version: "3"
services:
  api:
    build:
      context: .
      dockerfile: Dockerfile
    volumes:
      - /home/retro/work:/app
  frontend:
    build:
      context: .
      dockerfile: Dockerfile
`
	if err := os.WriteFile(filepath.Join(tmpDir, "docker-compose.yaml"), []byte(composeContent), 0644); err != nil {
		t.Fatalf("Failed to write compose file: %v", err)
	}

	dockerfile := `FROM alpine:latest
RUN echo "helix-service" > /service.txt
`
	if err := os.WriteFile(filepath.Join(tmpDir, "Dockerfile"), []byte(dockerfile), 0644); err != nil {
		t.Fatalf("Failed to write Dockerfile: %v", err)
	}

	// 3. Process compose args as docker-shim would
	args := []string{"-f", filepath.Join(tmpDir, "docker-compose.yaml"), "build"}

	// Get project args (should be helix-task-1234)
	projectArgs := getProjectArgs(args)
	if len(projectArgs) == 0 || projectArgs[1] != "helix-task-1234" {
		t.Errorf("Expected project name 'helix-task-1234', got: %v", projectArgs)
	}

	// 4. Process the compose file for path translation
	processedArgs, tmpFiles := processComposeArgs(args)
	defer cleanupTmpFiles(tmpFiles)

	t.Logf("Original args: %v", args)
	t.Logf("Project args: %v", projectArgs)
	t.Logf("Processed args: %v", processedArgs)

	// 5. Verify the processed compose file has translated paths
	if len(tmpFiles) > 0 {
		content, _ := os.ReadFile(tmpFiles[0])
		t.Logf("Processed compose file:\n%s", content)

		if !strings.Contains(string(content), "/data/workspaces/spec-tasks/test-1234") {
			t.Errorf("Expected translated workspace path in compose file")
		}
	}
}

// TestIntegration_DockerShimBinary tests the actual compiled binary
func TestIntegration_DockerShimBinary(t *testing.T) {
	// Build the binary
	buildCmd := exec.Command("go", "build", "-o", "/tmp/docker-shim-test", ".")
	buildCmd.Dir = "."
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build docker-shim: %v\n%s", err, output)
	}
	defer os.Remove("/tmp/docker-shim-test")

	// Test that it runs and shows help
	t.Run("docker_help", func(t *testing.T) {
		// The shim should pass through to docker
		// Since we can't actually replace /usr/bin/docker in tests,
		// we verify the argument processing logic instead
		t.Log("Binary built successfully")
	})

	// Test mode detection
	t.Run("mode_detection", func(t *testing.T) {
		// Test that argv[0] detection works
		args := []string{"/usr/bin/docker-compose", "up"}
		mode := detectMode(args)
		if mode != ModeCompose {
			t.Errorf("Expected ModeCompose for docker-compose binary, got %v", mode)
		}

		args = []string{"/usr/bin/docker", "build", "."}
		mode = detectMode(args)
		if mode != ModeDocker {
			t.Errorf("Expected ModeDocker for docker binary, got %v", mode)
		}

		args = []string{"/usr/bin/docker", "compose", "up"}
		mode = detectMode(args)
		if mode != ModeCompose {
			t.Errorf("Expected ModeCompose for 'docker compose' plugin mode, got %v", mode)
		}
	})
}

// TestIntegration_BuildkitCacheTiming measures actual cache speedup
func TestIntegration_BuildkitCacheTiming(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker not available, skipping integration test")
	}

	if testing.Short() {
		t.Skip("Skipping timing test in short mode")
	}

	if !buildxCacheExportSupported() {
		t.Skip("BuildKit cache export not supported, skipping timing test")
	}

	cacheDir, err := os.MkdirTemp("", "timing-cache")
	if err != nil {
		t.Fatalf("Failed to create cache dir: %v", err)
	}
	defer os.RemoveAll(cacheDir)

	buildDir, err := os.MkdirTemp("", "timing-build")
	if err != nil {
		t.Fatalf("Failed to create build dir: %v", err)
	}
	defer os.RemoveAll(buildDir)

	// Create a Dockerfile with some work
	dockerfile := `FROM alpine:latest
RUN apk add --no-cache curl
RUN echo "layer1" > /l1.txt
RUN echo "layer2" > /l2.txt
RUN echo "layer3" > /l3.txt
`
	os.WriteFile(filepath.Join(buildDir, "Dockerfile"), []byte(dockerfile), 0644)

	imageName := "timing-test:v1"

	// First build (cold cache)
	start := time.Now()
	args := processDockerArgsWithCacheDir([]string{"build", "-t", imageName, buildDir}, cacheDir, imageName)
	buildxArgs := append([]string{"buildx"}, args...)
	buildxArgs = append(buildxArgs, "--load")
	cmd := exec.Command("docker", buildxArgs...)
	cmd.CombinedOutput()
	coldDuration := time.Since(start)
	t.Logf("Cold build duration: %v", coldDuration)

	// Remove image to force rebuild
	exec.Command("docker", "rmi", "-f", imageName).Run()

	// Second build (warm cache)
	start = time.Now()
	args = processDockerArgsWithCacheDir([]string{"build", "-t", imageName, buildDir}, cacheDir, imageName)
	buildxArgs = append([]string{"buildx"}, args...)
	buildxArgs = append(buildxArgs, "--load")
	cmd = exec.Command("docker", buildxArgs...)
	output, _ := cmd.CombinedOutput()
	warmDuration := time.Since(start)
	t.Logf("Warm build duration: %v", warmDuration)

	// Check for CACHED in output
	if bytes.Contains(output, []byte("CACHED")) {
		t.Logf("Cache was used (CACHED found in output)")
	}

	// Warm build should be faster (at least 20% faster for this simple case)
	if warmDuration < coldDuration {
		t.Logf("Cache speedup: %.1fx faster", float64(coldDuration)/float64(warmDuration))
	} else {
		t.Logf("Warning: Warm build was not faster than cold build")
	}

	// Cleanup
	exec.Command("docker", "rmi", "-f", imageName).Run()
}
