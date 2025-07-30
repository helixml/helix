package runner

import (
	"bufio"
	"context"
	"fmt"
	"math/rand"
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
	usedMemory    uint64
	runnerOptions *Options
}

func NewGPUManager(ctx context.Context, runnerOptions *Options) *GPUManager {
	g := &GPUManager{
		runnerOptions: runnerOptions,
	}

	// These are slow, but run on startup so it's probably fine
	g.hasGPU = g.detectGPU()
	g.gpuMemory = g.fetchTotalMemory()

	// Fetch free memory once to initialize values before background updates
	g.freeMemory = g.fetchFreeMemory()
	log.Info().
		Bool("has_gpu", g.hasGPU).
		Uint64("total_memory", g.gpuMemory).
		Uint64("free_memory", g.freeMemory).
		Uint64("used_memory", g.usedMemory).
		Msg("GPU manager initialized")

	// In dev CPU mode, log the configuration
	if runnerOptions.DevelopmentCPUOnly {
		log.Info().
			Bool("development_cpu_only", true).
			Uint64("simulated_gpu_memory", g.gpuMemory).
			Msg("Running in development CPU-only mode")
	}

	// Start a background goroutine to refresh the free memory. We need to do this because it takes
	// about 8 seconds to query nvidia-smi, so on hot paths that's just too long.
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				g.freeMemory = g.fetchFreeMemory()
				// Log occasional updates for debugging
				if rand.Intn(20) == 0 { // ~5% chance to log an update
					log.Trace().
						Uint64("total_memory", g.gpuMemory).
						Uint64("free_memory", g.freeMemory).
						Uint64("used_memory", g.usedMemory).
						Msg("GPU memory periodic update")
				}
			}
		}
	}()
	return g
}

func (g *GPUManager) detectGPU() bool {
	// If in development CPU-only mode, pretend we have a GPU
	if g.runnerOptions.DevelopmentCPUOnly {
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

func (g *GPUManager) GetUsedMemory() uint64 {
	return g.usedMemory
}

func (g *GPUManager) fetchFreeMemory() uint64 {
	if !g.hasGPU && !g.runnerOptions.DevelopmentCPUOnly {
		return 0
	}

	// In development CPU-only mode, get actual free system memory
	if g.runnerOptions.DevelopmentCPUOnly {
		// For Linux in dev CPU mode, get actual free system memory from /proc/meminfo
		if runtime.GOOS == "linux" {
			cmd := exec.Command("grep", "MemAvailable", "/proc/meminfo")
			connectCmdStdErrToLogger(cmd)
			output, err := cmd.Output()
			if err == nil {
				// Example output: MemAvailable:    8765432 kB
				fields := strings.Fields(string(output))
				if len(fields) >= 2 {
					if mem, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
						freeMemory := mem * 1024 // Convert kB to bytes

						// Also get the used memory
						cmd = exec.Command("grep", "MemTotal", "/proc/meminfo")
						connectCmdStdErrToLogger(cmd)
						totalOutput, totalErr := cmd.Output()
						if totalErr == nil {
							totalFields := strings.Fields(string(totalOutput))
							if len(totalFields) >= 2 {
								if totalMem, totalErr := strconv.ParseUint(totalFields[1], 10, 64); totalErr == nil {
									totalMemory := totalMem * 1024 // Convert kB to bytes
									g.usedMemory = totalMemory - freeMemory
								}
							}
						}

						log.Trace().
							Str("platform", "linux").
							Uint64("free_memory_bytes", freeMemory).
							Uint64("used_memory_bytes", g.usedMemory).
							Msg("Using actual system memory in development CPU-only mode")
						return freeMemory
					}
				}
			}
			log.Warn().Msg("Failed to read free system memory from /proc/meminfo, falling back to g.gpuMemory")
		}
		g.usedMemory = 0
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
		log.Trace().Msg("Running nvidia-smi to get used memory")
		output, err := cmd.Output()
		if err != nil {
			log.Error().Err(err).Msg("Error running nvidia-smi to get used memory")
			g.usedMemory = 0
		} else {
			log.Trace().Str("nvidia_smi_output", string(output)).Msg("nvidia-smi output for used memory")
			actualUsedMemory := g.parseAndSumGPUMemory(string(output), "used")
			if actualUsedMemory == 0 {
				g.usedMemory = 0
			} else {
				g.usedMemory = actualUsedMemory
				log.Trace().
					Uint64("used_bytes", actualUsedMemory).
					Msg("Successfully parsed GPU used memory across all GPUs")
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
			g.usedMemory = 0
		}

		switch arch {
		case MacArchitectureIntel:
			log.Error().Msg("Intel Mac architecture not supported, please get in touch if you need this")
			freeMemory = 0
			g.usedMemory = 0
		case MacArchitectureApple:
			// If it is an Apple Silicon based mac, then it's unified memory, so just return the
			// amount of free memory
			free, totalMem, err := getMacSiliconMemory()
			if err != nil {
				log.Error().Err(err).Msg("failed to get Mac free memory")
				g.usedMemory = 0
				return 0
			}
			if free < freeMemory {
				freeMemory = free
			}
			g.usedMemory = totalMem - free
		}
	case "windows":
		log.Error().Msg("Windows not yet supported, please get in touch if you need this")
		freeMemory = 0
		g.usedMemory = 0
	default:
		freeMemory = 0
		g.usedMemory = 0
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
	if g.runnerOptions.MemoryBytes > 0 && (g.runnerOptions.MemoryBytes < totalMemory || g.runnerOptions.DevelopmentCPUOnly) {
		totalMemory = g.runnerOptions.MemoryBytes
	}

	return totalMemory
}

func (g *GPUManager) getActualTotalMemory() uint64 {
	if !g.hasGPU && !g.runnerOptions.DevelopmentCPUOnly {
		return 0
	}

	// In development CPU-only mode, use system memory
	if g.runnerOptions.DevelopmentCPUOnly {
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
			return g.parseAndSumGPUMemory(string(output), "total")
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

func getMacSiliconMemory() (uint64, uint64, error) {
	// Get page size
	cmd := exec.Command("sysctl", "-n", "hw.pagesize")
	connectCmdStdErrToLogger(cmd)
	output, err := cmd.Output()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get Mac page size: %w", err)
	}
	pageSize := strings.TrimSpace(string(output))
	pageSizeInt, err := strconv.ParseUint(pageSize, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse page size: %w", err)
	}

	// Get total memory
	cmd = exec.Command("sysctl", "hw.memsize")
	connectCmdStdErrToLogger(cmd)
	output, err = cmd.Output()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get Mac total memory: %w", err)
	}
	parts := strings.Split(string(output), ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("unexpected format for hw.memsize")
	}
	totalMemory, err := strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse total memory: %w", err)
	}

	// Get free memory from vm_stat
	cmd = exec.Command("vm_stat")
	connectCmdStdErrToLogger(cmd)
	output, err = cmd.Output()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get Mac vm_stat: %w", err)
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
		return 0, 0, fmt.Errorf("failed to find free pages in vm_stat output")
	}

	freePagesInt, err := strconv.ParseUint(freePages, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse free pages: %w", err)
	}

	freeMemory := freePagesInt * pageSizeInt
	// Used memory is total memory minus free memory
	return freeMemory, totalMemory, nil
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

// parseAndSumGPUMemory parses nvidia-smi output that may contain multiple lines (one per GPU)
// and sums the memory values across all GPUs
func (g *GPUManager) parseAndSumGPUMemory(output, memoryType string) uint64 {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var totalMemory uint64
	var gpuCount int

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		memory, err := strconv.ParseUint(line, 10, 64)
		if err != nil {
			log.Error().
				Err(err).
				Str("line", line).
				Str("memory_type", memoryType).
				Msg("Error parsing nvidia-smi memory output line")
			continue
		}

		memoryBytes := memory * 1024 * 1024 // Convert MiB to bytes
		totalMemory += memoryBytes
		gpuCount++
		log.Trace().
			Int("gpu_index", gpuCount-1).
			Uint64("memory_mib", memory).
			Uint64("memory_bytes", memoryBytes).
			Str("memory_type", memoryType).
			Msg("Parsed GPU memory for individual GPU")
	}

	log.Info().
		Int("gpu_count", gpuCount).
		Uint64("total_memory_bytes", totalMemory).
		Str("memory_type", memoryType).
		Msg("Successfully summed GPU memory across all GPUs")

	return totalMemory
}
