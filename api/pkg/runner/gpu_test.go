package runner

import (
	"context"
	"os"
	"runtime"
	"testing"
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
			name: "memory values are non-negative",
			validate: func(t *testing.T, g *GPUManager) {
				if free := g.GetFreeMemory(); free < 0 {
					t.Errorf("Free memory should not be negative, got %d", free)
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
