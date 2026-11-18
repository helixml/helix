package hosthistory

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/prometheus/procfs"
	"github.com/rs/zerolog/log"
	"golang.org/x/sys/unix"
)

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
