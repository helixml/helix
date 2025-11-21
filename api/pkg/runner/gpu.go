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
	gpuVendor     string              // "nvidia", "amd", or "" for CPU-only/Mac
	gpuMemory     uint64
	freeMemory    uint64
	usedMemory    uint64
	gpuCount      int
	gpuMemoryMap  map[int]*GPUInfo // Per-GPU memory tracking
	runnerOptions *Options
}

// GPUInfo tracks memory usage for individual GPUs
type GPUInfo struct {
	Index         int    `json:"index"`          // GPU index (0, 1, 2, etc.)
	TotalMemory   uint64 `json:"total_memory"`   // Total memory in bytes
	FreeMemory    uint64 `json:"free_memory"`    // Free memory in bytes
	UsedMemory    uint64 `json:"used_memory"`    // Used memory in bytes
	ModelName     string `json:"model_name"`     // GPU model name (e.g., "NVIDIA H100 PCIe", "AMD Radeon RX 7900 XTX")
	DriverVersion string `json:"driver_version"` // GPU driver version (NVIDIA or AMD)
	SDKVersion    string `json:"sdk_version"`    // GPU SDK version (CUDA for NVIDIA, ROCm for AMD)
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
		// Check for NVIDIA GPU first (nvidia-smi)
		if _, err := exec.LookPath("nvidia-smi"); err == nil {
			g.gpuVendor = "nvidia"
			log.Info().Str("vendor", "nvidia").Msg("Detected NVIDIA GPU via nvidia-smi")
			return true
		}

		// Check for AMD GPU (rocm-smi)
		if _, err := exec.LookPath("rocm-smi"); err == nil {
			g.gpuVendor = "amd"
			log.Info().Str("vendor", "amd").Msg("Detected AMD GPU via rocm-smi")
			return true
		}

		// Fallback: check for /dev/kfd (AMD ROCm Kernel Fusion Driver)
		if _, err := exec.Command("test", "-e", "/dev/kfd").Output(); err == nil {
			g.gpuVendor = "amd"
			log.Warn().Msg("Detected /dev/kfd but rocm-smi not found - AMD GPU may not be fully configured")
			return true
		}

		return false
	case "darwin":
		// Mac uses unified memory, no vendor distinction needed
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

// GetFreshUsedMemory forces a fresh nvidia-smi call to get current GPU memory usage
// This is useful when you need real-time memory data rather than cached values
func (g *GPUManager) GetFreshUsedMemory() uint64 {
	// Force a fresh memory fetch which will update g.usedMemory
	g.fetchFreeMemory()
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

		var cmd *exec.Cmd
		var toolName string

		switch g.gpuVendor {
		case "nvidia":
			cmd = exec.Command("nvidia-smi", "--query-gpu=memory.used", "--format=csv,noheader,nounits")
			toolName = "nvidia-smi"
		case "amd":
			// rocm-smi --showmeminfo vram --csv returns used memory in bytes
			cmd = exec.Command("rocm-smi", "--showmeminfo", "vram", "--csv")
			toolName = "rocm-smi"
		default:
			log.Error().Str("gpu_vendor", g.gpuVendor).Msg("CRITICAL: Unknown GPU vendor - cannot query memory usage")
			return 0
		}

		connectCmdStdErrToLogger(cmd)
		log.Trace().Str("tool", toolName).Msg("Running GPU monitoring tool to get used memory")
		output, err := cmd.Output()
		if err != nil {
			log.Error().Err(err).Str("tool", toolName).Msg("Error running GPU monitoring tool to get used memory")
			g.usedMemory = 0
		} else {
			log.Trace().Str("output", string(output)).Str("tool", toolName).Msg("GPU tool output for used memory")
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
			log.Error().Msg("CRITICAL: Failed to read system memory on macOS - cannot proceed without real memory data")
			return 0, 0

		case "windows":
			// Windows is not supported for production - fail explicitly
			log.Error().Msg("CRITICAL: Windows platform detected but GPU memory detection is not implemented")
			return 0, 0
		}

		// CRITICAL: Could not determine system memory - fail explicitly instead of using defaults
		log.Error().Msg("CRITICAL: Could not determine system memory - cannot proceed without real memory data")
		return 0, 0
	}

	switch runtime.GOOS {
	case "linux":
		var cmd *exec.Cmd
		switch g.gpuVendor {
		case "nvidia":
			cmd = exec.Command("nvidia-smi", "--query-gpu=memory.total", "--format=csv,noheader,nounits")
		case "amd":
			// rocm-smi --showmeminfo vram --csv returns total memory
			cmd = exec.Command("rocm-smi", "--showmeminfo", "vram", "--csv")
		default:
			log.Error().Str("gpu_vendor", g.gpuVendor).Msg("CRITICAL: Unknown GPU vendor - cannot query total memory")
			return 0, 0
		}

		connectCmdStdErrToLogger(cmd)
		output, err := cmd.Output()
		if err == nil {
			return g.parseAndSumGPUMemoryWithCount(string(output), "total")
		} else {
			log.Error().Err(err).Str("gpu_vendor", g.gpuVendor).Msg("Failed to query GPU total memory")
			return 0, 0
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

// parseAndSumGPUMemory parses GPU monitoring tool output that may contain multiple lines (one per GPU)
// and sums the memory values across all GPUs
func (g *GPUManager) parseAndSumGPUMemory(output, memoryType string) uint64 {
	totalMemory, _ := g.parseAndSumGPUMemoryWithCount(output, memoryType)
	return totalMemory
}

// parseAndSumGPUMemoryWithCount parses GPU monitoring tool output and returns both total memory and GPU count
// Handles both NVIDIA (nvidia-smi) and AMD (rocm-smi) output formats
func (g *GPUManager) parseAndSumGPUMemoryWithCount(output, memoryType string) (uint64, int) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var totalMemory uint64
	var gpuCount int

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "GPU") || strings.HasPrefix(line, "device,") {
			// Skip empty lines and headers (CSV header or GPU info)
			continue
		}

		var memoryMiB uint64
		var err error

		if g.gpuVendor == "amd" {
			// AMD rocm-smi CSV format: "device,vram_total,vram_used"
			// Example: "card0,16384,8192" (values in MiB)
			parts := strings.Split(line, ",")
			if len(parts) < 3 {
				log.Error().
					Str("line", line).
					Str("memory_type", memoryType).
					Msg("Error parsing rocm-smi CSV output - expected 3 columns")
				continue
			}

			// vram_total is column 1, vram_used is column 2 (0-indexed)
			valueIdx := 1 // total
			if memoryType == "used" {
				valueIdx = 2
			}

			memoryMiB, err = strconv.ParseUint(strings.TrimSpace(parts[valueIdx]), 10, 64)
		} else {
			// NVIDIA nvidia-smi format: simple numbers in MiB, one per line
			memoryMiB, err = strconv.ParseUint(line, 10, 64)
		}

		if err != nil {
			log.Error().
				Err(err).
				Str("line", line).
				Str("memory_type", memoryType).
				Str("gpu_vendor", g.gpuVendor).
				Msg("Error parsing GPU memory output line")
			continue
		}

		memoryBytes := memoryMiB * 1024 * 1024 // Convert MiB to bytes
		totalMemory += memoryBytes
		gpuCount++
		log.Trace().
			Int("gpu_index", gpuCount-1).
			Uint64("memory_mib", memoryMiB).
			Uint64("memory_bytes", memoryBytes).
			Str("memory_type", memoryType).
			Str("gpu_vendor", g.gpuVendor).
			Msg("Parsed GPU memory for individual GPU")
	}

	log.Trace().
		Int("gpu_count", gpuCount).
		Uint64("total_memory_bytes", totalMemory).
		Str("memory_type", memoryType).
		Str("gpu_vendor", g.gpuVendor).
		Msg("Successfully summed GPU memory across all GPUs")

	return totalMemory, gpuCount
}

// initializeGPUMemoryMap sets up per-GPU memory tracking with model information
func (g *GPUManager) initializeGPUMemoryMap() {
	if !g.hasGPU || g.gpuCount == 0 {
		return
	}

	switch runtime.GOOS {
	case "linux":
		var cmd *exec.Cmd
		switch g.gpuVendor {
		case "nvidia":
			cmd = exec.Command("nvidia-smi", "--query-gpu=index,name,memory.total,driver_version", "--format=csv,noheader,nounits")
		case "amd":
			// rocm-smi doesn't have a single command to get all info, so we'll get what we can
			// For now, just use basic info - we'll populate model/driver separately
			cmd = exec.Command("rocm-smi", "--showid", "--showproductname", "--csv")
		default:
			log.Error().Str("gpu_vendor", g.gpuVendor).Msg("CRITICAL: Unknown GPU vendor - cannot initialize GPU map")
			g.initializeFallbackGPUMap()
			return
		}

		connectCmdStdErrToLogger(cmd)
		output, err := cmd.Output()
		if err != nil {
			log.Warn().Err(err).Str("gpu_vendor", g.gpuVendor).Msg("Failed to get GPU information, using fallback")
			g.initializeFallbackGPUMap()
			return
		}

		// Get SDK version (CUDA for NVIDIA, ROCm for AMD)
		sdkVersion := g.getSDKVersion()

		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "GPU") || strings.HasPrefix(line, "device,") {
				continue
			}

			if g.gpuVendor == "nvidia" {
				// Parse NVIDIA CSV line: index,name,memory.total,driver_version
				parts := strings.Split(line, ", ")
				if len(parts) < 4 {
					log.Error().Str("line", line).Msg("Invalid NVIDIA GPU info format")
					continue
				}

				index, err := strconv.Atoi(strings.TrimSpace(parts[0]))
				if err != nil {
					log.Error().Err(err).Str("index", parts[0]).Msg("Error parsing GPU index")
					continue
				}

				modelName := strings.TrimSpace(parts[1])

				memory, err := strconv.ParseUint(strings.TrimSpace(parts[2]), 10, 64)
				if err != nil {
					log.Error().Err(err).Str("memory", parts[2]).Msg("Error parsing GPU memory")
					continue
				}

				driverVersion := strings.TrimSpace(parts[3])

				memoryBytes := memory * 1024 * 1024 // Convert MiB to bytes
				g.gpuMemoryMap[index] = &GPUInfo{
					Index:         index,
					TotalMemory:   memoryBytes,
					FreeMemory:    memoryBytes,
					UsedMemory:    0,
					ModelName:     modelName,
					DriverVersion: driverVersion,
					SDKVersion:    sdkVersion,
				}

				log.Info().
					Int("gpu_index", index).
					Str("model_name", modelName).
					Str("driver_version", driverVersion).
					Str("sdk_version", sdkVersion).
					Uint64("total_memory_bytes", memoryBytes).
					Msg("Initialized NVIDIA GPU with model information")
			} else if g.gpuVendor == "amd" {
				// Parse AMD CSV line: "device,GPU ID,Card series,Card model,Card vendor,Card SKU"
				// Example: "card0,0x1002,Radeon RX,7900 XTX,Advanced Micro Devices Inc,0x744c"
				// Note: AMD rocm-smi CSV format varies, so we'll use fallback for AMD
				log.Warn().Str("line", line).Msg("AMD GPU info parsing - using fallback initialization")
				g.initializeFallbackGPUMap()
				return
			}
		}
	default:
		// For non-Linux systems, use fallback
		g.initializeFallbackGPUMap()
	}

	// Update with actual free/used memory
	g.updateGPUMemoryMap()
}

// getSDKVersion retrieves the GPU SDK version (CUDA for NVIDIA, ROCm for AMD)
func (g *GPUManager) getSDKVersion() string {
	switch g.gpuVendor {
	case "nvidia":
		// Get CUDA version from nvidia-smi header output
		cmd := exec.Command("nvidia-smi")
		connectCmdStdErrToLogger(cmd)
		output, err := cmd.Output()
		if err != nil {
			log.Debug().Err(err).Msg("Failed to get CUDA version from nvidia-smi")
			// Try alternative method using nvcc if available
			cmd = exec.Command("nvcc", "--version")
			connectCmdStdErrToLogger(cmd)
			nvccOutput, nvccErr := cmd.Output()
			if nvccErr != nil {
				log.Debug().Err(nvccErr).Msg("Failed to get CUDA version from nvcc")
				return "unknown"
			}

			// Parse nvcc output to extract version
			lines := strings.Split(string(nvccOutput), "\n")
			for _, line := range lines {
				if strings.Contains(line, "release") {
					// Example: "Cuda compilation tools, release 12.2, V12.2.140"
					parts := strings.Split(line, "release ")
					if len(parts) > 1 {
						versionPart := strings.Split(parts[1], ",")[0]
						return "CUDA " + strings.TrimSpace(versionPart)
					}
				}
			}
			return "unknown"
		}

		// Parse nvidia-smi header to extract CUDA version
		// Header format: "| NVIDIA-SMI 575.57.08              Driver Version: 575.57.08      CUDA Version: 12.9     |"
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "CUDA Version:") {
				parts := strings.Split(line, "CUDA Version:")
				if len(parts) > 1 {
					// Extract version number and clean up
					versionPart := strings.TrimSpace(parts[1])
					versionPart = strings.Split(versionPart, " ")[0]  // Take first part before any spaces
					versionPart = strings.TrimRight(versionPart, "|") // Remove trailing |
					return "CUDA " + strings.TrimSpace(versionPart)
				}
			}
		}

		return "unknown"

	case "amd":
		// Get ROCm version from rocm-smi --showdriverversion
		cmd := exec.Command("rocm-smi", "--showdriverversion")
		connectCmdStdErrToLogger(cmd)
		output, err := cmd.Output()
		if err != nil {
			log.Debug().Err(err).Msg("Failed to get ROCm version from rocm-smi")
			return "unknown"
		}

		// Parse rocm-smi output to find ROCm version
		// Example output: "ROCm version: 6.0.2" or "Driver version: 6.0"
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "ROCm") || strings.Contains(line, "version") {
				// Extract version number from the line
				parts := strings.Split(line, ":")
				if len(parts) > 1 {
					version := strings.TrimSpace(parts[1])
					return "ROCm " + version
				}
			}
		}

		return "unknown"

	default:
		return "unknown"
	}
}

// initializeFallbackGPUMap creates a fallback GPU memory map when nvidia-smi is not available
// CRITICAL: Only use when we have REAL memory data - no assumptions about GPU count
func (g *GPUManager) initializeFallbackGPUMap() {
	if g.gpuCount == 0 {
		log.Error().Msg("CRITICAL: Cannot initialize GPU map with zero GPU count - real GPU detection failed")
		return
	}

	if g.gpuMemory == 0 {
		log.Error().Msg("CRITICAL: Cannot initialize GPU map with zero total memory - real memory detection failed")
		return
	}

	perGPUMemory := g.gpuMemory / uint64(g.gpuCount)
	for i := 0; i < g.gpuCount; i++ {
		g.gpuMemoryMap[i] = &GPUInfo{
			Index:         i,
			TotalMemory:   perGPUMemory,
			FreeMemory:    perGPUMemory,
			UsedMemory:    0,
			ModelName:     "unknown",
			DriverVersion: "unknown",
			SDKVersion:    "unknown",
		}
	}

	log.Debug().
		Int("gpu_count", g.gpuCount).
		Uint64("per_gpu_memory", perGPUMemory).
		Msg("Initialized fallback GPU memory map with REAL detected memory")
}

// updateGPUMemoryMap refreshes the free/used memory for each GPU
func (g *GPUManager) updateGPUMemoryMap() {
	if !g.hasGPU || len(g.gpuMemoryMap) == 0 {
		return
	}

	switch runtime.GOOS {
	case "linux":
		// Get per-GPU used memory
		var cmd *exec.Cmd
		switch g.gpuVendor {
		case "nvidia":
			cmd = exec.Command("nvidia-smi", "--query-gpu=memory.used", "--format=csv,noheader,nounits")
		case "amd":
			cmd = exec.Command("rocm-smi", "--showmeminfo", "vram", "--csv")
		default:
			log.Error().Str("gpu_vendor", g.gpuVendor).Msg("CRITICAL: Unknown GPU vendor - cannot update GPU memory map")
			return
		}

		connectCmdStdErrToLogger(cmd)
		output, err := cmd.Output()
		if err != nil {
			log.Trace().Err(err).Str("gpu_vendor", g.gpuVendor).Msg("Failed to update per-GPU used memory")
			return
		}

		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		gpuIdx := 0
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "GPU") || strings.HasPrefix(line, "device,") {
				// Skip empty lines and headers
				continue
			}

			var usedMemoryMiB uint64
			var err error

			if g.gpuVendor == "amd" {
				// AMD rocm-smi CSV format: "device,vram_total,vram_used"
				parts := strings.Split(line, ",")
				if len(parts) < 3 {
					log.Error().Str("line", line).Msg("Error parsing AMD GPU used memory - expected 3 columns")
					continue
				}
				usedMemoryMiB, err = strconv.ParseUint(strings.TrimSpace(parts[2]), 10, 64)
			} else {
				// NVIDIA format: simple number in MiB
				usedMemoryMiB, err = strconv.ParseUint(line, 10, 64)
			}

			if err != nil {
				log.Error().Err(err).Str("line", line).Str("gpu_vendor", g.gpuVendor).Msg("Error parsing GPU used memory")
				continue
			}

			usedMemoryBytes := usedMemoryMiB * 1024 * 1024 // Convert MiB to bytes
			if gpuInfo, exists := g.gpuMemoryMap[gpuIdx]; exists {
				gpuInfo.UsedMemory = usedMemoryBytes
				gpuInfo.FreeMemory = gpuInfo.TotalMemory - usedMemoryBytes

				log.Trace().
					Int("gpu_index", gpuIdx).
					Uint64("used_memory_bytes", usedMemoryBytes).
					Uint64("free_memory_bytes", gpuInfo.FreeMemory).
					Str("gpu_vendor", g.gpuVendor).
					Msg("Updated GPU memory usage")
			}
			gpuIdx++
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
