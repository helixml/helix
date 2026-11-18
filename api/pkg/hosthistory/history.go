package hosthistory

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
	"github.com/parquet-go/parquet-go"
	"github.com/prometheus/procfs"
	"github.com/rs/zerolog/log"
	"golang.org/x/sys/unix"
)

const interval = 5 * time.Minute
const Days = 30

type Sample struct {
	Timestamp int64   `json:"timestamp" parquet:"timestamp"`
	CPU       float64 `json:"cpu" parquet:"cpu"`
	Memory    float64 `json:"memory" parquet:"memory"`
	Disk      float64 `json:"disk" parquet:"disk"`
}

func path(dir string, t time.Time) string {
	return filepath.Join(dir, t.UTC().Format(time.DateOnly)+".parquet")
}

func Read(dir string, since, until time.Time) ([]Sample, error) {
	rows := []Sample{}
	for d := since.UTC().Truncate(24 * time.Hour); !d.After(until); d = d.AddDate(0, 0, 1) {
		samples, err := parquet.ReadFile[Sample](path(dir, d))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, s := range samples {
			if s.Timestamp >= since.Unix() && s.Timestamp <= until.Unix() {
				rows = append(rows, s)
			}
		}
	}
	return rows, nil
}

func appendSample(dir string, s Sample) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	lock := flock.New(filepath.Join(dir, "writer.lock"))
	if err := lock.Lock(); err != nil {
		return err
	}
	defer lock.Unlock()
	now := time.Unix(s.Timestamp, 0).UTC()
	dst := path(dir, now)
	rows, err := parquet.ReadFile[Sample](dst)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if len(rows) > 0 && rows[len(rows)-1].Timestamp/int64(interval.Seconds()) >= s.Timestamp/int64(interval.Seconds()) {
		return nil
	}
	tmp, err := os.CreateTemp(dir, ".history-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	err = parquet.Write(tmp, append(rows, s))
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(tmp.Name(), dst); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	cutoff := now.Truncate(24*time.Hour).AddDate(0, 0, -Days)
	for _, entry := range entries {
		d, err := time.Parse("2006-01-02.parquet", entry.Name())
		if err == nil && d.Before(cutoff) {
			if err = os.Remove(filepath.Join(dir, entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

func utilization(before, after procfs.CPUStat) (float64, error) {
	busy := func(c procfs.CPUStat) float64 { return c.User + c.Nice + c.System + c.IRQ + c.SoftIRQ + c.Steal }
	work := busy(after) - busy(before)
	idle := (after.Idle + after.Iowait) - (before.Idle + before.Iowait)
	if work < 0 || idle < 0 || work+idle <= 0 {
		return 0, fmt.Errorf("CPU counters reset or stalled")
	}
	return 100 * work / (work + idle), nil
}

func collect(fs procfs.FS, dir string, before, after procfs.CPUStat, now time.Time) error {
	cpu, err := utilization(before, after)
	if err != nil {
		return err
	}
	mem, err := fs.Meminfo()
	if err != nil {
		return err
	}
	if mem.MemTotalBytes == nil || mem.MemAvailableBytes == nil || *mem.MemTotalBytes == 0 {
		return errors.New("host memory counters unavailable")
	}
	var disk unix.Statfs_t
	if err := unix.Statfs(dir, &disk); err != nil {
		return err
	}
	if disk.Blocks == 0 {
		return errors.New("empty filesystem")
	}
	return appendSample(dir, Sample{Timestamp: now.Unix(), CPU: cpu, Memory: 100 * (1 - float64(*mem.MemAvailableBytes)/float64(*mem.MemTotalBytes)), Disk: 100 * (1 - float64(disk.Bfree)/float64(disk.Blocks))})
}

func Run(ctx context.Context, dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	fs, err := procfs.NewDefaultFS()
	if err != nil {
		return err
	}
	prev, err := fs.Stat()
	if err != nil {
		return err
	}
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-timer.C:
			next, err := fs.Stat()
			if err == nil {
				err = collect(fs, dir, prev.CPUTotal, next.CPUTotal, now)
				prev = next
			}
			if err != nil {
				log.Error().Err(err).Msg("recording host resource history")
			}
			timer.Reset(interval)
		}
	}
}
