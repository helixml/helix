package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestUpdateGitRepository_UnknownDefaultBranchIsBadRequest(t *testing.T) {
	filestoreBase := t.TempDir()
	repoPath := filepath.Join(filestoreBase, "git-repositories", "repo_1")
	out, err := exec.Command("git", "init", "--bare", "-b", "main", repoPath).CombinedOutput()
	require.NoError(t, err, string(out))

	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	repo := types.GitRepository{ID: "repo_1", OwnerID: "usr_alice", LocalPath: repoPath, DefaultBranch: "main"}
	mockStore.EXPECT().GetGitRepository(gomock.Any(), "repo_1").Return(&repo, nil).AnyTimes()
	mockStore.EXPECT().UpdateGitRepository(gomock.Any(), gomock.Any()).Times(0)

	server := &HelixAPIServer{
		Store:                mockStore,
		gitRepositoryService: services.NewGitRepositoryService(mockStore, filestoreBase, "http://helix.test", "Helix", "helix@example.com"),
	}

	payload, err := json.Marshal(map[string]any{"default_branch": "nope"})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/git/repositories/repo_1", bytes.NewReader(payload))
	req = req.WithContext(setTestRequestUser(req.Context(), &types.User{ID: "usr_alice"}))
	req = mux.SetURLVars(req, map[string]string{"id": "repo_1"})
	rr := httptest.NewRecorder()
	server.updateGitRepository(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
	require.Contains(t, rr.Body.String(), "branch does not exist")
}
