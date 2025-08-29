package types

// CreateStandardGPUConfig creates standard GPU configuration for memory estimation
// gpuCount: number of GPUs to create
// memoryGB: memory in GB for each GPU
func CreateStandardGPUConfig(gpuCount int, memoryGB int) []GPUInfoForEstimation {
	gpuMemory := uint64(memoryGB * 1024 * 1024 * 1024) // Convert GB to bytes

	config := make([]GPUInfoForEstimation, gpuCount)
	for i := 0; i < gpuCount; i++ {
		config[i] = GPUInfoForEstimation{
			Index:         i,
			Library:       "cuda",
			FreeMemory:    gpuMemory,
			TotalMemory:   gpuMemory,
			MinimumMemory: 512 * 1024 * 1024, // 512MB minimum
		}
	}
	return config
}
