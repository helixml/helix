package server

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/types"
)

func socketRequest() *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx := context.WithValue(req.Context(), http.LocalAddrContextKey,
		&net.UnixAddr{Name: "/socket/embeddings.sock", Net: "unix"})
	return req.WithContext(ctx)
}

// A unix-socket caller cannot choose its identity with a header.
func TestGetRequestUser_SocketIgnoresUserIDHeader(t *testing.T) {
	req := socketRequest()
	req.Header.Set("X-Helix-User-ID", "usr_someone_else")

	user := getRequestUser(req)
	require.NotNil(t, user)
	require.Equal(t, socketUserID, user.ID)
	require.Equal(t, types.OwnerTypeSocket, user.Type)
}

// A configured socket user (set in context by the socket middleware) wins,
// and the header still cannot override it.
func TestGetRequestUser_SocketUsesConfiguredUser(t *testing.T) {
	req := socketRequest()
	req.Header.Set("X-Helix-User-ID", "usr_someone_else")
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: "usr_configured"}))

	user := getRequestUser(req)
	require.NotNil(t, user)
	require.Equal(t, "usr_configured", user.ID)
}

func TestListenEmbeddingsSocket_NotWorldAccessible(t *testing.T) {
	path := filepath.Join(t.TempDir(), "embeddings.sock")
	require.NoError(t, os.WriteFile(path, []byte("stale"), 0o600))

	listener, err := listenEmbeddingsSocket(path)
	require.NoError(t, err)
	defer listener.Close()

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.NotZero(t, info.Mode()&os.ModeSocket, "stale file was not replaced by a socket")
	require.Equal(t, os.FileMode(0o660), info.Mode().Perm())
	require.Zero(t, info.Mode().Perm()&0o007, "socket is accessible to other users")
}

// The socket accepts connections at its final path and leaves no staging
// directory behind.
func TestListenEmbeddingsSocket_ServesAtFinalPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "embeddings.sock")

	listener, err := listenEmbeddingsSocket(path)
	require.NoError(t, err)
	defer listener.Close()

	go func() {
		if conn, err := listener.Accept(); err == nil {
			_ = conn.Close()
		}
	}()
	conn, err := net.Dial("unix", path)
	require.NoError(t, err)
	_ = conn.Close()

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "staging directory left behind")
	require.Equal(t, "embeddings.sock", entries[0].Name())
}
