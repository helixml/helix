package hydra

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// serviceLogTailPollInterval is how often each per-file tailer polls for
// new content. 500ms is a balance between log-line latency in the admin
// UI (operators expect near-real-time) and the syscall cost of a stat
// per file per tick. The number of files is small (one per long-running
// service inside helix-sandbox, today: dockerd, sandbox-heartbeat, hydra,
// compose-manager, inference-proxy = up to 5).
const serviceLogTailPollInterval = 500 * time.Millisecond

// serviceLogDirRescanInterval is how often we re-scan the service-log
// directory for newly-appeared log files. Services that start AFTER
// hydra (e.g. compose-manager) won't have created their log file yet
// when hydra's main() runs, so we poll for them periodically rather
// than only at startup.
const serviceLogDirRescanInterval = 10 * time.Second

// StartServiceLogTailers watches dir for *.log files and tails each one
// into buf with a "[<svc>] " prefix derived from the filename. The
// existing /logs WS endpoint streams from buf, so this surfaces ALL
// long-running services (compose-manager, inference-proxy, dockerd,
// sandbox-heartbeat) in the admin Runner Logs view alongside hydra's
// own zerolog output - not just hydra.
//
// Files appearing after this returns ARE picked up by a rescan goroutine.
// Files removed are abandoned (their tailer goroutine continues reading
// the last open file descriptor, which is harmless; new content at the
// same path after re-creation would be missed without rotation logic).
//
// Tailers start at end-of-file (not from the beginning) so the LogBuffer
// reflects live state, not a startup history dump that could displace
// recent legitimate lines from the ring.
//
// Errors are logged but do not abort - a single broken file path does
// not take down the whole aggregator. Safe to call with a nil buf
// (turns into a no-op).
func StartServiceLogTailers(ctx context.Context, buf *LogBuffer, dir string) {
	if buf == nil {
		return
	}

	// Best-effort mkdir so the wrappers' `tee -a` calls don't fail on a
	// fresh container. If this fails, we proceed anyway - a service that
	// successfully creates its own log file will still be picked up.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Warn().Err(err).Str("dir", dir).
			Msg("StartServiceLogTailers: mkdir failed; service logs may be unavailable")
	}

	tailed := &sync.Map{} // path -> struct{} sentinel; avoids double-tailing on rescan

	// Initial scan + spawn one tailer per existing .log file.
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			tailed.Store(path, struct{}{})
			go tailServiceLog(ctx, buf, path)
		}
	}

	// Rescan loop: pick up files created after hydra started.
	go func() {
		t := time.NewTicker(serviceLogDirRescanInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				entries, err := os.ReadDir(dir)
				if err != nil {
					continue
				}
				for _, e := range entries {
					if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
						continue
					}
					path := filepath.Join(dir, e.Name())
					if _, loaded := tailed.LoadOrStore(path, struct{}{}); loaded {
						continue
					}
					log.Info().Str("file", path).
						Msg("StartServiceLogTailers: tailing newly-appeared service log")
					go tailServiceLog(ctx, buf, path)
				}
			}
		}
	}()
}

// tailServiceLog follows a single log file, writing each appended line
// into buf with a "[<basename-without-.log>] " prefix. Starts from
// end-of-file. Survives transient read errors. Exits when ctx is done.
//
// Rotation is NOT handled: if a service rotates its log (rename + truncate
// or remove + recreate), we'll keep reading the unlinked inode until
// process exit. helix-sandbox today doesn't rotate, so this is fine.
// If/when rotation gets added inside the sandbox, use inotify or
// stat-based detection of size shrinkage to reopen.
func tailServiceLog(ctx context.Context, buf *LogBuffer, path string) {
	prefix := "[" + strings.TrimSuffix(filepath.Base(path), ".log") + "] "

	// Open create-if-missing so a tailer started before the producing
	// service can still attach. O_RDONLY would race the producer on
	// systems where O_CREAT semantics differ; here we want the file to
	// exist with the producer's eventual ownership / mode (the producer
	// opens with O_APPEND|O_CREATE itself in `tee -a`).
	f, err := os.OpenFile(path, os.O_RDONLY|os.O_CREATE, 0o644)
	if err != nil {
		log.Warn().Err(err).Str("path", path).
			Msg("tailServiceLog: open failed; abandoning tailer for this file")
		return
	}
	defer f.Close()

	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		log.Warn().Err(err).Str("path", path).
			Msg("tailServiceLog: seek-end failed; abandoning tailer for this file")
		return
	}

	reader := bufio.NewReader(f)
	poll := time.NewTicker(serviceLogTailPollInterval)
	defer poll.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-poll.C:
			// Drain everything available before sleeping again.
			for {
				line, err := reader.ReadString('\n')
				if line != "" {
					// Trim trailing newline (added back by the
					// LogBuffer consumers if they care about line
					// framing). Keep partial lines without a
					// trailing \n - bufio's ReadString returns them
					// only on EOF, so they represent a producer that
					// hasn't flushed yet; we ship them anyway to
					// minimise latency on burst-tail.
					buf.Write(prefix + strings.TrimRight(line, "\r\n"))
				}
				if err == io.EOF {
					break
				}
				if err != nil {
					log.Debug().Err(err).Str("path", path).
						Msg("tailServiceLog: read error; will retry next tick")
					break
				}
			}
		}
	}
}
