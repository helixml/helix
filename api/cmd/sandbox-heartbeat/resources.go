package main

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/helixml/helix/api/pkg/types"
	"golang.org/x/sys/unix"
)

// hostPaths are the kernel interfaces the heartbeat reads, overridable in tests.
type hostPaths struct {
	proc      string
	cgroup    string
	sysBlock  string
	mountinfo string
	statfs    func(path string, st *unix.Statfs_t) error
}

var defaultHostPaths = hostPaths{
	proc:      "/proc",
	cgroup:    "/sys/fs/cgroup",
	sysBlock:  "/sys/dev/block",
	mountinfo: "/proc/self/mountinfo",
	statfs:    unix.Statfs,
}

// containerCgroupDir returns the container's own cgroup. A desktop's init
// process lives in a nested child cgroup (…/<id>/desktop/display), so the
// process's cgroup alone would miss most of the container.
func containerCgroupDir(paths hostPaths, pid int, containerID string) (string, error) {
	cgroup, err := os.ReadFile(filepath.Join(paths.proc, strconv.Itoa(pid), "cgroup"))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(cgroup), "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 || parts[0] != "0" || parts[1] != "" {
			continue
		}
		rel := strings.TrimPrefix(filepath.Clean(parts[2]), string(filepath.Separator))
		for dir := rel; dir != "." && dir != string(filepath.Separator); dir = filepath.Dir(dir) {
			if strings.Contains(filepath.Base(dir), containerID) {
				return filepath.Join(paths.cgroup, dir), nil
			}
		}
		return "", fmt.Errorf("no cgroup ancestor of %q names container %s", rel, containerID)
	}
	return "", fmt.Errorf("cgroup v2 path not found for pid %d", pid)
}

func readKeyValues(path string) (map[string]uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	values := map[string]uint64{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if v, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
			values[fields[0]] = v
		}
	}
	return values, nil
}

// readLimit parses a single-value cgroup limit; "max" means unlimited (0).
func readLimit(path string) (uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	value := strings.TrimSpace(string(data))
	if value == "max" {
		return 0, nil
	}
	return strconv.ParseUint(value, 10, 64)
}

func readCPULimitMillicores(path string) (uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) != 2 {
		return 0, fmt.Errorf("unexpected cpu.max %q", data)
	}
	if fields[0] == "max" {
		return 0, nil
	}
	quota, err := strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return 0, err
	}
	period, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil || period == 0 {
		return 0, fmt.Errorf("unexpected cpu.max %q", data)
	}
	return quota * 1000 / period, nil
}

// readPressureTotal returns the "some" stall total. Kernels booted with psi=0
// have no pressure files; that reads as no recorded stall.
func readPressureTotal(path string) (uint64, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "some ") {
			continue
		}
		for _, field := range strings.Fields(line) {
			if total, ok := strings.CutPrefix(field, "total="); ok {
				return strconv.ParseUint(total, 10, 64)
			}
		}
	}
	return 0, fmt.Errorf("no some total in %s", path)
}

// topOfStack reports whether a block device is not itself underneath another
// one (device mapper, LVM, md). The kernel charges a container's reads at every
// layer of a stack but its writes only at the top, so only the top counts.
func topOfStack(sysBlock, device string, cache map[string]bool) bool {
	if top, ok := cache[device]; ok {
		return top
	}
	hasHolders := func(dir string) bool {
		entries, err := os.ReadDir(filepath.Join(dir, "holders"))
		return err == nil && len(entries) > 0
	}
	dir := filepath.Join(sysBlock, device)
	top := !hasHolders(dir)
	if entries, err := os.ReadDir(dir); top && err == nil {
		for _, entry := range entries {
			if _, err := os.Stat(filepath.Join(dir, entry.Name(), "partition")); err == nil && hasHolders(filepath.Join(dir, entry.Name())) {
				top = false
				break
			}
		}
	}
	cache[device] = top
	return top
}

func readIO(path, sysBlock string, cache map[string]bool, s *types.SessionResourceSample) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || !topOfStack(sysBlock, fields[0], cache) {
			continue
		}
		for _, field := range fields[1:] {
			key, raw, ok := strings.Cut(field, "=")
			if !ok {
				continue
			}
			v, err := strconv.ParseUint(raw, 10, 64)
			if err != nil {
				continue
			}
			switch key {
			case "rbytes":
				s.IOReadBytes += v
			case "wbytes":
				s.IOWriteBytes += v
			case "rios":
				s.IOReadOps += v
			case "wios":
				s.IOWriteOps += v
			}
		}
	}
	return nil
}

func readCgroupResources(dir, sysBlock string, cache map[string]bool, s *types.SessionResourceSample) error {
	cpu, err := readKeyValues(filepath.Join(dir, "cpu.stat"))
	if err != nil {
		return err
	}
	s.CPUUsageUsec = cpu["usage_usec"]
	s.CPUPeriods = cpu["nr_periods"]
	s.CPUThrottledPeriods = cpu["nr_throttled"]
	s.CPUThrottledUsec = cpu["throttled_usec"]
	if s.CPULimitMillicores, err = readCPULimitMillicores(filepath.Join(dir, "cpu.max")); err != nil {
		return err
	}
	if s.MemoryBytes, err = readLimit(filepath.Join(dir, "memory.current")); err != nil {
		return err
	}
	if s.MemoryLimitBytes, err = readLimit(filepath.Join(dir, "memory.max")); err != nil {
		return err
	}
	// memory.peak needs Linux 5.19.
	if s.MemoryPeakBytes, err = readLimit(filepath.Join(dir, "memory.peak")); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	memStat, err := readKeyValues(filepath.Join(dir, "memory.stat"))
	if err != nil {
		return err
	}
	s.MemoryAnonBytes, s.MemoryFileBytes = memStat["anon"], memStat["file"]
	events, err := readKeyValues(filepath.Join(dir, "memory.events"))
	if err != nil {
		return err
	}
	s.MemoryHighEvents, s.MemoryMaxEvents, s.OOMKills = events["high"], events["max"], events["oom_kill"]
	if err := readIO(filepath.Join(dir, "io.stat"), sysBlock, cache, s); err != nil {
		return err
	}
	for file, dst := range map[string]*uint64{
		"cpu.pressure": &s.CPUPressureUsec, "memory.pressure": &s.MemoryPressureUsec, "io.pressure": &s.IOPressureUsec,
	} {
		if *dst, err = readPressureTotal(filepath.Join(dir, file)); err != nil {
			return err
		}
	}
	if s.Pids, err = readLimit(filepath.Join(dir, "pids.current")); err != nil {
		return err
	}
	return nil
}

// readMountPoints lists this namespace's mount points from mountinfo.
func readMountPoints(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var points []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if fields := strings.Fields(scanner.Text()); len(fields) > 4 {
			points = append(points, fields[4])
		}
	}
	return points, scanner.Err()
}

func underPath(path, dir string) bool {
	return dir == "/" || path == dir || strings.HasPrefix(path, dir+"/")
}

type dockerMount struct {
	Type        string `json:"Type"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
}

// filesystemsFor groups the container's bind and volume mounts by the
// filesystem that holds their source. Several mount points can expose one
// filesystem (bind mounts of the same volume), so they are merged by fsid. A
// filesystem mounted at a path naming the session (its ZFS volume or quota
// disk) belongs to the session alone.
func filesystemsFor(sessionID string, mounts []dockerMount, mountPoints []string, statfs func(string, *unix.Statfs_t) error) []types.SessionFilesystemSample {
	type group struct {
		sample types.SessionFilesystemSample
		paths  []string
	}
	groups := map[unix.Fsid]*group{}
	for _, m := range mounts {
		if (m.Type != "bind" && m.Type != "volume") || m.Source == "" {
			continue
		}
		holder := ""
		for _, point := range mountPoints {
			if underPath(m.Source, point) && len(point) > len(holder) {
				holder = point
			}
		}
		var st unix.Statfs_t
		if holder == "" || statfs(holder, &st) != nil || st.Blocks == 0 {
			continue
		}
		g := groups[st.Fsid]
		if g == nil {
			g = &group{sample: types.SessionFilesystemSample{
				SizeBytes:  st.Blocks * uint64(st.Bsize),
				UsedBytes:  (st.Blocks - st.Bfree) * uint64(st.Bsize),
				Inodes:     st.Files,
				InodesUsed: st.Files - st.Ffree,
			}}
			groups[st.Fsid] = g
		}
		g.sample.Dedicated = g.sample.Dedicated || strings.Contains(holder, sessionID)
		g.paths = append(g.paths, m.Destination)
	}
	out := make([]types.SessionFilesystemSample, 0, len(groups))
	for _, g := range groups {
		slices.Sort(g.paths)
		g.sample.Paths = strings.Join(slices.Compact(g.paths), ", ")
		out = append(out, g.sample)
	}
	slices.SortFunc(out, func(a, b types.SessionFilesystemSample) int { return strings.Compare(a.Paths, b.Paths) })
	return out
}
