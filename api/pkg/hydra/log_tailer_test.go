package hydra

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestStartServiceLogTailers_AppendsToBufferWithPrefix verifies the happy
// path: a wrapper writes lines to /var/log/helix-services/<svc>.log,
// hydra's tailer picks them up, and they appear in the LogBuffer with the
// "[<svc>] " prefix the admin UI consumes.
func TestStartServiceLogTailers_AppendsToBufferWithPrefix(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "compose-manager.log")

	// Create the file BEFORE starting the tailer to exercise the initial-
	// scan code path. Append-mode so the tailer's seek-to-end positions
	// just before any future writes.
	f, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("create log file: %v", err)
	}
	defer f.Close()

	buf := NewLogBuffer(64)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	StartServiceLogTailers(ctx, buf, dir)

	// Give the tailer a moment to open the file + seek to end.
	time.Sleep(250 * time.Millisecond)

	wantLines := []string{
		"hello compose-manager",
		"second line",
		"third line with a UTF-8 char é",
	}
	for _, line := range wantLines {
		if _, err := fmt.Fprintln(f, line); err != nil {
			t.Fatalf("write line: %v", err)
		}
	}
	if err := f.Sync(); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Wait up to 3s for all lines to land in the buffer (poll interval
	// is 500ms; 3s = 6 ticks of slack for CI jitter).
	deadline := time.Now().Add(3 * time.Second)
	var snap []LogLine
	for time.Now().Before(deadline) {
		snap = buf.Snapshot(1024)
		if len(snap) >= len(wantLines) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if len(snap) < len(wantLines) {
		t.Fatalf("expected at least %d lines in buffer, got %d: %+v", len(wantLines), len(snap), snap)
	}

	prefix := "[compose-manager] "
	for i, want := range wantLines {
		got := snap[i].Line
		if !strings.HasPrefix(got, prefix) {
			t.Errorf("line %d: expected prefix %q, got %q", i, prefix, got)
		}
		if !strings.Contains(got, want) {
			t.Errorf("line %d: expected to contain %q, got %q", i, want, got)
		}
	}
}

// TestStartServiceLogTailers_PicksUpFileAppearingAfterStart verifies the
// rescan path: a wrapper that starts AFTER hydra (e.g. compose-manager
// in 12-start-... when hydra was in 10-start-...) creates its log file
// later in the boot, and the tailer must notice it within the rescan
// interval.
func TestStartServiceLogTailers_PicksUpFileAppearingAfterStart(t *testing.T) {
	dir := t.TempDir()

	buf := NewLogBuffer(64)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	StartServiceLogTailers(ctx, buf, dir)

	// No log file exists yet. Wait briefly so the initial scan completes
	// without finding anything.
	time.Sleep(250 * time.Millisecond)

	// Now create the file. Rescan interval is 10s; the test waits up to
	// 15s for the new file to be noticed. CI-friendly but slow if the
	// rescan path is broken.
	logPath := filepath.Join(dir, "inference-proxy.log")
	f, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("create late log file: %v", err)
	}
	defer f.Close()

	want := "late-arriving service line"
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := fmt.Fprintln(f, want); err != nil {
			t.Fatalf("write: %v", err)
		}
		_ = f.Sync()
		snap := buf.Snapshot(1024)
		for _, l := range snap {
			if strings.Contains(l.Line, want) && strings.HasPrefix(l.Line, "[inference-proxy] ") {
				return // pass
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("expected line with prefix [inference-proxy] not found in buffer; snapshot: %+v", buf.Snapshot(1024))
}

// TestStartServiceLogTailers_NilBufferIsNoOp guards against a panic if a
// caller forgets to construct the buffer before calling the tailer.
// hydra/main.go always provides one but defensive.
func TestStartServiceLogTailers_NilBufferIsNoOp(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Must not panic.
	StartServiceLogTailers(ctx, nil, t.TempDir())
}
