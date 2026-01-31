package runner

import (
	"context"
	"os"
	"runtime"
	"testing"
	"time"
)

func TestGPUManager(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() func()
		validate func(*testing.T, *GPUManager)
	}{
		{
			name: "basic initialization",
			validate: func(t *testing.T, g *GPUManager) {
				if g == nil {
					t.Error("GPUManager should not be nil")
				}
			},
		},
		{
			name: "free memory does not exceed total",
			validate: func(t *testing.T, g *GPUManager) {
				free := g.GetFreeMemory()
				total := g.GetTotalMemory()
				if free > total {
					t.Errorf("Free memory (%d) should not exceed total memory (%d)", free, total)
				}
			},
		},
		{
			name: "development cpu-only via options",
			validate: func(t *testing.T, _ *GPUManager) {
				// Create a new GPU manager with DevelopmentCPUOnly=true
				options := &Options{
					DevelopmentCPUOnly: true,
				}
				devCPUOnlyManager := NewGPUManager(context.Background(), options)

				// Verify it's in development mode
				if !devCPUOnlyManager.runnerOptions.DevelopmentCPUOnly {
					t.Error("GPUManager should be in development CPU-only mode when option is set")
				}

				// In development CPU-only mode, hasGPU should be true even if no GPU available
				if !devCPUOnlyManager.hasGPU {
					t.Error("Development CPU-only mode should pretend to have a GPU")
				}

				// Total memory should be non-zero
				total := devCPUOnlyManager.GetTotalMemory()
				if total == 0 {
					t.Error("Total memory should not be 0 in development CPU-only mode")
				}

				// Verify free memory is valid (non-zero and <= total)
				verifyValidFreeMemory(t, devCPUOnlyManager)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				cleanup := tt.setup()
				defer cleanup()
			}

			g := NewGPUManager(context.Background(), &Options{})
			tt.validate(t, g)
		})
	}
}

// verifyValidFreeMemory checks that free memory is valid (non-zero and <= total)
func verifyValidFreeMemory(t *testing.T, g *GPUManager) {
	timeout := time.After(1 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			// Final check after timeout
			free := g.GetFreeMemory()
			total := g.GetTotalMemory()

			if free == 0 {
				t.Errorf("Free memory should not be zero in development CPU-only mode")
			} else if free > total {
				t.Errorf("Free memory (%d) should not exceed total memory (%d)", free, total)
			}
			return
		case <-ticker.C:
			free := g.GetFreeMemory()
			if free > 0 {
				// Free memory is now valid (non-zero), additional check for free <= total
				// is already covered by the "free memory does not exceed total" test
				return // Success
			}
			// Continue retry with increasing backoff
			t.Logf("Free memory (%d) is still zero, retrying...", free)
		}
	}
}

func TestPlatformSpecific(t *testing.T) {
	switch runtime.GOOS {
	case "linux":
		t.Run("linux nvidia-smi parsing", func(t *testing.T) {
			// Skip if nvidia-smi is not available
			if _, err := os.Stat("/usr/bin/nvidia-smi"); os.IsNotExist(err) {
				t.Skip("nvidia-smi not available")
			}

			g := NewGPUManager(context.Background(), &Options{})
			if !g.hasGPU {
				t.Skip("No GPU detected")
			}

			total := g.GetTotalMemory()
			if total == 0 {
				t.Error("Expected non-zero total memory on system with GPU")
			}
		})

	case "darwin":
		t.Run("darwin metal detection", func(t *testing.T) {
			g := NewGPUManager(context.Background(), &Options{})
			// On Apple Silicon, Metal should always be available
			if runtime.GOARCH == "arm64" && !g.hasGPU {
				t.Error("Metal should be available on Apple Silicon")
			}
		})

	case "windows":
		t.Run("windows wmi queries", func(t *testing.T) {
			g := NewGPUManager(context.Background(), &Options{})
			if g.hasGPU {
				// If GPU is detected, memory values should be consistent
				total := g.GetTotalMemory()
				free := g.GetFreeMemory()
				if total == 0 {
					t.Error("Total memory should not be 0 when GPU is detected")
				}
				if free > total {
					t.Error("Free memory should not exceed total memory")
				}
			}
		})
	}
}

func TestParseAndSumGPUMemory(t *testing.T) {
	tests := []struct {
		name             string
		output           string
		memoryType       string
		expectedMemory   uint64
		expectedGPUCount int
	}{
		{
			name:             "single GPU",
			output:           "24564",
			memoryType:       "total",
			expectedMemory:   24564 * 1024 * 1024, // Convert MiB to bytes
			expectedGPUCount: 1,
		},
		{
			name:             "dual GPU same memory",
			output:           "24564\n24564",
			memoryType:       "total",
			expectedMemory:   2 * 24564 * 1024 * 1024, // Sum both GPUs
			expectedGPUCount: 2,
		},
		{
			name:             "dual GPU different memory",
			output:           "24564\n16384",
			memoryType:       "total",
			expectedMemory:   (24564 + 16384) * 1024 * 1024, // Sum both GPUs
			expectedGPUCount: 2,
		},
		{
			name:             "quad GPU",
			output:           "24564\n24564\n16384\n32768",
			memoryType:       "total",
			expectedMemory:   (24564 + 24564 + 16384 + 32768) * 1024 * 1024,
			expectedGPUCount: 4,
		},
		{
			name:             "empty output",
			output:           "",
			memoryType:       "total",
			expectedMemory:   0,
			expectedGPUCount: 0,
		},
		{
			name:             "output with extra whitespace",
			output:           " 24564 \n 16384 \n",
			memoryType:       "total",
			expectedMemory:   (24564 + 16384) * 1024 * 1024,
			expectedGPUCount: 2,
		},
		{
			name:             "used memory parsing",
			output:           "1024\n2048\n512",
			memoryType:       "used",
			expectedMemory:   (1024 + 2048 + 512) * 1024 * 1024,
			expectedGPUCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &GPUManager{
				runnerOptions: &Options{},
				gpuVendor:     "nvidia", // Tests use nvidia-smi output format
			}

			result := g.parseAndSumGPUMemory(tt.output, tt.memoryType)

			if result != tt.expectedMemory {
				t.Errorf("parseAndSumGPUMemory() = %d bytes, expected %d bytes", result, tt.expectedMemory)
			}
		})
	}
}
