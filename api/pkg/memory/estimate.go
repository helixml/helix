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

	// Use Ollama's exact memory estimation logic
	ollamaEstimate := estimateOllamaGPULayers(ollamaGPUs, ollamaModel, ollamaOpts, opts.NumParallel)

	// Convert back to our format
	return ConvertFromOllamaEstimate(ollamaEstimate, gpus, metadata, opts)
}

// calculateLayerSize calculates the memory size of a single transformer layer
func calculateLayerSize(metadata *ModelMetadata) uint64 {
	// Try to get size from first block layer
	if layerInfo, exists := metadata.Layers["blk.0"]; exists {
		var size uint64
		for _, tensor := range layerInfo.Tensors {
			size += tensor.Size
		}
		return size
	}

	// Fallback: estimate based on architecture parameters
	return estimateLayerSizeFromParams(metadata)
}

// calculateKVCachePerLayer calculates KV cache memory per layer (matches Ollama's approach)
func calculateKVCachePerLayer(metadata *ModelMetadata, opts EstimateOptions) uint64 {
	context := uint64(opts.NumCtx * opts.NumParallel)
	embeddingHeadsK := metadata.KeyLength
	embeddingHeadsV := metadata.ValueLength
	headsKV := metadata.HeadCountKV

	// Determine bytes per element based on KV cache type
	bytesPerElement := getKVCacheBytesPerElement(opts.KVCacheType)

	kvCacheSize := uint64(float64(context*(embeddingHeadsK+embeddingHeadsV)*headsKV) * bytesPerElement)

	// Debug logging for large context windows
	if opts.NumCtx > 32768 {
		log.Warn().
			Str("KV_CACHE_DEBUG", "large_context").
			Int("num_ctx", opts.NumCtx).
			Uint64("context_tokens", context).
			Uint64("embedding_heads_k", embeddingHeadsK).
			Uint64("embedding_heads_v", embeddingHeadsV).
			Uint64("heads_kv", headsKV).
			Float64("bytes_per_element", bytesPerElement).
			Str("kv_cache_type", opts.KVCacheType).
			Uint64("kv_cache_per_layer_mb", kvCacheSize/(1024*1024)).
			Msg("KV cache calculation for large context window")
	}

	return kvCacheSize
}

// calculateKVCache calculates the total KV cache memory requirements (legacy function)
func calculateKVCache(metadata *ModelMetadata, opts EstimateOptions) uint64 {
	kvPerLayer := calculateKVCachePerLayer(metadata, opts)
	totalKVCache := kvPerLayer * metadata.BlockCount

	// Debug logging for large context windows
	if opts.NumCtx > 32768 {
		log.Warn().
			Str("KV_CACHE_DEBUG", "total_calculation").
			Int("num_ctx", opts.NumCtx).
			Uint64("kv_per_layer_mb", kvPerLayer/(1024*1024)).
			Uint64("block_count", metadata.BlockCount).
			Uint64("total_kv_cache_mb", totalKVCache/(1024*1024)).
			Uint64("total_kv_cache_gb", totalKVCache/(1024*1024*1024)).
			Msg("Total KV cache calculation for large context window")
	}

	return totalKVCache
}

// calculateGraphMemory calculates graph memory requirements based on architecture
func calculateGraphMemory(metadata *ModelMetadata, opts EstimateOptions) (partial, full uint64) {
	batch := uint64(opts.NumBatch)
	context := uint64(opts.NumCtx * opts.NumParallel)
	embedding := metadata.EmbeddingLength
	heads := metadata.HeadCount
	headsKV := metadata.HeadCountKV
	vocab := metadata.VocabSize

	// Calculate embedding heads for compatibility
	var embeddingHeads, embeddingHeadsK uint64
	if heads > 0 {
		embeddingHeads = embedding / heads
		embeddingHeadsK = metadata.KeyLength
		// embeddingHeadsV would be metadata.ValueLength if needed
	}

	switch metadata.Architecture {
	case ArchitectureLlama, ArchitectureLlama4:
		full = max64(
			4*batch*(1+4*embedding+context*(1+heads)),
			4*batch*(embedding+vocab),
		)

		partial = 4 * batch * embedding
		partial += max64(
			4*batch*(1+embedding+max64(context, embedding))+embedding*embedding*9/16+4*context*(batch*heads+embeddingHeads*headsKV),
			4*batch*(embedding+vocab)+embedding*vocab*105/128,
		)

	case ArchitectureQwen2, ArchitectureQwen3:
		full = max64(
			4*batch*(embedding+vocab),
			4*batch*(1+2*embedding+context+context*heads),
		)

		partial = max64(
			4*batch*(embedding+vocab)+embedding*vocab*105/128,
			4*(batch*(1+2*embedding+context*(1+heads))+embedding*(1+context)),
		)

	case ArchitectureGemma, ArchitectureGemma2, ArchitectureGemma3:
		full = max64(
			4*batch*(embedding+vocab),
			4*batch*(2+context+context*heads+2*embedding+2*embeddingHeadsK*heads),
		)

		partial = max64(
			4*embedding*batch+embedding*vocab*105/128+4*vocab*batch,
			4*batch*(2*embedding+1+2*embeddingHeadsK*heads+context+context*heads)+
				4*embeddingHeadsK*context*8+
				embedding*embeddingHeadsK*heads*9/16,
		)

		// Gemma3 specific adjustments
		if metadata.Architecture == ArchitectureGemma3 {
			full *= 4
			partial *= 4
		}

	case ArchitectureCommandR:
		full = max64(
			4*batch*(embedding+vocab),
			4*batch*(2+4*embedding+context*(1+heads)),
		)

		partial = max64(
			4*batch*(embedding+vocab)+embedding*vocab*105/128,
			4*batch*(1+2*embedding+context*(1+heads))+4*embedding*context+embedding*embedding*9/16,
		)

	case ArchitecturePhi2:
		full = max64(
			4*batch*(embedding+vocab),
			4*batch*(1+4*embedding+context+context*heads),
		)

		partial = max64(
			4*batch*(2*embedding+vocab)+embedding*vocab*105/128,
			4*batch*(2+3*embedding+context+context*heads),
		)

	case ArchitectureStableLM:
		full = 4 * batch * (context*(1+heads) + 3*embedding + 2)
		partial = max64(
			4*batch*(vocab+2*embedding),
			full,
		)

	case ArchitectureDeepSeek2:
		full = max64(
			4*batch*(3*embedding+vocab),
			4*batch*(3*embedding+2+context*(1+headsKV)+2*embeddingHeadsK*headsKV),
		)

		partial = max64(
			4*batch*(3*embedding+vocab)+embedding*vocab*105/128,
			4*batch*(2*embedding+1+2*embeddingHeadsK*headsKV+context+context*headsKV)+4*embeddingHeadsK*context*headsKV+embedding*embeddingHeadsK*headsKV*9/16,
		)

	case ArchitectureChatGLM:
		full = 4 * batch * (embedding + vocab)
		partial = 4*batch*(embedding+vocab) + embedding*vocab*105/128

	case ArchitectureGPTOSS:
		// Special handling for GPT-OSS architecture
		partial = 2 * heads / max64(headsKV, 1) * calculateKVCache(metadata, opts) / 6
		full = partial

	default:
		// Fallback to LLaMA calculation for unknown architectures
		return calculateGraphMemory(&ModelMetadata{
			Architecture:    ArchitectureLlama,
			EmbeddingLength: metadata.EmbeddingLength,
			HeadCount:       metadata.HeadCount,
			HeadCountKV:     metadata.HeadCountKV,
			VocabSize:       metadata.VocabSize,
			BlockCount:      metadata.BlockCount,
		}, opts)
	}

	return partial, full
}

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
	} else {
		layerSize = estimateLayerSizeFromParams(metadata)
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
			kvTotal += kvPerLayer
		}
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
	totalSize := calculateTotalSize(metadata, 0)
	kvCache := calculateKVCache(metadata, opts)
	weights := calculateWeightsSize(metadata)

	return &MemoryEstimate{
		Layers:           0,
		Graph:            0,
		VRAMSize:         0,
		TotalSize:        totalSize,
		KVCache:          kvCache,
		Weights:          weights,
		GraphMem:         0,
		Architecture:     metadata.Architecture,
		EstimatedAt:      time.Now(),
		FullyLoaded:      false,
		RequiresFallback: true,
		Options:          opts,
		GPUs:             []GPUInfo{},
	}
}

// calculateTotalMemoryRequirement calculates the total memory requirement including overflow
// This matches Ollama's logic where TotalSize = GPU allocation + overflow (parts that don't fit on GPU)
func calculateTotalMemoryRequirement(metadata *ModelMetadata, layersOnGPU int, kvTotal, graphMem uint64) uint64 {
	// Calculate total model weights
	weights := calculateWeightsSize(metadata)

	// Total layers in the model (including output layer)
	totalLayers := int(metadata.BlockCount) + 1

	// If all layers fit on GPU, total size is just the weights + KV cache + graph
	if layersOnGPU >= totalLayers {
		return weights + kvTotal + graphMem
	}

	// If partial GPU loading, we need weights + KV cache + graph + some overhead
	// The KV cache and graph are always loaded, but weights might be split between GPU/CPU
	overhead := weights / 10 // 10% overhead for model loading and processing
	return weights + kvTotal + graphMem + overhead
}

// calculateTotalSize calculates the total memory requirement for the model (legacy function)
func calculateTotalSize(metadata *ModelMetadata, vramSize uint64) uint64 {
	weights := calculateWeightsSize(metadata)

	// Add some overhead for model loading and processing
	overhead := weights / 10 // 10% overhead

	return weights + overhead
}

// calculateWeightsSize calculates the total size of model weights
func calculateWeightsSize(metadata *ModelMetadata) uint64 {
	var totalSize uint64
	for _, layer := range metadata.Layers {
		for _, tensor := range layer.Tensors {
			totalSize += tensor.Size
		}
	}
	return totalSize
}

// estimateLayerSizeFromParams estimates layer size from architecture parameters
func estimateLayerSizeFromParams(metadata *ModelMetadata) uint64 {
	embedding := metadata.EmbeddingLength
	ffn := metadata.FFLength

	if embedding == 0 || ffn == 0 {
		return 0
	}

	// Approximate size for attention + FFN weights in a typical layer
	attnSize := embedding * embedding * 4 // Q, K, V, O projections
	ffnSize := embedding * ffn * 2        // Up and down projections

	// Account for quantization (rough approximation)
	bytesPerParam := uint64(2) // Assume average of 2 bytes per parameter for mixed quantization

	return (attnSize + ffnSize) * bytesPerParam
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
