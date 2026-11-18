// Package sessionresources keeps the sandbox resource samples that heartbeats
// report for desktop sessions, as Parquet files in the local filestore.
//
// Layout: <dir>/<YYYY-MM-DD>/<session_id>/<unix-ms>-<node>.parquet. Samples are
// buffered in memory and written as an immutable chunk per session every flush
// interval. Once a session-day has compactAt chunks they are merged into one,
// so a day holds at most compactAt files per session. Readers merge every file
// with the unflushed buffer and drop duplicate timestamps, which also covers a
// compaction interrupted between writing its output and removing its inputs.
package sessionresources

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/flock"
	"github.com/parquet-go/parquet-go"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/types"
)

const (
	// Days of history kept, matching host history.
	Days          = 30
	flushInterval = 5 * time.Minute
	compactAt     = 24
)

var sessionIDPattern = regexp.MustCompile(`^ses_[0-9a-z]+$`)

type Store struct {
	dir  string
	node string

	mu  sync.Mutex
	buf map[string][]types.SessionResourceSample
}

func New(dir, node string) *Store {
	if node == "" {
		node = "api"
	}
	return &Store{dir: dir, node: node, buf: map[string][]types.SessionResourceSample{}}
}

// Record buffers samples until the next flush.
func (s *Store) Record(samples []types.SessionResourceSample) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sample := range samples {
		s.buf[sample.SessionID] = append(s.buf[sample.SessionID], sample)
	}
}

// Run flushes the buffer periodically and once more on shutdown.
func (s *Store) Run(ctx context.Context) {
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			if err := s.Flush(time.Now()); err != nil {
				log.Error().Err(err).Msg("flushing session resource history on shutdown")
			}
			return
		case now := <-ticker.C:
			if err := s.Flush(now); err != nil {
				log.Error().Err(err).Msg("flushing session resource history")
			}
		}
	}
}

func day(ms int64) string {
	return time.UnixMilli(ms).UTC().Format(time.DateOnly)
}

// Flush writes buffered samples and removes days past retention.
func (s *Store) Flush(now time.Time) error {
	s.mu.Lock()
	pending := s.buf
	s.buf = map[string][]types.SessionResourceSample{}
	s.mu.Unlock()

	var errs []error
	for sessionID, samples := range pending {
		byDay := map[string][]types.SessionResourceSample{}
		for _, sample := range samples {
			byDay[day(sample.Timestamp)] = append(byDay[day(sample.Timestamp)], sample)
		}
		for d, rows := range byDay {
			if err := s.writeChunk(filepath.Join(s.dir, d, sessionID), rows); err != nil {
				errs = append(errs, fmt.Errorf("session %s %s: %w", sessionID, d, err))
			}
		}
	}
	return errors.Join(append(errs, s.prune(now))...)
}

func writeParquet(dir, dst string, rows []types.SessionResourceSample) error {
	tmp, err := os.CreateTemp(dir, ".chunk-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	err = parquet.Write(tmp, rows)
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dst)
}

func chunks(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), ".parquet") {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	return files, nil
}

func (s *Store) writeChunk(dir string, rows []types.SessionResourceSample) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	lock := flock.New(filepath.Join(dir, ".lock"))
	if err := lock.Lock(); err != nil {
		return err
	}
	defer lock.Unlock()

	name := fmt.Sprintf("%d-%s.parquet", rows[len(rows)-1].Timestamp, s.node)
	if err := writeParquet(dir, filepath.Join(dir, name), rows); err != nil {
		return err
	}
	files, err := chunks(dir)
	if err != nil || len(files) < compactAt {
		return err
	}
	merged, err := readFiles(files)
	if err != nil {
		return err
	}
	last := merged[len(merged)-1].Timestamp
	if err := writeParquet(dir, filepath.Join(dir, fmt.Sprintf("%d-%s-merged.parquet", last, s.node)), merged); err != nil {
		return err
	}
	for _, f := range files {
		if err := os.Remove(f); err != nil {
			return err
		}
	}
	return nil
}

// readFiles returns the samples of files sorted by time, one per timestamp.
func readFiles(files []string) ([]types.SessionResourceSample, error) {
	var rows []types.SessionResourceSample
	for _, f := range files {
		r, err := parquet.ReadFile[types.SessionResourceSample](f)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		rows = append(rows, r...)
	}
	return dedupe(rows), nil
}

func dedupe(rows []types.SessionResourceSample) []types.SessionResourceSample {
	slices.SortStableFunc(rows, func(a, b types.SessionResourceSample) int {
		return int(a.Timestamp - b.Timestamp)
	})
	return slices.CompactFunc(rows, func(a, b types.SessionResourceSample) bool { return a.Timestamp == b.Timestamp })
}

func (s *Store) prune(now time.Time) error {
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	cutoff := now.UTC().Truncate(24*time.Hour).AddDate(0, 0, -Days)
	for _, entry := range entries {
		d, err := time.Parse(time.DateOnly, entry.Name())
		if err == nil && entry.IsDir() && d.Before(cutoff) {
			if err := os.RemoveAll(filepath.Join(s.dir, entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

// Read returns a session's samples in [since, until], oldest first.
func (s *Store) Read(sessionID string, since, until time.Time) ([]types.SessionResourceSample, error) {
	if !sessionIDPattern.MatchString(sessionID) {
		return nil, fmt.Errorf("invalid session id %q", sessionID)
	}
	var files []string
	for d := since.UTC().Truncate(24 * time.Hour); !d.After(until); d = d.AddDate(0, 0, 1) {
		f, err := chunks(filepath.Join(s.dir, d.Format(time.DateOnly), sessionID))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		files = append(files, f...)
	}
	rows, err := readFiles(files)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	rows = append(rows, s.buf[sessionID]...)
	s.mu.Unlock()
	rows = dedupe(rows)
	return slices.DeleteFunc(rows, func(r types.SessionResourceSample) bool {
		return r.Timestamp < since.UnixMilli() || r.Timestamp > until.UnixMilli()
	}), nil
}
