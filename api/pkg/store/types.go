package store

import (
	"context"

	"github.com/lukemarsden/helix/api/pkg/types"
)

type GetBalanceTransfersQuery struct {
	Owner     string          `json:"owner"`
	OwnerType types.OwnerType `json:"owner_type"`
}

type GetJobsQuery struct {
	Owner     string          `json:"owner"`
	OwnerType types.OwnerType `json:"owner_type"`
}

type OwnerQuery struct {
	Owner     string          `json:"owner"`
	OwnerType types.OwnerType `json:"owner_type"`
}

type GetSessionsQuery struct {
	Owner     string          `json:"owner"`
	OwnerType types.OwnerType `json:"owner_type"`
}

type Store interface {
	// sessions
	GetSession(ctx context.Context, id string) (*types.Session, error)
	GetSessions(ctx context.Context, query GetSessionsQuery) ([]*types.Session, error)
	CreateSession(ctx context.Context, session types.Session) (*types.Session, error)
	UpdateSession(ctx context.Context, session types.Session) (*types.Session, error)
	DeleteSession(ctx context.Context, id string) (*types.Session, error)

	// balance transfers
	GetBalanceTransfers(ctx context.Context, query OwnerQuery) ([]*types.BalanceTransfer, error)
	CreateBalanceTransfer(ctx context.Context, balanceTransfer types.BalanceTransfer) error

	// api keys
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
