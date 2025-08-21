package memory

import (
	"fmt"
	"sort"
	"time"

	"github.com/rs/zerolog/log"
)

// EstimateGPULayers estimates how many layers can be loaded on the given GPUs
// This now uses Ollama's exact memory estimation code (MIT licensed)
func EstimateGPULayers(gpus []GPUInfo, metadata *ModelMetadata, opts EstimateOptions) *MemoryEstimate {
	if len(gpus) == 0 || (len(gpus) == 1 && gpus[0].Library == "cpu") {
		return estimateCPUOnly(metadata, opts)
	}

	// Convert our types to Ollama's format
	ollamaGPUs, ollamaModel, ollamaOpts := ConvertToOllamaTypes(gpus, metadata, opts)

	// Use Ollama's exact memory estimation logic (v0.11.4)
	ollamaEstimate := estimateGPULayers(ollamaGPUs, ollamaModel, ollamaOpts, opts.NumParallel)

	// Convert back to our format
	return ConvertFromOllamaEstimate(ollamaEstimate, gpus, metadata, opts)
}

// calculateGraphMemory calculates graph memory requirements based on architecture

// distributeLayers distributes model layers across available GPUs
func distributeLayers(gpus []GPUInfo, metadata *ModelMetadata, layerSize, kvTotal, graphPartial, graphFull uint64, opts EstimateOptions) (layerCount int, tensorSplit []int, gpuAllocations []uint64) {
	tensorSplit = make([]int, len(gpus))
	gpuAllocations = make([]uint64, len(gpus))

	// Reserve overhead and graph memory
	overhead := uint64(DefaultGPUOverhead)

	// Filter GPUs that have sufficient memory for at least overhead + graph + one layer
	var viableGPUs []int
	for i, gpu := range gpus {
		minRequired := overhead + max64(graphPartial, graphFull) + gpu.MinimumMemory + layerSize
		if gpu.FreeMemory >= minRequired {
			viableGPUs = append(viableGPUs, i)
		}
	}

	if len(viableGPUs) == 0 {
		return 0, tensorSplit, gpuAllocations
	}

	// Sort GPUs by available memory (descending)
	sort.Slice(viableGPUs, func(i, j int) bool {
		return gpus[viableGPUs[i]].FreeMemory > gpus[viableGPUs[j]].FreeMemory
	})

	totalLayers := int(metadata.BlockCount) + 1 // +1 for output layer

	// Distribute layers starting from the end (like Ollama does)
	for layerIdx := totalLayers - 1; layerIdx >= 0 && layerCount < totalLayers; layerIdx-- {
		if opts.NumGPU >= 0 && layerCount >= opts.NumGPU {
			break
		}

		// Find a GPU that can fit this layer
		for _, gpuIdx := range viableGPUs {
			gpu := gpus[gpuIdx]
			currentUsage := gpuAllocations[gpuIdx]
			graphMem := graphPartial
			if layerCount+1 >= totalLayers {
				graphMem = graphFull
			}

			required := overhead + gpu.MinimumMemory + currentUsage + graphMem + layerSize

			if gpu.FreeMemory >= required {
				gpuAllocations[gpuIdx] += layerSize
				tensorSplit[gpuIdx]++
				layerCount++
				break
			}
		}
	}

	// Add graph memory to allocations
	graphMem := graphPartial
	if layerCount >= totalLayers {
		graphMem = graphFull
	}

	for i := range gpuAllocations {
		if tensorSplit[i] > 0 {
			gpuAllocations[i] += graphMem
		}
	}

	return layerCount, tensorSplit, gpuAllocations
}

// distributeLayersLikeOllama implements Ollama's exact layer distribution logic with overflow tracking
func distributeLayersLikeOllama(gpus []GPUInfo, metadata *ModelMetadata, kvPerLayer, graphPartial, graphFull uint64, opts EstimateOptions) (layerCount int, tensorSplit []int, gpuAllocations []uint64, overflow uint64, kvTotal uint64, memoryWeights uint64) {
	tensorSplit = make([]int, len(gpus))
	gpuAllocations = make([]uint64, len(gpus))

	overhead := uint64(DefaultGPUOverhead)

	// Get actual layer size for blk.0 as buffer (like Ollama does)
	var layerSize uint64
	if layerInfo, exists := metadata.Layers["blk.0"]; exists {
		for _, tensor := range layerInfo.Tensors {
			layerSize += tensor.Size
		}
		layerSize += kvPerLayer
	}

	// Debug logging for layer distribution
	log.Debug().
		Str("LAYER_DEBUG", "distribution_start").
		Str("architecture", metadata.Architecture).
		Uint64("block_count", metadata.BlockCount).
		Uint64("layer_size_bytes", layerSize).
		Uint64("layer_size_mb", layerSize/(1024*1024)).
		Uint64("kv_per_layer_bytes", kvPerLayer).
		Uint64("kv_per_layer_mb", kvPerLayer/(1024*1024)).
		Uint64("overhead_bytes", overhead).
		Uint64("overhead_mb", overhead/(1024*1024)).
		Int("gpu_count", len(gpus)).
		Uint64("graph_partial_mb", graphPartial/(1024*1024)).
		Uint64("graph_full_mb", graphFull/(1024*1024)).
		Msg("Starting layer distribution")

	// Log GPU information
	for i, gpu := range gpus {
		log.Debug().
			Str("LAYER_DEBUG", "gpu_info").
			Int("gpu_index", i).
			Uint64("total_memory_bytes", gpu.TotalMemory).
			Uint64("total_memory_mb", gpu.TotalMemory/(1024*1024)).
			Uint64("free_memory_bytes", gpu.FreeMemory).
			Uint64("free_memory_mb", gpu.FreeMemory/(1024*1024)).
			Uint64("minimum_memory_bytes", gpu.MinimumMemory).
			Uint64("minimum_memory_mb", gpu.MinimumMemory/(1024*1024)).
			Msg("GPU configuration for layer distribution")
	}

	// Add KV cache to layer size (like Ollama line 207)
	if kvPerLayer > 0 {
		layerSize += kvPerLayer
	}

	// Filter GPUs that have sufficient space (like Ollama)
	type gpuWithSpace struct {
		i int
		g *GPUInfo
	}
	gpusWithSpace := []gpuWithSpace{}

	for i := range gpus {
		// Only include GPUs that can fit the graph, overhead, minimum memory and at least one layer
		minRequired := overhead + gpus[i].MinimumMemory + max64(graphPartial, graphFull) + 2*layerSize
		if gpus[i].FreeMemory >= minRequired {
			gpusWithSpace = append(gpusWithSpace, gpuWithSpace{i, &gpus[i]})
			gpuAllocations[i] += gpus[i].MinimumMemory + layerSize // Reserve space like Ollama
		}
	}

	if len(gpusWithSpace) == 0 {
		// No GPU has enough space, everything goes to overflow
		totalLayers := int(metadata.BlockCount) + 1
		log.Debug().
			Str("LAYER_DEBUG", "cpu_fallback").
			Int("total_layers", totalLayers).
			Msg("All layers will run on CPU")

		for layerIdx := totalLayers - 1; layerIdx >= 0; layerIdx-- {
			// Get actual layer size for this layer
			currentLayerSize := layerSize // Default
			if layerIdx < int(metadata.BlockCount) {
				if layerInfo, exists := metadata.Layers[fmt.Sprintf("blk.%d", layerIdx)]; exists {
					currentLayerSize = 0
					for _, tensor := range layerInfo.Tensors {
						currentLayerSize += tensor.Size
					}
					currentLayerSize += kvPerLayer
					memoryWeights += currentLayerSize - kvPerLayer
				}
			} else {
				// Output layer
				currentLayerSize = 0
				if layerInfo, exists := metadata.Layers["output_norm"]; exists {
					for _, tensor := range layerInfo.Tensors {
						currentLayerSize += tensor.Size
					}
				}
				if layerInfo, exists := metadata.Layers["output"]; exists {
					for _, tensor := range layerInfo.Tensors {
						currentLayerSize += tensor.Size
					}
				}
				memoryWeights += currentLayerSize
			}
			overflow += currentLayerSize
		}
		kvTotal = kvPerLayer * metadata.BlockCount

		log.Debug().
			Str("LAYER_DEBUG", "cpu_fallback_result").
			Uint64("total_overflow_bytes", overflow).
			Uint64("total_overflow_mb", overflow/(1024*1024)).
			Uint64("kv_total_bytes", kvTotal).
			Uint64("kv_total_mb", kvTotal/(1024*1024)).
			Uint64("memory_weights_bytes", memoryWeights).
			Uint64("memory_weights_mb", memoryWeights/(1024*1024)).
			Msg("CPU fallback calculation complete")

		return 0, tensorSplit, gpuAllocations, overflow, kvTotal, memoryWeights
	}

	// Distribute layers starting from the end (like Ollama does)
	totalLayers := int(metadata.BlockCount) + 1
	for layerIdx := totalLayers - 1; layerIdx >= 0; layerIdx-- {
		// Get actual layer size for this specific layer (like Ollama lines 295-299)
		currentLayerSize := layerSize // Default
		if layerIdx < int(metadata.BlockCount) {
			// Regular block layer
			if layerInfo, exists := metadata.Layers[fmt.Sprintf("blk.%d", layerIdx)]; exists {
				currentLayerSize = 0
				for _, tensor := range layerInfo.Tensors {
					currentLayerSize += tensor.Size
				}
				currentLayerSize += kvPerLayer                 // Add KV cache like Ollama line 297
				memoryWeights += currentLayerSize - kvPerLayer // Track weights separately
			}
		} else {
			// Output layer (like Ollama lines 236-243)
			currentLayerSize = 0
			if layerInfo, exists := metadata.Layers["output_norm"]; exists {
				for _, tensor := range layerInfo.Tensors {
					currentLayerSize += tensor.Size
				}
			}
			if layerInfo, exists := metadata.Layers["output"]; exists {
				for _, tensor := range layerInfo.Tensors {
					currentLayerSize += tensor.Size
				}
			} else if layerInfo, exists := metadata.Layers["token_embd"]; exists {
				for _, tensor := range layerInfo.Tensors {
					currentLayerSize += tensor.Size
				}
			}
			memoryWeights += currentLayerSize
		}

		kvTotal += kvPerLayer

		if opts.NumGPU >= 0 && layerCount >= opts.NumGPU {
			// Stop allocating on GPU once we hit the user's target NumGPU (like Ollama lines 301-305)
			overflow += currentLayerSize
			continue
		}

		// Try to fit this layer on available GPUs (like Ollama lines 307-319)
		fitted := false
		for j := len(gpusWithSpace); j > 0; j-- {
			g := gpusWithSpace[layerIdx%j]
			used := gpuAllocations[g.i] + max64(graphPartial, graphFull)
			if g.g.FreeMemory > overhead+used+currentLayerSize {
				gpuAllocations[g.i] += currentLayerSize
				tensorSplit[g.i]++
				layerCount++
				fitted = true
				break
			} else {
				// Remove GPU from available list if it's full
				gpusWithSpace = append(gpusWithSpace[:layerIdx%j], gpusWithSpace[layerIdx%j+1:]...)
			}
		}

		if !fitted {
			// Layer doesn't fit on any GPU, add to overflow (like Ollama lines 321-323)
			overflow += currentLayerSize
		}
	}

	// Add graph allocations to GPUs that have layers (like Ollama lines 352-361)
	for i := range gpus {
		if tensorSplit[i] <= 0 {
			continue
		}
		fullyLoaded := layerCount >= totalLayers
		if fullyLoaded {
			gpuAllocations[i] += graphFull
		} else {
			gpuAllocations[i] += graphPartial
		}
	}

	return layerCount, tensorSplit, gpuAllocations, overflow, kvTotal, memoryWeights
}

// estimateCPUOnly creates a CPU-only memory estimate
func estimateCPUOnly(metadata *ModelMetadata, opts EstimateOptions) *MemoryEstimate {
	// Simple CPU-only calculation
	var totalSize uint64
	for _, layer := range metadata.Layers {
		for _, tensor := range layer.Tensors {
			totalSize += tensor.Size
		}
	}

	return &MemoryEstimate{
		Layers:           0,
		Graph:            0,
		VRAMSize:         0,
		TotalSize:        totalSize,
		KVCache:          0,
		Weights:          totalSize,
		GraphMem:         0,
		Architecture:     metadata.Architecture,
		EstimatedAt:      time.Now(),
		FullyLoaded:      false,
		RequiresFallback: true,
		Options:          opts,
	}
}

// getKVCacheBytesPerElement returns bytes per element for KV cache type
func getKVCacheBytesPerElement(cacheType string) float64 {
	switch cacheType {
	case "q8_0":
		return KVCacheQ8_0
	case "q4_0":
		return KVCacheQ4_0
	case "f16", "":
		return KVCacheF16
	default:
		return KVCacheF16 // Default to f16
	}
}

// Helper functions
func max64(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}

func min64(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// EstimateModelMemory is the main entry point for memory estimation
func EstimateModelMemory(modelPath string, gpuConfig []GPUInfo, opts EstimateOptions) (*EstimationResult, error) {
	// Load model metadata
	metadata, err := LoadModelMetadata(modelPath)
	if err != nil {
		return &EstimationResult{
			ModelPath:   modelPath,
			Error:       err.Error(),
			EstimatedAt: time.Now(),
		}, err
	}

	result := &EstimationResult{
		ModelPath:   modelPath,
		Metadata:    metadata,
		EstimatedAt: time.Now(),
	}

	// Validate options
	if opts.NumCtx <= 0 {
		opts.NumCtx = 4096
	}
	if opts.NumBatch <= 0 {
		opts.NumBatch = 512
	}
	if opts.NumParallel <= 0 {
		opts.NumParallel = 1
	}

	// CPU-only estimate (always available)
	result.CPUOnly = estimateCPUOnly(metadata, opts)

	if len(gpuConfig) == 0 {
		result.Recommendation = "cpu_only"
		return result, nil
	}

	// Single GPU estimate
	if len(gpuConfig) >= 1 {
		singleGPU := []GPUInfo{gpuConfig[0]}
		result.SingleGPU = EstimateGPULayers(singleGPU, metadata, opts)
	}

	// Multi-GPU estimate (tensor parallel)
	if len(gpuConfig) > 1 {
		result.TensorParallel = EstimateGPULayers(gpuConfig, metadata, opts)
	}

	// Determine recommendation
	result.Recommendation = determineRecommendation(result)

	return result, nil
}

// determineRecommendation determines the best deployment strategy
func determineRecommendation(result *EstimationResult) string {
	// Priority: tensor parallel > single GPU > CPU only

	if result.TensorParallel != nil && result.TensorParallel.Layers > 0 {
		if result.TensorParallel.FullyLoaded {
			return "tensor_parallel"
		}
		// Check if tensor parallel is significantly better than single GPU
		if result.SingleGPU != nil {
			if result.TensorParallel.Layers > result.SingleGPU.Layers*2 {
				return "tensor_parallel"
			}
		}
	}

	if result.SingleGPU != nil && result.SingleGPU.Layers > 0 {
		// Single GPU is viable
		if result.SingleGPU.FullyLoaded {
			return "single_gpu"
		}
		// Check if partial GPU offload is better than CPU
		totalLayers := int(result.Metadata.BlockCount) + 1
		if result.SingleGPU.Layers >= totalLayers/2 {
			return "single_gpu"
		}
	}

	// Check if any GPU option is available
	hasGPUOption := (result.SingleGPU != nil && result.SingleGPU.Layers > 0) ||
		(result.TensorParallel != nil && result.TensorParallel.Layers > 0)

	if !hasGPUOption {
		return "insufficient_memory"
	}

	return "cpu_only"
}

// ValidateEstimateOptions validates estimation options
func ValidateEstimateOptions(opts EstimateOptions) error {
	if opts.NumCtx < 1 || opts.NumCtx > 1000000 {
		return &EstimationError{
			Type:    "invalid_options",
			Message: "invalid context size",
			Details: fmt.Sprintf("context size must be between 1 and 1,000,000, got %d", opts.NumCtx),
		}
	}

	if opts.NumBatch < 1 || opts.NumBatch > 10000 {
		return &EstimationError{
			Type:    "invalid_options",
			Message: "invalid batch size",
			Details: fmt.Sprintf("batch size must be between 1 and 10,000, got %d", opts.NumBatch),
		}
	}

	if opts.NumParallel < 1 || opts.NumParallel > 100 {
		return &EstimationError{
			Type:    "invalid_options",
			Message: "invalid parallel count",
			Details: fmt.Sprintf("parallel count must be between 1 and 100, got %d", opts.NumParallel),
		}
	}

	if opts.KVCacheType != "" && opts.KVCacheType != "f16" && opts.KVCacheType != "q8_0" && opts.KVCacheType != "q4_0" {
		return &EstimationError{
			Type:    "invalid_options",
			Message: "invalid KV cache type",
			Details: fmt.Sprintf("KV cache type must be one of: f16, q8_0, q4_0, got %s", opts.KVCacheType),
		}
	}

	return nil
}
