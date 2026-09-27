package services

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

func gitReq(method, target, repoID string) *http.Request {
	r := httptest.NewRequest(method, target, nil)
	return mux.SetURLVars(r, map[string]string{"repo_id": repoID})
}

// A bot instance key reads its own project's repositories and nothing else;
// an embed key never reaches git.
func TestGitKeyTypes(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	st.EXPECT().ListGitRepositories(gomock.Any(), &types.ListGitRepositoriesRequest{ProjectID: "prj_bot"}).
		Return([]*types.GitRepository{{ID: "repo_bot"}, {ID: "repo_skills"}}, nil).AnyTimes()
	s := &GitHTTPServer{store: st}

	instance := &types.ApiKey{Type: types.APIkeytypeBotInstance, ProjectID: "prj_bot", SessionID: "ses_A"}
	cases := []struct {
		name   string
		key    *types.ApiKey
		req    *http.Request
		expect bool
	}{
		{"instance clones its repo", instance, gitReq("GET", "/git/repo_bot/info/refs?service=git-upload-pack", "repo_bot"), true},
		{"instance fetches an attached repo", instance, gitReq("POST", "/git/repo_skills/git-upload-pack", "repo_skills"), true},
		{"instance cannot push", instance, gitReq("POST", "/git/repo_bot/git-receive-pack", "repo_bot"), false},
		{"instance cannot advertise for push", instance, gitReq("GET", "/git/repo_bot/info/refs?service=git-receive-pack", "repo_bot"), false},
		{"instance cannot read another repo", instance, gitReq("GET", "/git/repo_other/info/refs?service=git-upload-pack", "repo_other"), false},
		{"instance cannot read status", instance, gitReq("GET", "/git/repo_bot/status", "repo_bot"), false},
		{"instance key without a project reads nothing", &types.ApiKey{Type: types.APIkeytypeBotInstance}, gitReq("POST", "/git/repo_bot/git-upload-pack", "repo_bot"), false},
		{"embed key never reaches git", &types.ApiKey{Type: types.APIkeytypeEmbed}, gitReq("POST", "/git/repo_bot/git-upload-pack", "repo_bot"), false},
		{"ordinary key is unchanged", &types.ApiKey{Type: types.APIkeytypeAPI}, gitReq("POST", "/git/repo_other/git-receive-pack", "repo_other"), true},
	}
	for _, c := range cases {
		if got := s.keyTypeAllowsGit(c.req, c.key); got != c.expect {
			t.Errorf("%s: got %v, want %v", c.name, got, c.expect)
		}
	}
}
