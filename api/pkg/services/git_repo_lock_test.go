package services

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRepoLock_LockTimeout(t *testing.T) {
	l := newRepoLock()
	l.Lock()

	start := time.Now()
	err := l.LockTimeout(context.Background(), 50*time.Millisecond)
	require.ErrorIs(t, err, ErrRepoBusy)
	require.Less(t, time.Since(start), time.Second)

	l.Unlock()
	require.NoError(t, l.LockTimeout(context.Background(), 50*time.Millisecond))
	l.Unlock()
}

func TestRepoLock_UnlockOfUnlockedPanics(t *testing.T) {
	require.Panics(t, func() { newRepoLock().Unlock() })
}
