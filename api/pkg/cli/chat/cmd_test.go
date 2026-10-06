package chat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestChatContinuesSessionWithoutApp(t *testing.T) {
	type capturedRequest struct {
		authorization string
		path          string
		request       types.SessionChatRequest
	}
	requests := make(chan capturedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request types.SessionChatRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		requests <- capturedRequest{
			authorization: r.Header.Get("Authorization"),
			path:          r.URL.Path,
			request:       request,
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	t.Cleanup(server.Close)

	t.Setenv("HELIX_URL", server.URL)
	t.Setenv("HELIX_API_KEY", "test-key")

	agentID = ""
	sessionID = ""
	model = ""
	agentType = ""
	stream = false
	verbose = false
	timeout = 120
	t.Cleanup(func() {
		agentID = ""
		sessionID = ""
		model = ""
		agentType = ""
		stream = false
		verbose = false
		timeout = 120
		rootCmd.SetArgs(nil)
	})

	rootCmd.SetArgs([]string{"--session", "ses_task", "validate the task"})
	require.NoError(t, rootCmd.Execute())

	captured := <-requests
	require.Equal(t, "/api/v1/sessions/chat", captured.path)
	require.Equal(t, "Bearer test-key", captured.authorization)
	require.Empty(t, captured.request.AppID)
	require.Equal(t, "ses_task", captured.request.SessionID)
	message, ok := captured.request.Message()
	require.True(t, ok)
	require.Equal(t, "validate the task", message)
}
