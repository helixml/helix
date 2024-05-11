package store

import (
	"context"
	"errors"

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
	Owner         string          `json:"owner"`
	OwnerType     types.OwnerType `json:"owner_type"`
	ParentSession string          `json:"parent_session"`
	Offset        int             `json:"offset"`
	Limit         int             `json:"limit"`
}

type ListApiKeysQuery struct {
	Owner     string           `json:"owner"`
	OwnerType types.OwnerType  `json:"owner_type"`
	Type      types.APIKeyType `json:"type"`
	AppID     string           `json:"app_id"`
}

type ListToolsQuery struct {
	Owner     string          `json:"owner"`
	OwnerType types.OwnerType `json:"owner_type"`
	Global    bool            `json:"global"`
}

type ListAppsQuery struct {
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

	// usermeta
	GetUserMeta(ctx context.Context, id string) (*types.UserMeta, error)
	CreateUserMeta(ctx context.Context, UserMeta types.UserMeta) (*types.UserMeta, error)
	UpdateUserMeta(ctx context.Context, UserMeta types.UserMeta) (*types.UserMeta, error)
	EnsureUserMeta(ctx context.Context, UserMeta types.UserMeta) (*types.UserMeta, error)

	// api keys
	CreateAPIKey(ctx context.Context, apiKey *types.APIKey) (*types.APIKey, error)
	GetAPIKey(ctx context.Context, apiKey string) (*types.APIKey, error)
	ListAPIKeys(ctx context.Context, query *ListApiKeysQuery) ([]*types.APIKey, error)
	DeleteAPIKey(ctx context.Context, apiKey string) error

	// tools
	CreateTool(ctx context.Context, tool *types.Tool) (*types.Tool, error)
	UpdateTool(ctx context.Context, tool *types.Tool) (*types.Tool, error)
	GetTool(ctx context.Context, id string) (*types.Tool, error)
	ListTools(ctx context.Context, q *ListToolsQuery) ([]*types.Tool, error)
	DeleteTool(ctx context.Context, id string) error

	CreateSessionToolBinding(ctx context.Context, sessionID, toolID string) error
	ListSessionTools(ctx context.Context, sessionID string) ([]*types.Tool, error)
	DeleteSessionToolBinding(ctx context.Context, sessionID, toolID string) error

	// apps
	CreateApp(ctx context.Context, tool *types.App) (*types.App, error)
	UpdateApp(ctx context.Context, tool *types.App) (*types.App, error)
	GetApp(ctx context.Context, id string) (*types.App, error)
	ListApps(ctx context.Context, q *ListAppsQuery) ([]*types.App, error)
	DeleteApp(ctx context.Context, id string) error

	CreateGptScriptRunnerTask(ctx context.Context, task *types.GptScriptRunnerTask) (*types.GptScriptRunnerTask, error)
	ListGptScriptRunnerTasks(ctx context.Context, q *types.GptScriptRunnerTasksQuery) ([]*types.GptScriptRunnerTask, error)
	DeleteGptScriptRunnerTask(ctx context.Context, id string) error
}

var ErrNotFound = errors.New("not found")
