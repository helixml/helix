package store

import (
	"context"
	"errors"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

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

type ListToolsQuery struct {
	Owner     string          `json:"owner"`
	OwnerType types.OwnerType `json:"owner_type"`
}

//go:generate mockgen -source $GOFILE -destination store_mocks.go -package $GOPACKAGE

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

	// usermeta
	GetUserMeta(ctx context.Context, id string) (*types.UserMeta, error)
	CreateUserMeta(ctx context.Context, UserMeta types.UserMeta) (*types.UserMeta, error)
	UpdateUserMeta(ctx context.Context, UserMeta types.UserMeta) (*types.UserMeta, error)
	EnsureUserMeta(ctx context.Context, UserMeta types.UserMeta) (*types.UserMeta, error)

	// api keys
	CreateAPIKey(ctx context.Context, owner OwnerQuery, name string) (string, error)
	GetAPIKeys(ctx context.Context, query OwnerQuery) ([]*types.ApiKey, error)
	DeleteAPIKey(ctx context.Context, apiKey types.ApiKey) error
	CheckAPIKey(ctx context.Context, apiKey string) (*types.ApiKey, error)

	CreateTool(ctx context.Context, tool *types.Tool) (*types.Tool, error)
	GetTool(ctx context.Context, id string) (*types.Tool, error)
	ListTool(ctx context.Context, q *ListToolsQuery) ([]*types.Tool, error)
	DeleteTool(ctx context.Context, id string) error
}

var ErrNotFound = errors.New("not found")

type StoreOptions struct {
	Host        string
	Port        int
	Database    string
	Username    string
	Password    string
	AutoMigrate bool

	MaxConns        int           `envconfig:"DATABASE_MAX_CONNS" default:"50"`
	IdleConns       int           `envconfig:"DATABASE_IDLE_CONNS" default:"25"`
	MaxConnLifetime time.Duration `envconfig:"DATABASE_MAX_CONN_LIFETIME" default:"1h"`
	MaxConnIdleTime time.Duration `envconfig:"DATABASE_MAX_CONN_IDLE_TIME" default:"1m"`
}
