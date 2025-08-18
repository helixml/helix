// This file is adapted from Ollama's llm/memory.go
// Original source: https://github.com/ollama/ollama/blob/main/llm/memory.go
// License: MIT (https://github.com/ollama/ollama/blob/main/LICENSE)

package memory

import (
	"fmt"
	"log/slog"
	"time"
)

// OllamaMemoryEstimate matches Ollama's MemoryEstimate struct exactly
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

// OllamaGpuInfo matches what Ollama expects for GPU info
type OllamaGpuInfo struct {
	Library       string
	FreeMemory    uint64
	TotalMemory   uint64
	MinimumMemory uint64
	ID            string
	Index         int
}

// OllamaModelInfo represents the model information Ollama needs
type OllamaModelInfo struct {
	BlockCount      uint64
	HeadCountMax    uint64
	HeadCountKVMin  uint64
	EmbeddingLength uint64
	KeyLength       uint64
	ValueLength     uint64
	FFLength        uint64
	VocabSize       uint64
	Architecture    string
	Layers          map[string]OllamaLayerInfo
}

type OllamaLayerInfo struct {
	Tensors map[string]OllamaTensorInfo
}

type OllamaTensorInfo struct {
	Size uint64
}

// OllamaOptions matches Ollama's api.Options for memory estimation
type OllamaOptions struct {
	NumCtx   int
	NumBatch int
	NumGPU   int
}

// Copied from Ollama's memory.go - estimateGPULayers function
func estimateOllamaGPULayers(gpus []OllamaGpuInfo, modelInfo *OllamaModelInfo, opts OllamaOptions, numParallel int) OllamaMemoryEstimate {
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

	// DEBUG: Log model info
	slog.Debug("model_info",
		"block_count", modelInfo.BlockCount,
		"head_count_max", modelInfo.HeadCountMax,
		"head_count_kv_min", modelInfo.HeadCountKVMin,
		"embedding_length", modelInfo.EmbeddingLength,
		"key_length", modelInfo.KeyLength,
		"value_length", modelInfo.ValueLength,
		"layer_count", len(modelInfo.Layers))

	// Get layer size from blk.0 as a buffer
	if blk0, ok := modelInfo.Layers["blk.0"]; ok {
		for _, tensor := range blk0.Tensors {
			layerSize += tensor.Size
		}
		slog.Debug("blk.0 layer size", "size", formatBytes(layerSize), "tensor_count", len(blk0.Tensors))
	} else {
		slog.Warn("model missing blk.0 layer size")
	}

	// Calculate KV cache and graph sizes
	kv, graphPartialOffload, graphFullOffload := calculateOllamaGraphSize(modelInfo, uint64(opts.NumCtx), uint64(min(opts.NumCtx, opts.NumBatch)), numParallel)

	if len(kv) > 0 {
		layerSize += kv[0]
	}

	var kvTotal uint64
	for _, kvLayer := range kv {
		kvTotal += kvLayer
	}

	if graphPartialOffload == 0 {
		headsKV := modelInfo.HeadCountKVMin
		if headsKV == 0 {
			headsKV = 1
		}
		gqa := modelInfo.HeadCountMax / headsKV
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
	if layer, ok := modelInfo.Layers["output_norm"]; ok {
		for _, tensor := range layer.Tensors {
			memoryLayerOutput += tensor.Size
		}
	}
	if layer, ok := modelInfo.Layers["output"]; ok {
		for _, tensor := range layer.Tensors {
			memoryLayerOutput += tensor.Size
		}
	} else if layer, ok := modelInfo.Layers["token_embd"]; ok {
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
		// Only include GPUs that can fit the graph, gpu minimum, the layer buffer and at least more layer
		if gpus[i].FreeMemory < overhead+gzo+ollamaMax64(graphPartialOffload, graphFullOffload)+gpus[i].MinimumMemory+2*layerSize {
			slog.Debug("gpu has too little memory to allocate any layers",
				"id", gpus[i].ID,
				"library", gpus[i].Library,
				"name", "unknown",
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

	// For all the layers, find where they can fit on the GPU(s)
	for i := int(modelInfo.BlockCount) - 1; i >= 0; i-- {
		// Some models have inconsistent layer sizes
		if blk, ok := modelInfo.Layers[fmt.Sprintf("blk.%d", i)]; ok {
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
	if layerCount >= int(modelInfo.BlockCount) {
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

		if layerCount < int(modelInfo.BlockCount)+1 {
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

	estimate := OllamaMemoryEstimate{
		TotalSize: memoryRequiredTotal,
		Layers:    0,
		Graph:     0,
		VRAMSize:  0,
		GPUSizes:  []uint64{},

		inferenceLibrary:    gpus[0].Library,
		layersRequested:     opts.NumGPU,
		layersModel:         int(modelInfo.BlockCount) + 1,
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
		slog.Debug("insufficient VRAM to load any model layers")
		return estimate
	}
	estimate.Layers = layerCount
	estimate.Graph = graphOffload
	estimate.VRAMSize = memoryRequiredPartial
	estimate.TotalSize = memoryRequiredTotal
	estimate.TensorSplit = tensorSplit
	estimate.GPUSizes = gpuAllocations
	return estimate
}

// Helper functions
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func ollamaMax64(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}

func formatBytes(bytes uint64) string {
	if bytes == 0 {
		return "0 B"
	}

	const unit = 1024
	sizes := []string{"B", "KB", "MB", "GB", "TB"}

	size := float64(bytes)
	i := 0
	for size >= unit && i < len(sizes)-1 {
		size /= unit
		i++
	}

	return fmt.Sprintf("%.1f %s", size, sizes[i])
}

// calculateOllamaGraphSize calculates KV cache and graph memory (simplified version)
func calculateOllamaGraphSize(modelInfo *OllamaModelInfo, numCtx, numBatch uint64, numParallel int) ([]uint64, uint64, uint64) {
	// Calculate KV cache per layer
	context := numCtx * uint64(numParallel)
	embeddingHeadsK := modelInfo.KeyLength
	embeddingHeadsV := modelInfo.ValueLength
	headsKV := modelInfo.HeadCountKVMin
	if headsKV == 0 {
		headsKV = 1
	}

	// Use f16 for KV cache (2 bytes per element)
	bytesPerElement := float64(2.0)
	kvPerLayer := uint64(float64(context*(embeddingHeadsK+embeddingHeadsV)*headsKV) * bytesPerElement)

	// Create KV array for each layer
	kv := make([]uint64, modelInfo.BlockCount)
	for i := range kv {
		kv[i] = kvPerLayer
	}

	// Calculate graph sizes (simplified)
	var kvTotal uint64
	for _, kvLayer := range kv {
		kvTotal += kvLayer
	}

	// Graph memory estimation
	gqa := modelInfo.HeadCountMax / headsKV
	graphPartial := gqa * kvTotal / 6
	graphFull := graphPartial

	return kv, graphPartial, graphFull
}

// ConvertToOllamaTypes converts our types to Ollama types
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

	// Convert model info
	ollamaLayers := make(map[string]OllamaLayerInfo)
	for layerName, layerInfo := range metadata.Layers {
		ollamaTensors := make(map[string]OllamaTensorInfo)
		for tensorName, tensorInfo := range layerInfo.Tensors {
			ollamaTensors[tensorName] = OllamaTensorInfo{
				Size: tensorInfo.Size,
			}
		}
		ollamaLayers[layerName] = OllamaLayerInfo{
			Tensors: ollamaTensors,
		}
	}

	ollamaModel := &OllamaModelInfo{
		BlockCount:      metadata.BlockCount,
		HeadCountMax:    metadata.HeadCount,
		HeadCountKVMin:  metadata.HeadCountKV,
		EmbeddingLength: metadata.EmbeddingLength,
		KeyLength:       metadata.KeyLength,
		ValueLength:     metadata.ValueLength,
		FFLength:        metadata.FFLength,
		VocabSize:       metadata.VocabSize,
		Architecture:    metadata.Architecture,
		Layers:          ollamaLayers,
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
