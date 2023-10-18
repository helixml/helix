package store

import (
	"context"

	"github.com/bacalhau-project/lilysaas/api/pkg/types"
)

type GetJobsQuery struct {
	Owner     string          `json:"owner"`
	OwnerType types.OwnerType `json:"owner_type"`
}

type GetBalanceTransfersQuery struct {
	Owner     string          `json:"owner"`
	OwnerType types.OwnerType `json:"owner_type"`
}

type GetSessionsQuery struct {
	Owner     string          `json:"owner"`
	OwnerType types.OwnerType `json:"owner_type"`
}

type Store interface {
	// jobs
	GetJob(ctx context.Context, queryID string) (*types.Job, error)
	GetJobs(ctx context.Context, query GetJobsQuery) ([]*types.Job, error)
	CreateJob(ctx context.Context, job types.Job) error
	UpdateJob(ctx context.Context, id string, state string, status string, data types.JobData) error

	// balance transfers
	GetBalanceTransfers(ctx context.Context, query GetBalanceTransfersQuery) ([]*types.BalanceTransfer, error)
	CreateBalanceTransfer(ctx context.Context, balanceTransfer types.BalanceTransfer) error

	// sessions
	GetSession(ctx context.Context, id string) (*types.Session, error)
	GetSessions(ctx context.Context, query GetSessionsQuery) ([]*types.Session, error)
	CreateSession(ctx context.Context, session types.Session) (*types.Session, error)
	UpdateSession(ctx context.Context, session types.Session) (*types.Session, error)
	DeleteSession(ctx context.Context, id string) (*types.Session, error)
}


type StoreOptions struct {
	Host        string
	Port        int
	Database    string
	Username    string
	Password    string
	AutoMigrate bool
}
