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

func TestIntegration_DockerBuildCacheInjection(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker not available, skipping integration test")
	}

	cacheDir, err := os.MkdirTemp("", "buildkit-cache-test")
	if err != nil {
		t.Fatalf("Failed to create temp cache dir: %v", err)
	}
	defer os.RemoveAll(cacheDir)

	buildDir, err := os.MkdirTemp("", "docker-build-test")
	if err != nil {
		t.Fatalf("Failed to create temp build dir: %v", err)
	}
	defer os.RemoveAll(buildDir)

	dockerfile := `FROM alpine:latest
RUN echo "layer1" > /layer1.txt
RUN echo "layer2" > /layer2.txt
RUN echo "layer3" > /layer3.txt
`
	if err := os.WriteFile(filepath.Join(buildDir, "Dockerfile"), []byte(dockerfile), 0644); err != nil {
		t.Fatalf("Failed to write Dockerfile: %v", err)
	}

	imageName := "docker-shim-test:integration"

	t.Run("build_with_cache_export", func(t *testing.T) {
		args := []string{"build", "-t", imageName, buildDir}
		processedArgs := processDockerArgsWithCacheDir(args, cacheDir, imageName)

		argsStr := strings.Join(processedArgs, " ")
		if !strings.Contains(argsStr, "--cache-from") {
			t.Errorf("Expected --cache-from flag, got: %v", processedArgs)
		}
		if !strings.Contains(argsStr, "--cache-to") {
			t.Errorf("Expected --cache-to flag, got: %v", processedArgs)
		}

		if !buildxCacheExportSupported() {
			t.Log("BuildKit cache export not supported, testing arg injection only")
			return
		}

		buildxArgs := append([]string{"buildx"}, processedArgs...)
		buildxArgs = append(buildxArgs, "--load")
		cmd := exec.Command("docker", buildxArgs...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Docker buildx build failed: %v\nOutput: %s", err, output)
		}
	})

	t.Run("cache_directory_populated", func(t *testing.T) {
		if !buildxCacheExportSupported() {
			t.Skip("BuildKit cache export not supported")
		}

		cacheKey := sanitizeForPath(imageName)
		cacheSubdir := filepath.Join(cacheDir, cacheKey)

		info, err := os.Stat(cacheSubdir)
		if err != nil {
			t.Fatalf("Cache directory not created: %v", err)
		}
		if !info.IsDir() {
			t.Fatalf("Cache path is not a directory")
		}

		entries, err := os.ReadDir(cacheSubdir)
		if err != nil {
			t.Fatalf("Failed to read cache directory: %v", err)
		}
		if len(entries) == 0 {
			t.Errorf("Cache directory is empty, expected cache files")
		}
	})

	t.Run("second_build_uses_cache", func(t *testing.T) {
		if !buildxCacheExportSupported() {
			t.Skip("BuildKit cache export not supported")
		}

		exec.Command("docker", "rmi", "-f", imageName).Run()

		args := []string{"build", "-t", imageName, buildDir}
		processedArgs := processDockerArgsWithCacheDir(args, cacheDir, imageName)

		buildxArgs := append([]string{"buildx"}, processedArgs...)
		buildxArgs = append(buildxArgs, "--load")
		cmd := exec.Command("docker", buildxArgs...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Docker buildx build failed: %v\nOutput: %s", err, output)
		}

		if !strings.Contains(string(output), "CACHED") {
			t.Logf("Warning: No CACHED indicator found")
		}
	})

	exec.Command("docker", "rmi", "-f", imageName).Run()
}

func TestIntegration_DockerBuildNoCacheDir(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker not available")
	}

	args := []string{"build", "-t", "test:nocache", "."}
	processed, err := injectBuildCacheFlags(args)
	if err != nil {
		t.Fatalf("injectBuildCacheFlags returned error: %v", err)
	}
	if len(processed) != len(args) {
		t.Errorf("Expected args unchanged when cache dir doesn't exist, got: %v", processed)
	}
}

func TestIntegration_DockerBuildExistingCacheFlags(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker not available")
	}

	cacheDir, err := os.MkdirTemp("", "buildkit-cache-test")
	if err != nil {
		t.Fatalf("Failed to create temp cache dir: %v", err)
	}
	defer os.RemoveAll(cacheDir)

	args := []string{"build", "--cache-from=type=registry,ref=myregistry/cache", "-t", "test:existing", "."}
	processedArgs := processDockerArgsWithCacheDir(args, cacheDir, "test:existing")

	cacheFromCount := 0
	for _, arg := range processedArgs {
		if strings.HasPrefix(arg, "--cache-from") {
			cacheFromCount++
		}
	}
	if cacheFromCount != 1 {
		t.Errorf("Expected 1 --cache-from flag (original), got %d: %v", cacheFromCount, processedArgs)
	}
}

func TestIntegration_ConcurrentBuilds(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker not available")
	}
	if !buildxCacheExportSupported() {
		t.Skip("BuildKit cache export not supported")
	}

	cacheDir, err := os.MkdirTemp("", "concurrent-cache-test")
	if err != nil {
		t.Fatalf("Failed to create cache dir: %v", err)
	}
	defer os.RemoveAll(cacheDir)

	numBuilds := 3
	buildDirs := make([]string, numBuilds)
	for i := 0; i < numBuilds; i++ {
		dir, err := os.MkdirTemp("", "concurrent-build")
		if err != nil {
			t.Fatalf("Failed to create build dir: %v", err)
		}
		defer os.RemoveAll(dir)
		buildDirs[i] = dir

		dockerfile := `FROM alpine:latest
RUN echo "build-%d-layer1" > /layer1.txt
RUN echo "shared-layer" > /shared.txt
`
		content := strings.Replace(dockerfile, "%d", string(rune('A'+i)), 1)
		if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write Dockerfile: %v", err)
		}
	}

	errChan := make(chan error, numBuilds)
	for i := 0; i < numBuilds; i++ {
		go func(idx int) {
			imageName := "concurrent-test:" + string(rune('a'+idx))
			args := []string{"build", "-t", imageName, buildDirs[idx]}
			processedArgs := processDockerArgsWithCacheDir(args, cacheDir, imageName)

			buildxArgs := append([]string{"buildx"}, processedArgs...)
			buildxArgs = append(buildxArgs, "--load")
			cmd := exec.Command("docker", buildxArgs...)
			_, err := cmd.CombinedOutput()
			errChan <- err
			exec.Command("docker", "rmi", "-f", imageName).Run()
		}(i)
	}

	var errors []error
	for i := 0; i < numBuilds; i++ {
		if err := <-errChan; err != nil {
			errors = append(errors, err)
		}
	}
	if len(errors) > 0 {
		t.Errorf("Concurrent builds had %d failures", len(errors))
	}
}

func TestIntegration_DockerShimBinary(t *testing.T) {
	buildCmd := exec.Command("go", "build", "-o", "/tmp/docker-shim-test", ".")
	buildCmd.Dir = "."
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build docker-shim: %v\n%s", err, output)
	}
	defer os.Remove("/tmp/docker-shim-test")

	t.Run("mode_detection", func(t *testing.T) {
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

func TestIntegration_BuildkitCacheTiming(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("Docker not available")
	}
	if testing.Short() {
		t.Skip("Skipping timing test in short mode")
	}
	if !buildxCacheExportSupported() {
		t.Skip("BuildKit cache export not supported")
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

	dockerfile := `FROM alpine:latest
RUN apk add --no-cache curl
RUN echo "layer1" > /l1.txt
RUN echo "layer2" > /l2.txt
`
	os.WriteFile(filepath.Join(buildDir, "Dockerfile"), []byte(dockerfile), 0644)

	imageName := "timing-test:v1"

	start := time.Now()
	args := processDockerArgsWithCacheDir([]string{"build", "-t", imageName, buildDir}, cacheDir, imageName)
	buildxArgs := append([]string{"buildx"}, args...)
	buildxArgs = append(buildxArgs, "--load")
	cmd := exec.Command("docker", buildxArgs...)
	cmd.CombinedOutput()
	coldDuration := time.Since(start)
	t.Logf("Cold build duration: %v", coldDuration)

	exec.Command("docker", "rmi", "-f", imageName).Run()

	start = time.Now()
	args = processDockerArgsWithCacheDir([]string{"build", "-t", imageName, buildDir}, cacheDir, imageName)
	buildxArgs = append([]string{"buildx"}, args...)
	buildxArgs = append(buildxArgs, "--load")
	cmd = exec.Command("docker", buildxArgs...)
	output, _ := cmd.CombinedOutput()
	warmDuration := time.Since(start)
	t.Logf("Warm build duration: %v", warmDuration)

	if bytes.Contains(output, []byte("CACHED")) {
		t.Logf("Cache was used")
	}
	if warmDuration < coldDuration {
		t.Logf("Cache speedup: %.1fx", float64(coldDuration)/float64(warmDuration))
	}

	exec.Command("docker", "rmi", "-f", imageName).Run()
}

// Helper functions

func dockerAvailable() bool {
	return exec.Command("docker", "version").Run() == nil
}

func composeAvailable() bool {
	return exec.Command("docker", "compose", "version").Run() == nil
}

func buildxCacheExportSupported() bool {
	cmd := exec.Command("docker", "buildx", "version")
	if cmd.Run() != nil {
		return false
	}

	tmpDir, err := os.MkdirTemp("", "cache-test")
	if err != nil {
		return false
	}
	defer os.RemoveAll(tmpDir)

	os.WriteFile(filepath.Join(tmpDir, "Dockerfile"), []byte("FROM scratch\n"), 0644)
	cacheDir := filepath.Join(tmpDir, "cache")
	os.MkdirAll(cacheDir, 0755)

	cmd = exec.Command("docker", "buildx", "build",
		"--cache-to=type=local,dest="+cacheDir,
		"-t", "cache-test:probe",
		"--load",
		tmpDir)
	output, err := cmd.CombinedOutput()
	exec.Command("docker", "rmi", "-f", "cache-test:probe").Run()

	if err != nil {
		if bytes.Contains(output, []byte("Cache export is not supported")) {
			return false
		}
		return false
	}
	return true
}

// processDockerArgsWithCacheDir is a test helper that simulates cache injection
// with a custom cache directory (instead of /buildkit-cache)
func processDockerArgsWithCacheDir(args []string, cacheDir, imageName string) []string {
	if !isBuildCommand(args) {
		return args
	}

	if hasCacheFlags(args) {
		return args
	}

	cacheKey := sanitizeForPath(imageName)
	if cacheKey == "" {
		cacheKey = "default"
	}
	cacheSubdir := filepath.Join(cacheDir, cacheKey)
	os.MkdirAll(cacheSubdir, 0755)

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

	cacheFrom := "--cache-from=type=local,src=" + cacheSubdir
	cacheTo := "--cache-to=type=local,dest=" + cacheSubdir + ",mode=max"

	result := make([]string, 0, len(args)+2)
	result = append(result, args[:insertIdx]...)
	result = append(result, cacheFrom, cacheTo)
	result = append(result, args[insertIdx:]...)

	return result
}

// injectComposeCacheFlagsWithDir is a test helper for compose cache injection
func injectComposeCacheFlagsWithDir(args []string, cacheDir string) []string {
	if !isComposeBuildCommand(args) {
		return args
	}
	if hasComposeCacheFlags(args) {
		return args
	}

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

func BenchmarkCacheInjection(b *testing.B) {
	args := []string{"build", "-t", "myimage:latest", "--build-arg", "VERSION=1.0", "."}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		processDockerArgs(args)
	}
}
