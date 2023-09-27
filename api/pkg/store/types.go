package store

import (
	"context"

	"github.com/bacalhau-project/lilysaas/api/pkg/types"
)

type ListJobsQuery struct {
}

type Store interface {
	GetJob(ctx context.Context, queryID string) (*types.Job, error)
	CreateJob(ctx context.Context, job types.Job) error
	UpdateJob(ctx context.Context, id string, state string, status string) error
}

// type Store interface {
// 	ListJobs(ctx context.Context, query ListJobsQuery) ([]types.Job, error)
// 	GetJob(ctx context.Context, id string) (*types.Job, error)
// 	AddJob(ctx context.Context, data types.Job) error
// }

type StoreOptions struct {
	Host        string
	Port        int
	Database    string
	Username    string
	Password    string
	AutoMigrate bool
}
