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
			name: "development cpu-only mode",
			setup: func() func() {
				// Set development CPU-only mode via env var
				oldEnvValue := os.Getenv("DEVELOPMENT_CPU_ONLY")
				os.Setenv("DEVELOPMENT_CPU_ONLY", "true")
				return func() {
					// Reset environment variable
					os.Setenv("DEVELOPMENT_CPU_ONLY", oldEnvValue)
				}
			},
			validate: func(t *testing.T, g *GPUManager) {
				// In development CPU-only mode, hasGPU should be true even if no GPU available
				if !g.hasGPU {
					t.Error("Development CPU-only mode should pretend to have a GPU")
				}

				// Total memory should be non-zero
				total := g.GetTotalMemory()
				if total == 0 {
					t.Error("Total memory should not be 0 in development CPU-only mode")
				}

				// Use a retry loop with backoff instead of a fixed sleep
				// Check if free memory equals total memory with retries
				verifyFreeMemory(t, g, total)
			},
		},
		{
			name: "development cpu-only via options",
			setup: func() func() {
				return func() {}
			},
			validate: func(t *testing.T, g *GPUManager) {
				// Create a new GPU manager with DevelopmentCPUOnly=true
				options := &Options{
					DevelopmentCPUOnly: true,
				}
				devCpuOnlyManager := NewGPUManager(context.Background(), options)

				// Verify it's in development mode
				if !devCpuOnlyManager.devCpuOnly {
					t.Error("GPUManager should be in development CPU-only mode when option is set")
				}

				// In development CPU-only mode, hasGPU should be true even if no GPU available
				if !devCpuOnlyManager.hasGPU {
					t.Error("Development CPU-only mode should pretend to have a GPU")
				}

				// Total memory should be non-zero
				total := devCpuOnlyManager.GetTotalMemory()
				if total == 0 {
					t.Error("Total memory should not be 0 in development CPU-only mode")
				}

				// Use a retry loop with backoff instead of a fixed sleep
				// Check if free memory equals total memory with retries
				verifyFreeMemory(t, devCpuOnlyManager, total)
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

// verifyFreeMemory checks if free memory equals expected with retries and backoff
func verifyFreeMemory(t *testing.T, g *GPUManager, expected uint64) {
	timeout := time.After(1 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			// Final check after timeout
			free := g.GetFreeMemory()
			if free != expected {
				t.Errorf("Timed out waiting for free memory (%d) to equal expected memory (%d)", free, expected)
			}
			return
		case <-ticker.C:
			free := g.GetFreeMemory()
			if free == expected {
				return // Success
			}
			// Continue retry with increasing backoff
			t.Logf("Free memory (%d) not yet equal to expected memory (%d), retrying...", free, expected)
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
