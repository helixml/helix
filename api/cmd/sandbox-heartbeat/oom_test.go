package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCollectContainerOOMKills(t *testing.T) {
	procRoot := t.TempDir()
	cgroupRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(procRoot, "123"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(procRoot, "123", "cgroup"), []byte("0::/docker/live\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(cgroupRoot, "docker", "live"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cgroupRoot, "docker", "live", "memory.events"), []byte("oom 4\noom_kill 3\n"), 0o600); err != nil {
		t.Fatal(err)
	}

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
			_, _ = w.Write([]byte(`{"State":{"Pid":123,"OOMKilled":false}}`))
		case "/containers/exited/json":
			_, _ = w.Write([]byte(`{"State":{"Pid":0,"OOMKilled":true}}`))
		case "/containers/unreadable/json":
			_, _ = w.Write([]byte(`{"State":{"Pid":456,"OOMKilled":true}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	got := collectContainerOOMKillsFrom(context.Background(), server.Client(), server.URL, procRoot, cgroupRoot)
	if len(got) != 2 {
		t.Fatalf("got %d metrics, want 2: %#v", len(got), got)
	}
	if got[0].ContainerID != "live" || got[0].SessionID != "ses_live" || got[0].Count != 3 {
		t.Fatalf("running container metric = %#v", got[0])
	}
	if got[1].ContainerID != "exited" || got[1].SessionID != "sbx_exited" || got[1].Count != 1 {
		t.Fatalf("exited container metric = %#v", got[1])
	}
}
