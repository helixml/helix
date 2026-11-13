package services

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/helixml/helix/api/pkg/types"
)

// kodit_repo_id picks which Kodit index a repository's code-intelligence
// routes read, so caller-supplied metadata must never carry it.
func TestServerOwnedMetadataIsDropped(t *testing.T) {
	in := map[string]interface{}{
		"kodit_repo_id":  int64(42),
		"Kodit_Repo_ID":  "42",
		" kodit_status ": "x",
		"sample_type":    "web",
	}
	out := withoutServerOwnedMetadata(in)
	assert.Equal(t, map[string]interface{}{"sample_type": "web"}, out)
	assert.Nil(t, withoutServerOwnedMetadata(nil))
	assert.True(t, isServerOwnedMetadataKey("kodit_repo_id"))
	assert.False(t, isServerOwnedMetadataKey("created_from"))
}

func TestSameKoditCloneSource(t *testing.T) {
	own := "http://api:key-a@api:8080/git/repo_1"
	assert.True(t, sameKoditCloneSource("http://api@api:8080/git/repo_1", own), "same repo, credentials differ")
	assert.True(t, sameKoditCloneSource("http://api:key-b@API:8080/git/repo_1/", own))
	assert.False(t, sameKoditCloneSource("http://api@api:8080/git/repo_2", own), "another Helix repository")
	assert.False(t, sameKoditCloneSource("https://github.com/acme/private", own), "an upstream URL")
	assert.False(t, sameKoditCloneSource("", own))
	assert.False(t, sameKoditCloneSource("::", own))
	assert.True(t, sameKoditCloneSource("https://helix.example/git/repo_1", own), "KODIT_GIT_URL changed host and scheme family")
	assert.True(t, sameKoditCloneSource("file:///data/knowledge/k1", "file:///data/knowledge/k1"), "knowledge directory re-registration")
	assert.False(t, sameKoditCloneSource("file:///data/knowledge/k2", "file:///data/knowledge/k1"))
	assert.False(t, sameKoditCloneSource("file:///git/repo_1", own), "scheme must match")
}

func TestUpdateRepository_CannotRepointKoditIndex(t *testing.T) {
	st := &fakeStore{repo: &types.GitRepository{
		ID:       "repo-1",
		Metadata: map[string]interface{}{"kodit_repo_id": int64(7), "note": "a"},
	}}
	svc := NewGitRepositoryService(st, t.TempDir(), "http://localhost:8080", "test", "test@test.com")

	updated, err := svc.UpdateRepository(t.Context(), "repo-1", &types.GitRepositoryUpdateRequest{
		Metadata: map[string]interface{}{"kodit_repo_id": int64(99), "KODIT_REPO_ID": "99", "note": "b"},
	}, "")
	assert.NoError(t, err)
	assert.Equal(t, int64(7), updated.Metadata["kodit_repo_id"], "caller cannot repoint the Kodit index")
	assert.NotContains(t, updated.Metadata, "KODIT_REPO_ID")
	assert.Equal(t, "b", updated.Metadata["note"], "ordinary metadata still merges")
}
