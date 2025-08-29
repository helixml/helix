package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/types"
)

func TestCreateStandardGPUConfig(t *testing.T) {
	t.Run("SingleGPU", func(t *testing.T) {
		config := types.CreateStandardGPUConfig(1, 80)

		require.Len(t, config, 1, "Should create exactly 1 GPU")

		gpu := config[0]
		expectedMemory := uint64(80 * 1024 * 1024 * 1024) // 80GB in bytes
		expectedMinimum := uint64(512 * 1024 * 1024)      // 512MB in bytes

		assert.Equal(t, 0, gpu.Index, "GPU index should be 0")
		assert.Equal(t, "cuda", gpu.Library, "Library should be cuda")
		assert.Equal(t, expectedMemory, gpu.FreeMemory, "FreeMemory should be 80GB")
		assert.Equal(t, expectedMemory, gpu.TotalMemory, "TotalMemory should be 80GB")
		assert.Equal(t, expectedMinimum, gpu.MinimumMemory, "MinimumMemory should be 512MB")
	})

	t.Run("MultiGPU", func(t *testing.T) {
		config := types.CreateStandardGPUConfig(4, 24)

		require.Len(t, config, 4, "Should create exactly 4 GPUs")

		expectedMemory := uint64(24 * 1024 * 1024 * 1024) // 24GB in bytes
		expectedMinimum := uint64(512 * 1024 * 1024)      // 512MB in bytes

		for i, gpu := range config {
			assert.Equal(t, i, gpu.Index, "GPU index should match position")
			assert.Equal(t, "cuda", gpu.Library, "Library should be cuda")
			assert.Equal(t, expectedMemory, gpu.FreeMemory, "FreeMemory should be 24GB")
			assert.Equal(t, expectedMemory, gpu.TotalMemory, "TotalMemory should be 24GB")
			assert.Equal(t, expectedMinimum, gpu.MinimumMemory, "MinimumMemory should be 512MB")
		}
	})

	t.Run("ZeroGPU", func(t *testing.T) {
		config := types.CreateStandardGPUConfig(0, 80)

		require.Len(t, config, 0, "Should create no GPUs")
	})

	t.Run("FreeMemoryNotZero", func(t *testing.T) {
		// This is the critical test - ensures FreeMemory is set (fixing the bug)
		config := types.CreateStandardGPUConfig(1, 80)

		require.Len(t, config, 1, "Should create exactly 1 GPU")

		gpu := config[0]
		assert.NotZero(t, gpu.FreeMemory, "FreeMemory must not be zero - this was the bug!")
		assert.Equal(t, gpu.TotalMemory, gpu.FreeMemory, "FreeMemory should equal TotalMemory for clean configs")
	})
}
