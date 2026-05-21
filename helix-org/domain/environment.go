package domain

import (
	"errors"
	"time"
)

// Environment is a Worker's workspace — a directory on disk where the
// agent's files live. The system records the directory path and the
// Worker it belongs to; it does not own the file contents. The hiring
// manager populates the directory before calling hire_worker, and the
// agent manages their own files from then on.
type Environment struct {
	WorkerID  WorkerID
	Path      string
	CreatedAt time.Time
}

// NewEnvironment validates and constructs an Environment.
func NewEnvironment(workerID WorkerID, path string, createdAt time.Time) (Environment, error) {
	if workerID == "" {
		return Environment{}, errors.New("environment workerId is empty")
	}
	if path == "" {
		return Environment{}, errors.New("environment path is empty")
	}
	if createdAt.IsZero() {
		return Environment{}, errors.New("environment createdAt is zero")
	}
	return Environment{
		WorkerID:  workerID,
		Path:      path,
		CreatedAt: createdAt.UTC(),
	}, nil
}
