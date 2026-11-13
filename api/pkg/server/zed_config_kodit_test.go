package server

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/types"
)

// A plain chat's desktop outside any project is not offered Kodit; project
// sessions and helix-org workers still are (subject to the other gates).
func TestSessionHasKoditRepositories(t *testing.T) {
	require.False(t, sessionHasKoditRepositories(&types.Session{ID: "ses_chat"}))
	require.True(t, sessionHasKoditRepositories(&types.Session{ID: "ses_p", ProjectID: "prj_1"}))
	require.True(t, sessionHasKoditRepositories(&types.Session{ID: "ses_w", Metadata: types.SessionMetadata{OrgWorkerID: "chief-of-staff"}}))
}
