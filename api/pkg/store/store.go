package store

import (
	"context"
	"errors"
	"time"

	"github.com/helixml/helix/api/pkg/license"
	"github.com/helixml/helix/api/pkg/types"
)

type ListProjectsQuery struct {
	UserID         string
	OrganizationID string
}

type GetJobsQuery struct {
	Owner     string          `json:"owner"`
	OwnerType types.OwnerType `json:"owner_type"`
}

type OwnerQuery struct {
	Owner     string          `json:"owner"`
	OwnerType types.OwnerType `json:"owner_type"`
}

type ListSessionsQuery struct {
	Owner                  string          `json:"owner"`
	OwnerType              types.OwnerType `json:"owner_type"`
	ParentSession          string          `json:"parent_session"`
	OrganizationID         string          `json:"organization_id"` // The organization this session belongs to, if any
	Page                   int             `json:"page"`
	PerPage                int             `json:"per_page"`
	Search                 string          `json:"search"`
	QuestionSetID          string          `json:"question_set_id"`
	QuestionSetExecutionID string          `json:"question_set_execution_id"`
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
	Owner          string          `json:"owner"`
	OwnerType      types.OwnerType `json:"owner_type"`
	Global         bool            `json:"global"`
	OrganizationID string          `json:"organization_id"`
}

type ListDataEntitiesQuery struct {
	Owner     string          `json:"owner"`
	OwnerType types.OwnerType `json:"owner_type"`
}

type ListProviderEndpointsQuery struct {
	Owner      string
	OwnerType  types.OwnerType
	All        bool
	WithGlobal bool
}

type GetProviderEndpointsQuery struct {
	Owner     string
	OwnerType types.OwnerType
	ID        string
	Name      string
}

type GetTriggerConfigurationQuery struct {
	ID             string
	Owner          string
	OwnerType      types.OwnerType
	OrganizationID string
}

type ListTriggerConfigurationsQuery struct {
	AppID          string
	Owner          string
	OwnerType      types.OwnerType
	OrganizationID string
	TriggerType    types.TriggerType
	Enabled        bool
}

type ListTriggerExecutionsQuery struct {
	TriggerID string
	Offset    int
	Limit     int
}

type ListQuestionSetExecutionsQuery struct {
	QuestionSetID string
	AppID         string
	Offset        int
	Limit         int
}

type ListUsersQuery struct {
	TokenType types.TokenType `json:"token_type"`
	Admin     bool            `json:"admin"`
	Type      types.OwnerType `json:"type"`
	Email     string          `json:"email"`
	Username  string          `json:"username"`
	Page      int
	PerPage   int
	Order     string // Defaults to Created Desc
}

// SearchUsersQuery defines parameters for searching users with partial matching
type SearchUsersQuery struct {
	Query          string `json:"query"`           // Query to match against email, name, or username (LIKE query)
	OrganizationID string `json:"organization_id"` // Organization ID to filter users that are members of the org
	Limit          int    `json:"limit"`           // Maximum number of results to return
	Offset         int    `json:"offset"`          // Offset for pagination
}

type GetAggregatedUsageMetricsQuery struct {
	UserID         string
	OrganizationID string
	From           time.Time
	To             time.Time
}

var _ Store = &PostgresStore{}

//go:generate mockgen -source $GOFILE -destination store_mocks.go -package $GOPACKAGE

var (
	ErrNotFound = errors.New("not found")
	ErrMultiple = errors.New("multiple found")
	ErrConflict = errors.New("conflict")
)

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

	CreateTeamMembership(ctx context.Context, membership *types.TeamMembership) (*types.TeamMembership, error)
	GetTeamMembership(ctx context.Context, q *GetTeamMembershipQuery) (*types.TeamMembership, error)
	ListTeamMemberships(ctx context.Context, query *ListTeamMembershipsQuery) ([]*types.TeamMembership, error)
	DeleteTeamMembership(ctx context.Context, teamID, userID string) error

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
	GetSessionIncludingDeleted(ctx context.Context, id string) (*types.Session, error) // Includes soft-deleted sessions
	ListSessions(ctx context.Context, query ListSessionsQuery) ([]*types.Session, int64, error)
	CreateSession(ctx context.Context, session types.Session) (*types.Session, error)
	UpdateSessionName(ctx context.Context, sessionID, name string) error
	UpdateSession(ctx context.Context, session types.Session) (*types.Session, error)
	UpdateSessionMeta(ctx context.Context, data types.SessionMetaUpdate) (*types.Session, error)
	DeleteSession(ctx context.Context, id string) (*types.Session, error)

	// interactions
	ListInteractions(ctx context.Context, query *types.ListInteractionsQuery) ([]*types.Interaction, int64, error)
	CreateInteraction(ctx context.Context, interaction *types.Interaction) (*types.Interaction, error)
	CreateInteractions(ctx context.Context, interactions ...*types.Interaction) error
	GetInteraction(ctx context.Context, id string) (*types.Interaction, error)
	UpdateInteraction(ctx context.Context, interaction *types.Interaction) (*types.Interaction, error)
	DeleteInteraction(ctx context.Context, id string) error

	// slots
	CreateSlot(ctx context.Context, slot *types.RunnerSlot) (*types.RunnerSlot, error)
	GetSlot(ctx context.Context, id string) (*types.RunnerSlot, error)
	UpdateSlot(ctx context.Context, slot *types.RunnerSlot) (*types.RunnerSlot, error)
	DeleteSlot(ctx context.Context, id string) error
	ListSlots(ctx context.Context, runnerID string) ([]*types.RunnerSlot, error)
	ListAllSlots(ctx context.Context) ([]*types.RunnerSlot, error)

	// step infos
	CreateStepInfo(ctx context.Context, stepInfo *types.StepInfo) (*types.StepInfo, error)
	ListStepInfos(ctx context.Context, query *ListStepInfosQuery) ([]*types.StepInfo, error)
	DeleteStepInfo(ctx context.Context, sessionID string) error

	// users
	GetUser(ctx context.Context, q *GetUserQuery) (*types.User, error)
	CreateUser(ctx context.Context, user *types.User) (*types.User, error)
	UpdateUser(ctx context.Context, user *types.User) (*types.User, error)
	DeleteUser(ctx context.Context, id string) error
	ListUsers(ctx context.Context, query *ListUsersQuery) ([]*types.User, int64, error)
	SearchUsers(ctx context.Context, query *SearchUsersQuery) ([]*types.User, int64, error)
	CountUsers(ctx context.Context) (int64, error)

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

	CreateLLMCall(ctx context.Context, call *types.LLMCall) (*types.LLMCall, error)
	ListLLMCalls(ctx context.Context, q *ListLLMCallsQuery) ([]*types.LLMCall, int64, error)

	GetLicenseKey(ctx context.Context) (*types.LicenseKey, error)
	SetLicenseKey(ctx context.Context, licenseKey string) error

	GetDecodedLicense(ctx context.Context) (*license.License, error)

	CreateModel(ctx context.Context, model *types.Model) (*types.Model, error)
	UpdateModel(ctx context.Context, model *types.Model) (*types.Model, error)
	GetModel(ctx context.Context, id string) (*types.Model, error)
	ListModels(ctx context.Context, q *ListModelsQuery) ([]*types.Model, error)
	DeleteModel(ctx context.Context, id string) error

	// Model info for dynamic pricing
	CreateDynamicModelInfo(ctx context.Context, modelInfo *types.DynamicModelInfo) (*types.DynamicModelInfo, error)
	GetDynamicModelInfo(ctx context.Context, id string) (*types.DynamicModelInfo, error)
	UpdateDynamicModelInfo(ctx context.Context, modelInfo *types.DynamicModelInfo) (*types.DynamicModelInfo, error)
	DeleteDynamicModelInfo(ctx context.Context, id string) error
	ListDynamicModelInfos(ctx context.Context, q *types.ListDynamicModelInfosQuery) ([]*types.DynamicModelInfo, error)

	// OAuth Provider methods
	// ListOAuthProvidersQuery contains filters for listing OAuth providers
	// ListOAuthConnectionsQuery contains filters for listing OAuth connections
	CreateOAuthProvider(ctx context.Context, provider *types.OAuthProvider) (*types.OAuthProvider, error)
	GetOAuthProvider(ctx context.Context, id string) (*types.OAuthProvider, error)
	UpdateOAuthProvider(ctx context.Context, provider *types.OAuthProvider) (*types.OAuthProvider, error)
	DeleteOAuthProvider(ctx context.Context, id string) error
	ListOAuthProviders(ctx context.Context, query *ListOAuthProvidersQuery) ([]*types.OAuthProvider, error)

	// OAuth Connection methods
	CreateOAuthConnection(ctx context.Context, connection *types.OAuthConnection) (*types.OAuthConnection, error)
	GetOAuthConnection(ctx context.Context, id string) (*types.OAuthConnection, error)
	GetOAuthConnectionByUserAndProvider(ctx context.Context, userID, providerID string) (*types.OAuthConnection, error)
	UpdateOAuthConnection(ctx context.Context, connection *types.OAuthConnection) (*types.OAuthConnection, error)
	DeleteOAuthConnection(ctx context.Context, id string) error
	ListOAuthConnections(ctx context.Context, query *ListOAuthConnectionsQuery) ([]*types.OAuthConnection, error)
	GetOAuthConnectionsNearExpiry(ctx context.Context, expiresBefore time.Time) ([]*types.OAuthConnection, error)

	// OAuth Request Token methods
	CreateOAuthRequestToken(ctx context.Context, token *types.OAuthRequestToken) (*types.OAuthRequestToken, error)
	GetOAuthRequestToken(ctx context.Context, userID, providerID string) ([]*types.OAuthRequestToken, error)
	GetOAuthRequestTokenByState(ctx context.Context, state string) ([]*types.OAuthRequestToken, error)
	DeleteOAuthRequestToken(ctx context.Context, id string) error
	GenerateRandomState(ctx context.Context) (string, error)

	CreateUsageMetric(ctx context.Context, metric *types.UsageMetric) (*types.UsageMetric, error)
	GetAppUsageMetrics(ctx context.Context, appID string, from time.Time, to time.Time) ([]*types.UsageMetric, error)
	GetAppDailyUsageMetrics(ctx context.Context, appID string, from time.Time, to time.Time) ([]*types.AggregatedUsageMetric, error)
	DeleteUsageMetrics(ctx context.Context, appID string) error
	GetUserMonthlyTokenUsage(ctx context.Context, userID string, providers []string) (int, error)

	GetProviderDailyUsageMetrics(ctx context.Context, providerID string, from time.Time, to time.Time) ([]*types.AggregatedUsageMetric, error)

	GetUsersAggregatedUsageMetrics(ctx context.Context, provider string, from time.Time, to time.Time) ([]*types.UsersAggregatedUsageMetric, error)
	GetAppUsersAggregatedUsageMetrics(ctx context.Context, appID string, from time.Time, to time.Time) ([]*types.UsersAggregatedUsageMetric, error)

	GetAggregatedUsageMetrics(ctx context.Context, q *GetAggregatedUsageMetricsQuery) ([]*types.AggregatedUsageMetric, error)

	CreateSlackThread(ctx context.Context, thread *types.SlackThread) (*types.SlackThread, error)
	GetSlackThread(ctx context.Context, appID, channel, threadKey string) (*types.SlackThread, error)
	DeleteSlackThread(ctx context.Context, olderThan time.Time) error

	CreateCrispThread(ctx context.Context, thread *types.CrispThread) (*types.CrispThread, error)
	GetCrispThread(ctx context.Context, appID, crispSessionID string) (*types.CrispThread, error)
	DeleteCrispThread(ctx context.Context, olderThan time.Time) error

	// wallet methods
	CreateWallet(ctx context.Context, wallet *types.Wallet) (*types.Wallet, error)
	GetWallet(ctx context.Context, id string) (*types.Wallet, error)
	GetWalletByUser(ctx context.Context, userID string) (*types.Wallet, error)
	GetWalletByOrg(ctx context.Context, orgID string) (*types.Wallet, error)
	GetWalletByStripeCustomerID(ctx context.Context, stripeCustomerID string) (*types.Wallet, error)
	UpdateWallet(ctx context.Context, wallet *types.Wallet) (*types.Wallet, error)
	DeleteWallet(ctx context.Context, id string) error
	UpdateWalletBalance(ctx context.Context, walletID string, amount float64, meta types.TransactionMetadata) (*types.Wallet, error)

	// transaction methods
	ListTransactions(ctx context.Context, q *ListTransactionsQuery) ([]*types.Transaction, error)

	// topup methods
	ListTopUps(ctx context.Context, q *ListTopUpsQuery) ([]*types.TopUp, error)

	// trigger configurations
	CreateTriggerConfiguration(ctx context.Context, triggerConfig *types.TriggerConfiguration) (*types.TriggerConfiguration, error)
	GetTriggerConfiguration(ctx context.Context, q *GetTriggerConfigurationQuery) (*types.TriggerConfiguration, error)
	UpdateTriggerConfiguration(ctx context.Context, triggerConfig *types.TriggerConfiguration) (*types.TriggerConfiguration, error)
	DeleteTriggerConfiguration(ctx context.Context, id string) error
	ListTriggerConfigurations(ctx context.Context, q *ListTriggerConfigurationsQuery) ([]*types.TriggerConfiguration, error)

	ListTriggerExecutions(ctx context.Context, q *ListTriggerExecutionsQuery) ([]*types.TriggerExecution, error)
	CreateTriggerExecution(ctx context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, error)
	UpdateTriggerExecution(ctx context.Context, execution *types.TriggerExecution) (*types.TriggerExecution, error)
	ResetRunningExecutions(ctx context.Context) error

	// system settings
	GetSystemSettings(ctx context.Context) (*types.SystemSettings, error)
	GetEffectiveSystemSettings(ctx context.Context) (*types.SystemSettings, error)
	UpdateSystemSettings(ctx context.Context, req *types.SystemSettingsRequest) (*types.SystemSettings, error)

	// model seeding
	SeedModelsFromEnvironment(ctx context.Context) error

	// spec-driven tasks
	CreateSpecTask(ctx context.Context, task *types.SpecTask) error
	GetSpecTask(ctx context.Context, id string) (*types.SpecTask, error)
	UpdateSpecTask(ctx context.Context, task *types.SpecTask) error
	ListSpecTasks(ctx context.Context, filters *types.SpecTaskFilters) ([]*types.SpecTask, error)

	// spec-driven task work sessions
	CreateSpecTaskWorkSession(ctx context.Context, workSession *types.SpecTaskWorkSession) error
	GetSpecTaskWorkSession(ctx context.Context, id string) (*types.SpecTaskWorkSession, error)
	UpdateSpecTaskWorkSession(ctx context.Context, workSession *types.SpecTaskWorkSession) error
	DeleteSpecTaskWorkSession(ctx context.Context, id string) error
	ListSpecTaskWorkSessions(ctx context.Context, specTaskID string) ([]*types.SpecTaskWorkSession, error)
	ListWorkSessionsBySpecTask(ctx context.Context, specTaskID string, phase *types.SpecTaskPhase) ([]*types.SpecTaskWorkSession, error)
	GetSpecTaskWorkSessionByHelixSession(ctx context.Context, helixSessionID string) (*types.SpecTaskWorkSession, error)

	// spec-driven task zed threads
	CreateSpecTaskZedThread(ctx context.Context, zedThread *types.SpecTaskZedThread) error
	GetSpecTaskZedThread(ctx context.Context, id string) (*types.SpecTaskZedThread, error)
	GetSpecTaskZedThreadByWorkSession(ctx context.Context, workSessionID string) (*types.SpecTaskZedThread, error)
	UpdateSpecTaskZedThread(ctx context.Context, zedThread *types.SpecTaskZedThread) error
	DeleteSpecTaskZedThread(ctx context.Context, id string) error
	ListSpecTaskZedThreads(ctx context.Context, specTaskID string) ([]*types.SpecTaskZedThread, error)

	// spec-driven task implementation tasks
	CreateSpecTaskImplementationTask(ctx context.Context, implTask *types.SpecTaskImplementationTask) error
	GetSpecTaskImplementationTask(ctx context.Context, id string) (*types.SpecTaskImplementationTask, error)
	UpdateSpecTaskImplementationTask(ctx context.Context, implTask *types.SpecTaskImplementationTask) error
	DeleteSpecTaskImplementationTask(ctx context.Context, id string) error
	ListSpecTaskImplementationTasks(ctx context.Context, specTaskID string) ([]*types.SpecTaskImplementationTask, error)
	ParseAndCreateImplementationTasks(ctx context.Context, specTaskID string, implementationPlan string) ([]*types.SpecTaskImplementationTask, error)

	// spec-driven task design reviews
	CreateSpecTaskDesignReview(ctx context.Context, review *types.SpecTaskDesignReview) error
	GetSpecTaskDesignReview(ctx context.Context, id string) (*types.SpecTaskDesignReview, error)
	UpdateSpecTaskDesignReview(ctx context.Context, review *types.SpecTaskDesignReview) error
	DeleteSpecTaskDesignReview(ctx context.Context, id string) error
	ListSpecTaskDesignReviews(ctx context.Context, specTaskID string) ([]types.SpecTaskDesignReview, error)
	GetLatestDesignReview(ctx context.Context, specTaskID string) (*types.SpecTaskDesignReview, error)

	// design review comments
	CreateSpecTaskDesignReviewComment(ctx context.Context, comment *types.SpecTaskDesignReviewComment) error
	GetSpecTaskDesignReviewComment(ctx context.Context, id string) (*types.SpecTaskDesignReviewComment, error)
	UpdateSpecTaskDesignReviewComment(ctx context.Context, comment *types.SpecTaskDesignReviewComment) error
	DeleteSpecTaskDesignReviewComment(ctx context.Context, id string) error
	ListSpecTaskDesignReviewComments(ctx context.Context, reviewID string) ([]types.SpecTaskDesignReviewComment, error)
	ListUnresolvedComments(ctx context.Context, reviewID string) ([]types.SpecTaskDesignReviewComment, error)
	GetCommentByInteractionID(ctx context.Context, interactionID string) (*types.SpecTaskDesignReviewComment, error)
	GetUnresolvedCommentsForTask(ctx context.Context, specTaskID string) ([]types.SpecTaskDesignReviewComment, error)

	// design review comment replies
	CreateSpecTaskDesignReviewCommentReply(ctx context.Context, reply *types.SpecTaskDesignReviewCommentReply) error
	GetSpecTaskDesignReviewCommentReply(ctx context.Context, id string) (*types.SpecTaskDesignReviewCommentReply, error)
	ListSpecTaskDesignReviewCommentReplies(ctx context.Context, commentID string) ([]types.SpecTaskDesignReviewCommentReply, error)

	// git push events
	CreateSpecTaskGitPushEvent(ctx context.Context, event *types.SpecTaskGitPushEvent) error
	GetSpecTaskGitPushEvent(ctx context.Context, id string) (*types.SpecTaskGitPushEvent, error)
	GetSpecTaskGitPushEventByCommit(ctx context.Context, specTaskID, commitHash string) (*types.SpecTaskGitPushEvent, error)
	UpdateSpecTaskGitPushEvent(ctx context.Context, event *types.SpecTaskGitPushEvent) error
	ListSpecTaskGitPushEvents(ctx context.Context, specTaskID string) ([]types.SpecTaskGitPushEvent, error)
	ListUnprocessedGitPushEvents(ctx context.Context) ([]types.SpecTaskGitPushEvent, error)

	// git repositories
	CreateGitRepository(ctx context.Context, repo *types.GitRepository) error
	GetGitRepository(ctx context.Context, id string) (*types.GitRepository, error)
	UpdateGitRepository(ctx context.Context, repo *types.GitRepository) error
	DeleteGitRepository(ctx context.Context, id string) error
	ListGitRepositories(ctx context.Context, request *types.ListGitRepositoriesRequest) ([]*types.GitRepository, error)

	// spec-driven task multi-session management
	CreateImplementationSessions(ctx context.Context, specTaskID string, config *types.SpecTaskImplementationSessionsCreateRequest) ([]*types.SpecTaskWorkSession, error)
	SpawnWorkSession(ctx context.Context, parentSessionID string, config *types.SpecTaskWorkSessionSpawnRequest) (*types.SpecTaskWorkSession, error)
	GetSpecTaskMultiSessionOverview(ctx context.Context, specTaskID string) (*types.SpecTaskMultiSessionOverviewResponse, error)
	GetSpecTaskProgress(ctx context.Context, specTaskID string) (*types.SpecTaskProgressResponse, error)
	UpdateSpecTaskZedInstance(ctx context.Context, specTaskID string, zedInstanceID string) error

	// SpecTask External Agent methods (per-SpecTask agents spanning multiple sessions)
	CreateSpecTaskExternalAgent(ctx context.Context, agent *types.SpecTaskExternalAgent) error
	GetSpecTaskExternalAgent(ctx context.Context, specTaskID string) (*types.SpecTaskExternalAgent, error)
	GetSpecTaskExternalAgentByID(ctx context.Context, agentID string) (*types.SpecTaskExternalAgent, error)
	UpdateSpecTaskExternalAgent(ctx context.Context, agent *types.SpecTaskExternalAgent) error
	DeleteSpecTaskExternalAgent(ctx context.Context, agentID string) error
	ListSpecTaskExternalAgents(ctx context.Context, userID string) ([]*types.SpecTaskExternalAgent, error)

	// External Agent Activity methods (idle detection)
	UpsertExternalAgentActivity(ctx context.Context, activity *types.ExternalAgentActivity) error
	GetExternalAgentActivity(ctx context.Context, agentID string) (*types.ExternalAgentActivity, error)
	GetExternalAgentActivityByLobbyID(ctx context.Context, lobbyID string) (*types.ExternalAgentActivity, error)
	GetIdleExternalAgents(ctx context.Context, cutoff time.Time, agentTypes []string) ([]*types.ExternalAgentActivity, error)
	DeleteExternalAgentActivity(ctx context.Context, agentID string) error

	// Agent session methods
	CreateAgentSession(ctx context.Context, session *types.AgentSession) error
	GetAgentSession(ctx context.Context, sessionID string) (*types.AgentSession, error)
	UpdateAgentSession(ctx context.Context, session *types.AgentSession) error
	ListAgentSessions(ctx context.Context, query *ListAgentSessionsQuery) (*types.AgentSessionsListResponse, error)

	// Agent work item methods
	CreateAgentWorkItem(ctx context.Context, workItem *types.AgentWorkItem) error
	GetAgentWorkItem(ctx context.Context, workItemID string) (*types.AgentWorkItem, error)
	UpdateAgentWorkItem(ctx context.Context, workItem *types.AgentWorkItem) error
	ListAgentWorkItems(ctx context.Context, query *ListAgentWorkItemsQuery) (*types.AgentWorkItemsListResponse, error)

	// Help request methods
	GetHelpRequestByID(ctx context.Context, requestID string) (*types.HelpRequest, error)
	UpdateHelpRequest(ctx context.Context, request *types.HelpRequest) error
	ListActiveHelpRequests(ctx context.Context) ([]*types.HelpRequest, error)

	// Agent dashboard helper methods
	GetSessionsNeedingHelp(ctx context.Context) ([]*types.AgentSession, error)
	GetRecentCompletions(ctx context.Context, limit int) ([]*types.JobCompletion, error)
	GetPendingReviews(ctx context.Context) ([]*types.JobCompletion, error)

	// Session management methods
	MarkSessionAsNeedingHelp(ctx context.Context, sessionID string, task string) error
	MarkSessionAsCompleted(ctx context.Context, sessionID string, completionType string) error
	CleanupExpiredSessions(ctx context.Context, timeout time.Duration) error

	// Help request methods (additional)
	CreateHelpRequest(ctx context.Context, request *types.HelpRequest) error

	// Job completion methods
	CreateJobCompletion(ctx context.Context, completion *types.JobCompletion) error

	// Agent session status methods (additional)
	GetAgentSessionStatus(ctx context.Context, sessionID string) (*types.AgentSessionStatus, error)
	CreateAgentSessionStatus(ctx context.Context, status *types.AgentSessionStatus) error
	UpdateAgentSessionStatus(ctx context.Context, status *types.AgentSessionStatus) error
	ListAgentSessionStatus(ctx context.Context, query *ListAgentSessionsQuery) (*types.AgentSessionsResponse, error)

	// Agent work queue stats
	GetAgentWorkQueueStats(ctx context.Context) (*types.AgentWorkQueueStats, error)

	// Additional help request methods
	ListHelpRequests(ctx context.Context, query *ListHelpRequestsQuery) (*types.HelpRequestsListResponse, error)

	// Additional session management methods
	MarkSessionAsActive(ctx context.Context, sessionID string, task string) error

	// Agent runner methods
	CreateAgentRunner(ctx context.Context, runnerID string) (*types.AgentRunner, error)
	GetAgentRunner(ctx context.Context, runnerID string) (*types.AgentRunner, error)
	UpdateAgentRunner(ctx context.Context, runner *types.AgentRunner) error
	UpdateAgentRunnerStatus(ctx context.Context, runnerID, status string) error
	UpdateAgentRunnerHeartbeat(ctx context.Context, runnerID string) error
	ListAgentRunners(ctx context.Context, query types.ListAgentRunnersQuery) ([]*types.AgentRunner, int64, error)
	DeleteAgentRunner(ctx context.Context, runnerID string) error
	CleanupStaleAgentRunners(ctx context.Context, staleThreshold time.Duration) (int64, error)
	GetOrCreateAgentRunner(ctx context.Context, runnerID string) (*types.AgentRunner, error)

	// Project methods
	CreateProject(ctx context.Context, project *types.Project) (*types.Project, error)
	GetProject(ctx context.Context, projectID string) (*types.Project, error)
	ListProjects(ctx context.Context, query *ListProjectsQuery) ([]*types.Project, error)
	UpdateProject(ctx context.Context, project *types.Project) error
	DeleteProject(ctx context.Context, projectID string) error
	SetProjectPrimaryRepository(ctx context.Context, projectID string, repoID string) error
	AttachRepositoryToProject(ctx context.Context, projectID string, repoID string) error
	DetachRepositoryFromProject(ctx context.Context, repoID string) error
	GetProjectExploratorySession(ctx context.Context, projectID string) (*types.Session, error)

	// Sample Project methods
	CreateSampleProject(ctx context.Context, sample *types.SampleProject) (*types.SampleProject, error)
	GetSampleProject(ctx context.Context, id string) (*types.SampleProject, error)
	ListSampleProjects(ctx context.Context) ([]*types.SampleProject, error)
	DeleteSampleProject(ctx context.Context, id string) error

	// Personal Dev Environment methods (DEPRECATED - these are stubs for backward compatibility)
	// TODO: Remove these after cleaning up wolf_executor.go
	CreatePersonalDevEnvironment(ctx context.Context, pde *types.PersonalDevEnvironment) (*types.PersonalDevEnvironment, error)
	GetPersonalDevEnvironment(ctx context.Context, id string) (*types.PersonalDevEnvironment, error)
	ListPersonalDevEnvironments(ctx context.Context, userID string) ([]*types.PersonalDevEnvironment, error)
	UpdatePersonalDevEnvironment(ctx context.Context, pde *types.PersonalDevEnvironment) (*types.PersonalDevEnvironment, error)
	DeletePersonalDevEnvironment(ctx context.Context, id string) error

	// SSH Key methods
	CreateSSHKey(ctx context.Context, key *types.SSHKey) (*types.SSHKey, error)
	GetSSHKey(ctx context.Context, id string) (*types.SSHKey, error)
	ListSSHKeys(ctx context.Context, userID string) ([]*types.SSHKey, error)
	UpdateSSHKeyLastUsed(ctx context.Context, id string) error
	DeleteSSHKey(ctx context.Context, id string) error

	// Zed Settings Override methods
	UpsertZedSettingsOverride(ctx context.Context, override *types.ZedSettingsOverride) error
	GetZedSettingsOverride(ctx context.Context, sessionID string) (*types.ZedSettingsOverride, error)
	DeleteZedSettingsOverride(ctx context.Context, sessionID string) error

	// Memory methods
	CreateMemory(ctx context.Context, memory *types.Memory) (*types.Memory, error)
	UpdateMemory(ctx context.Context, memory *types.Memory) (*types.Memory, error)
	DeleteMemory(ctx context.Context, memory *types.Memory) error
	ListMemories(ctx context.Context, q *types.ListMemoryRequest) ([]*types.Memory, error)

	// Question set methods
	CreateQuestionSet(ctx context.Context, questionSet *types.QuestionSet) (*types.QuestionSet, error)
	GetQuestionSet(ctx context.Context, id string) (*types.QuestionSet, error)
	UpdateQuestionSet(ctx context.Context, questionSet *types.QuestionSet) (*types.QuestionSet, error)
	ListQuestionSets(ctx context.Context, req *types.ListQuestionSetsRequest) ([]*types.QuestionSet, error)
	DeleteQuestionSet(ctx context.Context, id string) error

	CreateQuestionSetExecution(ctx context.Context, execution *types.QuestionSetExecution) (*types.QuestionSetExecution, error)
	GetQuestionSetExecution(ctx context.Context, id string) (*types.QuestionSetExecution, error)
	UpdateQuestionSetExecution(ctx context.Context, execution *types.QuestionSetExecution) (*types.QuestionSetExecution, error)
	ListQuestionSetExecutions(ctx context.Context, q *ListQuestionSetExecutionsQuery) ([]*types.QuestionSetExecution, error)

	// Wolf instance methods
	RegisterWolfInstance(ctx context.Context, instance *types.WolfInstance) error
	UpdateWolfHeartbeat(ctx context.Context, id string, req *types.WolfHeartbeatRequest) error
	GetWolfInstance(ctx context.Context, id string) (*types.WolfInstance, error)
	ListWolfInstances(ctx context.Context) ([]*types.WolfInstance, error)
	DeregisterWolfInstance(ctx context.Context, id string) error
	UpdateWolfStatus(ctx context.Context, id string, status string) error
	IncrementWolfSandboxCount(ctx context.Context, id string) error
	DecrementWolfSandboxCount(ctx context.Context, id string) error
	ResetWolfInstanceOnReconnect(ctx context.Context, id string) error
	GetWolfInstancesOlderThanHeartbeat(ctx context.Context, olderThan time.Time) ([]*types.WolfInstance, error)

	// Disk usage history methods
	CreateDiskUsageHistory(ctx context.Context, history *types.DiskUsageHistory) error
	GetDiskUsageHistory(ctx context.Context, wolfInstanceID string, since time.Time) ([]*types.DiskUsageHistory, error)
	DeleteOldDiskUsageHistory(ctx context.Context, olderThan time.Time) (int64, error)
}

type EmbeddingsStore interface {
	CreateKnowledgeEmbedding(ctx context.Context, embeddings ...*types.KnowledgeEmbeddingItem) error
	DeleteKnowledgeEmbedding(ctx context.Context, knowledgeID string) error
	QueryKnowledgeEmbeddings(ctx context.Context, q *types.KnowledgeEmbeddingQuery) ([]*types.KnowledgeEmbeddingItem, error)
}
