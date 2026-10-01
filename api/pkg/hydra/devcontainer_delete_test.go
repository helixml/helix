package hydra

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/docker/docker/client"
	"github.com/stretchr/testify/require"
)

func TestRemoveSessionContainersPropagatesDockerRemoveFailure(t *testing.T) {
	removeCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/containers/json"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"Id":"container-live","Names":["/ubuntu-external-01abc"],"Labels":{"helix.session_id":"ses_01abc"}}]`))
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/containers/container-live"):
			removeCalled = true
			http.Error(w, `{"message":"remove failed"}`, http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	dockerClient, err := client.NewClientWithOpts(
		client.WithHost(strings.Replace(server.URL, "http://", "tcp://", 1)),
		client.WithHTTPClient(server.Client()),
		client.WithVersion("1.43"),
	)
	require.NoError(t, err)
	defer dockerClient.Close()

	err = removeSessionContainers(context.Background(), dockerClient, "ses_01abc")

	require.ErrorContains(t, err, "remove container container-live")
	require.True(t, removeCalled)
}
