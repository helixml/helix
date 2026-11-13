package services

import (
	"context"
	"errors"
	"time"
)

// ErrRepoBusy is returned when a repository lock could not be acquired in time,
// typically because an upstream sync is still running.
var ErrRepoBusy = errors.New("repository busy syncing with upstream")

// RepoLock serializes git operations on one repository. Unlike sync.Mutex it
// supports bounded waits, so request paths can fail fast or degrade instead of
// queueing behind a slow upstream sync.
type RepoLock struct {
	ch chan struct{}
}

func newRepoLock() *RepoLock {
	return &RepoLock{ch: make(chan struct{}, 1)}
}

func (l *RepoLock) Lock() {
	l.ch <- struct{}{}
}

func (l *RepoLock) Unlock() {
	select {
	case <-l.ch:
	default:
		panic("services: unlock of unlocked RepoLock")
	}
}

// LockContext waits for the lock until ctx is done.
func (l *RepoLock) LockContext(ctx context.Context) error {
	select {
	case l.ch <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ErrRepoBusy
	}
}

// LockTimeout waits for the lock for at most d (or until ctx is done).
func (l *RepoLock) LockTimeout(ctx context.Context, d time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	return l.LockContext(ctx)
}
