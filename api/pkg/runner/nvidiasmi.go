package runner

import (
	"fmt"
	"os/exec"
	"strings"
)

type GPUInfo struct {
	TotalMemory string
	Processes   []ProcessInfo
}

type ProcessInfo struct {
	PID         int
	MemoryUsage string
}

func LogGpuInfo() {
	gpuInfo, err := GetGPUInfo()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Println("Total GPU Memory:", gpuInfo.TotalMemory)
	fmt.Println("Processes:")
	for _, process := range gpuInfo.Processes {
		fmt.Printf("PID: %d, Memory Usage: %s\n", process.PID, process.MemoryUsage)
	}
}

func GetGPUInfo() (GPUInfo, error) {
	cmd := exec.Command("nvidia-smi", "--query-gpu=memory.total", "--format=csv,noheader,nounits")
	output, err := cmd.Output()
	if err != nil {
		return GPUInfo{}, err
	}

	totalMemory := strings.TrimSpace(string(output))

	cmd = exec.Command("nvidia-smi", "--query-compute-apps=pid,used_memory", "--format=csv,noheader")
	output, err = cmd.Output()
	if err != nil {
		return GPUInfo{}, err
	}

	processes := parseProcessInfo(string(output))

	return GPUInfo{
		TotalMemory: totalMemory,
		Processes:   processes,
	}, nil
}

func parseProcessInfo(output string) []ProcessInfo {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	processes := make([]ProcessInfo, len(lines))

	for i, line := range lines {
		fields := strings.Split(line, ",")
		pid := parseInt(fields[0])
		memoryUsage := strings.TrimSpace(fields[1])
		processes[i] = ProcessInfo{
			PID:         pid,
			MemoryUsage: memoryUsage,
		}
	}

	return processes
}

func parseInt(s string) int {
	var result int
	fmt.Sscanf(s, "%d", &result)
	return result
}
