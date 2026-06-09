package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// callForkHTTP runs the forkSession handler through its wrapper so the
// HTTPError → HTTP response path is exercised end-to-end. Returns the
// recorder so callers can assert on Code + Body.
func callForkHTTP(t *testing.T, srv *HelixAPIServer, user *types.User, sessionID string, body ForkSessionRequest) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+sessionID+"/fork", bytes.NewReader(raw))
	req = mux.SetURLVars(req, map[string]string{"id": sessionID})
	req = req.WithContext(setRequestUser(req.Context(), *user))
	rr := httptest.NewRecorder()
	system.Wrapper(srv.forkSession)(rr, req)
	return rr
}

// seedRunningParent creates a parent session in the store + N completed
// interactions, all owned by `user`, with no OrganizationID so the simple
// owner-equality auth check fires.
func seedRunningParent(t *testing.T, srv *HelixAPIServer, user *types.User, runtime types.CodeAgentRuntime, n int) *types.Session {
	t.Helper()
	parent := &types.Session{
		ID:        "ses_" + randSuffix(),
		Name:      "Parent",
		Owner:     user.ID,
		OwnerType: user.Type,
		Type:      types.SessionTypeText,
		Mode:      types.SessionModeInference,
		ProjectID: "prj_test",
		ParentApp: "app_test",
		Metadata: types.SessionMetadata{
			AgentType:        "zed_external",
			CodeAgentRuntime: runtime,
			ZedAgentName:     runtime.ZedAgentName(),
		},
	}
	_, err := srv.Store.CreateSession(context.Background(), *parent)
	require.NoError(t, err)
	for i := 0; i < n; i++ {
		_, err := srv.Store.CreateInteraction(context.Background(), &types.Interaction{
			SessionID:       parent.ID,
			GenerationID:    parent.GenerationID,
			Mode:            types.SessionModeInference,
			State:           types.InteractionStateComplete,
			PromptMessage:   "u" + itoaInt(i),
			ResponseMessage: "a" + itoaInt(i),
		})
		require.NoError(t, err)
	}
	return parent
}

func TestForkSessionHTTP_HappyPath(t *testing.T) {
	srv, _ := newForkTestServer(t)
	user := &types.User{ID: "user_owner", Type: types.OwnerTypeUser}
	parent := seedRunningParent(t, srv, user, types.CodeAgentRuntimeClaudeCode, 2)

	rr := callForkHTTP(t, srv, user, parent.ID, ForkSessionRequest{
		CodeAgentRuntime: types.CodeAgentRuntimeQwenCode,
	})

	require.Equal(t, http.StatusOK, rr.Code, "body: %s", rr.Body.String())
	var resp ForkSessionResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.NotEmpty(t, resp.NewSessionID)
	assert.NotEqual(t, parent.ID, resp.NewSessionID)

	// Parent should now be paused with a forked_to:<child> reason.
	freshParent, err := srv.Store.GetSession(context.Background(), parent.ID)
	require.NoError(t, err)
	assert.True(t, freshParent.Metadata.Paused)
	assert.Equal(t, "forked_to:"+resp.NewSessionID, freshParent.Metadata.PausedReason)
}

func TestForkSessionHTTP_RejectsForkFromPaused(t *testing.T) {
	srv, _ := newForkTestServer(t)
	user := &types.User{ID: "user_owner", Type: types.OwnerTypeUser}
	parent := seedRunningParent(t, srv, user, types.CodeAgentRuntimeClaudeCode, 1)
	parent.Metadata.Paused = true
	parent.Metadata.PausedReason = "forked_to:ses_alreadyforked"
	_, err := srv.Store.UpdateSession(context.Background(), *parent)
	require.NoError(t, err)

	rr := callForkHTTP(t, srv, user, parent.ID, ForkSessionRequest{
		CodeAgentRuntime: types.CodeAgentRuntimeQwenCode,
	})

	assert.Equal(t, http.StatusConflict, rr.Code, "body: %s", rr.Body.String())
	assert.Contains(t, rr.Body.String(), "paused")
}

func TestForkSessionHTTP_RejectsSameRuntime(t *testing.T) {
	srv, _ := newForkTestServer(t)
	user := &types.User{ID: "user_owner", Type: types.OwnerTypeUser}
	parent := seedRunningParent(t, srv, user, types.CodeAgentRuntimeClaudeCode, 1)

	rr := callForkHTTP(t, srv, user, parent.ID, ForkSessionRequest{
		CodeAgentRuntime: types.CodeAgentRuntimeClaudeCode,
	})

	assert.Equal(t, http.StatusBadRequest, rr.Code, "body: %s", rr.Body.String())
	assert.Contains(t, rr.Body.String(), "already using")
}

func TestForkSessionHTTP_RejectsNonZedExternalSource(t *testing.T) {
	srv, _ := newForkTestServer(t)
	user := &types.User{ID: "user_owner", Type: types.OwnerTypeUser}
	parent := seedRunningParent(t, srv, user, types.CodeAgentRuntimeClaudeCode, 1)
	parent.Metadata.AgentType = "helix"
	_, err := srv.Store.UpdateSession(context.Background(), *parent)
	require.NoError(t, err)

	rr := callForkHTTP(t, srv, user, parent.ID, ForkSessionRequest{
		CodeAgentRuntime: types.CodeAgentRuntimeQwenCode,
	})

	assert.Equal(t, http.StatusBadRequest, rr.Code, "body: %s", rr.Body.String())
	assert.Contains(t, rr.Body.String(), "not an external agent session")
}

func TestForkSessionHTTP_NotFound(t *testing.T) {
	srv, _ := newForkTestServer(t)
	user := &types.User{ID: "user_owner", Type: types.OwnerTypeUser}

	rr := callForkHTTP(t, srv, user, "ses_does_not_exist", ForkSessionRequest{
		CodeAgentRuntime: types.CodeAgentRuntimeQwenCode,
	})

	assert.Equal(t, http.StatusNotFound, rr.Code, "body: %s", rr.Body.String())
}

func TestForkSessionHTTP_ForbiddenForNonOwner(t *testing.T) {
	srv, _ := newForkTestServer(t)
	owner := &types.User{ID: "user_owner", Type: types.OwnerTypeUser}
	other := &types.User{ID: "user_someone_else", Type: types.OwnerTypeUser}
	parent := seedRunningParent(t, srv, owner, types.CodeAgentRuntimeClaudeCode, 1)

	rr := callForkHTTP(t, srv, other, parent.ID, ForkSessionRequest{
		CodeAgentRuntime: types.CodeAgentRuntimeQwenCode,
	})

	assert.Equal(t, http.StatusForbidden, rr.Code, "body: %s", rr.Body.String())
}

func TestSendSessionMessageHTTP_RejectsPaused(t *testing.T) {
	srv, _ := newForkTestServer(t)
	user := &types.User{ID: "user_owner", Type: types.OwnerTypeUser}
	parent := seedRunningParent(t, srv, user, types.CodeAgentRuntimeClaudeCode, 1)
	parent.Metadata.Paused = true
	parent.Metadata.PausedReason = "forked_to:ses_child"
	_, err := srv.Store.UpdateSession(context.Background(), *parent)
	require.NoError(t, err)

	body := SessionMessageRequest{Content: "hello"}
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+parent.ID+"/messages", bytes.NewReader(raw))
	req = mux.SetURLVars(req, map[string]string{"id": parent.ID})
	req = req.WithContext(setRequestUser(req.Context(), *user))
	rr := httptest.NewRecorder()
	system.Wrapper(srv.sendSessionMessage)(rr, req)

	assert.Equal(t, http.StatusConflict, rr.Code, "body: %s", rr.Body.String())
	assert.Contains(t, rr.Body.String(), "paused")
	assert.Contains(t, rr.Body.String(), "forked_to:ses_child")
}
