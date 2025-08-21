package memory

import (
	"fmt"
	"log/slog"
	"time"
)

// OllamaGpuInfo mirrors discover.GpuInfo from Ollama v0.11.4
type OllamaGpuInfo struct {
	Library       string
	ID            string
	Name          string
	Compute       string
	Index         int
	FreeMemory    uint64
	TotalMemory   uint64
	MinimumMemory uint64
	DriverMajor   int
	DriverMinor   int
}

// OllamaModelInfo mirrors the model info from Ollama v0.11.4
type OllamaModelInfo struct {
	BlockCount      uint64
	HeadCountMax    uint64
	HeadCountKVMin  uint64
	EmbeddingLength uint64
	KeyLength       uint64
	ValueLength     uint64
	Layers          map[string]LayerInfo
	Architecture    string
}

// OllamaOptions mirrors api.Options from Ollama v0.11.4
type OllamaOptions struct {
	NumCtx   int
	NumBatch int
	NumGPU   int
}

// OllamaMemoryEstimate mirrors MemoryEstimate from Ollama v0.11.4
type OllamaMemoryEstimate struct {
	// How many layers we predict we can load
	Layers int

	// The size of the graph which occupies the main GPU
	Graph uint64

	// How much VRAM will be allocated given the number of layers we predict
	VRAMSize uint64

	// The total size of the model if loaded into VRAM.  If all layers are loaded, VRAMSize == TotalSize
	TotalSize uint64

	// For multi-GPU scenarios, this provides the tensor split parameter
	TensorSplit []int

	// For multi-GPU scenarios, this is the size in bytes per GPU
	GPUSizes []uint64

	// internal fields for logging purposes
	inferenceLibrary    string
	layersRequested     int
	layersModel         int
	availableList       []string
	kv                  uint64
	allocationsList     []string
	memoryWeights       uint64
	memoryLayerOutput   uint64
	graphFullOffload    uint64
	graphPartialOffload uint64

	projectorWeights, projectorGraph uint64
}

// estimateGPULayers is the main estimation function from Ollama v0.11.4
func estimateGPULayers(gpus []OllamaGpuInfo, f *OllamaModelInfo, opts OllamaOptions, numParallel int) OllamaMemoryEstimate {
	// Graph size for a partial offload, applies to all GPUs
	var graphPartialOffload uint64

	// Graph size when all layers are offloaded, applies to all GPUs
	var graphFullOffload uint64

	// Final graph offload once we know full or partial
	var graphOffload uint64

	// Projectors loaded into GPU0 only
	var llamaEngineProjectorWeights uint64

	// Projectors loaded with output layer
	var ollamaEngineProjectorWeights uint64
	var ollamaEngineProjectorGraph uint64

	// Conditional output size on GPU 0
	var memoryLayerOutput uint64

	// The sizes of a layer
	var layerSize uint64

	// The sum of all the layer sizes (just for logging)
	var memoryWeights uint64

	// True if all the layers are loaded
	var fullyLoaded bool

	// Overflow that didn't fit into the GPU
	var overflow uint64

	overhead := uint64(512 * 1024 * 1024) // 512MB default GPU overhead
	availableList := make([]string, len(gpus))
	for i, gpu := range gpus {
		availableList[i] = formatBytes(gpu.FreeMemory)
	}
	slog.Debug("evaluating", "library", gpus[0].Library, "gpu_count", len(gpus), "available", availableList)

	// DEBUG: Log comprehensive model info to debug 0 layers issue
	slog.Info("ZERO_LAYERS_DEBUG: Model metadata",
		"block_count", f.BlockCount,
		"head_count_max", f.HeadCountMax,
		"head_count_kv_min", f.HeadCountKVMin,
		"embedding_length", f.EmbeddingLength,
		"key_length", f.KeyLength,
		"value_length", f.ValueLength,
		"layer_count", len(f.Layers),
		"num_ctx", opts.NumCtx,
		"num_batch", opts.NumBatch,
		"num_parallel", numParallel)

	// Get layer size from blk.0 as a buffer
	if blk0, ok := f.Layers["blk.0"]; ok {
		for _, tensor := range blk0.Tensors {
			layerSize += tensor.Size
		}
		slog.Info("ZERO_LAYERS_DEBUG: blk.0 found", "size_mb", layerSize/(1024*1024), "tensor_count", len(blk0.Tensors))
	} else {
		slog.Error("ZERO_LAYERS_DEBUG: model missing blk.0 layer - this will cause issues!")
	}

	// Calculate KV cache and graph sizes using v0.11.4 logic with architecture-specific handling
	slog.Info("GPTOSS_MEMORY_DEBUG: Starting graph calculation",
		"architecture", f.Architecture,
		"num_ctx", opts.NumCtx,
		"num_batch", ollamaMin(opts.NumCtx, opts.NumBatch),
		"num_parallel", numParallel,
		"head_count_max", f.HeadCountMax,
		"head_count_kv_min", f.HeadCountKVMin,
		"embedding_length", f.EmbeddingLength,
		"key_length", f.KeyLength,
		"value_length", f.ValueLength,
		"block_count", f.BlockCount)

	kv, graphPartialOffload, graphFullOffload := calculateGraphSizeV11_4(f, uint64(opts.NumCtx), uint64(ollamaMin(opts.NumCtx, opts.NumBatch)), numParallel, f.Architecture)

	slog.Info("GPTOSS_MEMORY_DEBUG: Graph calculation result",
		"graph_partial_bytes", graphPartialOffload,
		"graph_partial_gb", float64(graphPartialOffload)/(1024*1024*1024),
		"graph_full_bytes", graphFullOffload,
		"graph_full_gb", float64(graphFullOffload)/(1024*1024*1024),
		"kv_layers_count", len(kv))

	if len(kv) > 0 {
		layerSize += kv[0]
	}

	var kvTotal uint64
	for _, kvLayer := range kv {
		kvTotal += kvLayer
	}

	if graphPartialOffload == 0 {
		headsKV := f.HeadCountKVMin
		if headsKV == 0 {
			headsKV = 1
		}
		gqa := f.HeadCountMax / headsKV
		graphPartialOffload = gqa * kvTotal / 6
	}
	if graphFullOffload == 0 {
		graphFullOffload = graphPartialOffload
	}

	// on metal there's no partial offload overhead
	if gpus[0].Library == "metal" {
		graphPartialOffload = graphFullOffload
	} else if len(gpus) > 1 {
		// multigpu should always use the partial graph size
		graphFullOffload = graphPartialOffload
	}

	// Output layer handled at the end if we have space
	if layer, ok := f.Layers["output_norm"]; ok {
		for _, tensor := range layer.Tensors {
			memoryLayerOutput += tensor.Size
		}
	}
	if layer, ok := f.Layers["output"]; ok {
		for _, tensor := range layer.Tensors {
			memoryLayerOutput += tensor.Size
		}
	} else if layer, ok := f.Layers["token_embd"]; ok {
		for _, tensor := range layer.Tensors {
			memoryLayerOutput += tensor.Size
		}
	}

	gpuZeroOverhead := llamaEngineProjectorWeights

	// Reduce set of GPUs to only those that have sufficient space to fit overhead and at least one layer
	var layerCount int
	tensorSplit := make([]int, len(gpus))
	gpuAllocations := make([]uint64, len(gpus))
	type gs struct {
		i int
		g *OllamaGpuInfo
	}
	gpusWithSpace := []gs{}
	for i := range gpus {
		var gzo uint64
		if len(gpusWithSpace) == 0 {
			gzo = gpuZeroOverhead
		}

		requiredMemory := overhead + gzo + ollamaMax64(graphPartialOffload, graphFullOffload) + gpus[i].MinimumMemory + 2*layerSize

		// Only include GPUs that can fit the graph, gpu minimum, the layer buffer and at least one more layer
		if gpus[i].FreeMemory < requiredMemory {
			slog.Debug("gpu has too little memory to allocate any layers",
				"id", gpus[i].ID,
				"library", gpus[i].Library,
				"name", gpus[i].Name,
				"total", formatBytes(gpus[i].TotalMemory),
				"available", formatBytes(gpus[i].FreeMemory),
				"minimum_memory", gpus[i].MinimumMemory,
				"layer_size", formatBytes(layerSize),
				"gpu_zero_overhead", formatBytes(gzo),
				"partial_offload", formatBytes(graphPartialOffload),
				"full_offload", formatBytes(graphFullOffload),
			)
			continue
		}
		gpusWithSpace = append(gpusWithSpace, gs{i, &gpus[i]})
		gpuAllocations[i] += gpus[i].MinimumMemory + layerSize // We hold off on graph until we know partial vs. full
	}

	var gpuZeroID int
	if len(gpusWithSpace) > 0 {
		gpuZeroID = gpusWithSpace[0].i
		gpuAllocations[gpuZeroID] += gpuZeroOverhead
	} else {
		overflow += gpuZeroOverhead
	}

	slog.Debug("ZERO_LAYERS_DEBUG: Starting layer distribution",
		"block_count", f.BlockCount,
		"gpus_with_space", len(gpusWithSpace))

	// For all the layers, find where they can fit on the GPU(s)
	for i := int(f.BlockCount) - 1; i >= 0; i-- {
		// Some models have inconsistent layer sizes
		if blk, ok := f.Layers[fmt.Sprintf("blk.%d", i)]; ok {
			layerSize = 0
			for _, tensor := range blk.Tensors {
				layerSize += tensor.Size
			}
			layerSize += kv[i]
			for _, tensor := range blk.Tensors {
				memoryWeights += tensor.Size
			}
		}

		if opts.NumGPU >= 0 && layerCount >= opts.NumGPU {
			// Stop allocating on GPU(s) once we hit the users target NumGPU
			overflow += layerSize
			continue
		}

		// distribute the layers across the GPU(s) that have space
		for j := len(gpusWithSpace); j > 0; j-- {
			g := gpusWithSpace[i%j]
			used := gpuAllocations[g.i] + ollamaMax64(graphPartialOffload, graphFullOffload)
			if g.g.FreeMemory > overhead+used+layerSize {
				gpuAllocations[g.i] += layerSize
				tensorSplit[g.i]++
				layerCount++
				break
			} else {
				gpusWithSpace = append(gpusWithSpace[:i%j], gpusWithSpace[i%j+1:]...)
			}
		}

		if len(gpusWithSpace) == 0 {
			overflow += layerSize
		}
	}
	if layerCount >= int(f.BlockCount) {
		fullyLoaded = true
	}

	// Determine if we need to consider output then find where it fits
	memoryLastLayer := memoryLayerOutput + ollamaEngineProjectorWeights + ollamaEngineProjectorGraph
	if memoryLastLayer > 0 {
		if opts.NumGPU < 0 || layerCount < opts.NumGPU {
			for j := len(gpusWithSpace); j > 0; j-- {
				g := gpusWithSpace[layerCount%j]
				used := gpuAllocations[g.i] + ollamaMax64(graphPartialOffload, graphFullOffload)
				if g.g.FreeMemory > overhead+used+memoryLastLayer {
					gpuAllocations[g.i] += memoryLastLayer
					tensorSplit[g.i]++
					layerCount++
					break
				}
			}
		}

		if layerCount < int(f.BlockCount)+1 {
			fullyLoaded = false
			overflow += memoryLastLayer
		}
	}

	// Add the applicable (full or partial) graph allocations
	for i := range gpus {
		if tensorSplit[i] <= 0 {
			continue
		}
		if fullyLoaded {
			gpuAllocations[i] += graphFullOffload
		} else {
			gpuAllocations[i] += graphPartialOffload
		}
	}
	if fullyLoaded {
		graphOffload = graphFullOffload
	} else {
		graphOffload = graphPartialOffload
	}

	// Summaries for the log
	var memoryRequiredPartial, memoryRequiredTotal uint64
	for i := range gpuAllocations {
		memoryRequiredPartial += gpuAllocations[i]
	}
	memoryRequiredTotal = memoryRequiredPartial + overflow

	allocationsList := []string{}
	for _, a := range gpuAllocations {
		allocationsList = append(allocationsList, formatBytes(a))
	}

	slog.Info("GPTOSS_MEMORY_DEBUG: Final memory calculation summary",
		"memory_required_total_bytes", memoryRequiredTotal,
		"memory_required_total_gb", float64(memoryRequiredTotal)/(1024*1024*1024),
		"memory_required_partial_bytes", memoryRequiredPartial,
		"memory_required_partial_gb", float64(memoryRequiredPartial)/(1024*1024*1024),
		"kv_total_bytes", kvTotal,
		"kv_total_gb", float64(kvTotal)/(1024*1024*1024),
		"memory_weights_bytes", memoryWeights,
		"memory_weights_gb", float64(memoryWeights)/(1024*1024*1024),
		"graph_full_offload_bytes", graphFullOffload,
		"graph_full_offload_gb", float64(graphFullOffload)/(1024*1024*1024),
		"graph_partial_offload_bytes", graphPartialOffload,
		"graph_partial_offload_gb", float64(graphPartialOffload)/(1024*1024*1024),
		"layer_count", layerCount,
		"overflow_bytes", overflow,
		"overflow_gb", float64(overflow)/(1024*1024*1024))

	estimate := OllamaMemoryEstimate{
		TotalSize: memoryRequiredTotal,
		Layers:    0,
		Graph:     0,
		VRAMSize:  0,
		GPUSizes:  []uint64{},

		inferenceLibrary:    gpus[0].Library,
		layersRequested:     opts.NumGPU,
		layersModel:         int(f.BlockCount) + 1,
		availableList:       availableList,
		kv:                  kvTotal,
		allocationsList:     allocationsList,
		memoryWeights:       memoryWeights,
		memoryLayerOutput:   memoryLayerOutput,
		graphFullOffload:    graphFullOffload,
		graphPartialOffload: graphPartialOffload,
		projectorWeights:    llamaEngineProjectorWeights + ollamaEngineProjectorWeights,
		projectorGraph:      ollamaEngineProjectorGraph,
	}

	if gpus[0].Library == "cpu" {
		return estimate
	}
	if layerCount == 0 {
		slog.Error("ZERO_LAYERS_DEBUG: No layers could fit on GPU!",
			"gpu_count", len(gpus),
			"gpus_with_space_after_filtering", len(gpusWithSpace),
			"overhead_mb", overhead/(1024*1024),
			"graph_partial_mb", graphPartialOffload/(1024*1024),
			"graph_full_mb", graphFullOffload/(1024*1024),
			"layer_size_mb", layerSize/(1024*1024),
			"block_count", f.BlockCount,
			"kv_total_mb", kvTotal/(1024*1024))

		// Log each GPU's memory state for debugging
		for i, gpu := range gpus {
			requiredMemory := overhead + ollamaMax64(graphPartialOffload, graphFullOffload) + gpu.MinimumMemory + 2*layerSize
			slog.Error("ZERO_LAYERS_DEBUG: GPU memory check",
				"gpu_index", i,
				"free_memory_mb", gpu.FreeMemory/(1024*1024),
				"required_memory_mb", requiredMemory/(1024*1024),
				"passes_filter", gpu.FreeMemory >= requiredMemory)
		}

		return estimate
	}

	slog.Info("GPTOSS_MEMORY_DEBUG: Successful layer allocation",
		"layer_count", layerCount,
		"total_layers", int(f.BlockCount)+1,
		"fully_loaded", layerCount >= int(f.BlockCount)+1)
	estimate.Layers = layerCount
	estimate.Graph = graphOffload
	estimate.VRAMSize = memoryRequiredPartial
	estimate.TotalSize = memoryRequiredTotal
	estimate.TensorSplit = tensorSplit
	estimate.GPUSizes = gpuAllocations

	slog.Info("GPTOSS_MEMORY_DEBUG: Final estimate",
		"layers", estimate.Layers,
		"graph_bytes", estimate.Graph,
		"graph_gb", float64(estimate.Graph)/(1024*1024*1024),
		"vram_size_bytes", estimate.VRAMSize,
		"vram_size_gb", float64(estimate.VRAMSize)/(1024*1024*1024),
		"total_size_bytes", estimate.TotalSize,
		"total_size_gb", float64(estimate.TotalSize)/(1024*1024*1024))

	return estimate
}

// calculateGraphSizeV11_4 implements v0.11.4 graph size calculation with architecture-specific logic
func calculateGraphSizeV11_4(modelInfo *OllamaModelInfo, numCtx, numBatch uint64, numParallel int, architecture string) ([]uint64, uint64, uint64) {
	context := numCtx * uint64(numParallel)
	embeddingHeadsK := modelInfo.KeyLength
	embeddingHeadsV := modelInfo.ValueLength
	headsKV := modelInfo.HeadCountKVMin
	if headsKV == 0 {
		headsKV = 1
	}

	// Use q8_0 for KV cache (1 byte per element) - matches our runtime setting
	bytesPerElement := float64(1.0)

	// Create KV array for each layer - general calculation first
	kv := make([]uint64, modelInfo.BlockCount)
	var kvTotal uint64

	// Architecture-specific KV cache and graph calculations
	switch architecture {
	case "gptoss":
		slog.Info("GPTOSS_MEMORY_DEBUG: Starting gptoss-specific calculation",
			"embedding_heads_k", embeddingHeadsK,
			"embedding_heads_v", embeddingHeadsV,
			"heads_kv", headsKV,
			"bytes_per_element", bytesPerElement,
			"context", context,
			"num_batch", numBatch,
			"num_parallel", numParallel,
			"block_count", modelInfo.BlockCount)

		// Special gptoss KV cache calculation (from Ollama v0.11.4)
		for i := range kv {
			baseKV := uint64(float64((embeddingHeadsK+embeddingHeadsV)*headsKV) * bytesPerElement)
			if i%2 == 0 {
				kv[i] = baseKV * (uint64(numParallel)*4096 + numBatch)
				slog.Info("GPTOSS_MEMORY_DEBUG: Even layer KV calculation",
					"layer", i,
					"base_kv", baseKV,
					"multiplier", uint64(numParallel)*4096+numBatch,
					"final_kv_bytes", kv[i],
					"final_kv_mb", kv[i]/(1024*1024))
			} else {
				kv[i] = baseKV * context
				slog.Info("GPTOSS_MEMORY_DEBUG: Odd layer KV calculation",
					"layer", i,
					"base_kv", baseKV,
					"multiplier", context,
					"final_kv_bytes", kv[i],
					"final_kv_mb", kv[i]/(1024*1024))
			}
			kvTotal += kv[i]
		}

		slog.Info("GPTOSS_MEMORY_DEBUG: Total KV calculation complete",
			"kv_total_bytes", kvTotal,
			"kv_total_gb", float64(kvTotal)/(1024*1024*1024))

		// Graph calculation for gptoss: 4 * HeadCountMax / HeadCountKVMin * kvTotal / 6
		headCountMax := modelInfo.HeadCountMax
		headCountKVMin := modelInfo.HeadCountKVMin
		if headCountKVMin == 0 {
			headCountKVMin = 1
		}

		slog.Info("GPTOSS_MEMORY_DEBUG: Graph calculation inputs",
			"head_count_max", headCountMax,
			"head_count_kv_min", headCountKVMin,
			"kv_total", kvTotal,
			"formula", "4 * headCountMax / headCountKVMin * kvTotal / 6")

		graphFull := 4 * headCountMax / headCountKVMin * kvTotal / 6
		graphPartial := graphFull

		slog.Info("GPTOSS_MEMORY_DEBUG: Graph calculation result",
			"graph_full_bytes", graphFull,
			"graph_full_gb", float64(graphFull)/(1024*1024*1024),
			"graph_partial_bytes", graphPartial,
			"graph_partial_gb", float64(graphPartial)/(1024*1024*1024))

		return kv, graphPartial, graphFull

	default:
		// General calculation for other architectures
		kvPerLayer := uint64(float64(context*(embeddingHeadsK+embeddingHeadsV)*headsKV) * bytesPerElement)
		for i := range kv {
			kv[i] = kvPerLayer
			kvTotal += kv[i]
		}

		// Standard graph memory estimation
		gqa := modelInfo.HeadCountMax / headsKV
		graphPartial := gqa * kvTotal / 6
		graphFull := graphPartial

		return kv, graphPartial, graphFull
	}
}

// ConvertToOllamaTypes converts our types to Ollama types for v0.11.4 compatibility
func ConvertToOllamaTypes(gpus []GPUInfo, metadata *ModelMetadata, opts EstimateOptions) ([]OllamaGpuInfo, *OllamaModelInfo, OllamaOptions) {
	// Convert GPU info
	ollamaGPUs := make([]OllamaGpuInfo, len(gpus))
	for i, gpu := range gpus {
		ollamaGPUs[i] = OllamaGpuInfo{
			Library:       gpu.Library,
			FreeMemory:    gpu.FreeMemory,
			TotalMemory:   gpu.TotalMemory,
			MinimumMemory: gpu.MinimumMemory,
			ID:            gpu.ID,
			Index:         gpu.Index,
		}
	}

	// Convert model metadata
	ollamaModel := &OllamaModelInfo{
		BlockCount:      metadata.BlockCount,
		HeadCountMax:    metadata.HeadCount,
		HeadCountKVMin:  metadata.HeadCountKV,
		EmbeddingLength: metadata.EmbeddingLength,
		KeyLength:       metadata.KeyLength,
		ValueLength:     metadata.ValueLength,
		Layers:          metadata.Layers,
		Architecture:    metadata.Architecture,
	}

	// Convert options
	ollamaOpts := OllamaOptions{
		NumCtx:   opts.NumCtx,
		NumBatch: opts.NumBatch,
		NumGPU:   opts.NumGPU,
	}

	return ollamaGPUs, ollamaModel, ollamaOpts
}

// ConvertFromOllamaEstimate converts Ollama estimate back to our format
func ConvertFromOllamaEstimate(estimate OllamaMemoryEstimate, gpus []GPUInfo, metadata *ModelMetadata, opts EstimateOptions) *MemoryEstimate {
	return &MemoryEstimate{
		Layers:           estimate.Layers,
		Graph:            estimate.Graph,
		VRAMSize:         estimate.VRAMSize,
		TotalSize:        estimate.TotalSize,
		TensorSplit:      estimate.TensorSplit,
		GPUSizes:         estimate.GPUSizes,
		KVCache:          estimate.kv,
		Weights:          estimate.memoryWeights,
		GraphMem:         estimate.Graph,
		Projectors:       estimate.projectorWeights,
		Architecture:     metadata.Architecture,
		EstimatedAt:      time.Now(),
		FullyLoaded:      estimate.Layers >= int(metadata.BlockCount)+1,
		RequiresFallback: estimate.Layers < int(metadata.BlockCount)+1,
		Options:          opts,
		GPUs:             gpus,
	}
}

// Helper functions
func ollamaMax64(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}

func ollamaMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func formatBytes(bytes uint64) string {
	if bytes == 0 {
		return "0 B"
	}

	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
