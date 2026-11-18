package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCollectSessionContainers(t *testing.T) {
	root := t.TempDir()
	paths := hostPaths{
		proc:      filepath.Join(root, "proc"),
		cgroup:    filepath.Join(root, "cgroup"),
		sysBlock:  filepath.Join(root, "block"),
		mountinfo: filepath.Join(root, "mountinfo"),
	}
	// The desktop's init process sits in a nested cgroup; counters must come
	// from the container's own cgroup above it.
	writeFile(t, filepath.Join(paths.proc, "123", "cgroup"), "0::/docker/live/desktop/display\n")
	container := filepath.Join(paths.cgroup, "docker", "live")
	writeFile(t, filepath.Join(container, "desktop", "display", "memory.events"), "oom_kill 0\n")
	writeFile(t, filepath.Join(container, "memory.events"), "low 0\nhigh 7\nmax 2\noom 4\noom_kill 3\n")
	writeFile(t, filepath.Join(container, "cpu.stat"), "usage_usec 5000000\nnr_periods 100\nnr_throttled 25\nthrottled_usec 400000\n")
	writeFile(t, filepath.Join(container, "cpu.max"), "1200000 100000\n")
	writeFile(t, filepath.Join(container, "memory.current"), "3049381888\n")
	writeFile(t, filepath.Join(container, "memory.max"), "max\n")
	writeFile(t, filepath.Join(container, "memory.peak"), "4074336256\n")
	writeFile(t, filepath.Join(container, "memory.stat"), "anon 2589503488\nfile 339582976\nkernel 1\n")
	// nvme0n1 (259:0) holds dm-0 (252:0) through its partition; dm-0 holds dm-1.
	writeFile(t, filepath.Join(container, "io.stat"),
		"259:0 rbytes=1000 wbytes=0 rios=10 wios=0\n252:0 rbytes=1000 wbytes=90 rios=10 wios=4\n252:1 rbytes=1000 wbytes=500 rios=10 wios=14\n")
	writeFile(t, filepath.Join(paths.sysBlock, "259:0", "nvme0n1p3", "partition"), "3\n")
	writeFile(t, filepath.Join(paths.sysBlock, "259:0", "nvme0n1p3", "holders", "dm-0"), "")
	writeFile(t, filepath.Join(paths.sysBlock, "252:0", "holders", "dm-1"), "")
	if err := os.MkdirAll(filepath.Join(paths.sysBlock, "252:1", "holders"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(container, "cpu.pressure"), "some avg10=0.00 avg60=0.00 avg300=0.00 total=111\nfull avg10=0.00 avg60=0.00 avg300=0.00 total=1\n")
	writeFile(t, filepath.Join(container, "io.pressure"), "some avg10=92.86 avg60=93.99 avg300=94.30 total=333\nfull avg10=0 avg60=0 avg300=0 total=2\n")
	writeFile(t, filepath.Join(container, "pids.current"), "727\n")

	dedicated := filepath.Join(root, "zvols", "ses_live")
	shared := filepath.Join(root, "data")
	writeFile(t, filepath.Join(dedicated, "docker", ".keep"), "")
	writeFile(t, filepath.Join(shared, "workspaces", "ses_live", ".keep"), "")
	// Two bind-mount points of one shared volume, and the session's own volume.
	sharedAgain := filepath.Join(root, "container-docker")
	paths.statfs = func(path string, st *unix.Statfs_t) error {
		*st = unix.Statfs_t{Bsize: 4096, Blocks: 1000, Bfree: 400, Files: 100, Ffree: 75}
		if path == dedicated {
			st.Fsid = unix.Fsid{Val: [2]int32{2, 0}}
			st.Bfree = 900
		} else {
			st.Fsid = unix.Fsid{Val: [2]int32{1, 0}}
		}
		return nil
	}
	writeFile(t, paths.mountinfo,
		"1 0 8:1 / / rw - ext4 /dev/root rw\n"+
			"2 1 8:2 / "+shared+" rw - ext4 /dev/data rw\n"+
			"3 1 230:0 / "+dedicated+" rw - xfs /dev/zd0 rw\n"+
			"4 1 8:2 / "+sharedAgain+" rw - ext4 /dev/data rw\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/containers/json":
			_, _ = w.Write([]byte(`[
				{"Id":"live","Labels":{"helix.session_id":"ses_live"}},
				{"Id":"exited","Labels":{"helix.session_id":"sbx_exited"}},
				{"Id":"unreadable","Labels":{"helix.session_id":"ses_unreadable"}},
				{"Id":"other","Labels":{}}
			]`))
		case "/containers/live/json":
			_, _ = w.Write([]byte(`{"State":{"Pid":123,"OOMKilled":false},"Mounts":[
				{"Type":"bind","Source":"` + dedicated + `/docker","Destination":"/var/lib/docker"},
				{"Type":"bind","Source":"` + shared + `/workspaces/ses_live","Destination":"/workspace"},
				{"Type":"bind","Source":"` + shared + `/workspaces/ses_live","Destination":"/home/retro/work"},
				{"Type":"bind","Source":"` + sharedAgain + `/sessions/ses_live","Destination":"/run/user/1000"},
				{"Type":"tmpfs","Source":"","Destination":"/tmp"}
			]}`))
		case "/containers/exited/json":
			_, _ = w.Write([]byte(`{"State":{"Pid":0,"OOMKilled":true}}`))
		case "/containers/unreadable/json":
			_, _ = w.Write([]byte(`{"State":{"Pid":456,"OOMKilled":true}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	oom, resources := collectSessionContainersFrom(context.Background(), server.Client(), server.URL, paths)
	if len(oom) != 2 {
		t.Fatalf("got %d OOM metrics, want 2: %#v", len(oom), oom)
	}
	if oom[0].ContainerID != "live" || oom[0].SessionID != "ses_live" || oom[0].Count != 3 {
		t.Fatalf("running container OOM metric = %#v", oom[0])
	}
	if oom[1].ContainerID != "exited" || oom[1].SessionID != "sbx_exited" || oom[1].Count != 1 {
		t.Fatalf("exited container OOM metric = %#v", oom[1])
	}

	if len(resources) != 1 {
		t.Fatalf("got %d resource samples, want 1: %#v", len(resources), resources)
	}
	s := resources[0]
	if s.SessionID != "ses_live" || s.CPUUsageUsec != 5000000 || s.CPULimitMillicores != 12000 ||
		s.CPUPeriods != 100 || s.CPUThrottledPeriods != 25 || s.CPUThrottledUsec != 400000 {
		t.Fatalf("cpu: %+v", s)
	}
	if s.MemoryBytes != 3049381888 || s.MemoryLimitBytes != 0 || s.MemoryPeakBytes != 4074336256 ||
		s.MemoryAnonBytes != 2589503488 || s.MemoryFileBytes != 339582976 ||
		s.MemoryHighEvents != 7 || s.MemoryMaxEvents != 2 || s.OOMKills != 3 {
		t.Fatalf("memory: %+v", s)
	}
	if s.IOReadBytes != 1000 || s.IOWriteBytes != 500 || s.IOReadOps != 10 || s.IOWriteOps != 14 {
		t.Fatalf("io should count only the top of the device stack: %+v", s)
	}
	if s.CPUPressureUsec != 111 || s.MemoryPressureUsec != 0 || s.IOPressureUsec != 333 || s.Pids != 727 {
		t.Fatalf("pressure/pids: %+v", s)
	}
	if len(s.Filesystems) != 2 {
		t.Fatalf("filesystems: %+v", s.Filesystems)
	}
	if fs := s.Filesystems[0]; fs.Paths != "/home/retro/work, /run/user/1000, /workspace" || fs.Dedicated || fs.UsedBytes != 600*4096 || fs.InodesUsed != 25 {
		t.Fatalf("shared filesystem: %+v", fs)
	}
	if fs := s.Filesystems[1]; fs.Paths != "/var/lib/docker" || !fs.Dedicated || fs.UsedBytes != 100*4096 {
		t.Fatalf("dedicated filesystem: %+v", fs)
	}
}
