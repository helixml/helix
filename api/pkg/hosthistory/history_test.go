package hosthistory

import (
	"context"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParquetHistory(t *testing.T) {
	dir := t.TempDir()
	today := time.Now().UTC().Truncate(24 * time.Hour)
	for _, d := range []int{-35, -1, 0, 0} {
		require.NoError(t, appendSample(dir, Sample{Timestamp: today.AddDate(0, 0, d).Unix(), CPU: 52, Memory: 43, Disk: 78}))
	}
	rows, err := Read(dir, today.Add(-24*time.Hour), today)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, 52., rows[0].CPU)
	_, err = os.Stat(path(dir, today.AddDate(0, 0, -35)))
	require.ErrorIs(t, err, os.ErrNotExist)
	b, err := os.ReadFile(path(dir, today))
	require.NoError(t, err)
	require.Equal(t, "PAR1", string(b[:4]))
}
func TestLiveHostSample(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("host sampling requires Linux procfs")
	}
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, Run(ctx, dir))
	rows, err := Read(dir, time.Now().Add(-time.Minute), time.Now())
	require.NoError(t, err)
	require.Len(t, rows, 1)
	for _, v := range []float64{rows[0].CPU, rows[0].Memory, rows[0].Disk} {
		require.GreaterOrEqual(t, v, 0.)
		require.LessOrEqual(t, v, 100.)
	}
}
