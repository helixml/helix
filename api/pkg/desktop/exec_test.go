package desktop

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExecAllowsGeneralSandboxCommands(t *testing.T) {
	server := NewServer(Config{}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/exec", strings.NewReader(`{"command":["sh","-c","printf general-command"]}`))
	server.handleExec(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("returned %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"output":"general-command"`) {
		t.Fatalf("unexpected response: %s", recorder.Body.String())
	}
}
