package services

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// The prompt's Kodit list is exactly what the lister resolves for the task's
// agent; other indexed repositories of the organization are not named, and
// without a lister (or with code intelligence off) no Kodit list is shown.
func TestRepositorySectionForTask_KoditReposComeFromLister(t *testing.T) {
	local := &types.GitRepository{ID: "repo_local", Name: "local-repo"}
	reachable := &types.GitRepository{ID: "repo_reachable", Name: "reachable-repo", KoditIndexing: true, Metadata: map[string]any{"kodit_repo_id": int64(20)}}
	task := &types.SpecTask{ID: "spt_1", ProjectID: "prj_1", CreatedBy: "usr_a"}

	cases := []struct {
		name        string
		project     *types.Project
		lister      KoditRepoLister
		wantReached bool
	}{
		{"lister result is listed", &types.Project{ID: "prj_1", KoditEnabled: true}, func(_ context.Context, p *types.Project, userID string) ([]*types.GitRepository, error) {
			assert.Equal(t, "prj_1", p.ID)
			assert.Equal(t, "usr_a", userID)
			return []*types.GitRepository{local, reachable}, nil
		}, true},
		{"no lister lists nothing", &types.Project{ID: "prj_1", KoditEnabled: true}, nil, false},
		{"code intelligence off skips the lister", &types.Project{ID: "prj_1"}, func(context.Context, *types.Project, string) ([]*types.GitRepository, error) {
			t.Fatal("lister must not be called when Kodit is off")
			return nil, nil
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := store.NewMockStore(gomock.NewController(t))
			st.EXPECT().ListGitRepositories(gomock.Any(), &types.ListGitRepositoriesRequest{ProjectID: "prj_1"}).
				Return([]*types.GitRepository{local}, nil)

			section := repositorySectionForTask(context.Background(), st, tc.lister, task, tc.project)
			assert.Contains(t, section, "local-repo")
			assert.Equal(t, tc.wantReached, strings.Contains(section, "reachable-repo"))
			assert.Equal(t, tc.wantReached, strings.Contains(section, "Kodit repo ID: 20"))
		})
	}
}
