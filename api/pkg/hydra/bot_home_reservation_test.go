package hydra

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeDocker is a stateful Docker API on a unix socket. Container creation
// for names in blockCreate waits until the test releases it, so a test can
// hold one CreateDevContainer inside the window between the bot home
// ownership check and the container start.
type fakeDocker struct {
	mu          sync.Mutex
	containers  map[string]*fakeContainer
	creates     []string
	blockCreate map[string]chan struct{}
	createdCh   chan string
	// dropMounts creates containers without their bind mounts; failRemove
	// makes container removal fail.
	dropMounts bool
	failRemove bool
}

type fakeContainer struct {
	name    string
	running bool
	labels  map[string]string
	mounts  []map[string]string
}

func newFakeDocker(t *testing.T) (*fakeDocker, string) {
	t.Helper()
	f := &fakeDocker{containers: map[string]*fakeContainer{}, blockCreate: map[string]chan struct{}{}, createdCh: make(chan string, 8)}
	sock, err := os.CreateTemp("/tmp", "helix-hydra-bothome-*.sock")
	require.NoError(t, err)
	path := sock.Name()
	require.NoError(t, sock.Close())
	require.NoError(t, os.Remove(path))
	l, err := net.Listen("unix", path)
	require.NoError(t, err)
	srv := &http.Server{Handler: http.HandlerFunc(f.serve)}
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close(); _ = os.Remove(path) })
	return f, path
}

func (f *fakeDocker) serve(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	if i := strings.Index(p, "/containers"); i >= 0 {
		p = p[i:]
	}
	w.Header().Set("Content-Type", "application/json")
	switch {
	case strings.HasSuffix(r.URL.Path, "/_ping"):
		w.Header().Set("API-Version", "1.43")
		_, _ = w.Write([]byte("OK"))
	case r.Method == http.MethodGet && p == "/containers/json":
		f.list(w, r)
	case r.Method == http.MethodPost && p == "/containers/create":
		f.create(w, r)
	case r.Method == http.MethodPost && strings.HasSuffix(p, "/start"):
		f.mu.Lock()
		if c := f.containers[strings.TrimSuffix(strings.TrimPrefix(p, "/containers/"), "/start")]; c != nil {
			c.running = true
		}
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodGet && strings.HasSuffix(p, "/json"):
		f.inspect(w, strings.TrimSuffix(strings.TrimPrefix(p, "/containers/"), "/json"))
	case r.Method == http.MethodDelete && strings.HasPrefix(p, "/containers/"):
		if f.failRemove {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"device or resource busy"}`))
			return
		}
		f.mu.Lock()
		delete(f.containers, strings.TrimPrefix(p, "/containers/"))
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	default:
		// Networks and logs: accept so container creation proceeds.
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/networks/") {
			_, _ = w.Write([]byte(`{"Name":"helix-sandboxes","Id":"net"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}
}

func (f *fakeDocker) list(w http.ResponseWriter, r *http.Request) {
	var filters map[string]map[string]bool
	_ = json.Unmarshal([]byte(r.URL.Query().Get("filters")), &filters)
	all := r.URL.Query().Get("all") == "1" || r.URL.Query().Get("all") == "true"
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []map[string]any{}
	for _, c := range f.containers {
		if !c.running && !all {
			continue
		}
		match := true
		for want := range filters["label"] {
			k, v, hasV := strings.Cut(want, "=")
			got, ok := c.labels[k]
			if !ok || (hasV && got != v) {
				match = false
			}
		}
		if match {
			out = append(out, map[string]any{"Id": c.name, "Names": []string{"/" + c.name}, "Labels": c.labels})
		}
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (f *fakeDocker) create(w http.ResponseWriter, r *http.Request) {
	name, _ := url.QueryUnescape(r.URL.Query().Get("name"))
	var body struct {
		Labels     map[string]string
		HostConfig struct {
			Mounts []struct{ Source, Target string }
			Binds  []string
		}
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.mu.Lock()
	f.creates = append(f.creates, name)
	block := f.blockCreate[name]
	f.mu.Unlock()
	f.createdCh <- name
	if block != nil {
		<-block
	}
	c := &fakeContainer{name: name, labels: body.Labels}
	if f.dropMounts {
		body.HostConfig.Mounts, body.HostConfig.Binds = nil, nil
	}
	for _, m := range body.HostConfig.Mounts {
		c.mounts = append(c.mounts, map[string]string{"Type": "bind", "Source": m.Source, "Destination": m.Target})
	}
	for _, b := range body.HostConfig.Binds {
		parts := strings.Split(b, ":")
		if len(parts) >= 2 {
			c.mounts = append(c.mounts, map[string]string{"Type": "bind", "Source": parts[0], "Destination": parts[1]})
		}
	}
	f.mu.Lock()
	f.containers[name] = c
	f.mu.Unlock()
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"Id": name})
}

func (f *fakeDocker) inspect(w http.ResponseWriter, name string) {
	f.mu.Lock()
	c := f.containers[name]
	f.mu.Unlock()
	if c == nil {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"No such container: ` + name + `"}`))
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"Id": c.name, "Name": "/" + c.name,
		"State":           map[string]any{"Running": c.running, "Status": map[bool]string{true: "running", false: "created"}[c.running]},
		"Config":          map[string]any{"Labels": c.labels},
		"HostConfig":      map[string]any{"NetworkMode": "helix-sandboxes"},
		"NetworkSettings": map[string]any{"Networks": map[string]any{"helix-sandboxes": map[string]any{"IPAddress": "10.0.0.2"}}},
		"Mounts":          c.mounts,
	})
}

func (f *fakeDocker) createCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.creates)
}

func botHomeRequest(t *testing.T, sessionID, socket string, expect BotHomeExpectation) *CreateDevContainerRequest {
	t.Helper()
	home, err := BotHomeDir(testOrg, testBot)
	require.NoError(t, err)
	return &CreateDevContainerRequest{
		SessionID:           sessionID,
		Image:               "ubuntu:22.04",
		ContainerName:       "headless-external-" + strings.TrimPrefix(sessionID, "ses_"),
		ContainerType:       DevContainerTypeHeadless,
		DockerSocket:        socket,
		SkipImageValidation: true,
		Mounts:              []MountConfig{{Source: home, Destination: "/home/retro/work"}},
		BotHome:             &BotHomeSpec{OrganizationID: testOrg, BotID: testBot, Generation: testGen, Expect: expect},
	}
}

func newReservationManager(t *testing.T, socket string) *DevContainerManager {
	t.Helper()
	t.Setenv("HELIX_DISK_PRESSURE_ENABLED", "false")
	resetDiskPressureConfig()
	t.Cleanup(resetDiskPressureConfig)
	oldRuntime := sessionRuntimeBaseDir
	sessionRuntimeBaseDir = t.TempDir()
	t.Cleanup(func() { sessionRuntimeBaseDir = oldRuntime })
	return &DevContainerManager{
		containers:          make(map[string]*DevContainer),
		creatingContainers:  make(map[string]chan struct{}),
		defaultDockerSocket: socket,
	}
}

// Two sessions of the same bot start while the first is still between its
// ownership check and container start: only one may mount the home.
func TestBotHomeReservationAdmitsOneSessionPerHome(t *testing.T) {
	useTempWorkspaces(t)
	fd, socket := newFakeDocker(t)
	dm := newReservationManager(t, socket)
	_, err := prepareBotHome(&BotHomeSpec{OrganizationID: testOrg, BotID: testBot, Generation: testGen, Expect: BotHomeExpectNew}, "ses_seed")
	require.NoError(t, err)

	reqA := botHomeRequest(t, "ses_a", socket, BotHomeExpectExisting)
	reqB := botHomeRequest(t, "ses_b", socket, BotHomeExpectExisting)
	release := make(chan struct{})
	fd.blockCreate[reqA.ContainerName] = release

	errA := make(chan error, 1)
	go func() { _, err := dm.CreateDevContainer(context.Background(), reqA); errA <- err }()
	require.Equal(t, reqA.ContainerName, <-fd.createdCh, "A reaches container creation")

	errB := make(chan error, 1)
	go func() { _, err := dm.CreateDevContainer(context.Background(), reqB); errB <- err }()
	select {
	case name := <-fd.createdCh:
		t.Fatalf("second session %s reached container creation while the first held the home", name)
	case err := <-errB:
		t.Fatalf("second session returned before the first started: %v", err)
	case <-time.After(300 * time.Millisecond):
	}

	close(release)
	require.NoError(t, <-errA)
	err = <-errB
	require.ErrorIs(t, err, ErrBotHomeInUse)
	require.ErrorContains(t, err, "ses_a")
	require.Equal(t, 1, fd.createCount())
}

// An erase that arrives while a session is between its ownership check and
// container start waits, then refuses because the home is mounted; the
// mount source is never left to be recreated as an empty directory.
func TestBotHomeReservationBlocksDestroyUntilStartAndThenRefuses(t *testing.T) {
	useTempWorkspaces(t)
	fd, socket := newFakeDocker(t)
	dm := newReservationManager(t, socket)
	res, err := prepareBotHome(&BotHomeSpec{OrganizationID: testOrg, BotID: testBot, Generation: testGen, Expect: BotHomeExpectNew}, "ses_seed")
	require.NoError(t, err)
	writeFile(t, filepath.Join(res.Path, "canary.txt"), "keep-me", 0o600)

	reqA := botHomeRequest(t, "ses_a", socket, BotHomeExpectExisting)
	release := make(chan struct{})
	fd.blockCreate[reqA.ContainerName] = release
	errA := make(chan error, 1)
	go func() { _, err := dm.CreateDevContainer(context.Background(), reqA); errA <- err }()
	<-fd.createdCh

	destroyErr := make(chan error, 1)
	go func() {
		_, err := dm.DestroyBotHome(context.Background(), &DestroyBotHomeRequest{OrganizationID: testOrg, BotID: testBot, Generation: testGen, Mode: BotHomeRetire})
		destroyErr <- err
	}()
	select {
	case err := <-destroyErr:
		t.Fatalf("destroy ran while a start held the home: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	require.NoError(t, <-errA)
	require.True(t, errors.Is(<-destroyErr, ErrBotHomeInUse))
	require.Equal(t, "keep-me", readFile(t, filepath.Join(res.Path, "canary.txt")))
}

// The reaper takes the same reservation and re-checks use under it.
func TestReconcileOrphanBotHomesSkipsHomeMountedAfterSnapshot(t *testing.T) {
	useTempWorkspaces(t)
	res, err := prepareBotHome(&BotHomeSpec{OrganizationID: testOrg, BotID: "b-dead", Generation: "bh_d", Expect: BotHomeExpectNew}, "ses_dead")
	require.NoError(t, err)
	mounted := false
	inUse := func(string) bool { return mounted }
	reserve := func(string) func() {
		mounted = true // a start won the race between snapshot and removal
		return func() {}
	}
	reaped, _ := ReconcileOrphanBotHomes(map[string]bool{}, inUse, reserve, 0, time.Hour, false, time.Now())
	require.Empty(t, reaped)
	require.DirExists(t, res.Path)
}

// A container that does not really mount the home is removed; if removal
// fails the error says the container may still be running.
func TestBotHomeMountVerificationReportsRemoveFailure(t *testing.T) {
	for _, failRemove := range []bool{false, true} {
		useTempWorkspaces(t)
		fd, socket := newFakeDocker(t)
		fd.dropMounts = true
		fd.failRemove = failRemove
		dm := newReservationManager(t, socket)
		_, err := prepareBotHome(&BotHomeSpec{OrganizationID: testOrg, BotID: testBot, Generation: testGen, Expect: BotHomeExpectNew}, "ses_seed")
		require.NoError(t, err)

		_, err = dm.CreateDevContainer(context.Background(), botHomeRequest(t, "ses_a", socket, BotHomeExpectExisting))
		require.ErrorContains(t, err, "did not mount bot home")
		if failRemove {
			require.ErrorContains(t, err, "removing it failed (it may still be running)")
			require.ErrorContains(t, err, "device or resource busy")
		} else {
			require.ErrorContains(t, err, "removed it")
			require.NotContains(t, err.Error(), "removing it failed")
		}
	}
}
