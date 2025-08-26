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

		opts := CreateTestEstimateOptions(4096)

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

		opts := CreateTestEstimateOptions(4096)

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

	// Test 80GB GPU estimation to reproduce 0 layers issue
	t.Run("80GB_GPU", func(t *testing.T) {
		gpuInfos := []GPUInfo{
			{
				ID:            "0",
				Index:         0,
				Library:       "cuda",
				FreeMemory:    80 * 1024 * 1024 * 1024, // 80GB
				TotalMemory:   80 * 1024 * 1024 * 1024,
				MinimumMemory: 512 * 1024 * 1024,
				Name:          "NVIDIA H100",
			},
		}

		opts := CreateTestEstimateOptions(131072) // Large context like in our debug case

		estimate := EstimateGPULayers(gpuInfos, metadata, opts)
		require.NotNil(t, estimate)

		// This should NOT be 0 layers on an 80GB GPU
		assert.Greater(t, estimate.Layers, 0, "80GB GPU should fit at least some layers, not 0")
		assert.Greater(t, estimate.VRAMSize, uint64(0), "Should require some VRAM")
		assert.Greater(t, estimate.TotalSize, uint64(0), "Should have total size")

		t.Logf("80GB GPU - Layers: %d/%d, VRAM: %s, Total: %s, Graph: %s",
			estimate.Layers, metadata.BlockCount,
			FormatMemorySize(estimate.VRAMSize),
			FormatMemorySize(estimate.TotalSize),
			FormatMemorySize(estimate.Graph))

		// If this fails, we've reproduced the issue
		if estimate.Layers == 0 {
			t.Logf("REPRODUCED BUG: 0 layers fit on 80GB GPU")
			t.Logf("Debug info - KV Cache: %s, Weights: %s, GraphMem: %s",
				FormatMemorySize(estimate.KVCache),
				FormatMemorySize(estimate.Weights),
				FormatMemorySize(estimate.GraphMem))
		}
	})

	// Test to reproduce exact debug scenario with 131072 context
	t.Run("Debug_Scenario_131072_Context", func(t *testing.T) {
		gpuInfos := []GPUInfo{
			{
				ID:            "0",
				Index:         0,
				Library:       "cuda",
				FreeMemory:    80 * 1024 * 1024 * 1024, // 80GB
				TotalMemory:   80 * 1024 * 1024 * 1024,
				MinimumMemory: 512 * 1024 * 1024,
				Name:          "NVIDIA H100",
			},
		}

		// Use exact parameters from debug output
		opts := CreateTestEstimateOptions(131072) // Exact match from debug

		estimate := EstimateGPULayers(gpuInfos, metadata, opts)
		require.NotNil(t, estimate)

		t.Logf("Debug Scenario - Context 131072 - Layers: %d/%d, VRAM: %s, Total: %s, Graph: %s, KV: %s",
			estimate.Layers, metadata.BlockCount,
			FormatMemorySize(estimate.VRAMSize),
			FormatMemorySize(estimate.TotalSize),
			FormatMemorySize(estimate.Graph),
			FormatMemorySize(estimate.KVCache))

		// Check if we reproduced the 0 layers issue
		if estimate.Layers == 0 {
			t.Logf("REPRODUCED: 0 layers with 131072 context!")
			t.Logf("KV Cache per token: %d bytes", estimate.KVCache/(131072*36))
			t.Logf("KV Cache total: %s", FormatMemorySize(estimate.KVCache))
			t.Logf("This explains why 0 layers fit - KV cache is too large")
		}

		// Large context should still allow some layers on 80GB
		// If this fails, we know large context is the issue
		if estimate.KVCache > 70*1024*1024*1024 { // > 70GB
			t.Logf("ISSUE FOUND: KV cache (%s) uses most of the 80GB GPU memory",
				FormatMemorySize(estimate.KVCache))
		}
	})

	// Test GPTOSS architecture to match actual gpt-oss:20b model
	t.Run("GPTOSS_Architecture", func(t *testing.T) {
		// Create metadata matching GPTOSS architecture
		gptossMetadata := &ModelMetadata{
			Architecture:    "gptoss", // Use actual GPTOSS architecture
			FileType:        "Q4_K_M",
			BlockCount:      25, // Based on Ollama output showing 25 layers
			EmbeddingLength: 4096,
			ContextLength:   131072,
			HeadCount:       32,
			HeadCountKV:     8,
			KeyLength:       128,
			ValueLength:     128,
			FFLength:        12288,
			VocabSize:       151936,
			Layers:          make(map[string]LayerInfo),
		}

		// Add layers for GPTOSS model (25 blocks)
		for i := 0; i < 25; i++ {
			layerName := fmt.Sprintf("blk.%d", i)
			gptossMetadata.Layers[layerName] = LayerInfo{
				Tensors: map[string]TensorInfo{
					"attn_q.weight": {
						Type:  "Q4_K",
						Shape: []uint64{4096, 4096},
						Size:  4096 * 4096 / 2,
					},
					"attn_k.weight": {
						Type:  "Q4_K",
						Shape: []uint64{4096, 1024},
						Size:  4096 * 1024 / 2,
					},
					"attn_v.weight": {
						Type:  "F16",
						Shape: []uint64{4096, 1024},
						Size:  4096 * 1024 * 2,
					},
				},
			}
		}

		gpuInfos := []GPUInfo{
			{
				ID:            "0",
				Index:         0,
				Library:       "cuda",
				FreeMemory:    80 * 1024 * 1024 * 1024, // 80GB
				TotalMemory:   80 * 1024 * 1024 * 1024,
				MinimumMemory: 512 * 1024 * 1024,
				Name:          "NVIDIA H100",
			},
		}

		opts := CreateTestEstimateOptions(131072)

		estimate := EstimateGPULayers(gpuInfos, gptossMetadata, opts)
		require.NotNil(t, estimate)

		t.Logf("GPTOSS - Layers: %d/%d, VRAM: %s, Total: %s, Graph: %s, KV: %s",
			estimate.Layers, gptossMetadata.BlockCount,
			FormatMemorySize(estimate.VRAMSize),
			FormatMemorySize(estimate.TotalSize),
			FormatMemorySize(estimate.Graph),
			FormatMemorySize(estimate.KVCache))

		// Check if GPTOSS architecture causes the 0 layers issue
		if estimate.Layers == 0 {
			t.Logf("REPRODUCED BUG: GPTOSS architecture results in 0 layers!")
			t.Logf("This suggests the issue is with GPTOSS-specific calculations")
		} else {
			t.Logf("GPTOSS works fine: %d layers fit", estimate.Layers)
		}
	})

	// CPU-only estimation removed - not properly supported and adds confusion
	// All memory estimation should be GPU-based since we don't properly support CPU inference
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
