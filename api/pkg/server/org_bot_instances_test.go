package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/crypto"
	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/org/application/instances"
	helixorgstore "github.com/helixml/helix/api/pkg/org/domain/store"
	orgmemory "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
	helixorgserver "github.com/helixml/helix/api/pkg/org/interfaces/server"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

type BotInstancesDeleteSuite struct {
	suite.Suite
	ctrl      *gomock.Controller
	store     *store.MockStore
	executor  *external_agent.MockExecutor
	instances botInstances
}

func TestBotInstancesDeleteSuite(t *testing.T) { suite.Run(t, new(BotInstancesDeleteSuite)) }

func (s *BotInstancesDeleteSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.executor = external_agent.NewMockExecutor(s.ctrl)
	s.instances = botInstances{server: &HelixAPIServer{
		Store:                 s.store,
		externalAgentExecutor: s.executor,
		Cfg:                   &config.ServerConfig{},
	}}
}

func (s *BotInstancesDeleteSuite) TearDownTest() { s.ctrl.Finish() }

func instanceSession() *types.Session {
	return &types.Session{
		ID:             "ses_instance",
		Owner:          "usr_owner",
		OrganizationID: "org_one",
		SandboxID:      "sbx_host1",
		Metadata: types.SessionMetadata{
			OrgWorkerID: "b-broker",
			SessionRole: types.SessionRoleOrgBotInstance,
		},
	}
}

func callerCtx(userID string, role types.OrganizationRole) context.Context {
	ctx := runtimehelix.WithUserID(context.Background(), userID)
	return helixorgserver.WithOrgAuthorization(ctx, role, false)
}

// The owner's delete tears down in order: sandbox and its host data, then session.
func (s *BotInstancesDeleteSuite) TestOwnerDeletesSandboxAndSession() {
	s.store.EXPECT().GetSession(gomock.Any(), "ses_instance").Return(instanceSession(), nil)
	gomock.InOrder(
		s.executor.EXPECT().DestroyDesktop(gomock.Any(), "ses_instance", "").Return(nil),
		s.store.EXPECT().DeleteSession(gomock.Any(), "ses_instance").Return(instanceSession(), nil),
	)

	s.Require().NoError(s.instances.Delete(callerCtx("usr_owner", types.OrganizationRoleMember), "org_one", "b-broker", "ses_instance"))
}

func (s *BotInstancesDeleteSuite) TestOrgOwnerMayDeleteAnotherUsersInstance() {
	s.store.EXPECT().GetSession(gomock.Any(), "ses_instance").Return(instanceSession(), nil)
	s.executor.EXPECT().DestroyDesktop(gomock.Any(), "ses_instance", "").Return(nil)
	s.store.EXPECT().DeleteSession(gomock.Any(), "ses_instance").Return(instanceSession(), nil)

	s.Require().NoError(s.instances.Delete(callerCtx("usr_admin", types.OrganizationRoleOwner), "org_one", "b-broker", "ses_instance"))
}

func (s *BotInstancesDeleteSuite) TestMemberCannotDeleteAnotherUsersInstance() {
	s.store.EXPECT().GetSession(gomock.Any(), "ses_instance").Return(instanceSession(), nil)

	err := s.instances.Delete(callerCtx("usr_member", types.OrganizationRoleMember), "org_one", "b-broker", "ses_instance")
	s.Require().ErrorIs(err, instances.ErrForbidden)
}

// A session that isn't an instance of this bot (the bot's main session,
// another bot's instance, another org) must never be deleted through here.
func (s *BotInstancesDeleteSuite) TestOnlyThisBotsInstancesAreDeletable() {
	mainSession := instanceSession()
	mainSession.Metadata.SessionRole = "exploratory"
	otherBot := instanceSession()
	otherBot.Metadata.OrgWorkerID = "b-other"
	otherOrg := instanceSession()
	otherOrg.OrganizationID = "org_two"
	for _, session := range []*types.Session{mainSession, otherBot, otherOrg} {
		s.store.EXPECT().GetSession(gomock.Any(), "ses_instance").Return(session, nil)
		err := s.instances.Delete(callerCtx("usr_owner", types.OrganizationRoleOwner), "org_one", "b-broker", "ses_instance")
		s.Require().ErrorIs(err, helixorgstore.ErrNotFound)
	}
}

func (s *BotInstancesDeleteSuite) TestMissingSessionIsNotFound() {
	s.store.EXPECT().GetSession(gomock.Any(), "ses_gone").Return(nil, store.ErrNotFound)

	err := s.instances.Delete(callerCtx("usr_owner", types.OrganizationRoleOwner), "org_one", "b-broker", "ses_gone")
	s.Require().ErrorIs(err, helixorgstore.ErrNotFound)
}

// A sandbox that can't be destroyed keeps the session, so the delete can be
// retried rather than leaving orphaned host data behind.
func (s *BotInstancesDeleteSuite) TestDestroyFailureKeepsSession() {
	s.store.EXPECT().GetSession(gomock.Any(), "ses_instance").Return(instanceSession(), nil)
	s.executor.EXPECT().DestroyDesktop(gomock.Any(), "ses_instance", "").Return(errors.New("sandbox offline"))

	err := s.instances.Delete(callerCtx("usr_owner", types.OrganizationRoleMember), "org_one", "b-broker", "ses_instance")
	s.Require().ErrorContains(err, "sandbox offline")
}

// A profile change reaches every instance through a targeted write, and one
// failing instance neither stops the others nor is swallowed.
func TestBotInstancesSyncProfileAttemptsEveryInstance(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	orgStore := orgmemory.New()
	bot := mustBot(t, "b-broker", time.Now()).WithAgentID("app_broker")
	require.NoError(t, orgStore.Nodes.Create(context.Background(), bot))
	instances := botInstances{server: &HelixAPIServer{Store: st}, store: orgStore}

	first := &types.Session{ID: "ses_one", Metadata: types.SessionMetadata{OrgWorkerID: "b-broker"}}
	second := &types.Session{ID: "ses_two", Metadata: types.SessionMetadata{OrgWorkerID: "b-broker"}}
	st.EXPECT().ListSessions(gomock.Any(), gomock.Any()).Return([]*types.Session{first, second}, int64(2), nil)
	profile := bot.EffectiveInstanceProfile()
	st.EXPECT().SetSessionBotInstanceProfile(gomock.Any(), "ses_one", profile).Return(errors.New("db down"))
	st.EXPECT().SetSessionBotInstanceProfile(gomock.Any(), "ses_two", profile).Return(nil)

	err := instances.SyncProfile(context.Background(), "org-test", "b-broker")
	require.ErrorContains(t, err, "instance ses_one: db down")
}

// Deleting a bot must not stall on an unreachable instance host: the instance
// row still goes, and the orphan reaper removes its host data.
func TestBotInstancesDeleteAllContinuesWhenDestroyFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	executor := external_agent.NewMockExecutor(ctrl)
	orgStore := orgmemory.New()
	bot := mustBot(t, "b-broker", time.Now()).WithAgentID("app_broker")
	require.NoError(t, orgStore.Nodes.Create(context.Background(), bot))
	instances := botInstances{server: &HelixAPIServer{Store: st, externalAgentExecutor: executor}, store: orgStore}

	first := &types.Session{ID: "ses_one", Metadata: types.SessionMetadata{OrgWorkerID: "b-broker"}}
	second := &types.Session{ID: "ses_two", Metadata: types.SessionMetadata{OrgWorkerID: "b-broker"}}
	st.EXPECT().ListSessions(gomock.Any(), gomock.Any()).Return([]*types.Session{first, second}, int64(2), nil)
	executor.EXPECT().DestroyDesktop(gomock.Any(), "ses_one", "").Return(errors.New("sandbox offline"))
	executor.EXPECT().DestroyDesktop(gomock.Any(), "ses_two", "").Return(nil)
	st.EXPECT().DeleteSession(gomock.Any(), "ses_one").Return(first, nil)
	st.EXPECT().DeleteSession(gomock.Any(), "ses_two").Return(second, nil)

	require.NoError(t, instances.DeleteAll(context.Background(), bot.OrganizationID, "b-broker"))
}

func TestBotInstanceCredentialsRejectSubscriptions(t *testing.T) {
	require.NoError(t, validateBotInstanceCredentials(&types.AssistantConfig{CodeAgentCredentialType: types.CodeAgentCredentialTypeAPIKey}))
	err := validateBotInstanceCredentials(&types.AssistantConfig{CodeAgentCredentialType: types.CodeAgentCredentialTypeSubscription})
	require.ErrorIs(t, err, instances.ErrInvalidRequest)
	require.ErrorContains(t, err, "do not support subscription credentials")
}

func TestNormalizeBotInstanceDiskSize(t *testing.T) {
	got, err := normalizeBotInstanceDiskSize(0)
	require.NoError(t, err)
	require.Equal(t, types.DefaultBotInstanceDiskSizeGB, got)
	got, err = normalizeBotInstanceDiskSize(24)
	require.NoError(t, err)
	require.Equal(t, 24, got)
	got, err = normalizeBotInstanceDiskSize(types.MaxBotInstanceDiskSizeGB)
	require.NoError(t, err)
	require.Equal(t, types.MaxBotInstanceDiskSizeGB, got)
	for _, invalid := range []int{-1, types.MaxBotInstanceDiskSizeGB + 1} {
		_, err := normalizeBotInstanceDiskSize(invalid)
		require.ErrorIs(t, err, instances.ErrInvalidRequest)
	}
}

func TestValidateInstanceSecrets(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	manager := botInstances{server: &HelixAPIServer{Store: st}}
	available := []*types.Secret{
		{Name: "DEV", Scope: types.SecretScopeDev},
		{Name: "LEGACY"},
		{Name: "BOTH", Scope: types.SecretScopeBoth},
		{Name: "PROD", Scope: types.SecretScopeProd},
	}

	st.EXPECT().ListProjectSecrets(gomock.Any(), "prj_one").Return(available, nil).Times(4)
	selected, err := manager.validateInstanceSecrets(context.Background(), "prj_one", []string{"BOTH", "DEV", "LEGACY"})
	require.NoError(t, err)
	require.Equal(t, []string{"BOTH", "DEV", "LEGACY"}, selected)

	_, err = manager.validateInstanceSecrets(context.Background(), "prj_one", []string{"PROD"})
	require.ErrorIs(t, err, instances.ErrInvalidRequest)
	_, err = manager.validateInstanceSecrets(context.Background(), "prj_one", []string{"MISSING"})
	require.ErrorIs(t, err, instances.ErrInvalidRequest)
	_, err = manager.validateInstanceSecrets(context.Background(), "prj_one", []string{"DEV", "DEV"})
	require.ErrorIs(t, err, instances.ErrInvalidRequest)

	selected, err = manager.validateInstanceSecrets(context.Background(), "prj_one", nil)
	require.NoError(t, err)
	require.NotNil(t, selected)
	require.Empty(t, selected)
}

func TestGetProjectSecretsAsEnvVarsByNameDecryptsOnlySelected(t *testing.T) {
	t.Setenv("HELIX_ENCRYPTION_KEY", "bot-instance-secret-selection-test")
	key, err := crypto.GetEncryptionKey()
	require.NoError(t, err)
	encrypted, err := crypto.EncryptAES256GCM([]byte("selected-value"), key)
	require.NoError(t, err)

	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: st}
	st.EXPECT().ListProjectSecrets(gomock.Any(), "prj_one").Return([]*types.Secret{
		{Name: "SELECTED", Scope: types.SecretScopeDev, Value: []byte(encrypted)},
		// Invalid ciphertext proves an unselected secret is skipped before
		// decryption rather than loaded and discarded afterwards.
		{Name: "UNSELECTED", Scope: types.SecretScopeDev, Value: []byte("not-ciphertext")},
	}, nil)

	env, err := server.GetProjectSecretsAsEnvVarsByName(context.Background(), "prj_one", types.SecretScopeDev, []string{"SELECTED"})
	require.NoError(t, err)
	require.Equal(t, []string{"SELECTED=selected-value"}, env)
}

func TestGetProjectSecretsAsEnvVarsByNameEmptySkipsStore(t *testing.T) {
	ctrl := gomock.NewController(t)
	server := &HelixAPIServer{Store: store.NewMockStore(ctrl)}
	env, err := server.GetProjectSecretsAsEnvVarsByName(context.Background(), "prj_one", types.SecretScopeDev, []string{})
	require.NoError(t, err)
	require.NotNil(t, env)
	require.Empty(t, env)
}
