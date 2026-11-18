package sessionresources

import (
	"math"
	"slices"

	"github.com/helixml/helix/api/pkg/types"
)

// interval is the change between two consecutive samples of one container.
type interval struct {
	end                                  types.SessionResourceSample
	wallUsec                             float64
	cpuUsec, throttledUsec               uint64
	cpuPressure, memPressure, ioPressure uint64
	ioRead, ioWrite                      uint64
	memoryHighEvents, oomKills           uint64
}

// pct is part's share of whole wall time, capped at 100: samples are stamped
// when the API receives them, so delivery jitter can make an interval look
// shorter than the counters it covers.
func pct(part, whole float64) float64 {
	if whole <= 0 {
		return 0
	}
	return min(100, 100*part/whole)
}

// intervals pairs consecutive samples. A counter going backwards means the
// container restarted, so that pair has no meaningful delta and is skipped.
func intervals(samples []types.SessionResourceSample) []interval {
	var out []interval
	for i := 1; i < len(samples); i++ {
		a, b := samples[i-1], samples[i]
		if b.Timestamp <= a.Timestamp || b.CPUUsageUsec < a.CPUUsageUsec || b.CPUThrottledUsec < a.CPUThrottledUsec ||
			b.CPUPressureUsec < a.CPUPressureUsec || b.MemoryPressureUsec < a.MemoryPressureUsec || b.IOPressureUsec < a.IOPressureUsec ||
			b.IOReadBytes < a.IOReadBytes || b.IOWriteBytes < a.IOWriteBytes || b.MemoryHighEvents < a.MemoryHighEvents || b.OOMKills < a.OOMKills {
			continue
		}
		out = append(out, interval{
			end:              b,
			wallUsec:         float64(b.Timestamp-a.Timestamp) * 1000,
			cpuUsec:          b.CPUUsageUsec - a.CPUUsageUsec,
			throttledUsec:    b.CPUThrottledUsec - a.CPUThrottledUsec,
			cpuPressure:      b.CPUPressureUsec - a.CPUPressureUsec,
			memPressure:      b.MemoryPressureUsec - a.MemoryPressureUsec,
			ioPressure:       b.IOPressureUsec - a.IOPressureUsec,
			ioRead:           b.IOReadBytes - a.IOReadBytes,
			ioWrite:          b.IOWriteBytes - a.IOWriteBytes,
			memoryHighEvents: b.MemoryHighEvents - a.MemoryHighEvents,
			oomKills:         b.OOMKills - a.OOMKills,
		})
	}
	return out
}

func dedicatedUsed(s types.SessionResourceSample) uint64 {
	var used uint64
	for _, fs := range s.Filesystems {
		if fs.Dedicated {
			used += fs.UsedBytes
		}
	}
	return used
}

// point aggregates consecutive intervals: rates are time-weighted averages,
// gauges are maxima and events are sums.
func point(group []interval) types.SessionResourcePoint {
	var wall, cpu, throttled, cpuP, memP, ioP, read, write float64
	p := types.SessionResourcePoint{Timestamp: group[len(group)-1].end.Timestamp}
	for _, iv := range group {
		wall += iv.wallUsec
		cpu += float64(iv.cpuUsec)
		throttled += float64(iv.throttledUsec)
		cpuP += float64(iv.cpuPressure)
		memP += float64(iv.memPressure)
		ioP += float64(iv.ioPressure)
		read += float64(iv.ioRead)
		write += float64(iv.ioWrite)
		p.MemoryBytes = max(p.MemoryBytes, iv.end.MemoryBytes)
		p.DiskUsedBytes = max(p.DiskUsedBytes, dedicatedUsed(iv.end))
		p.OOMKills += iv.oomKills
	}
	p.CPUCores = cpu / wall
	p.CPUThrottledPct = pct(throttled, wall)
	p.CPUPressurePct = pct(cpuP, wall)
	p.MemoryPressurePct = pct(memP, wall)
	p.IOPressurePct = pct(ioP, wall)
	p.IOReadBps = read / wall * 1e6
	p.IOWriteBps = write / wall * 1e6
	return p
}

// percentile is the nearest-rank percentile: the smallest value with at least
// q of the values at or below it.
func percentile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	return sorted[max(int(math.Ceil(q*float64(len(sorted))))-1, 0)]
}

// Summarize turns samples (oldest first) into a summary and at most maxPoints
// chart points.
func Summarize(sessionID string, samples []types.SessionResourceSample, maxPoints int) *types.SessionResources {
	out := &types.SessionResources{SessionID: sessionID, Points: []types.SessionResourcePoint{}, Filesystems: []types.SessionFilesystemSample{}}
	out.Summary.Samples = len(samples)
	if len(samples) == 0 {
		return out
	}
	first, last := samples[0], samples[len(samples)-1]
	out.Summary.From, out.Summary.To = first.Timestamp, last.Timestamp
	out.Summary.CPULimitCores = float64(last.CPULimitMillicores) / 1000
	out.Summary.MemoryLimitBytes = last.MemoryLimitBytes
	if last.Filesystems != nil {
		out.Filesystems = last.Filesystems
	}
	for _, s := range samples {
		out.Summary.MemoryMaxBytes = max(out.Summary.MemoryMaxBytes, s.MemoryBytes)
		out.Summary.MemoryPeakBytes = max(out.Summary.MemoryPeakBytes, s.MemoryPeakBytes)
	}

	ivs := intervals(samples)
	if len(ivs) == 0 {
		return out
	}
	all := point(ivs)
	s := &out.Summary
	s.CPUThrottledPct, s.CPUPressurePct, s.MemoryPressurePct, s.IOPressurePct =
		all.CPUThrottledPct, all.CPUPressurePct, all.MemoryPressurePct, all.IOPressurePct
	cores := make([]float64, len(ivs))
	for i, iv := range ivs {
		cores[i] = float64(iv.cpuUsec) / iv.wallUsec
		s.IOReadBytes += iv.ioRead
		s.IOWriteBytes += iv.ioWrite
		s.MemoryHighEvents += iv.memoryHighEvents
		s.OOMKills += iv.oomKills
	}
	slices.Sort(cores)
	s.CPUCoresP50, s.CPUCoresP95, s.CPUCoresMax = percentile(cores, 0.5), percentile(cores, 0.95), cores[len(cores)-1]

	per := (len(ivs) + maxPoints - 1) / maxPoints
	for i := 0; i < len(ivs); i += per {
		out.Points = append(out.Points, point(ivs[i:min(i+per, len(ivs))]))
	}
	return out
}
