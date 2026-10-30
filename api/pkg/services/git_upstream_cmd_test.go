package services

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// stallingListener accepts connections and never responds, like a proxy that
// swallows a request body.
func stallingListener(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	var mu sync.Mutex
	var conns []net.Conn
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, c)
			mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, c := range conns {
			_ = c.Close()
		}
	})
	return ln.Addr().String()
}

func gitProcessesMatching(t *testing.T, needle string) []string {
	t.Helper()
	entries, err := os.ReadDir("/proc")
	require.NoError(t, err)
	var found []string
	for _, e := range entries {
		cmdline, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
		if err != nil {
			continue
		}
		c := strings.ReplaceAll(string(cmdline), "\x00", " ")
		if strings.Contains(c, needle) && strings.Contains(c, "git") {
			found = append(found, e.Name()+" "+c)
		}
	}
	return found
}

func TestRunUpstreamGit_StalledRemoteIsKilled(t *testing.T) {
	addr := stallingListener(t)
	remote := fmt.Sprintf("http://%s/stalled.git", addr)

	start := time.Now()
	_, _, err := runUpstreamGit(context.Background(), t.TempDir(), 2*time.Second, nil, "ls-remote", "--heads", remote)
	elapsed := time.Since(start)

	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, elapsed, 2*time.Second+upstreamGitWaitDelay)

	// The git-remote-http helper must not outlive the call.
	require.Eventually(t, func() bool {
		return len(gitProcessesMatching(t, addr)) == 0
	}, 5*time.Second, 100*time.Millisecond, "orphan git processes: %v", gitProcessesMatching(t, addr))
}

func TestRunUpstreamGit_CallerCancelKillsProcessGroup(t *testing.T) {
	addr := stallingListener(t)
	remote := fmt.Sprintf("http://%s/stalled.git", addr)

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(500*time.Millisecond, cancel)

	start := time.Now()
	_, _, err := runUpstreamGit(ctx, t.TempDir(), time.Minute, nil, "ls-remote", remote)
	require.Error(t, err)
	require.Less(t, time.Since(start), 500*time.Millisecond+upstreamGitWaitDelay)
	require.Eventually(t, func() bool {
		return len(gitProcessesMatching(t, addr)) == 0
	}, 5*time.Second, 100*time.Millisecond)
}

func TestRunUpstreamGit_CredentialsSentAsHeaderNotURL(t *testing.T) {
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.String()
		http.NotFound(w, r)
	}))
	defer srv.Close()

	remote := upstreamRemote{URL: srv.URL + "/repo.git", Username: "oauth2", Password: "s3cret-token"}
	_, _, err := runUpstreamGit(context.Background(), t.TempDir(), 30*time.Second, remote.env(), "ls-remote", remote.URL)
	require.Error(t, err) // 404

	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("oauth2:s3cret-token"))
	require.Equal(t, want, gotAuth)
	require.NotContains(t, gotPath, "s3cret-token")
}

func TestUpstreamRemoteEnv(t *testing.T) {
	env := upstreamRemote{URL: "https://gitlab.example/x.git"}.env()
	joined := strings.Join(env, "\n")
	require.Contains(t, joined, "GIT_CONFIG_COUNT=3")
	require.Contains(t, joined, "http.postBuffer")
	require.Contains(t, joined, "http.lowSpeedLimit")
	require.NotContains(t, joined, "extraHeader")

	env = upstreamRemote{URL: "https://gitlab.example/x.git", Password: "tok"}.env()
	require.Contains(t, strings.Join(env, "\n"), "GIT_CONFIG_COUNT=4")

	// Package-level defaults must not be mutated by appending the auth header.
	require.Len(t, upstreamGitConfig, 3)
}

func TestScrubRemoteCredentials(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	_, _, err := runUpstreamGit(ctx, dir, time.Minute, nil, "init", "--bare", ".")
	require.NoError(t, err)
	_, _, err = runUpstreamGit(ctx, dir, time.Minute, nil, "remote", "add", "origin", "https://oauth2:tok@gitlab.example/g/r.git")
	require.NoError(t, err)

	require.NoError(t, scrubRemoteCredentials(ctx, dir))

	out, _, err := runUpstreamGit(ctx, dir, time.Minute, nil, "config", "--get", "remote.origin.url")
	require.NoError(t, err)
	require.Equal(t, "https://gitlab.example/g/r.git", strings.TrimSpace(out))
}
