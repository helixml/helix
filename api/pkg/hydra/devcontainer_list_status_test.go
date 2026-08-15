package hydra

import (
	"context"
	"path/filepath"
	"testing"
)

// newListTestManager builds a DevContainerManager whose tracked containers point
// at a Docker socket that does not exist, so every ContainerInspect fails. That
// is the "cannot confirm" case the status contract is about.
func newListTestManager(containers map[string]*DevContainer) *DevContainerManager {
	return &DevContainerManager{containers: containers}
}

// The bug: dc.Status is only written when hydra itself stops a container or
// lazily by GetDevContainer, so a container whose entrypoint exited on its own
// keeps a cached "running" forever. ListDevContainers is the control plane's
// live-set, so that stale value resurrected dead sessions on every reconnect.
// A status that cannot be confirmed against Docker must be reported stopped.
func TestListDevContainers_UnconfirmableContainerIsNotReportedRunning(t *testing.T) {
	missingSocket := filepath.Join(t.TempDir(), "does-not-exist.sock")
	dm := newListTestManager(map[string]*DevContainer{
		"ses_dead": {
			SessionID:     "ses_dead",
			ContainerID:   "cid_dead",
			ContainerName: "headless-external-ses_dead",
			// Stale cached value, exactly as a self-exited container leaves it.
			Status:       DevContainerStatusRunning,
			IPAddress:    "10.213.0.5",
			DockerSocket: missingSocket,
		},
	})

	resp := dm.ListDevContainers(context.Background())

	if len(resp.Containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(resp.Containers))
	}
	if got := resp.Containers[0].Status; got != DevContainerStatusStopped {
		t.Errorf("unconfirmable container reported as %q, want %q", got, DevContainerStatusStopped)
	}
	// The cached entry must be corrected too, so a later GetDevContainer that
	// also fails to reach Docker doesn't hand back the stale "running".
	if got := dm.containers["ses_dead"].Status; got == DevContainerStatusRunning {
		t.Errorf("cached status left at %q after a failed inspect", got)
	}
}

// The same contract on the single-container path used by HasRunningContainer and
// the reconcile probe.
func TestGetDevContainer_UnconfirmableContainerIsNotReportedRunning(t *testing.T) {
	missingSocket := filepath.Join(t.TempDir(), "does-not-exist.sock")
	dm := newListTestManager(map[string]*DevContainer{
		"ses_dead": {
			SessionID:    "ses_dead",
			ContainerID:  "cid_dead",
			Status:       DevContainerStatusRunning,
			DockerSocket: missingSocket,
		},
	})

	resp, err := dm.GetDevContainer(context.Background(), "ses_dead")
	if err != nil {
		t.Fatalf("GetDevContainer returned error: %v", err)
	}
	if resp.Status != DevContainerStatusStopped {
		t.Errorf("unconfirmable container reported as %q, want %q", resp.Status, DevContainerStatusStopped)
	}
}

func TestGetDevContainer_UnknownSessionErrors(t *testing.T) {
	dm := newListTestManager(map[string]*DevContainer{})

	if _, err := dm.GetDevContainer(context.Background(), "ses_missing"); err == nil {
		t.Fatal("expected an error for an untracked session")
	}
}

func TestListDevContainers_EmptyManager(t *testing.T) {
	dm := newListTestManager(map[string]*DevContainer{})

	resp := dm.ListDevContainers(context.Background())
	if len(resp.Containers) != 0 {
		t.Fatalf("expected no containers, got %d", len(resp.Containers))
	}
}
