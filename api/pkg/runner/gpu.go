package runner

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// GPU is a collection of functions that help with GPU management

type GPUManager struct {
	hasGPU        bool
	gpuMemory     uint64
	freeMemory    uint64
	runnerOptions *Options
	devCPUOnly    bool // Flag to indicate that we are in development CPU only mode
}

func NewGPUManager(ctx context.Context, runnerOptions *Options) *GPUManager {
	g := &GPUManager{
		runnerOptions: runnerOptions,
		// Check both environment variable and Options struct for DEVELOPMENT_CPU_ONLY
		devCPUOnly: strings.ToLower(getEnvOrDefault("DEVELOPMENT_CPU_ONLY", "false", runnerOptions)) == "true" || runnerOptions.DevelopmentCPUOnly,
	}

	// These are slow, but run on startup so it's probably fine
	g.hasGPU = g.detectGPU()
	g.gpuMemory = g.fetchTotalMemory()

	// In dev CPU mode, log the configuration
	if g.devCPUOnly {
		log.Info().
			Bool("development_cpu_only", true).
			Uint64("simulated_gpu_memory", g.gpuMemory).
			Msg("Running in development CPU-only mode")
	}

	// Start a background goroutine to refresh the free memory. We need to do this because it takes
	// about 8 seconds to query nvidia-smi, so on hot paths that's just too long.
	go func() {
		for {
			// Keep spinning until the context is cancelled
			select {
			case <-ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
				g.freeMemory = g.fetchFreeMemory()
			}
		}
	}()
	return g
}

func (g *GPUManager) detectGPU() bool {
	// If in development CPU-only mode, pretend we have a GPU
	if g.devCPUOnly {
		return true
	}

	switch runtime.GOOS {
	case "linux":
		// Check for nvidia-smi
		if _, err := exec.LookPath("nvidia-smi"); err == nil {
			return true
		}
	case "darwin":
		return true
	case "windows":
		log.Error().Msg("Windows not yet supported, please get in touch if you need this")
		return false
	}
	return false
}

func (g *GPUManager) GetFreeMemory() uint64 {
	return g.freeMemory
}

func (g *GPUManager) fetchFreeMemory() uint64 {
	if !g.hasGPU && !g.devCPUOnly {
		return 0
	}

	// In development CPU-only mode, just use the total memory as free memory
	if g.devCPUOnly {
		return g.gpuMemory
	}

	// Default to the user set max memory value
	freeMemory := g.runnerOptions.MemoryBytes

	switch runtime.GOOS {
	case "linux":
		// We can't use memory.free because it's based on the actual GPU memory. The user may have
		// chosen to specify a lesser value, so we need to calculate the virtual free memory.
		cmd := exec.Command("nvidia-smi", "--query-gpu=memory.used", "--format=csv,noheader,nounits")
		connectCmdStdErrToLogger(cmd)
		output, err := cmd.Output()
		if err == nil {
			if used, err := strconv.ParseUint(strings.TrimSpace(string(output)), 10, 64); err == nil {
				actualUsedMemory := used * 1024 * 1024 // Convert MiB to bytes
				virtualFreeMemory := g.gpuMemory - actualUsedMemory
				if virtualFreeMemory < freeMemory {
					freeMemory = virtualFreeMemory
				}
			}
		}
	case "darwin":
		arch, err := getMacArchitecture()
		if err != nil {
			log.Error().Err(err).Msg("failed to get Mac architecture")
			freeMemory = 0
		}

		switch arch {
		case MacArchitectureIntel:
			log.Error().Msg("Intel Mac architecture not supported, please get in touch if you need this")
			freeMemory = 0
		case MacArchitectureApple:
			// If it is an Apple Silicon based mac, then it's unified memory, so just return the
			// amount of free memory
			free, err := getMacSiliconFreeMemory()
			if err != nil {
				log.Error().Err(err).Msg("failed to get Mac free memory")
				return 0
			}
			if free < freeMemory {
				freeMemory = free
			}
		}
	case "windows":
		log.Error().Msg("Windows not yet supported, please get in touch if you need this")
		freeMemory = 0
	}
	return freeMemory
}

// Use a static value for the total memory, because that isn't going to change
func (g *GPUManager) GetTotalMemory() uint64 {
	return g.gpuMemory
}

func (g *GPUManager) fetchTotalMemory() uint64 {
	totalMemory := g.getActualTotalMemory()

	// If the user has manually set the total memory, then use that
	// But make sure it is less than the actual total memory
	if g.runnerOptions.MemoryBytes > 0 && (g.runnerOptions.MemoryBytes < totalMemory || g.devCPUOnly) {
		totalMemory = g.runnerOptions.MemoryBytes
	}

	return totalMemory
}

func (g *GPUManager) getActualTotalMemory() uint64 {
	if !g.hasGPU && !g.devCPUOnly {
		return 0
	}

	// In development CPU-only mode, use system memory
	if g.devCPUOnly {
		// Get system memory based on platform
		var systemMemory uint64

		switch runtime.GOOS {
		case "linux":
			// Read from /proc/meminfo
			cmd := exec.Command("grep", "MemTotal", "/proc/meminfo")
			connectCmdStdErrToLogger(cmd)
			output, err := cmd.Output()
			if err == nil {
				// Example output: MemTotal:       16333764 kB
				fields := strings.Fields(string(output))
				if len(fields) >= 2 {
					if mem, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
						systemMemory = mem * 1024 // Convert kB to bytes
						log.Info().
							Str("platform", "linux").
							Uint64("system_memory_bytes", systemMemory).
							Msg("Using actual system memory for development CPU-only mode")
						return systemMemory
					}
				}
			}
			// If we couldn't read system memory, log an error and return a reasonable default
			log.Error().Msg("Failed to read system memory from /proc/meminfo, using 16GB default")
			return 16 * 1024 * 1024 * 1024 // 16GB default as fallback

		case "darwin":
			// Use the same approach we use for Mac Silicon
			cmd := exec.Command("sysctl", "hw.memsize")
			connectCmdStdErrToLogger(cmd)
			output, err := cmd.Output()
			if err == nil {
				// Example output: hw.memsize: 17179869184
				parts := strings.Split(string(output), ":")
				if len(parts) == 2 {
					if total, err := strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 64); err == nil {
						systemMemory = total
						log.Info().
							Str("platform", "darwin").
							Uint64("system_memory_bytes", systemMemory).
							Msg("Using actual system memory for development CPU-only mode")
						return systemMemory
					}
				}
			}
			log.Error().Msg("Failed to read system memory on macOS, using 16GB default")
			return 16 * 1024 * 1024 * 1024 // 16GB default as fallback

		case "windows":
			// Use a simple default for Windows - we don't develop on Windows
			log.Info().Msg("Windows platform detected, using 16GB default for development CPU-only mode")
			return 16 * 1024 * 1024 * 1024 // 16GB default
		}

		// Fallback if we couldn't determine the system memory
		log.Warn().Msg("Could not determine system memory, using 16GB default for development CPU-only mode")
		return 16 * 1024 * 1024 * 1024 // 16GB default
	}

	switch runtime.GOOS {
	case "linux":
		cmd := exec.Command("nvidia-smi", "--query-gpu=memory.total", "--format=csv,noheader,nounits")
		connectCmdStdErrToLogger(cmd)
		output, err := cmd.Output()
		if err == nil {
			if total, err := strconv.ParseUint(strings.TrimSpace(string(output)), 10, 64); err == nil {
				return total * 1024 * 1024 // Convert MiB to bytes
			}
		}
	case "darwin":
		arch, err := getMacArchitecture()
		if err != nil {
			log.Error().Err(err).Msg("failed to get Mac architecture")
			return 0
		}

		switch arch {
		// If it is an intel based mac, try to get any external VRAM from the in-built GPU
		case MacArchitectureIntel:
			log.Error().Msg("Intel Mac architecture not supported, please get in touch if you need this")
			return 0
		case MacArchitectureApple:
			// If it is an Apple Silicon based mac, then it's unified memory, so just return the
			// total memory
			cmd := exec.Command("sysctl", "hw.memsize")
			connectCmdStdErrToLogger(cmd)
			output, err := cmd.Output()
			if err == nil {
				// Example output: hw.memsize: 17179869184
				parts := strings.Split(string(output), ":")
				if len(parts) == 2 {
					if total, err := strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 64); err == nil {
						return total
					}
				}
			}
		}
	case "windows":
		log.Error().Msg("Windows not yet supported, please get in touch if you need this")
	}
	return 0
}

type MacArchitecture string

const (
	MacArchitectureIntel MacArchitecture = "x86_64"
	MacArchitectureApple MacArchitecture = "arm64"
)

func getMacArchitecture() (MacArchitecture, error) {
	cmd := exec.Command("sysctl", "-n", "hw.machine")
	connectCmdStdErrToLogger(cmd)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get Mac architecture: %w", err)
	}

	arch := strings.TrimSpace(string(output))
	switch arch {
	case string(MacArchitectureIntel):
		return MacArchitectureIntel, nil
	case string(MacArchitectureApple):
		return MacArchitectureApple, nil
	default:
		return "", fmt.Errorf("unknown Mac architecture: %s", arch)
	}
}

func getMacSiliconFreeMemory() (uint64, error) {
	cmd := exec.Command("sysctl", "-n", "hw.pagesize")
	connectCmdStdErrToLogger(cmd)
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to get Mac free memory: %w", err)
	}
	pageSize := strings.TrimSpace(string(output))

	cmd = exec.Command("vm_stat")
	connectCmdStdErrToLogger(cmd)
	output, err = cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to get Mac free memory: %w", err)
	}

	var freePages string
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "Pages free:") {
			freePages = strings.TrimSpace(strings.Split(line, ":")[1])
			// Remove trailing period if present
			freePages = strings.TrimSuffix(freePages, ".")
			break
		}
	}

	if freePages == "" {
		return 0, fmt.Errorf("failed to find free pages in vm_stat output")
	}

	pageSizeInt, err := strconv.ParseUint(pageSize, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to get Mac free memory: %w", err)
	}

	freePagesInt, err := strconv.ParseUint(freePages, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to get Mac free memory: %w", err)
	}

	return freePagesInt * pageSizeInt, nil
}

func connectCmdStdErrToLogger(cmd *exec.Cmd) {
	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Error().Err(err).Msg("failed to get stderr pipe")
		return
	}
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			log.Error().Msg(scanner.Text())
		}
	}()
}

// Helper function to get an environment variable with a default value
// Also checks the Options struct for the value
func getEnvOrDefault(key, defaultValue string, options *Options) string {
	value := os.Getenv(key)
	if value == "" {
		// For specific keys, check the Options struct
		if key == "DEVELOPMENT_CPU_ONLY" && options != nil && options.DevelopmentCPUOnly {
			return "true"
		}
		return defaultValue
	}
	return value
}
