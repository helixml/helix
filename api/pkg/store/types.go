package store

import (
	"context"

	"github.com/bacalhau-project/lilysaas/api/pkg/types"
)

type GetJobsQuery struct {
	Owner     string          `json:"owner"`
	OwnerType types.OwnerType `json:"owner_type"`
}

type OwnerQuery struct {
	Owner     string          `json:"owner"`
	OwnerType types.OwnerType `json:"owner_type"`
}

type Store interface {
	GetJob(ctx context.Context, queryID string) (*types.Job, error)
	GetJobs(ctx context.Context, query GetJobsQuery) ([]*types.Job, error)
	CreateJob(ctx context.Context, job types.Job) error
	UpdateJob(ctx context.Context, id string, state string, status string, data types.JobData) error
	GetBalanceTransfers(ctx context.Context, query OwnerQuery) ([]*types.BalanceTransfer, error)
	CreateBalanceTransfer(ctx context.Context, balanceTransfer types.BalanceTransfer) error
	CreateAPIKey(ctx context.Context, owner OwnerQuery, name string) (string, error)
	GetAPIKeys(ctx context.Context, query OwnerQuery) ([]*types.ApiKey, error)
	DeleteAPIKey(ctx context.Context, apiKey types.ApiKey) error
	CheckAPIKey(ctx context.Context, apiKey string) (*types.ApiKey, error)
}

type StoreOptions struct {
	Host        string
	Port        int
	Database    string
	Username    string
	Password    string
	AutoMigrate bool
}
