package services

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
)

// A server-side merge into an internal repo's default branch fires the same
// hook as a push to it (the web-service auto-deploy); other branches and
// external repos do not.
func TestNotifyDefaultBranchMerged(t *testing.T) {
	fired := make(chan string, 4)
	s := &GitHTTPServer{}
	s.SetOnDefaultBranchPush(func(_ context.Context, repoID, branch, userID string) {
		fired <- repoID + "/" + branch + "/" + userID
	})

	internal := &types.GitRepository{ID: "repo_internal", DefaultBranch: "main"}
	s.NotifyDefaultBranchMerged(internal, "main", "usr_1")
	select {
	case got := <-fired:
		require.Equal(t, "repo_internal/main/usr_1", got)
	case <-time.After(2 * time.Second):
		t.Fatal("merge into the default branch did not fire the hook")
	}

	s.NotifyDefaultBranchMerged(internal, "feature/x", "usr_1")
	s.NotifyDefaultBranchMerged(&types.GitRepository{ID: "repo_ext", DefaultBranch: "main", IsExternal: true}, "main", "usr_1")
	var nilServer *GitHTTPServer
	nilServer.NotifyDefaultBranchMerged(internal, "main", "usr_1")
	select {
	case got := <-fired:
		t.Fatalf("unexpected hook call %s", got)
	case <-time.After(200 * time.Millisecond):
	}
}
