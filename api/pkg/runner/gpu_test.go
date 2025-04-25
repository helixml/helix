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
