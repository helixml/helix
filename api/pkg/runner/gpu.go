package runner

import (
	"bufio"
	"context"
	"fmt"
	"math/rand"
	"os/exec"
	"runtime"
	"sort"
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
	gpuCount      int
	gpuMemoryMap  map[int]*GPUInfo // Per-GPU memory tracking
	runnerOptions *Options
}

// GPUInfo tracks memory usage for individual GPUs
type GPUInfo struct {
	Index       int    `json:"index"`        // GPU index (0, 1, 2, etc.)
	TotalMemory uint64 `json:"total_memory"` // Total memory in bytes
	FreeMemory  uint64 `json:"free_memory"`  // Free memory in bytes
	UsedMemory  uint64 `json:"used_memory"`  // Used memory in bytes
}

func NewGPUManager(ctx context.Context, runnerOptions *Options) *GPUManager {
	g := &GPUManager{
		runnerOptions: runnerOptions,
		gpuMemoryMap:  make(map[int]*GPUInfo),
	}

	// These are slow, but run on startup so it's probably fine
	g.hasGPU = g.detectGPU()
	g.gpuMemory, g.gpuCount = g.fetchTotalMemoryAndCount()

	// Initialize per-GPU memory tracking
	g.initializeGPUMemoryMap()

	// Fetch free memory once to initialize values before background updates
	g.freeMemory = g.fetchFreeMemory()
	log.Info().
		Bool("has_gpu", g.hasGPU).
		Uint64("total_memory", g.gpuMemory).
		Uint64("free_memory", g.freeMemory).
		Uint64("used_memory", g.usedMemory).
		Int("gpu_count", g.gpuCount).
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
				g.updateGPUMemoryMap() // Update per-GPU memory tracking
				// Log occasional updates for debugging
				if rand.Intn(20) == 0 { // ~5% chance to log an update
					log.Trace().
						Uint64("total_memory", g.gpuMemory).
						Uint64("free_memory", g.freeMemory).
						Uint64("used_memory", g.usedMemory).
						Int("gpu_count", g.gpuCount).
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

// GetGPUCount returns the number of GPUs detected
func (g *GPUManager) GetGPUCount() int {
	return g.gpuCount
}

// GetPerGPUMemory returns the memory per individual GPU
// For single-GPU VLLM models, this is what should be used for memory calculations
func (g *GPUManager) GetPerGPUMemory() uint64 {
	if g.gpuCount == 0 {
		return g.gpuMemory // Fallback to total memory if no GPU count
	}
	return g.gpuMemory / uint64(g.gpuCount)
}

// GetGPUInfo returns information about all individual GPUs
func (g *GPUManager) GetGPUInfo() map[int]*GPUInfo {
	return g.gpuMemoryMap
}

// GetBestGPUForModel returns the GPU index with the most free memory that can fit the model
// Returns -1 if no GPU has enough memory
func (g *GPUManager) GetBestGPUForModel(modelMemoryRequirement uint64) int {
	bestGPU := -1
	maxFreeMemory := uint64(0)

	for gpuIndex, gpuInfo := range g.gpuMemoryMap {
		// Check if this GPU has enough free memory for the model
		if gpuInfo.FreeMemory >= modelMemoryRequirement {
			// Select the GPU with the most free memory to balance load
			if gpuInfo.FreeMemory > maxFreeMemory {
				maxFreeMemory = gpuInfo.FreeMemory
				bestGPU = gpuIndex
			}
		}
	}

	log.Debug().
		Int("selected_gpu", bestGPU).
		Uint64("model_memory_requirement", modelMemoryRequirement).
		Uint64("max_free_memory", maxFreeMemory).
		Int("total_gpus", len(g.gpuMemoryMap)).
		Msg("Selected GPU for VLLM model")

	return bestGPU
}

// GetBestGPUsForMultiGPUModel selects the best set of GPUs for a multi-GPU model
// Returns GPU indices and whether the allocation is possible
func (g *GPUManager) GetBestGPUsForMultiGPUModel(modelMemoryRequirement uint64, tensorParallelSize int) ([]int, bool) {
	if !g.hasGPU || g.gpuCount == 0 || tensorParallelSize <= 0 {
		log.Warn().
			Bool("has_gpu", g.hasGPU).
			Int("gpu_count", g.gpuCount).
			Int("tensor_parallel_size", tensorParallelSize).
			Msg("Cannot schedule multi-GPU model: insufficient GPU resources")
		return nil, false
	}

	// For multi-GPU models, we need to distribute memory across GPUs
	// VLLM will split the model across GPUs, so each GPU needs roughly modelMemory/tensorParallelSize
	memoryPerGPU := modelMemoryRequirement / uint64(tensorParallelSize)

	// Add some buffer for overhead (10%)
	memoryPerGPU = uint64(float64(memoryPerGPU) * 1.1)

	// Find GPUs with sufficient memory
	var candidateGPUs []int
	for gpuIndex, gpu := range g.gpuMemoryMap {
		if gpu.FreeMemory >= memoryPerGPU {
			candidateGPUs = append(candidateGPUs, gpuIndex)
		}
	}

	// Check if we have enough GPUs
	if len(candidateGPUs) < tensorParallelSize {
		log.Warn().
			Int("available_gpus", len(candidateGPUs)).
			Int("required_gpus", tensorParallelSize).
			Uint64("memory_per_gpu_required", memoryPerGPU).
			Uint64("total_model_memory", modelMemoryRequirement).
			Msg("Insufficient GPUs available for multi-GPU model")
		return nil, false
	}

	// Sort candidates by available memory (descending) and select the best ones
	sort.Slice(candidateGPUs, func(i, j int) bool {
		return g.gpuMemoryMap[candidateGPUs[i]].FreeMemory > g.gpuMemoryMap[candidateGPUs[j]].FreeMemory
	})

	selectedGPUs := candidateGPUs[:tensorParallelSize]

	log.Info().
		Ints("selected_gpus", selectedGPUs).
		Int("tensor_parallel_size", tensorParallelSize).
		Uint64("memory_per_gpu", memoryPerGPU).
		Uint64("total_model_memory", modelMemoryRequirement).
		Msg("Selected GPUs for multi-GPU model")

	return selectedGPUs, true
}

func (g *GPUManager) fetchTotalMemoryAndCount() (uint64, int) {
	totalMemory, gpuCount := g.getActualTotalMemoryAndCount()

	// If the user has manually set the total memory, then use that
	// But make sure it is less than the actual total memory
	if g.runnerOptions.MemoryBytes > 0 && (g.runnerOptions.MemoryBytes < totalMemory || g.runnerOptions.DevelopmentCPUOnly) {
		totalMemory = g.runnerOptions.MemoryBytes
		// Note: We keep the actual GPU count even when memory is manually set
	}

	return totalMemory, gpuCount
}

func (g *GPUManager) getActualTotalMemoryAndCount() (uint64, int) {
	if !g.hasGPU && !g.runnerOptions.DevelopmentCPUOnly {
		return 0, 0
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
						return systemMemory, 1 // CPU mode simulates 1 GPU
					}
				}
			}
			// If we couldn't read system memory, log an error and return a reasonable default
			log.Error().Msg("Failed to read system memory from /proc/meminfo, using 16GB default")
			return 16 * 1024 * 1024 * 1024, 1 // 16GB default as fallback, CPU mode simulates 1 GPU

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
						return systemMemory, 1 // CPU mode simulates 1 GPU
					}
				}
			}
			log.Error().Msg("Failed to read system memory on macOS, using 16GB default")
			return 16 * 1024 * 1024 * 1024, 1 // 16GB default as fallback, CPU mode simulates 1 GPU

		case "windows":
			// Use a simple default for Windows - we don't develop on Windows
			log.Info().Msg("Windows platform detected, using 16GB default for development CPU-only mode")
			return 16 * 1024 * 1024 * 1024, 1 // 16GB default, CPU mode simulates 1 GPU
		}

		// Fallback if we couldn't determine the system memory
		log.Warn().Msg("Could not determine system memory, using 16GB default for development CPU-only mode")
		return 16 * 1024 * 1024 * 1024, 1 // 16GB default, CPU mode simulates 1 GPU
	}

	switch runtime.GOOS {
	case "linux":
		cmd := exec.Command("nvidia-smi", "--query-gpu=memory.total", "--format=csv,noheader,nounits")
		connectCmdStdErrToLogger(cmd)
		output, err := cmd.Output()
		if err == nil {
			return g.parseAndSumGPUMemoryWithCount(string(output), "total")
		}
	case "darwin":
		arch, err := getMacArchitecture()
		if err != nil {
			log.Error().Err(err).Msg("failed to get Mac architecture")
			return 0, 0
		}

		switch arch {
		// If it is an intel based mac, try to get any external VRAM from the in-built GPU
		case MacArchitectureIntel:
			log.Error().Msg("Intel Mac architecture not supported, please get in touch if you need this")
			return 0, 0
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
						return total, 1 // Mac Silicon has unified memory, count as 1 GPU
					}
				}
			}
		}
	case "windows":
		log.Error().Msg("Windows not yet supported, please get in touch if you need this")
	}
	return 0, 0
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
	totalMemory, _ := g.parseAndSumGPUMemoryWithCount(output, memoryType)
	return totalMemory
}

// parseAndSumGPUMemoryWithCount parses nvidia-smi output and returns both total memory and GPU count
func (g *GPUManager) parseAndSumGPUMemoryWithCount(output, memoryType string) (uint64, int) {
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

	log.Trace().
		Int("gpu_count", gpuCount).
		Uint64("total_memory_bytes", totalMemory).
		Str("memory_type", memoryType).
		Msg("Successfully summed GPU memory across all GPUs")

	return totalMemory, gpuCount
}

// initializeGPUMemoryMap sets up per-GPU memory tracking
func (g *GPUManager) initializeGPUMemoryMap() {
	if !g.hasGPU || g.gpuCount == 0 {
		return
	}

	// Get per-GPU total memory
	switch runtime.GOOS {
	case "linux":
		cmd := exec.Command("nvidia-smi", "--query-gpu=memory.total", "--format=csv,noheader,nounits")
		connectCmdStdErrToLogger(cmd)
		output, err := cmd.Output()
		if err != nil {
			log.Warn().Err(err).Msg("Failed to get per-GPU total memory, using fallback")
			g.initializeFallbackGPUMap()
			return
		}

		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		for i, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			memory, err := strconv.ParseUint(line, 10, 64)
			if err != nil {
				log.Error().Err(err).Str("line", line).Msg("Error parsing GPU total memory")
				continue
			}

			memoryBytes := memory * 1024 * 1024 // Convert MiB to bytes
			g.gpuMemoryMap[i] = &GPUInfo{
				Index:       i,
				TotalMemory: memoryBytes,
				FreeMemory:  memoryBytes, // Initially assume all memory is free
				UsedMemory:  0,
			}

			log.Debug().
				Int("gpu_index", i).
				Uint64("total_memory_bytes", memoryBytes).
				Msg("Initialized GPU memory tracking")
		}
	default:
		// For non-Linux systems, use fallback
		g.initializeFallbackGPUMap()
	}

	// Update with actual free/used memory
	g.updateGPUMemoryMap()
}

// initializeFallbackGPUMap creates a fallback GPU memory map when nvidia-smi is not available
func (g *GPUManager) initializeFallbackGPUMap() {
	if g.gpuCount == 0 {
		g.gpuCount = 1 // Assume at least one GPU
	}

	perGPUMemory := g.gpuMemory / uint64(g.gpuCount)
	for i := 0; i < g.gpuCount; i++ {
		g.gpuMemoryMap[i] = &GPUInfo{
			Index:       i,
			TotalMemory: perGPUMemory,
			FreeMemory:  perGPUMemory,
			UsedMemory:  0,
		}
	}

	log.Debug().
		Int("gpu_count", g.gpuCount).
		Uint64("per_gpu_memory", perGPUMemory).
		Msg("Initialized fallback GPU memory map")
}

// updateGPUMemoryMap refreshes the free/used memory for each GPU
func (g *GPUManager) updateGPUMemoryMap() {
	if !g.hasGPU || len(g.gpuMemoryMap) == 0 {
		return
	}

	switch runtime.GOOS {
	case "linux":
		// Get per-GPU used memory
		cmd := exec.Command("nvidia-smi", "--query-gpu=memory.used", "--format=csv,noheader,nounits")
		connectCmdStdErrToLogger(cmd)
		output, err := cmd.Output()
		if err != nil {
			log.Trace().Err(err).Msg("Failed to update per-GPU used memory")
			return
		}

		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		for i, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			usedMemory, err := strconv.ParseUint(line, 10, 64)
			if err != nil {
				log.Error().Err(err).Str("line", line).Msg("Error parsing GPU used memory")
				continue
			}

			usedMemoryBytes := usedMemory * 1024 * 1024 // Convert MiB to bytes
			if gpuInfo, exists := g.gpuMemoryMap[i]; exists {
				gpuInfo.UsedMemory = usedMemoryBytes
				gpuInfo.FreeMemory = gpuInfo.TotalMemory - usedMemoryBytes

				log.Trace().
					Int("gpu_index", i).
					Uint64("used_memory_bytes", usedMemoryBytes).
					Uint64("free_memory_bytes", gpuInfo.FreeMemory).
					Msg("Updated GPU memory usage")
			}
		}
	default:
		// For non-Linux systems, we can't track per-GPU memory accurately
		// Just distribute the aggregated used memory evenly
		if g.gpuCount > 0 {
			perGPUUsedMemory := g.usedMemory / uint64(g.gpuCount)
			for _, gpuInfo := range g.gpuMemoryMap {
				gpuInfo.UsedMemory = perGPUUsedMemory
				if gpuInfo.TotalMemory > perGPUUsedMemory {
					gpuInfo.FreeMemory = gpuInfo.TotalMemory - perGPUUsedMemory
				} else {
					gpuInfo.FreeMemory = 0
				}
			}
		}
	}
}
