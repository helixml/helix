package server

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	dto "github.com/prometheus/client_model/go"
	"go.uber.org/mock/gomock"
)

func TestSandboxOOMMetricsResolveSessionAndCountDeltas(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: st}
	report := types.ContainerOOMKills{ContainerID: "container-test-session", SessionID: "ses_oom_test", Count: 2}
	labels := []string{"runner-test", "runner.test", "sbx_oom_test", "ses_oom_test", "container-test-session", "prj_oom_test", "OOM project", "org_oom_test", "OOM org"}
	now := time.Now()

	st.EXPECT().GetSandboxInstance(gomock.Any(), "runner-test").Return(&types.SandboxInstance{Hostname: "runner.test"}, nil)
	st.EXPECT().GetSession(gomock.Any(), "ses_oom_test").Return(&types.Session{
		ID: "ses_oom_test", ProjectID: "prj_oom_test", OrganizationID: "org_oom_test",
	}, nil)
	st.EXPECT().GetSandboxBySession(gomock.Any(), "ses_oom_test").Return(&types.Sandbox{ID: "sbx_oom_test"}, nil)
	st.EXPECT().GetProject(gomock.Any(), "prj_oom_test").Return(&types.Project{
		ID: "prj_oom_test", Name: "OOM project", OrganizationID: "org_oom_test",
	}, nil)
	st.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{ID: "org_oom_test"}).Return(&types.Organization{
		ID: "org_oom_test", Name: "OOM org",
	}, nil)

	server.updateSandboxOOMMetrics(context.Background(), "runner-test", []types.ContainerOOMKills{report}, now)
	report.Count = 5
	server.updateSandboxOOMMetrics(context.Background(), "runner-test", []types.ContainerOOMKills{report}, now.Add(time.Minute))
	report.Count = 1
	server.updateSandboxOOMMetrics(context.Background(), "runner-test", []types.ContainerOOMKills{report}, now.Add(2*time.Minute))

	var metric dto.Metric
	if err := sandboxOOMKills.WithLabelValues(labels...).Write(&metric); err != nil {
		t.Fatal(err)
	}
	if got := metric.GetCounter().GetValue(); got != 6 {
		t.Fatalf("counter = %v, want 6", got)
	}

	server.updateSandboxOOMMetrics(context.Background(), "runner-test", nil, now.Add(30*time.Minute))
	sandboxOOMState.Lock()
	_, retained := sandboxOOMState.containers["runner-test\x00container-test-session"]
	sandboxOOMState.Unlock()
	if !retained {
		t.Fatal("metric removed before alert window elapsed")
	}
	server.updateSandboxOOMMetrics(context.Background(), "runner-test", nil, now.Add(2*time.Hour))
	sandboxOOMState.Lock()
	_, retained = sandboxOOMState.containers["runner-test\x00container-test-session"]
	sandboxOOMState.Unlock()
	if retained {
		t.Fatal("metric retained beyond cleanup window")
	}
}

func TestSandboxOOMLabelsResolveSandboxID(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: st}
	st.EXPECT().GetSandbox(gomock.Any(), "sbx_direct_test").Return(&types.Sandbox{
		ID: "sbx_direct_test", ProjectID: "prj_direct_test", OrganizationID: "org_direct_test",
	}, nil)
	st.EXPECT().GetProject(gomock.Any(), "prj_direct_test").Return(&types.Project{
		ID: "prj_direct_test", Name: "Direct project", OrganizationID: "org_direct_test",
	}, nil)
	st.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{ID: "org_direct_test"}).Return(&types.Organization{
		ID: "org_direct_test", Name: "Direct org",
	}, nil)

	labels, err := server.sandboxOOMLabels(context.Background(), "runner-direct", "direct.test", types.ContainerOOMKills{
		ContainerID: "container-direct", SessionID: "sbx_direct_test",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"runner-direct", "direct.test", "sbx_direct_test", "sbx_direct_test", "container-direct", "prj_direct_test", "Direct project", "org_direct_test", "Direct org"}
	if !slices.Equal(labels, want) {
		t.Fatalf("labels = %#v, want %#v", labels, want)
	}
}

func TestSandboxOOMLabelsKeepKnownIDsWhenRowsAreGone(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: st}
	st.EXPECT().GetSession(gomock.Any(), "ses_deleted_test").Return(&types.Session{
		ID: "ses_deleted_test", ProjectID: "prj_deleted_test", OrganizationID: "org_deleted_test",
	}, nil)
	st.EXPECT().GetSandboxBySession(gomock.Any(), "ses_deleted_test").Return(nil, store.ErrNotFound)
	st.EXPECT().GetProject(gomock.Any(), "prj_deleted_test").Return(nil, store.ErrNotFound)
	st.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{ID: "org_deleted_test"}).Return(nil, errors.New("deleted"))

	labels, err := server.sandboxOOMLabels(context.Background(), "runner-deleted", "deleted.test", types.ContainerOOMKills{
		ContainerID: "container-deleted", SessionID: "ses_deleted_test",
	})
	if err == nil {
		t.Fatal("expected incomplete metadata error")
	}
	want := []string{"runner-deleted", "deleted.test", "", "ses_deleted_test", "container-deleted", "prj_deleted_test", "", "org_deleted_test", ""}
	if !slices.Equal(labels, want) {
		t.Fatalf("labels = %#v, want %#v", labels, want)
	}
}

func TestSandboxOOMMetricsRetryIncompleteMetadataBeforeFirstKill(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: st}
	report := types.ContainerOOMKills{ContainerID: "container-retry", SessionID: "ses_retry", Count: 0}
	st.EXPECT().GetSandboxInstance(gomock.Any(), "runner-retry").Return(&types.SandboxInstance{Hostname: "retry.test"}, nil)
	st.EXPECT().GetSession(gomock.Any(), "ses_retry").Return(&types.Session{ID: "ses_retry", ProjectID: "prj_retry", OrganizationID: "org_retry"}, nil)
	st.EXPECT().GetSandboxBySession(gomock.Any(), "ses_retry").Return(nil, store.ErrNotFound)
	st.EXPECT().GetProject(gomock.Any(), "prj_retry").Return(nil, store.ErrNotFound)
	st.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{ID: "org_retry"}).Return(nil, store.ErrNotFound)

	now := time.Now()
	server.updateSandboxOOMMetrics(context.Background(), "runner-retry", []types.ContainerOOMKills{report}, now)

	st.EXPECT().GetSession(gomock.Any(), "ses_retry").Return(&types.Session{ID: "ses_retry", ProjectID: "prj_retry", OrganizationID: "org_retry"}, nil)
	st.EXPECT().GetSandboxBySession(gomock.Any(), "ses_retry").Return(&types.Sandbox{ID: "sbx_retry"}, nil)
	st.EXPECT().GetProject(gomock.Any(), "prj_retry").Return(&types.Project{ID: "prj_retry", Name: "Retry project", OrganizationID: "org_retry"}, nil)
	st.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{ID: "org_retry"}).Return(&types.Organization{ID: "org_retry", Name: "Retry org"}, nil)
	server.updateSandboxOOMMetrics(context.Background(), "runner-retry", []types.ContainerOOMKills{report}, now.Add(time.Minute))

	sandboxOOMState.Lock()
	state := sandboxOOMState.containers["runner-retry\x00container-retry"]
	sandboxOOMState.Unlock()
	want := []string{"runner-retry", "retry.test", "sbx_retry", "ses_retry", "container-retry", "prj_retry", "Retry project", "org_retry", "Retry org"}
	if !state.metadataComplete || !slices.Equal(state.labels, want) {
		t.Fatalf("state = %#v, want labels %#v with complete metadata", state, want)
	}
}

func TestSandboxOOMLabelsUsePersonalProjectOrganization(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: st}
	st.EXPECT().GetSession(gomock.Any(), "ses_personal").Return(&types.Session{
		ID: "ses_personal", ProjectID: "prj_personal", OrganizationID: "org_stale",
	}, nil)
	st.EXPECT().GetSandboxBySession(gomock.Any(), "ses_personal").Return(&types.Sandbox{ID: "sbx_personal"}, nil)
	st.EXPECT().GetProject(gomock.Any(), "prj_personal").Return(&types.Project{
		ID: "prj_personal", Name: "Personal project", OrganizationID: "",
	}, nil)

	labels, err := server.sandboxOOMLabels(context.Background(), "runner-personal", "personal.test", types.ContainerOOMKills{
		ContainerID: "container-personal", SessionID: "ses_personal",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"runner-personal", "personal.test", "sbx_personal", "ses_personal", "container-personal", "prj_personal", "Personal project", "", "personal"}
	if !slices.Equal(labels, want) {
		t.Fatalf("labels = %#v, want %#v", labels, want)
	}
}

func TestSandboxOOMLabelsAllowSessionWithoutSandboxRow(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: st}
	st.EXPECT().GetSession(gomock.Any(), "ses_only").Return(&types.Session{
		ID: "ses_only", ProjectID: "prj_only", OrganizationID: "org_only",
	}, nil)
	st.EXPECT().GetSandboxBySession(gomock.Any(), "ses_only").Return(nil, store.ErrNotFound)
	st.EXPECT().GetProject(gomock.Any(), "prj_only").Return(&types.Project{
		ID: "prj_only", Name: "Session-only project", OrganizationID: "org_only",
	}, nil)
	st.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{ID: "org_only"}).Return(&types.Organization{
		ID: "org_only", Name: "Session-only org",
	}, nil)

	labels, err := server.sandboxOOMLabels(context.Background(), "runner-only", "only.test", types.ContainerOOMKills{
		ContainerID: "container-only", SessionID: "ses_only",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"runner-only", "only.test", "", "ses_only", "container-only", "prj_only", "Session-only project", "org_only", "Session-only org"}
	if !slices.Equal(labels, want) {
		t.Fatalf("labels = %#v, want %#v", labels, want)
	}
}
