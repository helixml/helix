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
	Offset    int             `json:"offset"`
	Limit     int             `json:"limit"`
}

type GetBotsQuery struct {
	Owner     string          `json:"owner"`
	OwnerType types.OwnerType `json:"owner_type"`
}

type Store interface {
	// sessions
	GetSession(ctx context.Context, id string) (*types.Session, error)
	GetSessions(ctx context.Context, query GetSessionsQuery) ([]*types.Session, error)
	GetSessionsCounter(ctx context.Context, query GetSessionsQuery) (*types.Counter, error)
	CreateSession(ctx context.Context, session types.Session) (*types.Session, error)
	UpdateSession(ctx context.Context, session types.Session) (*types.Session, error)
	UpdateSessionMeta(ctx context.Context, data types.SessionMetaUpdate) (*types.Session, error)
	DeleteSession(ctx context.Context, id string) (*types.Session, error)

	// bots
	GetBot(ctx context.Context, id string) (*types.Bot, error)
	GetBots(ctx context.Context, query GetBotsQuery) ([]*types.Bot, error)
	CreateBot(ctx context.Context, Bot types.Bot) (*types.Bot, error)
	UpdateBot(ctx context.Context, Bot types.Bot) (*types.Bot, error)
	DeleteBot(ctx context.Context, id string) (*types.Bot, error)

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
