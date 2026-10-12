package webservice

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	dto "github.com/prometheus/client_model/go"
	"go.uber.org/mock/gomock"
)

func TestHealthMonitorPublishesWebServiceMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	const projectID = "prj_metrics_metadata"
	defer forgetProjectMetrics(projectID)

	st.EXPECT().ListActiveWebServices(gomock.Any()).Return([]*types.ProjectWebServiceState{{
		ProjectID: projectID,
	}}, nil)
	st.EXPECT().GetProject(gomock.Any(), projectID).Return(&types.Project{
		ID:             projectID,
		Name:           "Find AI",
		OrganizationID: "org_helix",
	}, nil)
	st.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{ID: "org_helix"}).Return(&types.Organization{
		ID:   "org_helix",
		Name: "helix",
	}, nil)
	st.EXPECT().ListWebServiceDeploys(gomock.Any(), projectID, 1).Return([]*types.WebServiceDeploy{{
		Status:    types.WebServiceDeployStatusBuilding,
		StartedAt: time.Now(),
	}}, nil)

	NewHealthMonitor(st, nil).runOnce(context.Background())

	var metric dto.Metric
	if err := metricInfo.WithLabelValues(projectID, "Find AI", "org_helix", "helix").Write(&metric); err != nil {
		t.Fatal(err)
	}
	if got := metric.GetGauge().GetValue(); got != 1 {
		t.Fatalf("metadata metric = %v, want 1", got)
	}
}

func TestHealthMonitorStillPublishesHealthWhenMetadataLookupFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	const projectID = "prj_metrics_lookup_failure"
	defer forgetProjectMetrics(projectID)

	st.EXPECT().ListActiveWebServices(gomock.Any()).Return([]*types.ProjectWebServiceState{{
		ProjectID: projectID,
	}}, nil)
	st.EXPECT().GetProject(gomock.Any(), projectID).Return(nil, errors.New("database unavailable"))
	st.EXPECT().ListWebServiceDeploys(gomock.Any(), projectID, 1).Return([]*types.WebServiceDeploy{{
		Status:    types.WebServiceDeployStatusBuilding,
		StartedAt: time.Now(),
	}}, nil)

	NewHealthMonitor(st, nil).runOnce(context.Background())

	var metric dto.Metric
	if err := metricUp.WithLabelValues(projectID).Write(&metric); err != nil {
		t.Fatal(err)
	}
	if got := metric.GetGauge().GetValue(); got != 1 {
		t.Fatalf("health metric = %v, want 1", got)
	}
}

func TestRefreshProjectInfoPublishesPersonalProject(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	const projectID = "prj_metrics_personal"
	defer forgetProjectMetrics(projectID)

	st.EXPECT().GetProject(gomock.Any(), projectID).Return(&types.Project{
		ID:   projectID,
		Name: "Personal site",
	}, nil)

	NewHealthMonitor(st, nil).refreshProjectInfo(context.Background(), projectID)

	var metric dto.Metric
	if err := metricInfo.WithLabelValues(projectID, "Personal site", "", "personal").Write(&metric); err != nil {
		t.Fatal(err)
	}
	if got := metric.GetGauge().GetValue(); got != 1 {
		t.Fatalf("personal metadata metric = %v, want 1", got)
	}
}

func TestRefreshProjectInfoReplacesRenamedLabels(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	const projectID = "prj_metrics_renamed"
	defer forgetProjectMetrics(projectID)

	gomock.InOrder(
		st.EXPECT().GetProject(gomock.Any(), projectID).Return(&types.Project{
			ID: projectID, Name: "Old project", OrganizationID: "org_old",
		}, nil),
		st.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{ID: "org_old"}).Return(&types.Organization{
			ID: "org_old", Name: "old-org",
		}, nil),
		st.EXPECT().GetProject(gomock.Any(), projectID).Return(&types.Project{
			ID: projectID, Name: "New project", OrganizationID: "org_new",
		}, nil),
		st.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{ID: "org_new"}).Return(&types.Organization{
			ID: "org_new", Name: "new-org",
		}, nil),
	)

	m := NewHealthMonitor(st, nil)
	m.refreshProjectInfo(context.Background(), projectID)
	m.refreshProjectInfo(context.Background(), projectID)

	var metric dto.Metric
	if err := metricInfo.WithLabelValues(projectID, "New project", "org_new", "new-org").Write(&metric); err != nil {
		t.Fatal(err)
	}
	if got := metric.GetGauge().GetValue(); got != 1 {
		t.Fatalf("renamed metadata metric = %v, want 1", got)
	}
	if got := metricInfo.DeletePartialMatch(map[string]string{"project_id": projectID}); got != 1 {
		t.Fatalf("metadata series after rename = %d, want 1", got)
	}
}
