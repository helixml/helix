// Package gpudetect probes the local hardware for NVIDIA / AMD GPUs and
// returns a slice of types.GPUStatus populated with vendor, architecture,
// VRAM, and (for NVIDIA) compute capability. Used by the sandbox heartbeat
// to feed the API server's profile-compatibility check.
//
// Safe to call on hosts with no GPUs — returns nil, nil.
package gpudetect

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/runner/gpuarch"
	"github.com/helixml/helix/api/pkg/types"
)

// Detect runs vendor-specific probes (nvidia-smi, rocm-smi) and returns
// the merged GPU inventory. Returns an empty slice (not nil) on a host
// with no GPUs to make caller code simpler.
//
// Probe failure is not an error — a missing nvidia-smi just means no
// NVIDIA GPUs. Genuine errors (probe present but failing) are logged
// to stderr by the caller; this function returns whatever it could
// successfully detect.
func Detect(ctx context.Context) []types.GPUStatus {
	var out []types.GPUStatus
	out = append(out, detectNVIDIA(ctx)...)
	out = append(out, detectAMD(ctx)...)
	if out == nil {
		out = []types.GPUStatus{}
	}
	return out
}

// detectNVIDIA shells out to nvidia-smi. Format:
//   index,name,memory.total,driver_version,compute_cap
//   0, NVIDIA RTX 2000 Ada Generation, 16380 MiB, 570.211.01, 8.9
func detectNVIDIA(ctx context.Context) []types.GPUStatus {
	cmd := exec.CommandContext(ctx, "nvidia-smi",
		"--query-gpu=index,name,memory.total,memory.used,memory.free,driver_version,compute_cap",
		"--format=csv,noheader,nounits")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// nvidia-smi missing or failing. Not an error — just no NVIDIA.
		return nil
	}
	return parseNVIDIASmiCSV(stdout.String())
}

func parseNVIDIASmiCSV(out string) []types.GPUStatus {
	var gpus []types.GPUStatus
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := splitCSVTrim(line)
		if len(fields) < 7 {
			continue
		}
		idx, _ := strconv.Atoi(fields[0])
		name := fields[1]
		totalMiB, _ := strconv.ParseUint(fields[2], 10, 64)
		usedMiB, _ := strconv.ParseUint(fields[3], 10, 64)
		freeMiB, _ := strconv.ParseUint(fields[4], 10, 64)
		driver := fields[5]
		cc := fields[6]
		gpus = append(gpus, types.GPUStatus{
			Index:             idx,
			ModelName:         name,
			TotalMemory:       totalMiB * 1024 * 1024,
			UsedMemory:        usedMiB * 1024 * 1024,
			FreeMemory:        freeMiB * 1024 * 1024,
			DriverVersion:     driver,
			SDKVersion:        "", // SDK = CUDA runtime version; not reported by --query-gpu
			Vendor:            types.GPUVendorNVIDIA,
			Architecture:      gpuarch.FromNVIDIAComputeCapability(cc),
			ComputeCapability: cc,
		})
	}
	return gpus
}

// detectAMD shells out to rocm-smi. The CSV format from `rocm-smi
// --showid --showproductname --showmeminfo vram --csv` is:
//   device,Card series,Card model,Card vendor,Card SKU,VRAM Total Memory (B),VRAM Total Used Memory (B)
//
// Architecture (gfx target) requires `--showhw` and is reported in a
// separate column. To keep the parsing simple and tolerant of rocm-smi
// version drift we run two probes and join on device index.
func detectAMD(ctx context.Context) []types.GPUStatus {
	if _, err := exec.LookPath("rocm-smi"); err != nil {
		return nil
	}
	// 1. Inventory + memory (totals/used in bytes).
	cmd := exec.CommandContext(ctx, "rocm-smi",
		"--showproductname", "--showmeminfo", "vram", "--csv")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil
	}
	rows := parseROCmCSV(stdout.String())

	// 2. Architecture (gfx target).
	cmd2 := exec.CommandContext(ctx, "rocm-smi", "--showhw", "--csv")
	var stdout2 bytes.Buffer
	cmd2.Stdout = &stdout2
	archByDevice := map[string]string{}
	if err := cmd2.Run(); err == nil {
		for _, row := range parseROCmCSV(stdout2.String()) {
			if dev := row["device"]; dev != "" {
				archByDevice[dev] = row["Card SKU"]
			}
		}
	}

	var gpus []types.GPUStatus
	for _, row := range rows {
		dev := row["device"]
		idx, _ := strconv.Atoi(strings.TrimPrefix(dev, "card"))
		total, _ := strconv.ParseUint(row["VRAM Total Memory (B)"], 10, 64)
		used, _ := strconv.ParseUint(row["VRAM Total Used Memory (B)"], 10, 64)
		gfx := archByDevice[dev]
		gpus = append(gpus, types.GPUStatus{
			Index:        idx,
			ModelName:    strings.TrimSpace(row["Card model"] + " " + row["Card SKU"]),
			TotalMemory:  total,
			UsedMemory:   used,
			FreeMemory:   total - used,
			Vendor:       types.GPUVendorAMD,
			Architecture: gpuarch.FromAMDGFX(gfx),
		})
	}
	return gpus
}

// parseROCmCSV parses rocm-smi --csv output into a slice of header→value
// maps. rocm-smi emits a header row followed by one row per device.
func parseROCmCSV(out string) []map[string]string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		return nil
	}
	headers := splitCSVTrim(lines[0])
	var rows []map[string]string
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := splitCSVTrim(line)
		row := map[string]string{}
		for i, h := range headers {
			if i < len(fields) {
				row[h] = fields[i]
			}
		}
		rows = append(rows, row)
	}
	return rows
}

// splitCSVTrim splits on commas and strips surrounding whitespace from
// each field. Doesn't handle quoted fields with embedded commas — fine
// for nvidia-smi/rocm-smi output which never quotes.
func splitCSVTrim(line string) []string {
	parts := strings.Split(line, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// errCmdMissing is returned when a probe binary isn't on PATH. Helps
// callers distinguish "no GPUs of this vendor" from "probe broken."
var errCmdMissing = errors.New("probe binary not found on PATH")

// runWithTimeout runs cmd with a timeout, returning ErrCmdMissing if the
// binary is absent. Used internally; exported variant could come later.
func runWithTimeout(name string, args []string, timeout time.Duration) (string, error) {
	if _, err := exec.LookPath(name); err != nil {
		return "", errCmdMissing
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	return string(out), nil
}
