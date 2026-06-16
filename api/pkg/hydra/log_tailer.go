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

// tailServiceLog follows a single log file, writing each newline-terminated
// line into buf with a "[<basename-without-.log>] " prefix. Survives
// transient read errors. Exits when ctx is done.
//
// Reads from the START of the file (NOT seek-to-end). The cont-init.d
// wrappers truncate their respective files at boot, so on a fresh
// helix-sandbox container the file is empty when this tailer opens it -
// safe to read from the beginning. On hydra reconnect after its own
// crash-and-supervisor-restart, the file may contain pre-restart
// content; we replay that into the LogBuffer too (better to show stale
// context on the admin UI than to silently swallow the diagnostic
// content the operator most needs - if a service crashed before hydra
// came back up, its last-words are exactly what we want surfaced).
//
// Partial-line buffering: if the producer writes "pulling layer abc... "
// without a trailing newline, the previous version of this code shipped
// the partial line, then shipped the rest on the next tick - resulting
// in two LogBuffer entries with broken prefixes. We now accumulate
// partials in `pending` and only Write when we have a complete line
// terminator, with a safety cap (partialLineCap) so a producer that
// writes a million bytes without a newline can't OOM us.
//
// Rotation is NOT handled: if a service rotates its log (rename +
// recreate), we'll keep reading the unlinked inode until process exit.
// helix-sandbox today doesn't rotate inside its lifetime. If/when that
// changes, add stat-based detection of file-size shrinkage and reopen.
func tailServiceLog(ctx context.Context, buf *LogBuffer, path string) {
	prefix := "[" + strings.TrimSuffix(filepath.Base(path), ".log") + "] "

	// Open create-if-missing so a tailer started before the producing
	// service can still attach. O_RDONLY because we never write.
	f, err := os.OpenFile(path, os.O_RDONLY|os.O_CREATE, 0o644)
	if err != nil {
		log.Warn().Err(err).Str("path", path).
			Msg("tailServiceLog: open failed; abandoning tailer for this file")
		return
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	poll := time.NewTicker(serviceLogTailPollInterval)
	defer poll.Stop()

	// Buffer for the trailing partial line (no \n yet). Flushed when a
	// later read sees the terminator, OR when it grows past partialLineCap
	// (defensive against producers that never newline-terminate).
	var pending strings.Builder

	flushComplete := func() {
		// Drain everything available before sleeping again. We loop
		// because bufio's read buffer may return EOF in the middle of a
		// larger physical chunk if the producer is mid-write.
		for {
			line, err := reader.ReadString('\n')
			if line != "" {
				if strings.HasSuffix(line, "\n") {
					// Complete line - prepend any pending partial,
					// trim, ship.
					if pending.Len() > 0 {
						line = pending.String() + line
						pending.Reset()
					}
					buf.Write(prefix + strings.TrimRight(line, "\r\n"))
				} else {
					// Partial line (only happens at EOF). Append to
					// pending and bail to the cap check below.
					pending.WriteString(line)
					if pending.Len() > partialLineCap {
						// Producer is misbehaving (or it's a binary
						// blob). Flush what we have rather than buffer
						// forever, mark with a [partial] suffix so the
						// operator knows the line was cut.
						buf.Write(prefix + pending.String() + " [partial: line exceeded cap]")
						pending.Reset()
					}
				}
			}
			if err == io.EOF {
				return
			}
			if err != nil {
				log.Debug().Err(err).Str("path", path).
					Msg("tailServiceLog: read error; will retry next tick")
				return
			}
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-poll.C:
			flushComplete()
		}
	}
}

// partialLineCap bounds how much we'll buffer waiting for a newline.
// Long enough for any realistic log line including JSON-encoded
// heartbeat payloads or dockerd image-pull progress lines, short
// enough to bound memory if a producer streams a binary blob.
const partialLineCap = 256 * 1024 // 256 KiB
