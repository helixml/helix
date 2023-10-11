package store

import (
	"context"

	"github.com/bacalhau-project/lilysaas/api/pkg/types"
)

type GetJobsQuery struct {
	Owner     string `json:"owner"`
	OwnerType string `json:"owner_type"`
}

type GetBalanceTransfersQuery struct {
	Owner     string `json:"owner"`
	OwnerType string `json:"owner_type"`
}

type Store interface {
	GetJob(ctx context.Context, queryID string) (*types.Job, error)
	GetJobs(ctx context.Context, query GetJobsQuery) ([]*types.Job, error)
	CreateJob(ctx context.Context, job types.Job) error
	UpdateJob(ctx context.Context, id string, state string, status string) error
	GetBalanceTransfers(ctx context.Context, query GetBalanceTransfersQuery) ([]*types.BalanceTransfer, error)
	CreateBalanceTransfer(ctx context.Context, balanceTransfer types.BalanceTransfer) error
}

type StoreOptions struct {
	Host        string
	Port        int
	Database    string
	Username    string
	Password    string
	AutoMigrate bool
}
