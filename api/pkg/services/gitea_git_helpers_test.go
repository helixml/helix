package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRunGitFetchWithRetryRecoversFromDNSFailure(t *testing.T) {
	attempts := 0
	var waits []time.Duration

	stderr, err := runGitFetchWithRetry(context.Background(), "/repo", func() (string, error) {
		attempts++
		if attempts < 3 {
			return "fatal: Could not resolve host: github.com", errors.New("exit status 128")
		}
		return "", nil
	}, func(_ context.Context, delay time.Duration) error {
		waits = append(waits, delay)
		return nil
	})

	require.NoError(t, err)
	require.Empty(t, stderr)
	require.Equal(t, 3, attempts)
	require.Equal(t, []time.Duration{time.Second, 2 * time.Second}, waits)
}

func TestRunGitFetchWithRetryDoesNotRetryPermanentFailure(t *testing.T) {
	attempts := 0
	stderr, err := runGitFetchWithRetry(context.Background(), "/repo", func() (string, error) {
		attempts++
		return "fatal: Authentication failed", errors.New("exit status 128")
	}, func(context.Context, time.Duration) error {
		t.Fatal("wait called for permanent failure")
		return nil
	})

	require.Error(t, err)
	require.Equal(t, "fatal: Authentication failed", stderr)
	require.Equal(t, 1, attempts)
}

func TestRunGitFetchWithRetryIsBounded(t *testing.T) {
	attempts := 0
	var waits []time.Duration
	stderr, err := runGitFetchWithRetry(context.Background(), "/repo", func() (string, error) {
		attempts++
		return "fatal: unable to access remote: Connection timed out", errors.New("exit status 128")
	}, func(_ context.Context, delay time.Duration) error {
		waits = append(waits, delay)
		return nil
	})

	require.Error(t, err)
	require.Contains(t, stderr, "Connection timed out")
	require.Equal(t, 6, attempts)
	require.Equal(t, []time.Duration{
		time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
	}, waits)
}

func TestRunGitFetchWithRetryHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := runGitFetchWithRetry(ctx, "/repo", func() (string, error) {
		return "fatal: Could not resolve host: github.com", errors.New("exit status 128")
	}, waitForGitFetchRetry)

	require.ErrorIs(t, err, context.Canceled)
}
