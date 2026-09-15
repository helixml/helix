package external_agent

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestDiscoverContainersFromSandbox_ResurrectsStoppedSession is a deterministic
// recreation of the bug in https://github.com/helixml/helix/issues/3067.
//
// The live bug: DiscoverContainersFromSandbox takes one snapshot of the
// sandbox's containers, and in phase 2 re-adds any session it finds in the
// snapshot but not in h.sessions — trusting the snapshot without re-checking
// that the container is still alive. StopDesktop deletes the in-memory entry
// AND the container. If the stop lands in the window between the snapshot and
// phase 2's per-session lock, discovery re-inserts the dead session as
// "running". StartDesktop then short-circuits on that entry
// (HydraExecutor.HasRunningContainer) and never recreates the container, so
// every resume hangs and times out with "external agent not ready" forever,
// until the API process restarts.
//
// The race is deterministic here by pre-applying StopDesktop's effect (delete
// from h.sessions, container gone on the sandbox) before running discovery
// with a snapshot taken BEFORE the stop — exactly the ordering the race
// produces. The fake connman also answers a live GetDevContainer probe with
// 404, proving the container is really gone: phase 2 resurrects anyway,
// because it never probes.
//
// This test documents the bug: it passes while the bug exists. Once phase 2
// re-probes container liveness under the per-session lock (the fix), it must
// be flipped to assert the session is NOT resurrected.
func TestDiscoverContainersFromSandbox_ResurrectsStoppedSession(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := store.NewMockStore(ctrl)
	h := newTestExecutor(mockStore)
	h.connman = fakeConnman{}

	sandboxID := "sbox_1"
	const sessionID = "ses_x"
	const containerID = "c1"
	containerName := "ubuntu-external-" + sessionID

	// 1. Pre-race state: a live in-memory session backed by container c1.
	h.sessions[sessionID] = &ZedSession{
		SessionID:     sessionID,
		Status:        "running",
		ContainerID:   containerID,
		ContainerName: containerName,
		ContainerIP:   "10.0.0.9",
		SandboxID:     sandboxID,
	}

	// Discovery resyncs the active_sandboxes counter from the snapshot.
	mockStore.EXPECT().
		SetSandboxContainerCount(gomock.Any(), sandboxID, 1).
		Return(nil)

	// The stale-reconcile pass has nothing to do: no DB rows, and the
	// in-memory map is emptied in step 2 below.
	mockStore.EXPECT().
		ListSessionsBySandbox(gomock.Any(), sandboxID).
		Return([]*types.Session{}, nil)

	// With the fix, phase 2 re-probes liveness under the per-session lock and
	// the probe 404s, so it never reaches the DB: no GetSession read and no
	// UpdateSession write. The session stays in its terminal "terminated_idle"
	// state and is not resurrected.

	// 2. Apply StopDesktop's effect deterministically: evict the in-memory
	//    entry (hydra_executor.go StopDesktop deletes h.sessions before the
	//    container teardown) and treat c1 as deleted on the sandbox. This is
	//    the world the snapshot in step 3 predates.
	delete(h.sessions, sessionID)

	// 3. Run discovery. The fake connman serves a ListDevContainers snapshot
	//    that still lists c1 as running — the pre-stop snapshot that the race
	//    is about, captured before the stop but consumed after it.
	require.NoError(t, h.DiscoverContainersFromSandbox(context.Background(), sandboxID))

	// 4. FIXED: phase 2 re-probes container liveness under the per-session lock
	//    before resurrecting the entry; the probe returns 404 (the container was
	//    deleted in step 2), so the dead session must NOT be resurrected as
	//    "running" and the DB must NOT be written back to running.
	h.mutex.RLock()
	_, stillTracked := h.sessions[sessionID]
	h.mutex.RUnlock()
	assert.False(t, stillTracked, "phantom session must not be resurrected by discovery")
	// No GetSession/UpdateSession calls are set up above: with the fix, phase 2
	// skips after the 404 probe and never touches the DB. ctrl.Finish() below
	// fails the test if any store method was called unexpectedly, so the DB
	// staying in its terminal state is guaranteed by construction.
}

// fakeConnman serves canned HTTP responses over an in-memory connection,
// routing by request method and path. It lets a test drive
// hydra.NewRevDialClient (constructed inside HydraExecutor methods) without a
// live sandbox.
type fakeConnman struct{}

func (fakeConnman) Dial(ctx context.Context, deviceID string) (net.Conn, error) {
	return &fakeConn{}, nil
}

type fakeConn struct {
	req []byte
	r   *bytes.Reader
}

func (c *fakeConn) Write(p []byte) (int, error) {
	c.req = append(c.req, p...)
	return len(p), nil
}

func (c *fakeConn) Read(p []byte) (int, error) {
	if c.r == nil {
		c.r = bytes.NewReader(c.response())
	}
	return c.r.Read(p)
}

// response routes by the request written so far. GET /api/v1/dev-containers is
// the stale snapshot (still lists the container); any single-container GET is
// the live probe (404 — the container is provably gone); DELETE succeeds so a
// concurrent StopDesktop-style call would also work through this fake.
func (c *fakeConn) response() []byte {
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(c.req)))
	if err != nil {
		return httpResponse("500 Internal Server Error", "", "")
	}
	switch {
	case req.Method == "GET" && req.URL.Path == "/api/v1/dev-containers":
		body := `{"containers":[{"session_id":"ses_x","container_id":"c1","container_name":"ubuntu-external-ses_x","ip_address":"10.0.0.9","status":"running"}]}`
		return httpResponse("200 OK", "application/json", body)
	case req.Method == "GET":
		return httpResponse("404 Not Found", "text/plain", "dev container not found")
	case req.Method == "DELETE":
		return httpResponse("200 OK", "application/json", `{"session_id":"ses_x","container_id":"c1","status":"stopped"}`)
	default:
		return httpResponse("404 Not Found", "text/plain", "not found")
	}
}

func (c *fakeConn) Close() error                  { return nil }
func (c *fakeConn) LocalAddr() net.Addr          { return fakeAddr{} }
func (c *fakeConn) RemoteAddr() net.Addr         { return fakeAddr{} }
func (c *fakeConn) Deadline() (time.Time, time.Time, error) {
	return time.Time{}, time.Time{}, nil
}
func (c *fakeConn) SetDeadline(t time.Time) error    { return nil }
func (c *fakeConn) SetReadDeadline(t time.Time) error { return nil }
func (c *fakeConn) SetWriteDeadline(t time.Time) error { return nil }

type fakeAddr struct{}

func (fakeAddr) Network() string { return "fake" }
func (fakeAddr) String() string  { return "fake" }

func httpResponse(status, contentType, body string) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "HTTP/1.1 %s\r\n", status)
	if contentType != "" {
		fmt.Fprintf(&b, "Content-Type: %s\r\n", contentType)
	}
	fmt.Fprintf(&b, "Content-Length: %d\r\n", len(body))
	b.WriteString("\r\n")
	b.WriteString(body)
	return b.Bytes()
}
