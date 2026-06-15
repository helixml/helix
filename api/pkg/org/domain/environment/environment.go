// Package environment owns the Environment entity — a Worker's
// workspace directory on disk. The system records the directory path
// and the Worker it belongs to; it does not own the file contents.
//
// Lifted from api/pkg/org/domain/environment.go in the DDD restructure.
package environment

import (
	"errors"
	"time"
)

// Environment is a Worker's workspace — a directory on disk where
// the agent's files live. The hiring manager populates the directory
// before calling hire_worker, and the agent manages their own files
// from then on.
//
// WorkerID is an orgchart.WorkerID carried as a plain string to keep
// domain/environment from importing domain/orgchart.
type Environment struct {
	OrganizationID string
	WorkerID       string // orgchart.WorkerID
	Path           string
	CreatedAt      time.Time
}

// New validates and constructs an Environment. orgID is required.
func New(workerID string, path string, createdAt time.Time, orgID string) (Environment, error) {
	if workerID == "" {
		return Environment{}, errors.New("environment workerId is empty")
	}
	if path == "" {
		return Environment{}, errors.New("environment path is empty")
	}
	if createdAt.IsZero() {
		return Environment{}, errors.New("environment createdAt is zero")
	}
	if orgID == "" {
		return Environment{}, errors.New("environment orgID is empty")
	}
	return Environment{
		OrganizationID: orgID,
		WorkerID:       workerID,
		Path:           path,
		CreatedAt:      createdAt.UTC(),
	}, nil
}
