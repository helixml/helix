package hydra

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareWorkspaceIdentity(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sessions", "ses_1")
	mounts := []MountConfig{
		{Source: "/var/run/docker.sock", Destination: "/var/run/docker.sock"},
		{Source: root, Destination: workspaceMountDestination},
	}

	first, err := prepareWorkspaceIdentity(mounts)
	require.NoError(t, err)
	assert.True(t, first.Created, "a missing workspace dir is a new workspace")
	assert.NotEmpty(t, first.ID)

	again, err := prepareWorkspaceIdentity(mounts)
	require.NoError(t, err)
	assert.False(t, again.Created)
	assert.Equal(t, first.ID, again.ID, "the id is stable while the directory survives")

	// What the orphan reaper does: the directory goes, and with it every
	// harness's thread store.
	require.NoError(t, os.RemoveAll(root))
	recreated, err := prepareWorkspaceIdentity(mounts)
	require.NoError(t, err)
	assert.True(t, recreated.Created)
	assert.NotEqual(t, first.ID, recreated.ID)
}

// A directory that exists without an id (pre-created by someone else, or from
// before identity stamping) gets one, without being reported as created.
func TestPrepareWorkspaceIdentity_ExistingDirWithoutID(t *testing.T) {
	root := t.TempDir()
	identity, err := prepareWorkspaceIdentity([]MountConfig{{Source: root, Destination: workspaceMountDestination}})
	require.NoError(t, err)
	assert.False(t, identity.Created)
	data, err := os.ReadFile(filepath.Join(root, workspaceIDFile))
	require.NoError(t, err)
	assert.Equal(t, identity.ID+"\n", string(data))
}

func TestPrepareWorkspaceIdentity_NoWorkspaceMount(t *testing.T) {
	identity, err := prepareWorkspaceIdentity([]MountConfig{{Type: "volume", Source: "docker-data-x", Destination: workspaceMountDestination}})
	require.NoError(t, err)
	assert.Equal(t, workspaceIdentity{}, identity)
}
