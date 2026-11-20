package external_agent

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

type fakeBotHomes struct {
	records map[string]BotHomeRecord
	saves   []BotHomeRecord
	live    []string
	listErr error
	saveErr error
}

func (f *fakeBotHomes) LoadBotHome(_ context.Context, orgID, botID string) (BotHomeRecord, error) {
	return f.records[hydra.BotHomeKey(orgID, botID)], nil
}

func (f *fakeBotHomes) SaveBotHome(_ context.Context, orgID, botID string, rec BotHomeRecord) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saves = append(f.saves, rec)
	cur := f.records[hydra.BotHomeKey(orgID, botID)]
	rec.SessionID = cur.SessionID
	f.records[hydra.BotHomeKey(orgID, botID)] = rec
	return nil
}

func (f *fakeBotHomes) ListLiveBotHomes(context.Context) ([]string, error) {
	return f.live, f.listErr
}

func botSession(id string, mutate func(*types.Session)) *types.Session {
	s := &types.Session{ID: id, OrganizationID: "org_a"}
	s.Metadata.OrgWorkerID = "b-bird"
	s.Metadata.SessionRole = "exploratory"
	if mutate != nil {
		mutate(s)
	}
	return s
}

func newBotHomeExecutor(t *testing.T, session *types.Session, homes *fakeBotHomes) *HydraExecutor {
	t.Helper()
	ms := store.NewMockStore(gomock.NewController(t))
	if session != nil {
		ms.EXPECT().GetSession(gomock.Any(), session.ID).Return(session, nil).AnyTimes()
	}
	h := newTestExecutor(ms)
	h.workspaceBasePathForCloning = "/data/workspaces"
	h.botHomes = homes
	return h
}

func TestResolveBotHome_OnlyForBotMainSession(t *testing.T) {
	cases := map[string]func(*types.Session){
		"bot instance":    func(s *types.Session) { s.Metadata.BotInstance = &types.BotInstanceProfile{} },
		"non-exploratory": func(s *types.Session) { s.Metadata.SessionRole = "planning" },
		"another session": nil,
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			homes := &fakeBotHomes{records: map[string]BotHomeRecord{}}
			if name == "another session" {
				homes.records["org_a/b-bird"] = BotHomeRecord{State: BotHomeEstablished, Generation: "bh_1", SandboxID: "sb1", SessionID: "ses_main"}
			}
			h := newBotHomeExecutor(t, botSession("ses_fork", mutate), homes)
			b, err := h.resolveBotHome(context.Background(), &types.DesktopAgent{SessionID: "ses_fork", OrgWorkerID: "b-bird"})
			require.NoError(t, err)
			require.Nil(t, b)
		})
	}
	t.Run("not an org worker", func(t *testing.T) {
		h := newBotHomeExecutor(t, nil, &fakeBotHomes{records: map[string]BotHomeRecord{}})
		b, err := h.resolveBotHome(context.Background(), &types.DesktopAgent{SessionID: "ses_x"})
		require.NoError(t, err)
		require.Nil(t, b)
	})
}

func TestResolveBotHome_FirstStartReservesBeforeHydra(t *testing.T) {
	homes := &fakeBotHomes{records: map[string]BotHomeRecord{}}
	h := newBotHomeExecutor(t, botSession("ses_main", nil), homes)
	agent := &types.DesktopAgent{SessionID: "ses_main", OrgWorkerID: "b-bird"}
	b, err := h.resolveBotHome(context.Background(), agent)
	require.NoError(t, err)
	require.Equal(t, hydra.BotHomeExpectNew, b.spec.Expect)
	require.True(t, strings.HasPrefix(b.spec.Generation, "bh_"))
	require.Empty(t, agent.SandboxID, "a first start is placed normally")
	require.Equal(t, filepath.Join("/data/workspaces", "org-bots", "org_a", "b-bird", "home"), b.workspaceDir(h.workspaceBasePathForCloning))

	require.NoError(t, b.reserve(context.Background(), homes, "sb1"))
	require.Equal(t, []BotHomeRecord{{State: BotHomePending, Generation: b.spec.Generation, SandboxID: "sb1"}}, homes.saves)

	// A retry after a crash reuses the pending generation and runner.
	agent2 := &types.DesktopAgent{SessionID: "ses_main", OrgWorkerID: "b-bird"}
	b2, err := h.resolveBotHome(context.Background(), agent2)
	require.NoError(t, err)
	require.Equal(t, b.spec.Generation, b2.spec.Generation)
	require.Equal(t, hydra.BotHomeExpectNew, b2.spec.Expect)
	require.Equal(t, "sb1", agent2.SandboxID)

	b2.establish(context.Background(), homes, &hydra.BotHomeResult{Generation: b.spec.Generation})
	require.Equal(t, BotHomeEstablished, homes.records["org_a/b-bird"].State)
}

func TestResolveBotHome_EstablishedHomePinsRunnerAndNeverSeeds(t *testing.T) {
	homes := &fakeBotHomes{records: map[string]BotHomeRecord{
		"org_a/b-bird": {State: BotHomeEstablished, Generation: "bh_1", SandboxID: "sb1", SessionID: "ses_new"},
	}}
	h := newBotHomeExecutor(t, botSession("ses_new", nil), homes)

	agent := &types.DesktopAgent{SessionID: "ses_new", OrgWorkerID: "b-bird"}
	b, err := h.resolveBotHome(context.Background(), agent)
	require.NoError(t, err)
	require.Equal(t, hydra.BotHomeSpec{OrganizationID: "org_a", BotID: "b-bird", Generation: "bh_1", Expect: hydra.BotHomeExpectExisting}, b.spec)
	require.Equal(t, "sb1", agent.SandboxID)

	// Placement fell back to another runner (sb1's row vanished): refuse.
	require.ErrorContains(t, b.reserve(context.Background(), homes, "sb2"), "runner loss recovery is not supported")
	require.Empty(t, homes.saves)

	_, err = h.resolveBotHome(context.Background(), &types.DesktopAgent{SessionID: "ses_new", OrgWorkerID: "b-bird", SandboxID: "sb2"})
	require.ErrorContains(t, err, "runner loss recovery is not supported")
}

func TestResolveBotHome_RefusesIncompleteErase(t *testing.T) {
	for _, state := range []BotHomeState{BotHomeErasing, BotHomeEraseFailed} {
		homes := &fakeBotHomes{records: map[string]BotHomeRecord{
			"org_a/b-bird": {State: state, Generation: "bh_1", SandboxID: "sb1", Detail: "purge failed"},
		}}
		h := newBotHomeExecutor(t, botSession("ses_main", nil), homes)
		_, err := h.resolveBotHome(context.Background(), &types.DesktopAgent{SessionID: "ses_main", OrgWorkerID: "b-bird"})
		require.ErrorContains(t, err, "home reset did not complete")
	}
}

func TestEraseBotHome_FailurePersistsThatDataMayRemain(t *testing.T) {
	homes := &fakeBotHomes{records: map[string]BotHomeRecord{
		"org_a/b-bird": {State: BotHomeEstablished, Generation: "bh_1", SandboxID: "sb1", SessionID: "ses_main"},
	}}
	h := newBotHomeExecutor(t, nil, homes)
	h.connman = failingConnman{err: errors.New("runner offline")}

	_, err := h.EraseBotHome(context.Background(), "org_a", "b-bird", hydra.BotHomeRetire)
	require.ErrorContains(t, err, "runner offline")
	rec := homes.records["org_a/b-bird"]
	require.Equal(t, BotHomeEraseFailed, rec.State)
	require.Equal(t, "bh_1", rec.Generation)
	require.Equal(t, "sb1", rec.SandboxID)
	require.Contains(t, rec.Detail, "may still exist")
	require.Equal(t, BotHomeErasing, homes.saves[0].State, "erasing is recorded before hydra is called")

	// Nothing to erase is a no-op without touching hydra.
	homes.records["org_a/b-other"] = BotHomeRecord{}
	resp, err := h.EraseBotHome(context.Background(), "org_a", "b-other", hydra.BotHomeRetire)
	require.NoError(t, err)
	require.False(t, resp.Existed)
}

func TestWithLiveBotHomes(t *testing.T) {
	homes := &fakeBotHomes{records: map[string]BotHomeRecord{}, live: []string{"org_a/b-bird"}}
	h := newBotHomeExecutor(t, nil, homes)
	req := &hydra.GCReconcileRequest{LiveSessionIDs: []string{"ses_1"}}
	out := h.withLiveBotHomes(context.Background(), req)
	require.True(t, out.BotHomesAuthoritative)
	require.Equal(t, []string{"org_a/b-bird"}, out.LiveBotHomes)
	require.False(t, req.BotHomesAuthoritative, "request is not mutated")

	homes.listErr = errors.New("db down")
	out = h.withLiveBotHomes(context.Background(), req)
	require.False(t, out.BotHomesAuthoritative, "an unreadable live set must not reap homes")
}
