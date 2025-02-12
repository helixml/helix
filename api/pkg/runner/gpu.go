package runner

import (
	"bufio"
	"context"
	"fmt"
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
}

func NewGPUManager(ctx context.Context, runnerOptions *Options) *GPUManager {
	g := &GPUManager{
		runnerOptions: runnerOptions,
	}

	// These are slow, but run on startup so it's probably fine
	g.hasGPU = g.detectGPU()
	g.gpuMemory = g.fetchTotalMemory()

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
	if !g.hasGPU {
		return 0
	}

	// Default to the user set max memory value
	freeMemory := g.runnerOptions.MemoryBytes

	switch runtime.GOOS {
	case "linux":
		cmd := exec.Command("nvidia-smi", "--query-gpu=memory.free", "--format=csv,noheader,nounits")
		connectCmdStdErrToLogger(cmd)
		output, err := cmd.Output()
		if err == nil {
			if free, err := strconv.ParseUint(strings.TrimSpace(string(output)), 10, 64); err == nil {
				actualFreeMemory := free * 1024 * 1024 // Convert MiB to bytes
				if actualFreeMemory < freeMemory {
					freeMemory = actualFreeMemory
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
	if g.runnerOptions.MemoryBytes > 0 && g.runnerOptions.MemoryBytes < totalMemory {
		totalMemory = g.runnerOptions.MemoryBytes
	}

	return totalMemory
}

func (g *GPUManager) getActualTotalMemory() uint64 {
	if !g.hasGPU {
		return 0
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
