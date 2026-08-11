package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
)

// Embed keys are handed to an untrusted browser, so these tests are written from
// the attacker's side: given a key for task A / session A, what can it reach?

func embedUser() *types.User {
	return &types.User{
		ID:         "user_service",
		APIKeyType: types.APIkeytypeEmbed,
		SpecTaskID: "spt_A",
		SessionID:  "ses_A",
	}
}

func req(method, target string) *http.Request {
	return httptest.NewRequest(method, target, nil)
}

func TestEmbedKeyAllowsItsOwnTaskAndSession(t *testing.T) {
	u := embedUser()
	allowed := []struct{ method, path string }{
		{"GET", "/api/v1/config"},
		{"GET", "/api/v1/status"},
		{"GET", "/api/v1/auth/authenticated"},
		{"GET", "/api/v1/auth/user"},
		{"GET", "/api/v1/spec-tasks/spt_A"},
		{"GET", "/api/v1/spec-tasks/spt_A/execution-config"},
		{"GET", "/api/v1/spec-tasks/spt_A/zed-threads"},
		{"GET", "/api/v1/spec-tasks/spt_A/attachments"},
		{"GET", "/api/v1/sessions/ses_A"},
		{"GET", "/api/v1/sessions/ses_A/interactions"},
		{"GET", "/api/v1/sessions/ses_A/step-info"},
		{"POST", "/api/v1/sessions/ses_A/cancel"},
		{"POST", "/api/v1/sessions/chat"},
		{"GET", "/api/v1/external-agents/ses_A/file"},
	}
	for _, c := range allowed {
		if !embedKeyAllows(u, req(c.method, c.path)) {
			t.Errorf("should allow %s %s", c.method, c.path)
		}
	}
}

// The important one: another candidate's task or session must be unreachable.
func TestEmbedKeyCannotReachAnotherTaskOrSession(t *testing.T) {
	u := embedUser()
	denied := []struct{ method, path string }{
		{"GET", "/api/v1/spec-tasks/spt_B"},
		{"GET", "/api/v1/spec-tasks/spt_B/execution-config"},
		{"GET", "/api/v1/spec-tasks/spt_B/attachments"},
		{"GET", "/api/v1/sessions/ses_B"},
		{"GET", "/api/v1/sessions/ses_B/interactions"},
		{"POST", "/api/v1/sessions/ses_B/cancel"},
		{"GET", "/api/v1/external-agents/ses_B/file"},
	}
	for _, c := range denied {
		if embedKeyAllows(u, req(c.method, c.path)) {
			t.Errorf("SECURITY: embed key reached another tenant's %s %s", c.method, c.path)
		}
	}
}

// Enumeration: the list endpoint returns every task in the project, i.e. every
// other candidate's conversation. It must never be on the allowlist.
func TestEmbedKeyCannotEnumerate(t *testing.T) {
	u := embedUser()
	denied := []struct{ method, path string }{
		{"GET", "/api/v1/spec-tasks"},
		{"GET", "/api/v1/spec-tasks?project_id=prj_x"},
		{"GET", "/api/v1/agents"},
		{"GET", "/api/v1/organizations"},
		{"GET", "/api/v1/projects/prj_x"},
		{"GET", "/api/v1/projects/prj_x/repositories"},
		{"GET", "/api/v1/users/user_other"},
		{"GET", "/api/v1/apps"},
		{"GET", "/api/v1/api_keys"},
	}
	for _, c := range denied {
		if embedKeyAllows(u, req(c.method, c.path)) {
			t.Errorf("SECURITY: embed key could enumerate %s %s", c.method, c.path)
		}
	}
}

// An end user must not be able to reconfigure or re-point the agent run they
// are talking to, nor drive the desktop.
func TestEmbedKeyCannotMutateOrDriveDesktop(t *testing.T) {
	u := embedUser()
	denied := []struct{ method, path string }{
		{"PUT", "/api/v1/spec-tasks/spt_A"},
		{"PATCH", "/api/v1/spec-tasks/spt_A/execution-config"},
		{"PATCH", "/api/v1/spec-tasks/spt_A/archive"},
		{"POST", "/api/v1/spec-tasks/spt_A/start-planning"},
		{"POST", "/api/v1/spec-tasks/spt_A/clone"},
		{"DELETE", "/api/v1/sessions/ses_A/stop-external-agent"},
		{"POST", "/api/v1/sessions/ses_A/resume"},
		{"GET", "/api/v1/external-agents/ses_A/screenshot"},
		{"GET", "/api/v1/external-agents/ses_A/ws/stream"},
		{"POST", "/api/v1/external-agents/ses_A/clipboard"},
		{"GET", "/api/v1/sessions/ses_A/terminal"},
		{"GET", "/api/v1/external-agents/ses_A/workspace-files"},
	}
	for _, c := range denied {
		if embedKeyAllows(u, req(c.method, c.path)) {
			t.Errorf("SECURITY: embed key allowed %s %s", c.method, c.path)
		}
	}
}

// The websocket names its subject in a query param, so it needs its own bound.
func TestEmbedKeyWebsocketIsBoundToItsSession(t *testing.T) {
	u := embedUser()
	if !embedKeyAllows(u, req("GET", "/api/v1/ws/user?session_id=ses_A")) {
		t.Error("should allow its own session stream")
	}
	if embedKeyAllows(u, req("GET", "/api/v1/ws/user?session_id=ses_B")) {
		t.Error("SECURITY: embed key subscribed to another session's stream")
	}
}

// A key with nothing bound to it addresses nothing.
func TestEmbedKeyWithNoTaskIsInert(t *testing.T) {
	u := &types.User{APIKeyType: types.APIkeytypeEmbed}
	for _, p := range []string{"/api/v1/config", "/api/v1/spec-tasks/spt_A", "/api/v1/sessions/ses_A"} {
		if embedKeyAllows(u, req("GET", p)) {
			t.Errorf("SECURITY: unbound embed key reached %s", p)
		}
	}
}

// Path-shape tricks must not slip past the single-segment rule.
func TestEmbedKeyRejectsPathTraversalShapes(t *testing.T) {
	u := embedUser()
	denied := []string{
		"/api/v1/spec-tasks/spt_A/../spt_B",
		"/api/v1/spec-tasks/spt_B/attachments/../../spt_A",
		"/api/v1/sessions/ses_A/interactions/extra",
		"/api/v1/spec-tasks/spt_A/zed-threads/nested",
	}
	for _, p := range denied {
		if embedKeyAllows(u, req("GET", p)) {
			t.Errorf("SECURITY: embed key allowed odd path %s", p)
		}
	}
}
