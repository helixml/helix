package hosthistory

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
	"github.com/parquet-go/parquet-go"
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
