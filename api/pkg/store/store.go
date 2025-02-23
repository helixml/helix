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
	Owner     string
	OwnerType types.OwnerType

	WithGlobal bool
}

type GetProviderEndpointsQuery struct {
	Owner     string
	OwnerType types.OwnerType
	ID        string
	Name      string
}

type ListUsersQuery struct {
	TokenType types.TokenType `json:"token_type"`
	Admin     bool            `json:"admin"`
	Type      types.OwnerType `json:"type"`
	Email     string          `json:"email"`
	Username  string          `json:"username"`
}

var _ Store = &PostgresStore{}

//go:generate mockgen -source $GOFILE -destination store_mocks.go -package $GOPACKAGE

type Store interface {
	//  Auth + Authz
	CreateOrganization(ctx context.Context, org *types.Organization) (*types.Organization, error)
	GetOrganization(ctx context.Context, q *GetOrganizationQuery) (*types.Organization, error)
	UpdateOrganization(ctx context.Context, org *types.Organization) (*types.Organization, error)
	DeleteOrganization(ctx context.Context, id string) error
	ListOrganizations(ctx context.Context, query *ListOrganizationsQuery) ([]*types.Organization, error)

	CreateOrganizationMembership(ctx context.Context, membership *types.OrganizationMembership) (*types.OrganizationMembership, error)
	GetOrganizationMembership(ctx context.Context, q *GetOrganizationMembershipQuery) (*types.OrganizationMembership, error)
	UpdateOrganizationMembership(ctx context.Context, membership *types.OrganizationMembership) (*types.OrganizationMembership, error)
	DeleteOrganizationMembership(ctx context.Context, organizationID, userID string) error
	ListOrganizationMemberships(ctx context.Context, query *ListOrganizationMembershipsQuery) ([]*types.OrganizationMembership, error)

	CreateTeam(ctx context.Context, team *types.Team) (*types.Team, error)
	GetTeam(ctx context.Context, q *GetTeamQuery) (*types.Team, error)
	UpdateTeam(ctx context.Context, team *types.Team) (*types.Team, error)
	DeleteTeam(ctx context.Context, id string) error
	ListTeams(ctx context.Context, query *ListTeamsQuery) ([]*types.Team, error)

	CreateRole(ctx context.Context, role *types.Role) (*types.Role, error)
	GetRole(ctx context.Context, id string) (*types.Role, error)
	UpdateRole(ctx context.Context, role *types.Role) (*types.Role, error)
	DeleteRole(ctx context.Context, id string) error
	ListRoles(ctx context.Context, organizationID string) ([]*types.Role, error)

	CreateAccessGrant(ctx context.Context, resourceAccess *types.AccessGrant, roles []*types.Role) (*types.AccessGrant, error)
	ListAccessGrants(ctx context.Context, q *ListAccessGrantsQuery) ([]*types.AccessGrant, error)
	DeleteAccessGrant(ctx context.Context, id string) error

	CreateAccessGrantRoleBinding(ctx context.Context, binding *types.AccessGrantRoleBinding) (*types.AccessGrantRoleBinding, error)
	DeleteAccessGrantRoleBinding(ctx context.Context, accessGrantID, roleID string) error
	GetAccessGrantRoleBindings(ctx context.Context, q *GetAccessGrantRoleBindingsQuery) ([]*types.AccessGrantRoleBinding, error)

	// sessions
	GetSession(ctx context.Context, id string) (*types.Session, error)
	GetSessions(ctx context.Context, query GetSessionsQuery) ([]*types.Session, error)
	GetSessionsCounter(ctx context.Context, query GetSessionsQuery) (*types.Counter, error)
	CreateSession(ctx context.Context, session types.Session) (*types.Session, error)
	UpdateSessionName(ctx context.Context, sessionID, name string) error
	UpdateSession(ctx context.Context, session types.Session) (*types.Session, error)
	UpdateSessionMeta(ctx context.Context, data types.SessionMetaUpdate) (*types.Session, error)
	DeleteSession(ctx context.Context, id string) (*types.Session, error)

	// users
	GetUser(ctx context.Context, q *GetUserQuery) (*types.User, error)
	CreateUser(ctx context.Context, user *types.User) (*types.User, error)
	UpdateUser(ctx context.Context, user *types.User) (*types.User, error)
	DeleteUser(ctx context.Context, id string) error
	ListUsers(ctx context.Context, query *ListUsersQuery) ([]*types.User, error)

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
	GetProviderEndpoint(ctx context.Context, q *GetProviderEndpointsQuery) (*types.ProviderEndpoint, error)
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
