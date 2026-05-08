package hydra

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
)

func TestSandboxExecGetRequiresMatchingSession(t *testing.T) {
	ops := NewSandboxOps(nil)
	ops.saveRecord(&SandboxCmdRecord{
		ID:        "sbcmd_1",
		SandboxID: "sbx_owner",
		Cmd:       "echo",
		Status:    CmdStatusFinished,
		StartedAt: time.Now(),
	})
	server := &Server{sandboxOps: ops}

	req := httptest.NewRequest(http.MethodGet, "/dev-containers/sbx_other/exec/sbcmd_1", nil)
	req = mux.SetURLVars(req, map[string]string{
		"session_id": "sbx_other",
		"cmd_id":     "sbcmd_1",
	})
	rec := httptest.NewRecorder()

	server.handleSandboxExecGet(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-sandbox command lookup, got %d", rec.Code)
	}
}

func TestSandboxKillCommandRequiresMatchingSession(t *testing.T) {
	ops := NewSandboxOps(nil)
	ops.saveRecord(&SandboxCmdRecord{
		ID:        "sbcmd_1",
		SandboxID: "sbx_owner",
		Cmd:       "sleep",
		Status:    CmdStatusRunning,
		StartedAt: time.Now(),
	})

	err := ops.KillCommand(context.Background(), "sbx_other", "sbcmd_1", "TERM")
	if !errors.Is(err, ErrSandboxCommandNotFound) {
		t.Fatalf("expected ErrSandboxCommandNotFound, got %v", err)
	}
}

func TestSandboxSignalNameValidation(t *testing.T) {
	for _, signal := range []string{"TERM", "KILL", "SIGTERM", "9"} {
		if !isSafeSignalName(signal) {
			t.Fatalf("expected signal %q to be accepted", signal)
		}
	}
	for _, signal := range []string{"term", "TERM;id", "TERM && id", "", "TOO-LONG-SIGNAL-NAME"} {
		if isSafeSignalName(signal) {
			t.Fatalf("expected signal %q to be rejected", signal)
		}
	}
}
