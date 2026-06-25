package webservice

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"go.uber.org/mock/gomock"
)

// TestHealthMonitorDeployInProgress: the monitor must NOT treat a project as
// unhealthy while its latest deploy is still pending/building — otherwise it
// races (and can interrupt) an in-flight initial deploy.
func TestHealthMonitorDeployInProgress(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	st := store.NewMockStore(ctrl)
	m := &HealthMonitor{store: st}
	ctx := context.Background()

	cases := []struct {
		name   string
		status types.WebServiceDeployStatus
		want   bool
	}{
		{"pending", types.WebServiceDeployStatusPending, true},
		{"building", types.WebServiceDeployStatusBuilding, true},
		{"live", types.WebServiceDeployStatusLive, false},
		{"failed", types.WebServiceDeployStatusFailed, false},
		{"superseded", types.WebServiceDeployStatusSuperseded, false},
	}
	for _, c := range cases {
		st.EXPECT().ListWebServiceDeploys(gomock.Any(), c.name, 1).
			Return([]*types.WebServiceDeploy{{Status: c.status}}, nil)
		if got := m.deployInProgress(ctx, c.name); got != c.want {
			t.Errorf("%s: deployInProgress = %v, want %v", c.name, got, c.want)
		}
	}

	// No deploys yet → not in progress (don't block probing a never-deployed row).
	st.EXPECT().ListWebServiceDeploys(gomock.Any(), "none", 1).Return(nil, nil)
	if m.deployInProgress(ctx, "none") {
		t.Error("no deploys should not count as in-progress")
	}
}
