package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/orgstore"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newSandboxTestStore(t *testing.T) *PostgresStore {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.Sandbox{}, &types.SandboxRun{}, &types.ProjectWebServiceState{}))

	return &PostgresStore{gdb: db, Store: orgstore.New(db)}
}

func TestSetActiveWebServiceSandboxRequiresEligibleSandbox(t *testing.T) {
	tests := []struct {
		name      string
		projectID string
		purpose   string
		deleted   bool
		wantErr   bool
	}{
		{name: "web service", projectID: "prj_1", purpose: types.SandboxPurposeWebService},
		{name: "deleted", projectID: "prj_1", purpose: types.SandboxPurposeWebService, deleted: true, wantErr: true},
		{name: "wrong project", projectID: "prj_2", purpose: types.SandboxPurposeWebService, wantErr: true},
		{name: "ordinary sandbox", projectID: "prj_1", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			store := newSandboxTestStore(t)
			require.NoError(t, store.UpsertProjectWebServiceState(ctx, &types.ProjectWebServiceState{
				ProjectID:       "prj_1",
				ActiveSandboxID: "sbx_current",
			}))

			sb, err := store.CreateSandbox(ctx, &types.Sandbox{
				ID:             "sbx_candidate",
				OrganizationID: "org_1",
				ProjectID:      tt.projectID,
				Owner:          "user_1",
				Runtime:        types.SandboxRuntimeHeadlessUbuntu,
				Purpose:        tt.purpose,
			})
			require.NoError(t, err)
			if tt.deleted {
				require.NoError(t, store.DeleteSandbox(ctx, sb.ID))
			}

			err = store.SetActiveWebServiceSandbox(ctx, "prj_1", sb.ID)
			if tt.wantErr {
				require.ErrorIs(t, err, ErrNotFound)
			} else {
				require.NoError(t, err)
			}

			state, err := store.GetProjectWebServiceState(ctx, "prj_1")
			require.NoError(t, err)
			if tt.wantErr {
				require.Equal(t, "sbx_current", state.ActiveSandboxID)
			} else {
				require.Equal(t, sb.ID, state.ActiveSandboxID)
			}
		})
	}
}

func TestSetSandboxContainerIgnoresDeletedRows(t *testing.T) {
	ctx := context.Background()
	store := newSandboxTestStore(t)

	sb, err := store.CreateSandbox(ctx, &types.Sandbox{
		ID:             "sbx_deleted_container",
		OrganizationID: "org_1",
		Owner:          "user_1",
		Runtime:        types.SandboxRuntimeHeadlessUbuntu,
		Status:         types.SandboxStatusPending,
	})
	require.NoError(t, err)
	require.NoError(t, store.DeleteSandbox(ctx, sb.ID))

	err = store.SetSandboxContainer(ctx, sb.ID, "host-a", "ctr-a")
	require.ErrorIs(t, err, ErrNotFound)

	var persisted types.Sandbox
	require.NoError(t, store.gdb.WithContext(ctx).Unscoped().Where("id = ?", sb.ID).First(&persisted).Error)
	require.Empty(t, persisted.HostDeviceID)
	require.Empty(t, persisted.ContainerID)
}

func TestSetSandboxStatusIgnoresDeletedRows(t *testing.T) {
	ctx := context.Background()
	store := newSandboxTestStore(t)

	sb, err := store.CreateSandbox(ctx, &types.Sandbox{
		ID:             "sbx_deleted_status",
		OrganizationID: "org_1",
		Owner:          "user_1",
		Runtime:        types.SandboxRuntimeHeadlessUbuntu,
		Status:         types.SandboxStatusPending,
	})
	require.NoError(t, err)
	require.NoError(t, store.DeleteSandbox(ctx, sb.ID))

	err = store.SetSandboxStatus(ctx, sb.ID, types.SandboxStatusRunning, "")
	require.ErrorIs(t, err, ErrNotFound)

	var persisted types.Sandbox
	require.NoError(t, store.gdb.WithContext(ctx).Unscoped().Where("id = ?", sb.ID).First(&persisted).Error)
	require.Equal(t, types.SandboxStatusStopped, persisted.Status)
	require.Nil(t, persisted.StartedAt)
}

// Regression: the column behind types.Sandbox.VCPUs is `v_cpus`, not `vcpus` —
// GORM's naming strategy splits the leading acronym. A raw-map update with the
// wrong key fails at the database, which mock-store unit tests cannot see.
func TestSetSandboxResourcesPersistsNewAllocation(t *testing.T) {
	ctx := context.Background()
	store := newSandboxTestStore(t)

	sb, err := store.CreateSandbox(ctx, &types.Sandbox{
		ID:             "sbx_resize",
		OrganizationID: "org_1",
		Owner:          "user_1",
		Runtime:        types.SandboxRuntimeUbuntuDesktop,
		Status:         types.SandboxStatusRunning,
		VCPUs:          1,
		MemoryMB:       2048,
	})
	require.NoError(t, err)

	require.NoError(t, store.SetSandboxResources(ctx, sb.ID, 8, 16384))

	updated, err := store.GetSandbox(ctx, sb.ID)
	require.NoError(t, err)
	require.Equal(t, 8, updated.VCPUs)
	require.Equal(t, 16384, updated.MemoryMB)
}

func TestSetSandboxResourcesRejectsNonPositiveValues(t *testing.T) {
	ctx := context.Background()
	store := newSandboxTestStore(t)

	_, err := store.CreateSandbox(ctx, &types.Sandbox{
		ID:             "sbx_resize_bad",
		OrganizationID: "org_1",
		Owner:          "user_1",
		Runtime:        types.SandboxRuntimeUbuntuDesktop,
		Status:         types.SandboxStatusRunning,
	})
	require.NoError(t, err)

	require.Error(t, store.SetSandboxResources(ctx, "sbx_resize_bad", 0, 8192))
	require.ErrorIs(t, store.SetSandboxResources(ctx, "sbx_missing", 4, 8192), ErrNotFound)
}

func TestGetSandboxBySessionSkipsDeletedRows(t *testing.T) {
	ctx := context.Background()
	store := newSandboxTestStore(t)

	sb, err := store.CreateSandbox(ctx, &types.Sandbox{
		ID:             "sbx_session",
		OrganizationID: "org_1",
		Owner:          "user_1",
		SessionID:      "ses_1",
		SpecTaskID:     "spt_1",
		Runtime:        types.SandboxRuntimeUbuntuDesktop,
		Status:         types.SandboxStatusRunning,
	})
	require.NoError(t, err)

	found, err := store.GetSandboxBySession(ctx, "ses_1")
	require.NoError(t, err)
	require.Equal(t, sb.ID, found.ID)
	require.True(t, found.SessionBacked())
	require.Equal(t, "ses_1", found.HydraOpsID())

	// After teardown the session must look unmetered again, so a resumed task
	// opens a fresh row rather than reusing a deleted one.
	require.NoError(t, store.DeleteSandbox(ctx, sb.ID))
	_, err = store.GetSandboxBySession(ctx, "ses_1")
	require.ErrorIs(t, err, ErrNotFound)
}

// Session-backed rows are created with a negative TTL so expires_at stays NULL
// and the TTL reaper never tears down a desktop the task still owns.
func TestNegativeTimeoutLeavesSandboxOutOfTheExpiryReaper(t *testing.T) {
	ctx := context.Background()
	store := newSandboxTestStore(t)

	sb, err := store.CreateSandbox(ctx, &types.Sandbox{
		ID:             "sbx_never_expires",
		OrganizationID: "org_1",
		Owner:          "user_1",
		SessionID:      "ses_1",
		Runtime:        types.SandboxRuntimeUbuntuDesktop,
		Status:         types.SandboxStatusRunning,
		TimeoutSeconds: -1,
	})
	require.NoError(t, err)
	require.Nil(t, sb.ExpiresAt)

	expired, err := store.ListExpiredSandboxes(ctx, time.Now().Add(365*24*time.Hour))
	require.NoError(t, err)
	require.Empty(t, expired)
}

func TestListSandboxesVisibility(t *testing.T) {
	ctx := context.Background()
	st := newSandboxTestStore(t)
	rows := []*types.Sandbox{
		{ID: "sbx_plain", Owner: "usr_other"},
		{ID: "sbx_in_v", ProjectID: "prj_v", Owner: "usr_other"},
		{ID: "sbx_in_m", ProjectID: "prj_m", Owner: "usr_other"},
		{ID: "sbx_desk_v", ProjectID: "prj_v", SessionID: "ses_v", Owner: "usr_other"},
		{ID: "sbx_desk_mine", ProjectID: "prj_v", SessionID: "ses_mine", Owner: "usr_me"},
		{ID: "sbx_chat_other", SessionID: "ses_chat", Owner: "usr_other"},
		{ID: "sbx_other_org", OrganizationID: "org_2", Owner: "usr_me"},
	}
	for _, row := range rows {
		if row.OrganizationID == "" {
			row.OrganizationID = "org_1"
		}
		row.Runtime = types.SandboxRuntimeHeadlessUbuntu
		_, err := st.CreateSandbox(ctx, row)
		require.NoError(t, err)
	}
	ids := func(v *SandboxVisibility) []string {
		got, err := st.ListSandboxes(ctx, &ListSandboxesQuery{OrganizationID: "org_1", Visibility: v})
		require.NoError(t, err)
		var out []string
		for _, sb := range got {
			out = append(out, sb.ID)
		}
		return out
	}

	require.ElementsMatch(t, []string{"sbx_plain", "sbx_in_v", "sbx_in_m", "sbx_desk_v", "sbx_desk_mine", "sbx_chat_other"}, ids(nil), "no visibility filter")
	require.ElementsMatch(t, []string{"sbx_plain", "sbx_in_m", "sbx_desk_mine"},
		ids(&SandboxVisibility{ProjectIDs: []string{"prj_m"}, SessionOwner: "usr_me", IncludeUnscoped: true}), "member of project M only")
	require.ElementsMatch(t, []string{"sbx_in_m"}, ids(&SandboxVisibility{ProjectIDs: []string{"prj_m"}}), "key scoped to project M")
	require.Empty(t, ids(&SandboxVisibility{}), "empty visibility matches nothing")
}

func sandboxRuns(t *testing.T, store *PostgresStore, sandboxID string) []types.SandboxRun {
	t.Helper()
	var runs []types.SandboxRun
	require.NoError(t, store.gdb.Where("sandbox_id = ?", sandboxID).Order("started_at ASC, id ASC").Find(&runs).Error)
	return runs
}

// The run ledger is the only record of how long a session-backed sandbox ran
// across restarts: the row itself is reopened and forgets earlier runs.
func TestSandboxRunLedgerFollowsLifecycle(t *testing.T) {
	ctx := context.Background()
	store := newSandboxTestStore(t)

	sb, err := store.CreateSandbox(ctx, &types.Sandbox{
		ID:             "sbx_runs",
		OrganizationID: "org_1",
		Owner:          "user_1",
		Runtime:        types.SandboxRuntimeUbuntuDesktop,
		VCPUs:          2,
	})
	require.NoError(t, err)
	require.Empty(t, sandboxRuns(t, store, sb.ID), "a pending sandbox has not run")

	require.NoError(t, store.SetSandboxStatus(ctx, sb.ID, types.SandboxStatusRunning, ""))
	require.NoError(t, store.SetSandboxStatus(ctx, sb.ID, types.SandboxStatusRunning, ""))
	runs := sandboxRuns(t, store, sb.ID)
	require.Len(t, runs, 1, "a duplicate running write must not open a second run")
	require.Nil(t, runs[0].StoppedAt)
	require.Equal(t, 2, runs[0].VCPUs)
	require.Equal(t, "org_1", runs[0].OrganizationID)
	require.Equal(t, types.SandboxRuntimeUbuntuDesktop, runs[0].Runtime)

	require.NoError(t, store.SetSandboxResources(ctx, sb.ID, 8, 16384))
	runs = sandboxRuns(t, store, sb.ID)
	require.Len(t, runs, 2, "a resize closes the run at the old size and opens one at the new size")
	require.NotNil(t, runs[0].StoppedAt)
	require.Nil(t, runs[1].StoppedAt)
	require.Equal(t, 8, runs[1].VCPUs)

	require.NoError(t, store.SetSandboxStatus(ctx, sb.ID, types.SandboxStatusStopped, ""))
	for _, run := range sandboxRuns(t, store, sb.ID) {
		require.NotNil(t, run.StoppedAt, "stopping closes every run")
	}

	// A restart reopens the same row as pending, then marks it running.
	reopened, err := store.GetSandbox(ctx, sb.ID)
	require.NoError(t, err)
	reopened.Status = types.SandboxStatusPending
	reopened.StartedAt = nil
	reopened.StoppedAt = nil
	_, err = store.UpdateSandbox(ctx, reopened)
	require.NoError(t, err)
	require.Len(t, sandboxRuns(t, store, sb.ID), 2)
	require.NoError(t, store.SetSandboxStatus(ctx, sb.ID, types.SandboxStatusRunning, ""))
	runs = sandboxRuns(t, store, sb.ID)
	require.Len(t, runs, 3, "the restart is a new run; earlier runs are kept")
	require.Nil(t, runs[2].StoppedAt)

	require.NoError(t, store.DeleteSandbox(ctx, sb.ID))
	for _, run := range sandboxRuns(t, store, sb.ID) {
		require.NotNil(t, run.StoppedAt, "delete closes the open run")
	}
}

// A row still marked running when it is reopened (the stop never reached the
// API) must not leave its run open across the gap.
func TestSandboxRunLedgerClosesRunOnReopenOfRunningRow(t *testing.T) {
	ctx := context.Background()
	store := newSandboxTestStore(t)

	sb, err := store.CreateSandbox(ctx, &types.Sandbox{
		ID:             "sbx_runs_reopen",
		OrganizationID: "org_1",
		Owner:          "user_1",
		Runtime:        types.SandboxRuntimeHeadlessUbuntu,
	})
	require.NoError(t, err)
	require.NoError(t, store.SetSandboxStatus(ctx, sb.ID, types.SandboxStatusRunning, ""))

	reopened, err := store.GetSandbox(ctx, sb.ID)
	require.NoError(t, err)
	reopened.Status = types.SandboxStatusPending
	_, err = store.UpdateSandbox(ctx, reopened)
	require.NoError(t, err)

	runs := sandboxRuns(t, store, sb.ID)
	require.Len(t, runs, 1)
	require.NotNil(t, runs[0].StoppedAt)
}
