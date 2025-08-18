package memory

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryEstimation(t *testing.T) {
	// Test data for Qwen3:8b model (based on the metadata we saw earlier)
	metadata := &ModelMetadata{
		Architecture:    "qwen3",
		FileType:        "Q4_K_M",
		BlockCount:      36,
		EmbeddingLength: 4096,
		ContextLength:   40960,
		HeadCount:       32,
		HeadCountKV:     8,
		KeyLength:       128,
		ValueLength:     128,
		FFLength:        12288,
		VocabSize:       151936,
		Layers:          make(map[string]LayerInfo),
	}

	// Add some sample layers
	for i := 0; i < 36; i++ {
		layerName := fmt.Sprintf("blk.%d", i)
		metadata.Layers[layerName] = LayerInfo{
			Tensors: map[string]TensorInfo{
				"attn_q.weight": {
					Type:  "Q4_K",
					Shape: []uint64{4096, 4096},
					Size:  4096 * 4096 / 2, // Q4_K is approximately 0.5 bytes per element
				},
				"attn_k.weight": {
					Type:  "Q4_K",
					Shape: []uint64{4096, 1024},
					Size:  4096 * 1024 / 2,
				},
				"attn_v.weight": {
					Type:  "F16",
					Shape: []uint64{4096, 1024},
					Size:  4096 * 1024 * 2, // F16 is 2 bytes per element
				},
				"ffn_gate.weight": {
					Type:  "Q4_K",
					Shape: []uint64{4096, 12288},
					Size:  4096 * 12288 / 2,
				},
				"ffn_up.weight": {
					Type:  "Q4_K",
					Shape: []uint64{4096, 12288},
					Size:  4096 * 12288 / 2,
				},
				"ffn_down.weight": {
					Type:  "Q6_K",
					Shape: []uint64{12288, 4096},
					Size:  12288 * 4096 * 3 / 4, // Q6_K is approximately 0.75 bytes per element
				},
			},
		}
	}

	// Test single GPU estimation
	t.Run("SingleGPU", func(t *testing.T) {
		gpuInfos := []GPUInfo{
			{
				ID:            "0",
				Index:         0,
				Library:       "cuda",
				FreeMemory:    24 * 1024 * 1024 * 1024, // 24GB
				TotalMemory:   24 * 1024 * 1024 * 1024,
				MinimumMemory: 512 * 1024 * 1024,
				Name:          "NVIDIA RTX 4090",
			},
		}

		opts := EstimateOptions{
			NumCtx:      4096,
			NumBatch:    512,
			NumParallel: 1,
			NumGPU:      1,
			KVCacheType: "q8_0", // Match Ollama's actual KV cache configuration
		}

		estimate := EstimateGPULayers(gpuInfos, metadata, opts)
		require.NotNil(t, estimate)

		// Test the actual MemoryEstimate fields
		assert.Equal(t, "qwen3", estimate.Architecture)
		assert.Greater(t, estimate.Layers, 0, "Should estimate at least some layers")
		assert.Greater(t, estimate.VRAMSize, uint64(0), "Should require some VRAM")
		assert.Greater(t, estimate.TotalSize, uint64(0), "Should have total size")
		assert.Greater(t, estimate.KVCache, uint64(0), "Should have KV cache size")
		assert.Greater(t, estimate.Weights, uint64(0), "Should have weights size")
		assert.Equal(t, len(gpuInfos), len(estimate.GPUSizes), "Should have allocation for each GPU")

		t.Logf("Single GPU - Layers: %d/%d, VRAM: %s, Total: %s",
			estimate.Layers, metadata.BlockCount,
			FormatMemorySize(estimate.VRAMSize),
			FormatMemorySize(estimate.TotalSize))
	})

	// Test dual GPU estimation
	t.Run("DualGPU", func(t *testing.T) {
		gpuInfos := []GPUInfo{
			{
				ID:            "0",
				Index:         0,
				Library:       "cuda",
				FreeMemory:    24 * 1024 * 1024 * 1024, // 24GB
				TotalMemory:   24 * 1024 * 1024 * 1024,
				MinimumMemory: 512 * 1024 * 1024,
				Name:          "NVIDIA RTX 4090",
			},
			{
				ID:            "1",
				Index:         1,
				Library:       "cuda",
				FreeMemory:    24 * 1024 * 1024 * 1024, // 24GB
				TotalMemory:   24 * 1024 * 1024 * 1024,
				MinimumMemory: 512 * 1024 * 1024,
				Name:          "NVIDIA RTX 4090",
			},
		}

		opts := EstimateOptions{
			NumCtx:      4096,
			NumBatch:    512,
			NumParallel: 1,
			NumGPU:      -1,     // Auto-detect
			KVCacheType: "q8_0", // Match Ollama's actual KV cache configuration
		}

		estimate := EstimateGPULayers(gpuInfos, metadata, opts)
		require.NotNil(t, estimate)

		// Test tensor parallel estimation with multiple GPUs
		assert.Equal(t, "qwen3", estimate.Architecture)
		assert.Greater(t, estimate.Layers, 0, "Should estimate at least some layers")
		assert.Greater(t, estimate.VRAMSize, uint64(0), "Should require some VRAM")
		assert.Equal(t, len(gpuInfos), len(estimate.GPUSizes), "Should have allocation for each GPU")
		assert.Equal(t, len(gpuInfos), len(estimate.TensorSplit), "Should have tensor split for each GPU")

		// With dual GPUs, should be able to fit more layers or distribute better
		t.Logf("Dual GPU - Layers: %d/%d, Total VRAM: %s, GPU0: %s, GPU1: %s",
			estimate.Layers, metadata.BlockCount,
			FormatMemorySize(estimate.VRAMSize),
			FormatMemorySize(estimate.GPUSizes[0]),
			FormatMemorySize(estimate.GPUSizes[1]))
	})

	// Test CPU-only estimation
	t.Run("CPUOnly", func(t *testing.T) {
		gpuInfos := []GPUInfo{
			{
				ID:            "cpu",
				Index:         0,
				Library:       "cpu",
				FreeMemory:    32 * 1024 * 1024 * 1024, // 32GB RAM
				TotalMemory:   32 * 1024 * 1024 * 1024,
				MinimumMemory: 1024 * 1024 * 1024,
				Name:          "CPU",
			},
		}

		opts := EstimateOptions{
			NumCtx:      4096,
			NumBatch:    512,
			NumParallel: 1,
			NumGPU:      0,      // Force CPU
			KVCacheType: "q8_0", // Match Ollama's actual KV cache configuration
		}

		estimate := EstimateGPULayers(gpuInfos, metadata, opts)
		require.NotNil(t, estimate)

		// For CPU-only, layers should be 0 (all on CPU), but total size should be calculated
		assert.Equal(t, "qwen3", estimate.Architecture)
		assert.Equal(t, 0, estimate.Layers, "CPU-only should have 0 GPU layers")
		assert.Greater(t, estimate.TotalSize, uint64(0), "Should have total memory requirement")
		assert.Greater(t, estimate.Weights, uint64(0), "Should have weights size")
		assert.Greater(t, estimate.KVCache, uint64(0), "Should have KV cache size")

		t.Logf("CPU Only - Total Size: %s, Weights: %s, KV Cache: %s",
			FormatMemorySize(estimate.TotalSize),
			FormatMemorySize(estimate.Weights),
			FormatMemorySize(estimate.KVCache))
	})
}

func TestFormatMemorySize(t *testing.T) {
	tests := []struct {
		bytes    uint64
		expected string
	}{
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
		{1536 * 1024 * 1024, "1.5 GB"},
		{24 * 1024 * 1024 * 1024, "24.0 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := FormatMemorySize(tt.bytes)
			assert.Equal(t, tt.expected, result)
		})
	}
}
