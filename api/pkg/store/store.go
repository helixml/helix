package store

import (
	"context"
	"errors"

	"github.com/helixml/helix/api/pkg/license"
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

type ListAPIKeysQuery struct {
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

type ListSecretsQuery struct {
	Owner     string          `json:"owner"`
	OwnerType types.OwnerType `json:"owner_type"`
}

type ListAppsQuery struct {
	Owner     string          `json:"owner"`
	OwnerType types.OwnerType `json:"owner_type"`
	Global    bool            `json:"global"`
}

type ListDataEntitiesQuery struct {
	Owner     string          `json:"owner"`
	OwnerType types.OwnerType `json:"owner_type"`
}

type ListProviderEndpointsQuery struct {
	Owner        string                     `json:"owner"`
	EndpointType types.ProviderEndpointType `json:"endpoint_type"`
}

//go:generate mockgen -source $GOFILE -destination store_mocks.go -package $GOPACKAGE

type Store interface {
	// sessions
	GetSession(ctx context.Context, id string) (*types.Session, error)
	GetSessions(ctx context.Context, query GetSessionsQuery) ([]*types.Session, error)
	GetSessionsCounter(ctx context.Context, query GetSessionsQuery) (*types.Counter, error)
	CreateSession(ctx context.Context, session types.Session) (*types.Session, error)
	UpdateSessionName(ctx context.Context, sessionID, name string) error
	UpdateSession(ctx context.Context, session types.Session) (*types.Session, error)
	UpdateSessionMeta(ctx context.Context, data types.SessionMetaUpdate) (*types.Session, error)
	DeleteSession(ctx context.Context, id string) (*types.Session, error)

	// usermeta
	GetUserMeta(ctx context.Context, id string) (*types.UserMeta, error)
	CreateUserMeta(ctx context.Context, UserMeta types.UserMeta) (*types.UserMeta, error)
	UpdateUserMeta(ctx context.Context, UserMeta types.UserMeta) (*types.UserMeta, error)
	EnsureUserMeta(ctx context.Context, UserMeta types.UserMeta) (*types.UserMeta, error)

	// api keys
	CreateAPIKey(ctx context.Context, apiKey *types.ApiKey) (*types.ApiKey, error)
	GetAPIKey(ctx context.Context, apiKey string) (*types.ApiKey, error)
	ListAPIKeys(ctx context.Context, query *ListAPIKeysQuery) ([]*types.ApiKey, error)
	DeleteAPIKey(ctx context.Context, apiKey string) error

	// tools
	CreateTool(ctx context.Context, tool *types.Tool) (*types.Tool, error)
	UpdateTool(ctx context.Context, tool *types.Tool) (*types.Tool, error)
	GetTool(ctx context.Context, id string) (*types.Tool, error)
	ListTools(ctx context.Context, q *ListToolsQuery) ([]*types.Tool, error)
	DeleteTool(ctx context.Context, id string) error

	// provider endpoints
	CreateProviderEndpoint(ctx context.Context, providerEndpoint *types.ProviderEndpoint) (*types.ProviderEndpoint, error)
	UpdateProviderEndpoint(ctx context.Context, providerEndpoint *types.ProviderEndpoint) (*types.ProviderEndpoint, error)
	GetProviderEndpoint(ctx context.Context, id string) (*types.ProviderEndpoint, error)
	ListProviderEndpoints(ctx context.Context, q *ListProviderEndpointsQuery) ([]*types.ProviderEndpoint, error)
	DeleteProviderEndpoint(ctx context.Context, id string) error

	CreateSecret(ctx context.Context, secret *types.Secret) (*types.Secret, error)
	UpdateSecret(ctx context.Context, secret *types.Secret) (*types.Secret, error)
	GetSecret(ctx context.Context, id string) (*types.Secret, error)
	ListSecrets(ctx context.Context, q *ListSecretsQuery) ([]*types.Secret, error)
	DeleteSecret(ctx context.Context, id string) error

	// apps
	CreateApp(ctx context.Context, tool *types.App) (*types.App, error)
	UpdateApp(ctx context.Context, tool *types.App) (*types.App, error)
	GetApp(ctx context.Context, id string) (*types.App, error)
	GetAppWithTools(ctx context.Context, id string) (*types.App, error)
	ListApps(ctx context.Context, q *ListAppsQuery) ([]*types.App, error)
	DeleteApp(ctx context.Context, id string) error

	// data entities
	CreateDataEntity(ctx context.Context, dataEntity *types.DataEntity) (*types.DataEntity, error)
	UpdateDataEntity(ctx context.Context, dataEntity *types.DataEntity) (*types.DataEntity, error)
	GetDataEntity(ctx context.Context, id string) (*types.DataEntity, error)
	ListDataEntities(ctx context.Context, q *ListDataEntitiesQuery) ([]*types.DataEntity, error)
	DeleteDataEntity(ctx context.Context, id string) error

	// Knowledge
	CreateKnowledge(ctx context.Context, knowledge *types.Knowledge) (*types.Knowledge, error)
	GetKnowledge(ctx context.Context, id string) (*types.Knowledge, error)
	LookupKnowledge(ctx context.Context, q *LookupKnowledgeQuery) (*types.Knowledge, error)
	UpdateKnowledge(ctx context.Context, knowledge *types.Knowledge) (*types.Knowledge, error)
	UpdateKnowledgeState(ctx context.Context, id string, state types.KnowledgeState, message string) error
	ListKnowledge(ctx context.Context, q *ListKnowledgeQuery) ([]*types.Knowledge, error)
	DeleteKnowledge(ctx context.Context, id string) error

	CreateKnowledgeVersion(ctx context.Context, version *types.KnowledgeVersion) (*types.KnowledgeVersion, error)
	GetKnowledgeVersion(ctx context.Context, id string) (*types.KnowledgeVersion, error)
	ListKnowledgeVersions(ctx context.Context, q *ListKnowledgeVersionQuery) ([]*types.KnowledgeVersion, error)
	DeleteKnowledgeVersion(ctx context.Context, id string) error

	// GPTScript runs history table
	CreateScriptRun(ctx context.Context, task *types.ScriptRun) (*types.ScriptRun, error)
	ListScriptRuns(ctx context.Context, q *types.GptScriptRunsQuery) ([]*types.ScriptRun, error)
	DeleteScriptRun(ctx context.Context, id string) error

	CreateLLMCall(ctx context.Context, call *types.LLMCall) (*types.LLMCall, error)
	ListLLMCalls(ctx context.Context, q *ListLLMCallsQuery) ([]*types.LLMCall, int64, error)

	GetLicenseKey(ctx context.Context) (*types.LicenseKey, error)
	SetLicenseKey(ctx context.Context, licenseKey string) error

	GetDecodedLicense(ctx context.Context) (*license.License, error)
}

type EmbeddingsStore interface {
	CreateKnowledgeEmbedding(ctx context.Context, embeddings ...*types.KnowledgeEmbeddingItem) error
	DeleteKnowledgeEmbedding(ctx context.Context, knowledgeID string) error
	QueryKnowledgeEmbeddings(ctx context.Context, q *types.KnowledgeEmbeddingQuery) ([]*types.KnowledgeEmbeddingItem, error)
}

var ErrNotFound = errors.New("not found")
