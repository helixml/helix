package api

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Expectation building blocks. Every row lists the actors it cares about;
// strangers() adds the four that must never reach organization A's data.

// strangers: another tenant, a user with no organization, an account still
// waiting for approval, and no credential.
func strangers(e expect) expect {
	out := expect{bOwner: deny, outsider: deny, waitlisted: deny, anonymous: deny}
	for k, v := range e {
		out[k] = v
	}
	return out
}

func tamper(field string) map[string]string {
	return map[string]string{field: tamperMarker}
}

// TestAuthzMatrix runs every table against one fixture. Build cost is paid
// once; each table is its own subtest so `-run 'TestAuthzMatrix/projects'`
// narrows a run.
func TestAuthzMatrix(t *testing.T) {
	requireAuthzServer(t)
	f := newAuthzFixture(t)

	t.Run("projects", func(t *testing.T) { runRoutes(t, f, projectRoutes()) })
	t.Run("sessions", func(t *testing.T) { runRoutes(t, f, sessionRoutes()) })
	t.Run("agents", func(t *testing.T) { runRoutes(t, f, agentRoutes()) })
	t.Run("repositories", func(t *testing.T) { runRoutes(t, f, repositoryRoutes()) })
	t.Run("git-smart-http", func(t *testing.T) { runRoutes(t, f, gitSmartHTTPRoutes()) })
	t.Run("git-cli", func(t *testing.T) { gitCLIChecks(t, f) })
	t.Run("spec-tasks", func(t *testing.T) { runRoutes(t, f, specTaskRoutes()) })
	t.Run("secrets", func(t *testing.T) { runRoutes(t, f, append(secretRoutes(), secretListRoutes()...)) })
	t.Run("knowledge", func(t *testing.T) { runRoutes(t, f, knowledgeRoutes()) })
	t.Run("triggers", func(t *testing.T) { runRoutes(t, f, triggerRoutes()) })
	t.Run("providers", func(t *testing.T) { runRoutes(t, f, providerRoutes()) })
	t.Run("artifacts", func(t *testing.T) { runRoutes(t, f, artifactRoutes()) })
	t.Run("organizations", func(t *testing.T) { runRoutes(t, f, organizationRoutes()) })
	t.Run("helix-org", func(t *testing.T) { runRoutes(t, f, helixOrgRoutes()) })
	t.Run("keys-and-admin", func(t *testing.T) { runRoutes(t, f, keyAndAdminRoutes()) })
	t.Run("filestore", func(t *testing.T) { runRoutes(t, f, filestoreRoutes()) })
	t.Run("external-agents", func(t *testing.T) { runRoutes(t, f, externalAgentRoutes()) })
	t.Run("zed", func(t *testing.T) { runRoutes(t, f, zedRoutes()) })
	t.Run("mcp", func(t *testing.T) { runRoutes(t, f, mcpRoutes()) })
	t.Run("llm-proxy", func(t *testing.T) { runRoutes(t, f, llmProxyRoutes()) })
	t.Run("lists", func(t *testing.T) { runListRows(t, f, listRows()) })
	t.Run("organizations-destructive", func(t *testing.T) { runRoutes(t, f, organizationDestructiveRoutes()) })
}

func projectRoutes() []route {
	return []route{
		{
			Method: http.MethodGet, Path: "/api/v1/projects/{projA}",
			Expect: strangers(expect{aOwner: allow, aOwnerOrgKey: allow, aMember: allow, projKey: allow, admin: allow, appKey: deny}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/projects/{projA2}",
			Expect: strangers(expect{aOwner: allow, aMember: deny, projKey: deny}),
		},
		{
			Method: http.MethodPut, Path: "/api/v1/projects/{projA}", Body: tamper("description"),
			Expect: strangers(expect{aOwner: allow, aMember: deny}),
			Fresh:  freshProject, Check: "/api/v1/projects/{projA}",
		},
		{
			Method: http.MethodPut, Path: "/api/v1/projects/{projA2}", Body: tamper("description"),
			Expect: expect{aOwner: allow, projKey: deny},
			Fresh:  freshProject2, Check: "/api/v1/projects/{projA2}",
		},
		{
			Method: http.MethodDelete, Path: "/api/v1/projects/{projA}",
			Expect: strangers(expect{aOwner: allow, aMember: deny}),
			Fresh:  freshProject, Check: "/api/v1/projects/{projA}",
		},
		{
			Method: http.MethodDelete, Path: "/api/v1/projects/{projA2}",
			Expect: expect{aOwner: allow, projKey: deny},
			Fresh:  freshProject2, Check: "/api/v1/projects/{projA2}",
		},
		{
			Method: http.MethodGet, Path: "/api/v1/projects/{projA}/repositories",
			Expect: strangers(expect{aOwner: allow, aMember: allow, projKey: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/projects/{projA2}/repositories",
			Expect: expect{aOwner: allow, projKey: deny},
		},
		{
			Method: http.MethodGet, Path: "/api/v1/projects/{projA}/access-grants",
			Expect: strangers(expect{aOwner: allow, projKey: deny}),
		},
		{
			Method: http.MethodPost, Path: "/api/v1/projects/{projA}/access-grants",
			Body:   map[string]any{"user_reference": "{aMemberID}", "roles": []string{"admin"}},
			Expect: strangers(expect{aOwner: allow, aMember: deny, projKey: deny}),
			Fresh:  freshProject,
		},
		{
			Method: http.MethodDelete, Path: "/api/v1/projects/{projA}/access-grants/{grantProjA}",
			Expect: strangers(expect{aOwner: allow, aMember: deny, projKey: deny}),
			Fresh:  freshProjectGrant, Check: "/api/v1/projects/{projA}/access-grants",
		},
		{
			Method: http.MethodGet, Path: "/api/v1/projects/{projA}/secrets",
			Expect: strangers(expect{aOwner: allow, projKey: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/projects/{projA2}/secrets",
			Expect: expect{aOwner: allow, projKey: deny},
		},
		{
			Method: http.MethodPost, Path: "/api/v1/projects/{projA}/secrets",
			Body:   map[string]string{"name": "AUTHZ_NEW", "value": "v"},
			Expect: strangers(expect{aOwner: allow, aMember: deny}),
			Fresh:  freshProject,
		},
		{
			Method: http.MethodPost, Path: "/api/v1/projects/{projA2}/secrets",
			Body:   map[string]string{"name": "AUTHZ_NEW", "value": "v"},
			Expect: expect{aOwner: allow, projKey: deny},
			Fresh:  freshProject2,
		},
		{
			Method: http.MethodGet, Path: "/api/v1/projects/{projA}/audit-logs",
			Expect: strangers(expect{aOwner: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/projects/{projA}/usage",
			Expect: strangers(expect{aOwner: allow, aMember: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/projects/{projA}/guidelines-history",
			Expect: strangers(expect{aOwner: allow, aMember: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/projects/{projA}/spec-task-agents",
			Expect: strangers(expect{aOwner: allow, aMember: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/projects/{projA}/web-service",
			Expect: strangers(expect{aOwner: allow, aMember: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/projects/{projA}/tasks-progress",
			Expect: strangers(expect{aOwner: allow, aMember: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/projects/{projA}/labels",
			Expect: strangers(expect{aOwner: allow, aMember: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/projects/{projA}/exploratory-session",
			Expect: strangers(expect{aOwner: reached}),
		},
		{
			Method: http.MethodPost, Path: "/api/v1/projects/{projA}/move/preview",
			Body:   map[string]string{"organization_id": "{orgB}"},
			Expect: strangers(expect{aOwner: reached, aMember: deny}),
		},
		{
			Method: http.MethodPost, Path: "/api/v1/projects/{projA}/pin",
			Expect: strangers(expect{aOwner: allow}),
		},
	}
}

func sessionRoutes() []route {
	return []route{
		{
			Method: http.MethodGet, Path: "/api/v1/sessions/{sessA}",
			Expect: strangers(expect{aOwner: allow, aOwnerOrgKey: allow, aMember: allow, projKey: allow, admin: allow, appKey: deny}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/sessions/{sessA2}",
			Expect: strangers(expect{aOwner: allow, aMember: deny, projKey: deny}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/sessions/{sessChat}",
			Expect: strangers(expect{aOwner: allow, sessKey: allow, aMember: deny, projKey: deny}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/sessions/{sessOther}",
			Expect: expect{aOwner: allow, sessKey: deny},
			Skip:   "tracked: a session key minted without a project still reaches every other session of its owner",
		},
		{
			Method: http.MethodPut, Path: "/api/v1/sessions/{sessA}", Body: tamper("name"),
			Expect: strangers(expect{aOwner: allow, aMember: deny}),
			Fresh:  freshSession, Check: "/api/v1/sessions/{sessA}",
		},
		{
			Method: http.MethodPut, Path: "/api/v1/sessions/{sessA2}", Body: tamper("name"),
			Expect: expect{aOwner: allow, projKey: deny},
			Fresh:  freshSession2, Check: "/api/v1/sessions/{sessA2}",
		},
		{
			Method: http.MethodDelete, Path: "/api/v1/sessions/{sessA}",
			Expect: strangers(expect{aOwner: allow, aMember: deny}),
			Fresh:  freshSession, Check: "/api/v1/sessions/{sessA}",
		},
		{
			Method: http.MethodDelete, Path: "/api/v1/sessions/{sessA2}",
			Expect: expect{aOwner: allow, projKey: deny},
			Fresh:  freshSession2, Check: "/api/v1/sessions/{sessA2}",
		},
		{
			Method: http.MethodDelete, Path: "/api/v1/sessions/{sessChat}",
			Expect: expect{aOwner: allow, projKey: deny, bOwner: deny},
			Fresh:  freshPersonalSession, Check: "/api/v1/sessions/{sessChat}",
		},
		{
			Method: http.MethodGet, Path: "/api/v1/sessions/{sessA}/interactions",
			Expect: strangers(expect{aOwner: allow, aMember: allow, projKey: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/sessions/{sessA2}/interactions",
			Expect: expect{aOwner: allow, projKey: deny},
		},
		{
			Method: http.MethodGet, Path: "/api/v1/sessions/{sessA}/usage",
			Expect: strangers(expect{aOwner: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/sessions/{sessA}/step-info",
			Expect: strangers(expect{aOwner: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/sessions/{sessA}/toc",
			Expect: strangers(expect{aOwner: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/sessions/{sessA}/search?q=authz",
			Expect: strangers(expect{aOwner: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/sessions/{sessA}/execution-config",
			Expect: strangers(expect{aOwner: reached}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/sessions/{sessA}/workspace-status",
			Expect: strangers(expect{aOwner: reached}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/sessions/{sessA}/output",
			Expect: strangers(expect{aOwner: reached}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/sessions/{sessA}/sandbox-state",
			Expect: strangers(expect{aOwner: reached}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/sessions/{sessA}/operator-access",
			Expect: strangers(expect{aOwner: reached}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/sessions/{sessA}/rdp-connection",
			Expect:        strangers(expect{aOwner: reached}),
			ReachedMarker: "Streaming not available",
		},
		{
			Method: http.MethodGet, Path: "/api/v1/sessions/{sessA}/zed-config",
			Expect: strangers(expect{aOwner: reached}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/sessions/{sessA}/zed-settings",
			Expect: strangers(expect{aOwner: reached}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/sessions/{sessA}/claude-credentials",
			Expect:        strangers(expect{aOwner: reached, aMember: deny}),
			ReachedMarker: "no Claude subscription found",
		},
		{
			Method: http.MethodGet, Path: "/api/v1/sessions/{sessA}/codex-credentials",
			Expect:        strangers(expect{aOwner: reached, aMember: deny}),
			ReachedMarker: "no Codex subscription found",
		},
		{
			Method: http.MethodGet, Path: "/api/v1/sessions/{sessA}/preview-tokens",
			Expect: strangers(expect{aOwner: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/sessions/{sessA}/terminal/sessions",
			Expect: strangers(expect{aOwner: reached}),
		},
		{
			Method: http.MethodPost, Path: "/api/v1/sessions/{sessA}/cancel",
			Expect: strangers(expect{aOwner: reached, aMember: deny}),
		},
	}
}

func agentRoutes() []route {
	return []route{
		{
			Method: http.MethodGet, Path: "/api/v1/apps/{appA}",
			Expect: strangers(expect{aOwner: allow, aOwnerOrgKey: allow, aMember: allow, admin: allow, projKey: deny, appKey: deny}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/agents/{orgAgentA}",
			Expect: strangers(expect{aOwner: allow, projKey: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/apps/{appPersonal}",
			Expect: strangers(expect{aOwner: allow, aMember: deny, projKey: deny}),
		},
		{
			Method: http.MethodPut, Path: "/api/v1/apps/{appA}",
			Body:   map[string]any{"organization_id": "{orgA}", "config": map[string]any{"helix": map[string]any{"name": tamperMarker}}},
			Expect: strangers(expect{aOwner: allow, aMember: deny, projKey: deny}),
			Fresh:  freshApp, Check: "/api/v1/apps/{appA}",
		},
		{
			Method: http.MethodDelete, Path: "/api/v1/apps/{appA}",
			Expect: strangers(expect{aOwner: allow, aMember: deny, projKey: deny}),
			Fresh:  freshApp, Check: "/api/v1/apps/{appA}",
		},
		{
			Method: http.MethodGet, Path: "/api/v1/apps/{appA}/access-grants",
			Expect: strangers(expect{aOwner: allow, projKey: deny}),
		},
		{
			Method: http.MethodPost, Path: "/api/v1/apps/{appA}/access-grants",
			Body:   map[string]any{"user_reference": "{aMemberID}", "roles": []string{"admin"}},
			Expect: strangers(expect{aOwner: allow, aMember: deny, projKey: deny}),
			Fresh:  freshApp,
		},
		{
			Method: http.MethodDelete, Path: "/api/v1/apps/{appA}/access-grants/{grantAppA}",
			Expect: strangers(expect{aOwner: allow, aMember: deny, projKey: deny}),
			Fresh:  freshAppGrant, Check: "/api/v1/apps/{appA}/access-grants",
		},
		{
			// Answers "what may I do with this agent"; a stranger is told
			// nothing beyond can_read=false.
			Method: http.MethodGet, Path: "/api/v1/apps/{appA}/user-access",
			Expect: expect{aOwner: allow, aMember: allow, bOwner: allow, waitlisted: deny, anonymous: deny},
		},
		{
			Method: http.MethodGet, Path: "/api/v1/apps/{appA}/daily-usage",
			Expect: strangers(expect{aOwner: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/apps/{appA}/llm-calls",
			Expect: strangers(expect{aOwner: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/apps/{appA}/interactions",
			Expect: strangers(expect{aOwner: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/apps/{appA}/memories",
			Expect: strangers(expect{aOwner: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/apps/{appA}/triggers",
			Expect: strangers(expect{aOwner: allow, aMember: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/apps/{appA}/evaluation-suites",
			Expect: strangers(expect{aOwner: allow}),
		},
		{
			Method: http.MethodPost, Path: "/api/v1/apps/{appA}/duplicate",
			Expect: strangers(expect{aOwner: allow, projKey: deny}),
		},
	}
}

func repositoryRoutes() []route {
	return []route{
		{
			Method: http.MethodGet, Path: "/api/v1/git/repositories/{repoA}",
			Expect: strangers(expect{aOwner: allow, aOwnerOrgKey: allow, aMember: allow, projKey: allow, admin: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/git/repositories/{repoA2}",
			Expect: strangers(expect{aOwner: allow, aMember: deny, projKey: deny}),
		},
		{
			Method: http.MethodPut, Path: "/api/v1/git/repositories/{repoA}", Body: tamper("description"),
			Expect: strangers(expect{aOwner: allow, aMember: deny}),
			Fresh:  freshRepo, Check: "/api/v1/git/repositories/{repoA}",
		},
		{
			Method: http.MethodPut, Path: "/api/v1/git/repositories/{repoA2}", Body: tamper("description"),
			Expect: expect{aOwner: allow, projKey: deny},
			Fresh:  freshRepo2, Check: "/api/v1/git/repositories/{repoA2}",
		},
		{
			Method: http.MethodDelete, Path: "/api/v1/git/repositories/{repoA}",
			Expect: strangers(expect{aOwner: allow, aMember: deny}),
			Fresh:  freshRepo, Check: "/api/v1/git/repositories/{repoA}",
		},
		{
			Method: http.MethodGet, Path: "/api/v1/git/repositories/{repoA}/branches",
			Expect: strangers(expect{aOwner: allow, aMember: allow, projKey: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/git/repositories/{repoA2}/branches",
			Expect: expect{aOwner: allow, projKey: deny},
		},
		{
			Method: http.MethodGet, Path: "/api/v1/git/repositories/{repoA}/tree?path=.&branch=main",
			Expect: strangers(expect{aOwner: allow, aMember: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/git/repositories/{repoA}/contents?path=README.md&branch=main",
			Expect: strangers(expect{aOwner: allow, aMember: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/git/repositories/{repoA2}/contents?path=README.md&branch=main",
			Expect: expect{aOwner: allow, projKey: deny},
		},
		{
			Method: http.MethodPut, Path: "/api/v1/git/repositories/{repoA}/contents",
			Body:   map[string]string{"path": "authz.txt", "branch": "main", "message": "authz", "content": tamperMarker, "author": "a", "email": "a@example.com"},
			Expect: strangers(expect{aOwner: allow, aMember: deny}),
			Fresh:  freshRepo,
		},
		{
			Method: http.MethodGet, Path: "/api/v1/git/repositories/{repoA}/commits?branch=main",
			Expect: strangers(expect{aOwner: allow, aMember: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/git/repositories/{repoA}/clone-command",
			Expect: strangers(expect{aOwner: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/git/repositories/{repoA}/access-grants",
			Expect: strangers(expect{aOwner: allow, projKey: deny}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/git/repositories/{repoA}/pull-requests",
			Expect: strangers(expect{aOwner: reached}),
		},
	}
}

// gitSmartHTTPRoutes covers the git protocol endpoints, which do their own
// basic-auth authentication outside the API router.
func gitSmartHTTPRoutes() []route {
	basic := map[string]string{basicAuthHeader: "1"}
	return []route{
		{
			Method: http.MethodGet, Path: "/git/{repoA}/info/refs?service=git-upload-pack", Header: basic,
			Expect: strangers(expect{aOwner: allow, aOwnerOrgKey: allow, aMember: allow, projKey: allow}),
		},
		{
			Method: http.MethodGet, Path: "/git/{repoA2}/info/refs?service=git-upload-pack", Header: basic,
			Expect: strangers(expect{aOwner: allow, aMember: deny, projKey: deny}),
		},
		{
			Method: http.MethodGet, Path: "/git/{repoA}/info/refs?service=git-receive-pack", Header: basic,
			Expect: strangers(expect{aOwner: allow, aMember: deny}),
		},
		{
			Method: http.MethodGet, Path: "/git/{repoA2}/info/refs?service=git-receive-pack", Header: basic,
			Expect: expect{aOwner: allow, projKey: deny},
		},
		{
			Method: http.MethodGet, Path: "/git/{repoA}/clone-info", Header: basic,
			Expect: strangers(expect{aOwner: allow}),
		},
		{
			Method: http.MethodGet, Path: "/git/{repoA}/status", Header: basic,
			Expect: strangers(expect{aOwner: allow}),
		},
	}
}

func specTaskRoutes() []route {
	return []route{
		{
			Method: http.MethodGet, Path: "/api/v1/spec-tasks/{taskA}",
			Expect: strangers(expect{aOwner: allow, aOwnerOrgKey: allow, aMember: allow, projKey: allow, admin: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/spec-tasks/{taskA2}",
			Expect: strangers(expect{aOwner: allow, aMember: deny, projKey: deny}),
		},
		{
			Method: http.MethodPut, Path: "/api/v1/spec-tasks/{taskA}", Body: tamper("description"),
			Expect: strangers(expect{aOwner: allow, aMember: deny}),
			Fresh:  freshTask, Check: "/api/v1/spec-tasks/{taskA}",
		},
		{
			Method: http.MethodPut, Path: "/api/v1/spec-tasks/{taskA2}", Body: tamper("description"),
			Expect: expect{aOwner: allow, projKey: deny},
			Fresh:  freshTask2, Check: "/api/v1/spec-tasks/{taskA2}",
		},
		{
			Method: http.MethodDelete, Path: "/api/v1/spec-tasks/{taskA}",
			Expect: strangers(expect{aOwner: allow, aMember: deny}),
			Fresh:  freshArchivedTask, Check: "/api/v1/spec-tasks/{taskA}",
		},
		{
			Method: http.MethodDelete, Path: "/api/v1/spec-tasks/{taskA2}",
			Expect: expect{aOwner: allow, projKey: deny},
			Fresh:  freshArchivedTask2, Check: "/api/v1/spec-tasks/{taskA2}",
		},
		{
			Method: http.MethodGet, Path: "/api/v1/spec-tasks/{taskA}/specs",
			Expect: strangers(expect{aOwner: allow, aMember: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/spec-tasks/{taskA}/progress",
			Expect: strangers(expect{aOwner: reached}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/spec-tasks/{taskA}/execution-config",
			Expect: strangers(expect{aOwner: reached}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/spec-tasks/{taskA}/attachments",
			Expect: strangers(expect{aOwner: allow, aMember: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/spec-tasks/{taskA}/design-reviews",
			Expect: strangers(expect{aOwner: allow, aMember: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/spec-tasks/{taskA}/pr-proposals",
			Expect: strangers(expect{aOwner: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/spec-tasks/{taskA}/zed-threads",
			Expect: strangers(expect{aOwner: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/spec-tasks/{taskA}/clone-groups",
			Expect: strangers(expect{aOwner: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/spec-tasks/{taskA}/usage",
			Expect: strangers(expect{aOwner: allow}),
		},
		{
			Method: http.MethodPost, Path: "/api/v1/spec-tasks/{taskA}/labels",
			Body:   map[string]string{"label": "authz"},
			Expect: strangers(expect{aOwner: allow, aMember: deny}),
			Fresh:  freshTask,
		},
		{
			Method: http.MethodPatch, Path: "/api/v1/spec-tasks/{taskA}/archive",
			Body:   map[string]bool{"archived": true},
			Expect: strangers(expect{aOwner: allow, aMember: deny}),
			Fresh:  freshTask,
		},
		{
			Method: http.MethodPost, Path: "/api/v1/spec-tasks/{taskBacklog}/approve-implementation",
			Expect:        strangers(expect{aOwner: reached, aMember: deny}),
			ReachedMarker: "unexpected status",
		},
		{
			// Approving its own work is a human decision, as it is for pull
			// request proposals and task completion.
			Method: http.MethodPost, Path: "/api/v1/spec-tasks/{taskBacklog}/approve-implementation",
			Expect: expect{aOwner: reached, projKey: deny},
			Skip:   "tracked: a project-scoped agent key can approve its own spec task's implementation",
		},
	}
}

func secretRoutes() []route {
	return []route{
		{
			Method: http.MethodPut, Path: "/api/v1/secrets/{secretProj}", Body: tamper("name"),
			Expect: strangers(expect{aOwner: allow, aMember: deny}),
			Fresh:  freshProjectSecret, Check: "/api/v1/projects/{projA}/secrets", CheckMarker: "secretProj",
		},
		{
			Method: http.MethodPut, Path: "/api/v1/secrets/{secretProj2}", Body: tamper("name"),
			Expect: expect{aOwner: allow, projKey: deny},
			Fresh:  freshProjectSecret2, Check: "/api/v1/projects/{projA2}/secrets", CheckMarker: "secretProj2",
		},
		{
			Method: http.MethodPut, Path: "/api/v1/secrets/{secretPersonal}", Body: tamper("name"),
			Expect: strangers(expect{aOwner: allow, aMember: deny, projKey: deny}),
			Fresh:  freshPersonalSecret, Check: "/api/v1/secrets", CheckMarker: "secretPersonal",
		},
		{
			Method: http.MethodDelete, Path: "/api/v1/secrets/{secretProj}",
			Expect: strangers(expect{aOwner: allow, aMember: deny}),
			Fresh:  freshProjectSecret, Check: "/api/v1/projects/{projA}/secrets", CheckMarker: "secretProj",
		},
		{
			Method: http.MethodDelete, Path: "/api/v1/secrets/{secretProj2}",
			Expect: expect{aOwner: allow, projKey: deny},
			Fresh:  freshProjectSecret2, Check: "/api/v1/projects/{projA2}/secrets", CheckMarker: "secretProj2",
		},
		{
			Method: http.MethodDelete, Path: "/api/v1/secrets/{secretPersonal}",
			Expect: strangers(expect{aOwner: allow, aMember: deny, projKey: deny}),
			Fresh:  freshPersonalSecret, Check: "/api/v1/secrets", CheckMarker: "secretPersonal",
		},
	}
}

func secretListRoutes() []route {
	return []route{
		{
			Method: http.MethodGet, Path: "/api/v1/secrets?organization_id={orgA}",
			Expect: strangers(expect{aOwner: allow, aMember: allow}),
		},
	}
}

func knowledgeRoutes() []route {
	return []route{
		{
			// Knowledge by id is its owner's alone; readers of the agent see
			// it through GET /knowledge?app_id=.
			Method: http.MethodGet, Path: "/api/v1/knowledge/{knowledgeA}",
			Expect: strangers(expect{aOwner: allow, aMember: deny, projKey: deny}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/knowledge/{knowledgeA}/versions",
			Expect: strangers(expect{aOwner: allow, aMember: deny, projKey: deny}),
		},
		{
			Method: http.MethodDelete, Path: "/api/v1/knowledge/{knowledgeA}",
			Expect: strangers(expect{aOwner: allow, aMember: deny, projKey: deny}),
			Fresh:  freshKnowledge, Check: "/api/v1/knowledge/{knowledgeA}",
		},
		{
			Method: http.MethodPost, Path: "/api/v1/knowledge/{knowledgeA}/refresh",
			Expect: strangers(expect{aOwner: reached, aMember: deny, projKey: deny}),
		},
	}
}

func triggerRoutes() []route {
	return []route{
		{
			Method: http.MethodPut, Path: "/api/v1/triggers/{triggerA}",
			Body:   map[string]any{"app_id": "{appA}", "name": tamperMarker, "trigger": map[string]any{"cron": map[string]any{"enabled": true, "schedule": "0 0 1 1 *"}}},
			Expect: strangers(expect{aOwner: allow, aMember: deny, projKey: deny}),
			Fresh:  freshTrigger, Check: "/api/v1/apps/{appA}/triggers", CheckMarker: "triggerA",
		},
		{
			Method: http.MethodDelete, Path: "/api/v1/triggers/{triggerA}",
			Expect: strangers(expect{aOwner: allow, aMember: deny, projKey: deny}),
			Fresh:  freshTrigger, Check: "/api/v1/apps/{appA}/triggers", CheckMarker: "triggerA",
		},
		{
			Method: http.MethodGet, Path: "/api/v1/triggers/{triggerA}/executions",
			Expect: strangers(expect{aOwner: allow, aMember: deny, projKey: deny}),
		},
		{
			// The legacy webhook is public; only the trigger's own secret
			// authenticates it, never a user's API key.
			Method: http.MethodPost, Path: "/api/v1/webhooks/{webhookTriggerA}?token={webhookSecretA}", Body: map[string]string{},
			Expect: expect{anonymous: reached},
		},
		{
			// No user credential stands in for the secret, not even the
			// trigger owner's or an admin's.
			Method: http.MethodPost, Path: "/api/v1/webhooks/{webhookTriggerA}?token=wrong", Body: map[string]string{},
			Expect:          expect{aOwner: deny, bOwner: deny, anonymous: deny, admin: deny},
			PositiveControl: "the ?token={webhookSecretA} row above",
		},
		{
			Method: http.MethodPost, Path: "/api/v1/triggers",
			Body:   map[string]any{"app_id": "{appA}", "name": "authz", "trigger": map[string]any{"cron": map[string]any{"enabled": false, "schedule": "0 0 1 1 *"}}},
			Expect: strangers(expect{aOwner: allow, projKey: deny}),
		},
	}
}

func providerRoutes() []route {
	return []route{
		{
			Method: http.MethodPut, Path: "/api/v1/provider-endpoints/{providerA}",
			Body:   map[string]any{"name": tamperMarker, "base_url": "https://evil.authz-matrix.invalid/v1", "endpoint_type": "org", "owner_type": "org", "owner": "{orgA}"},
			Expect: strangers(expect{aOwner: allow, aMember: deny, projKey: deny}),
			Fresh:  freshOrgProvider,
		},
		{
			Method: http.MethodDelete, Path: "/api/v1/provider-endpoints/{providerA}",
			Expect: strangers(expect{aOwner: allow, aMember: deny, projKey: deny}),
			Fresh:  freshOrgProvider,
		},
		{
			Method: http.MethodDelete, Path: "/api/v1/provider-endpoints/{providerPersonal}",
			Expect: strangers(expect{aOwner: allow, aMember: deny, projKey: deny}),
			Fresh:  freshPersonalProvider,
		},
		{
			Method: http.MethodGet, Path: "/api/v1/provider-endpoints/{providerA}/daily-usage",
			Expect: strangers(expect{aOwner: allow, aMember: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/provider-endpoints/{providerPersonal}/users-daily-usage",
			Expect: strangers(expect{aOwner: allow, aMember: deny}),
		},
	}
}

func artifactRoutes() []route {
	return []route{
		{
			Method: http.MethodGet, Path: "/api/v1/artifacts/{artifactA}",
			Expect: strangers(expect{aOwner: allow, aMember: allow, projKey: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/artifacts/{artifactA2}",
			Expect: strangers(expect{aOwner: allow, aMember: deny, projKey: deny}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/artifacts/{artifactA}/versions",
			Expect: strangers(expect{aOwner: allow, aMember: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/artifacts/{artifactA}/download",
			Expect: strangers(expect{aOwner: allow, aMember: allow, projKey: allow}),
		},
		{
			Method: http.MethodDelete, Path: "/api/v1/artifacts/{artifactA}",
			Expect: strangers(expect{aOwner: allow, aMember: deny}),
			Fresh:  freshArtifact, Check: "/api/v1/artifacts/{artifactA}",
		},
		{
			Method: http.MethodDelete, Path: "/api/v1/artifacts/{artifactA2}",
			Expect: expect{aOwner: allow, projKey: deny},
			Fresh:  freshArtifact2, Check: "/api/v1/artifacts/{artifactA2}",
		},
		{
			Method: http.MethodGet, Path: "/api/v1/projects/{projA}/artifacts",
			Expect: strangers(expect{aOwner: allow, aMember: allow, projKey: allow}),
		},
	}
}

func organizationRoutes() []route {
	return []route{
		{
			Method: http.MethodGet, Path: "/api/v1/organizations/{orgA}",
			Expect: strangers(expect{aOwner: allow, aOwnerOrgKey: allow, aMember: allow, admin: allow}),
		},
		{
			Method: http.MethodPut, Path: "/api/v1/organizations/{orgA}",
			Body:   map[string]string{"display_name": tamperMarker},
			Expect: strangers(expect{aOwner: allow, aMember: deny, projKey: deny, sessKey: deny, workerKey: deny}),
			Fresh:  freshOrg, Check: "/api/v1/organizations/{orgA}",
		},
		{
			Method: http.MethodGet, Path: "/api/v1/organizations/{orgA}/members",
			Expect: strangers(expect{aOwner: allow, aMember: allow}),
		},
		{
			Method: http.MethodPost, Path: "/api/v1/organizations/{orgA}/members",
			Body:   map[string]string{"user_reference": "{bOwnerID}", "role": "owner"},
			Expect: strangers(expect{aOwner: allow, aMember: deny, projKey: deny, sessKey: deny}),
			Fresh:  freshOrg,
		},
		{
			Method: http.MethodPut, Path: "/api/v1/organizations/{orgA}/members/{aMemberID}",
			Body:   map[string]string{"role": "owner"},
			Expect: strangers(expect{aOwner: allow, aMember: deny, projKey: deny, sessKey: deny}),
			Fresh:  freshOrgWithMember,
		},
		{
			Method: http.MethodGet, Path: "/api/v1/organizations/{orgA}/invitations",
			Expect: strangers(expect{aOwner: allow, aMember: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/organizations/{orgA}/teams",
			Expect: strangers(expect{aOwner: allow, aMember: allow}),
		},
		{
			Method: http.MethodPost, Path: "/api/v1/organizations/{orgA}/teams",
			Body:   map[string]string{"name": "authz-team"},
			Expect: strangers(expect{aOwner: allow, aMember: deny, projKey: deny}),
			Fresh:  freshOrg,
		},
		{
			Method: http.MethodGet, Path: "/api/v1/organizations/{orgA}/api_keys",
			Expect: strangers(expect{aOwner: allow, aMember: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/organizations/{orgA}/webhook-endpoints",
			Expect: strangers(expect{aOwner: allow, aMember: deny}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/organizations/{orgA}/code-agent-harnesses",
			Expect: strangers(expect{aOwner: allow, aMember: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/organizations/{orgA}/sandboxes",
			Expect: strangers(expect{aOwner: allow, aMember: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/organizations/{orgA}/guidelines-history",
			Expect: strangers(expect{aOwner: allow, aMember: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/organizations/{orgA}/users/lookup?email=x@example.com",
			Expect: strangers(expect{aOwner: reached}),
		},
	}
}

// organizationDestructiveRoutes run last: when one of these refusals
// regresses, the request deletes the shared organization and every table
// after it would fail for the wrong reason.
func organizationDestructiveRoutes() []route {
	return []route{
		{
			Method: http.MethodDelete, Path: "/api/v1/organizations/{orgA}",
			Expect: strangers(expect{aOwner: allow, aMember: deny, projKey: deny, sessKey: deny, workerKey: deny}),
			Fresh:  freshOrg, Check: "/api/v1/organizations/{orgA}",
		},
	}
}

func helixOrgRoutes() []route {
	return []route{
		{
			Method: http.MethodGet, Path: "/api/v1/orgs/{orgAName}/bots",
			Expect: strangers(expect{aOwner: allow, aMember: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/orgs/{orgAName}/triggers",
			Expect: strangers(expect{aOwner: allow, aMember: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/orgs/{orgAName}/processors",
			Expect: strangers(expect{aOwner: allow, aMember: allow}),
		},
		{
			// Readable by members (values are redacted); changing them is
			// the owner's alone, below.
			Method: http.MethodGet, Path: "/api/v1/orgs/{orgAName}/settings",
			Expect: strangers(expect{aOwner: allow, aMember: allow, projKey: allow}),
		},
		{
			Method: http.MethodPut, Path: "/api/v1/orgs/{orgAName}/settings/authz.matrix",
			Body:   map[string]string{"value": tamperMarker},
			Expect: strangers(expect{aOwner: reached, aMember: deny, projKey: deny, sessKey: deny, workerKey: deny}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/orgs/{orgAName}/overview",
			Expect: strangers(expect{aOwner: allow, aMember: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/orgs/{orgAName}/assets",
			Expect: strangers(expect{aOwner: allow, aMember: allow}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/orgs/{orgAName}/slack/workspaces",
			Expect: strangers(expect{aOwner: allow}),
		},
	}
}

func keyAndAdminRoutes() []route {
	return []route{
		{
			// Everyone may mint a personal key for themselves; a scoped
			// key may not mint one (it would shed its scope).
			Method: http.MethodPost, Path: "/api/v1/api_keys?name=authz",
			Expect: expect{aOwner: allow, bOwner: allow, aOwnerOrgKey: deny, projKey: deny, sessKey: deny, workerKey: deny, appKey: deny, waitlisted: deny, anonymous: deny},
		},
		{
			Method: http.MethodGet, Path: "/api/v1/api_keys",
			Expect: expect{aOwner: allow, bOwner: allow, aOwnerOrgKey: deny, projKey: deny, sessKey: deny, waitlisted: deny, anonymous: deny},
		},
		{
			// An app key is a credential for one agent: minting one for an
			// agent the caller cannot use must be refused.
			Method: http.MethodPost, Path: "/api/v1/api_keys",
			Body:   map[string]any{"name": "authz", "type": "app", "app_id": "{appA}"},
			Expect: strangers(expect{aOwner: allow, aMember: allow, projKey: deny}),
		},
		{
			Method: http.MethodPost, Path: "/api/v1/api_keys",
			Body:   map[string]any{"name": "authz", "type": "app", "app_id": "{appPersonal}"},
			Expect: expect{aOwner: allow, aMember: deny, bOwner: deny},
		},
		{
			Method: http.MethodPost, Path: "/api/v1/organizations/{orgA}/api_keys",
			Body:   map[string]string{"name": "authz"},
			Expect: strangers(expect{aOwner: allow, aMember: allow, projKey: deny, sessKey: deny, workerKey: deny}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/users",
			Expect: strangers(expect{admin: allow, aOwner: deny, aOwnerOrgKey: deny, projKey: deny, sessKey: deny}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/admin/orgs",
			Expect: strangers(expect{admin: allow, aOwner: deny, projKey: deny}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/system/settings",
			Expect: strangers(expect{admin: allow, aOwner: deny, projKey: deny}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/llm_calls",
			Expect: strangers(expect{admin: allow, aOwner: deny}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/admin/audit-events",
			Expect: strangers(expect{admin: allow, aOwner: deny}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/users/{aOwnerID}",
			Expect: expect{aOwner: allow, aMember: allow, bOwner: deny, outsider: deny, anonymous: deny},
		},
		{
			Method: http.MethodGet, Path: "/api/v1/status",
			Expect: expect{aOwner: allow, waitlisted: deny, anonymous: deny},
		},
	}
}

func filestoreRoutes() []route {
	return []route{
		{
			Method: http.MethodGet, Path: "/api/v1/filestore/list?path=apps/{appA}",
			Expect: strangers(expect{aOwner: allow, aMember: allow, projKey: deny}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/filestore/list?path=apps",
			Expect: strangers(expect{admin: allow, aOwner: deny}),
		},
		{
			// Another user's folder by path traversal.
			Method: http.MethodGet, Path: "/api/v1/filestore/list?path=../{aOwnerID}",
			Expect: expect{bOwner: reached},
			Custom: func(t *testing.T, f *authzFixture, who actor, resp authzResponse) {
				if resp.status >= 200 && resp.status < 300 {
					t.Fatalf("%s listed another user's filestore folder: %d %q", who, resp.status, truncate(resp.body, 300))
				}
			},
		},
	}
}

// externalAgentRoutes need a running desktop to succeed. With none, the owner
// is expected to get past authorization and fail later (404/500/503 from the
// sandbox lookup); everyone else must be refused before that.
func externalAgentRoutes() []route {
	return []route{
		{
			// Viewing is a read: aMember, who may read projA, may watch.
			Method: http.MethodGet, Path: "/api/v1/external-agents/{sessA}/screenshot",
			Expect: strangers(expect{aOwner: reached, aMember: reached, projKey: reached}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/external-agents/{sessA2}/screenshot",
			Expect: expect{aOwner: reached, projKey: deny},
		},
		{
			Method: http.MethodGet, Path: "/api/v1/external-agents/{sessA}/clipboard",
			Expect:        strangers(expect{aOwner: reached, aMember: reached}),
			ReachedMarker: "External agent container not found",
		},
		{
			Method: http.MethodPost, Path: "/api/v1/external-agents/{sessA}/clipboard",
			Body:          map[string]string{"type": "text", "data": tamperMarker},
			Expect:        strangers(expect{aOwner: reached, aMember: deny}),
			ReachedMarker: "External agent container not found",
		},
		{
			Method: http.MethodPost, Path: "/api/v1/external-agents/{sessA}/exec",
			Body:   map[string]any{"command": []string{"true"}},
			Expect: strangers(expect{aOwner: reached, aMember: deny}),
		},
		{
			Method: http.MethodPost, Path: "/api/v1/external-agents/{sessA2}/exec",
			Body:   map[string]any{"command": []string{"true"}},
			Expect: expect{aOwner: reached, projKey: deny},
		},
		{
			Method: http.MethodGet, Path: "/api/v1/external-agents/{sessA}/workspace-files",
			Expect: strangers(expect{aOwner: reached}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/external-agents/{sessA}/workspace-file?path=README.md",
			Expect: strangers(expect{aOwner: reached}),
		},
		{
			Method: http.MethodPut, Path: "/api/v1/external-agents/{sessA}/workspace-file?path=authz.txt",
			Body:   tamperMarker,
			Expect: strangers(expect{aOwner: reached, aMember: deny}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/external-agents/{sessA}/workspaces",
			Expect: strangers(expect{aOwner: reached}),
		},
		{
			Method: http.MethodPost, Path: "/api/v1/external-agents/{sessA}/command",
			Body:   map[string]string{"type": "noop"},
			Expect: strangers(expect{aOwner: reached, aMember: deny}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/external-agents/{sessA}/ws/stream",
			Header: map[string]string{"Connection": "Upgrade", "Upgrade": "websocket", "Sec-WebSocket-Version": "13", "Sec-WebSocket-Key": "dGhlIHNhbXBsZSBub25jZQ=="},
			Expect: strangers(expect{aOwner: reached, aMember: reached}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/external-agents/{sessA}/ws/input",
			Header: map[string]string{"Connection": "Upgrade", "Upgrade": "websocket", "Sec-WebSocket-Version": "13", "Sec-WebSocket-Key": "dGhlIHNhbXBsZSBub25jZQ=="},
			Expect: strangers(expect{aOwner: reached, aMember: deny}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/sessions/{sessA}/terminal",
			Header: map[string]string{"Connection": "Upgrade", "Upgrade": "websocket", "Sec-WebSocket-Version": "13", "Sec-WebSocket-Key": "dGhlIHNhbXBsZSBub25jZQ=="},
			Expect: strangers(expect{aOwner: reached, aMember: deny}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/external-agents/sync?session_id={sessA}",
			Header: map[string]string{"Connection": "Upgrade", "Upgrade": "websocket", "Sec-WebSocket-Version": "13", "Sec-WebSocket-Key": "dGhlIHNhbXBsZSBub25jZQ=="},
			Expect: strangers(expect{aOwner: reached, aMember: deny}),
		},
	}
}

func zedRoutes() []route {
	return []route{
		{
			Method: http.MethodGet, Path: "/api/v1/spec-tasks/{taskA}/zed-instance",
			Expect: strangers(expect{aOwner: reached, aMember: reached}),
		},
		{
			Method: http.MethodDelete, Path: "/api/v1/spec-tasks/{taskA}/zed-instance",
			Expect: strangers(expect{aOwner: reached, aMember: deny}),
		},
	}
}

func mcpRoutes() []route {
	initialize := map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "authz", "version": "1"}},
	}
	accept := map[string]string{"Accept": "application/json, text/event-stream"}
	return []route{
		{
			Method: http.MethodPost, Path: "/api/v1/mcp/helix/message?app_id={appA}", Body: initialize, Header: accept,
			Expect: strangers(expect{aOwner: reached, aMember: reached, projKey: deny}),
		},
		{
			// The org and bot come from the worker's session, never the URL:
			// only a worker session key gets in.
			Method: http.MethodPost, Path: "/api/v1/mcp/helix-org", Body: initialize, Header: accept,
			Expect: strangers(expect{workerKey: allow, aOwner: deny, aOwnerOrgKey: deny, projKey: deny, sessKey: deny}),
		},
		{
			Method: http.MethodPost, Path: "/api/v1/mcp/helix-tasks", Body: initialize, Header: accept,
			Expect:        strangers(expect{projKey: reached, aOwner: deny, sessKey: deny, workerKey: deny}),
			ReachedMarker: "no Helix tools are enabled",
		},
	}
}

func llmProxyRoutes() []route {
	chat := map[string]any{"model": "authz-matrix-missing-model", "messages": []map[string]string{{"role": "user", "content": "hi"}}, "max_tokens": 1}
	return []route{
		{
			// Choosing an agent you cannot use must be refused before any
			// provider is selected.
			Method: http.MethodPost, Path: "/v1/chat/completions?app_id={appA}", Body: chat,
			Expect: strangers(expect{aOwner: reached, aMember: reached, projKey: deny}),
		},
		{
			// orgAgentA is projA's agent, so a projA reader may use it.
			Method: http.MethodPost, Path: "/v1/chat/completions?app_id={orgAgentA}", Body: chat,
			Expect: strangers(expect{aOwner: reached, projKey: reached, aMember: reached}),
		},
		{
			Method: http.MethodPost, Path: "/v1/chat/completions?app_id={appPersonal}", Body: chat,
			Expect: strangers(expect{aOwner: reached, aMember: deny}),
		},
		{
			// An app key carries its agent; it cannot be pointed at another.
			Method: http.MethodPost, Path: "/v1/chat/completions?app_id={appPersonal}", Body: chat,
			Expect: expect{appKey: reached},
			Custom: func(t *testing.T, f *authzFixture, who actor, resp authzResponse) {
				if resp.status != http.StatusBadRequest || !strings.Contains(resp.body, "does not match") {
					t.Fatalf("app key was not held to its own agent: %d %q", resp.status, truncate(resp.body, 300))
				}
			},
		},
		{
			// A provider is chosen by name from the caller's own scope. The
			// owner's request resolves org A's endpoint and tries to reach
			// it; anyone else's must not.
			Method: http.MethodPost, Path: "/v1/chat/completions", Body: map[string]any{
				"model": "{providerA}/authz-model", "messages": []map[string]string{{"role": "user", "content": "hi"}}, "max_tokens": 1,
			},
			Expect: expect{aOwnerOrgKey: reached, bOwner: reached, aMember: reached},
			Custom: func(t *testing.T, f *authzFixture, who actor, resp authzResponse) {
				usedProvider := strings.Contains(resp.body, "authz-matrix.invalid")
				if who == aOwnerOrgKey && !usedProvider {
					t.Fatalf("org key did not select org A's provider: %d %q", resp.status, truncate(resp.body, 300))
				}
				if who != aOwnerOrgKey && usedProvider {
					t.Fatalf("%s reached org A's provider endpoint: %d %q", who, resp.status, truncate(resp.body, 300))
				}
			},
		},
		{
			Method: http.MethodPost, Path: "/v1/messages", Body: chat,
			Expect: expect{aOwner: reached, projKey: reached, waitlisted: deny, anonymous: deny},
		},
		{
			Method: http.MethodPost, Path: "/v1/embeddings", Body: map[string]any{"model": "authz-matrix-missing-model", "input": "hi"},
			Expect: expect{aOwner: reached, waitlisted: deny, anonymous: deny},
		},
		{
			Method: http.MethodGet, Path: "/v1/models",
			Expect: expect{aOwner: allow, waitlisted: deny, anonymous: deny},
		},
		{
			Method: http.MethodPost, Path: "/v1/responses", Body: chat,
			Expect: expect{projKey: reached, waitlisted: deny, anonymous: deny},
		},
	}
}

func listRows() []listRow {
	return []listRow{
		{Path: "/api/v1/projects?organization_id={orgA}", Actor: aOwner, Contains: []string{"projA", "projA2"}},
		{Path: "/api/v1/projects?organization_id={orgA}", Actor: aMember, Contains: []string{"projA"}, Excludes: []string{"projA2"}},
		{Path: "/api/v1/projects?organization_id={orgA}", Actor: projKey, Excludes: []string{"projA2"}},
		{Path: "/api/v1/projects?organization_id={orgA}", Actor: bOwner, Excludes: []string{"projA", "projA2"}},

		{Path: "/api/v1/apps?organization_id={orgA}", Actor: aOwner, Contains: []string{"appA", "orgAgentA"}},
		{Path: "/api/v1/apps?organization_id={orgA}", Actor: bOwner, Excludes: []string{"appA", "orgAgentA"}},
		{Path: "/api/v1/apps", Actor: aOwner, Contains: []string{"appPersonal"}},
		{Path: "/api/v1/apps", Actor: bOwner, Excludes: []string{"appA", "orgAgentA", "appPersonal"}},
		{Path: "/api/v1/apps", Actor: aMember, Excludes: []string{"appPersonal"}},

		{Path: "/api/v1/sessions?org_id={orgA}", Actor: aOwner, Contains: []string{"sessA", "sessA2"}},
		{Path: "/api/v1/sessions?org_id={orgA}", Actor: bOwner, Excludes: []string{"sessA", "sessA2"}},
		{Path: "/api/v1/sessions?org_id={orgA}", Actor: projKey, Excludes: []string{"sessA2"}},
		{Path: "/api/v1/sessions", Actor: aOwner, Contains: []string{"sessChat"}},
		{Path: "/api/v1/sessions", Actor: bOwner, Excludes: []string{"sessChat", "sessOther"}},
		{Path: "/api/v1/sessions", Actor: projKey, Excludes: []string{"sessChat", "sessOther"}},

		{Path: "/api/v1/git/repositories?organization_id={orgA}", Actor: aOwner, Contains: []string{"repoA", "repoA2"}},
		{Path: "/api/v1/git/repositories?organization_id={orgA}", Actor: aMember, Excludes: []string{"repoA2"}},
		{Path: "/api/v1/git/repositories?organization_id={orgA}", Actor: projKey, Excludes: []string{"repoA2"}},
		{Path: "/api/v1/git/repositories?organization_id={orgA}", Actor: bOwner, Excludes: []string{"repoA", "repoA2"}},
		{Path: "/api/v1/git/repositories", Actor: aOwner, Contains: []string{"repoA"}},
		{Path: "/api/v1/git/repositories", Actor: bOwner, Excludes: []string{"repoA", "repoA2"}},

		{Path: "/api/v1/spec-tasks?project_id={projA}", Actor: aOwner, Contains: []string{"taskA"}},
		{Path: "/api/v1/spec-tasks?project_id={projA}", Actor: bOwner, Excludes: []string{"taskA"}},
		{Path: "/api/v1/spec-tasks?project_id={projA2}", Actor: aOwner, Contains: []string{"taskA2"}},
		{Path: "/api/v1/spec-tasks?project_id={projA2}", Actor: projKey, Excludes: []string{"taskA2"}},
		{Path: "/api/v1/spec-tasks?organization_id={orgA}", Actor: aOwner, Contains: []string{"taskA", "taskA2"}},
		{Path: "/api/v1/spec-tasks?organization_id={orgA}", Actor: aMember, Excludes: []string{"taskA2"}},
		{Path: "/api/v1/spec-tasks?organization_id={orgA}", Actor: projKey, Excludes: []string{"taskA2"}},
		{Path: "/api/v1/spec-tasks?organization_id={orgA}", Actor: bOwner, Excludes: []string{"taskA", "taskA2"}},

		{Path: "/api/v1/secrets", Actor: aOwner, Contains: []string{"secretPersonal"}},
		{Path: "/api/v1/secrets", Actor: bOwner, Excludes: []string{"secretPersonal", "secretProj"}},
		{Path: "/api/v1/projects/{projA}/secrets", Actor: aOwner, Contains: []string{"secretProj"}},

		{Path: "/api/v1/knowledge?app_id={appA}", Actor: aOwner, Contains: []string{"knowledgeA"}},
		{Path: "/api/v1/knowledge?app_id={appA}", Actor: bOwner, Excludes: []string{"knowledgeA"}},
		{Path: "/api/v1/knowledge", Actor: aOwner, Contains: []string{"knowledgeA"}},
		{Path: "/api/v1/knowledge", Actor: bOwner, Excludes: []string{"knowledgeA"}},

		{Path: "/api/v1/triggers?org_id={orgA}", Actor: aOwner, Contains: []string{"triggerA"}},
		{Path: "/api/v1/triggers?org_id={orgA}", Actor: bOwner, Excludes: []string{"triggerA"}},
		{Path: "/api/v1/triggers?org_id={orgA}&trigger_type=azure_devops", Actor: aOwner, Contains: []string{"webhookTriggerA"}},
		{Path: "/api/v1/triggers?org_id={orgA}&trigger_type=azure_devops", Actor: bOwner, Excludes: []string{"webhookTriggerA"}},

		{Path: "/api/v1/provider-endpoints?org_id={orgA}", Actor: aOwner, Contains: []string{"providerA"}},
		{Path: "/api/v1/provider-endpoints?org_id={orgA}", Actor: bOwner, Excludes: []string{"providerA"}},
		{Path: "/api/v1/provider-endpoints", Actor: aOwner, Contains: []string{"providerPersonal"}},
		{Path: "/api/v1/provider-endpoints", Actor: bOwner, Excludes: []string{"providerA", "providerPersonal"}},
		{Path: "/api/v1/provider-endpoints", Actor: aMember, Excludes: []string{"providerPersonal"}},

		{Path: "/api/v1/organizations", Actor: aOwner, Contains: []string{"orgA"}},
		{Path: "/api/v1/organizations", Actor: bOwner, Excludes: []string{"orgA"}},

		{Path: "/api/v1/projects/{projA}/artifacts", Actor: aOwner, Contains: []string{"artifactA"}},
		{Path: "/api/v1/projects/{projA}/artifacts", Actor: aMember, Contains: []string{"artifactA"}, Excludes: []string{"artifactA2"}},

		{Path: "/api/v1/search?q=authzmark", Actor: aOwner, Contains: []string{"projA"}},
		{Path: "/api/v1/search?q=authzmark", Actor: bOwner, Excludes: []string{"projA", "projA2", "appA", "sessA", "taskA", "repoA"}},
		{Path: "/api/v1/search?q=authzmark", Actor: projKey, Excludes: []string{"projA2", "sessA2", "taskA2", "repoA2"}},
	}
}

// gitCLIChecks drives the real git client against the smart-HTTP endpoints,
// with the actor's key as the basic-auth password the way users clone.
func gitCLIChecks(t *testing.T, f *authzFixture) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	cloneURL := func(who actor, repo string) string {
		u, err := url.Parse(integrationServerURL() + "/git/" + f.vars[repo])
		if err != nil {
			t.Fatalf("parse url: %v", err)
		}
		u.User = url.UserPassword("api", f.keys[who])
		return u.String()
	}
	git := func(dir string, args ...string) (string, error) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=true",
			"GIT_AUTHOR_NAME=authz", "GIT_AUTHOR_EMAIL=authz@example.com",
			"GIT_COMMITTER_NAME=authz", "GIT_COMMITTER_EMAIL=authz@example.com")
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	clone := func(t *testing.T, who actor, repo string) (string, error) {
		dir := t.TempDir()
		out, err := git(dir, "clone", "--quiet", cloneURL(who, repo), "repo")
		return filepath.Join(dir, "repo"), errorWithOutput(err, out)
	}
	push := func(t *testing.T, dir string, who actor, repo string) error {
		branch := "authz-" + string(who)
		for _, args := range [][]string{
			{"checkout", "--quiet", "-b", branch},
			{"commit", "--quiet", "--allow-empty", "-m", "authz matrix"},
		} {
			if out, err := git(dir, args...); err != nil {
				t.Fatalf("git %v: %v %s", args, err, out)
			}
		}
		out, err := git(dir, "push", "--quiet", cloneURL(who, repo), branch)
		return errorWithOutput(err, out)
	}

	t.Run("clone", func(t *testing.T) {
		for who, ok := range map[actor]bool{aOwner: true, aMember: true, projKey: true, bOwner: false, outsider: false, waitlisted: false} {
			_, err := clone(t, who, "repoA")
			if ok && err != nil {
				t.Errorf("%s could not clone repoA: %v", who, err)
			}
			if !ok && err == nil {
				t.Errorf("%s cloned org A's repoA", who)
			}
		}
		if _, err := clone(t, projKey, "repoA2"); err == nil {
			t.Errorf("project key cloned a repository outside its project")
		}
	})

	t.Run("push", func(t *testing.T) {
		dir, err := clone(t, aOwner, "repoA")
		if err != nil {
			t.Fatalf("owner clone: %v", err)
		}
		if err := push(t, dir, aMember, "repoA"); err == nil {
			t.Errorf("read-only member pushed to repoA")
		}
		if err := push(t, dir, bOwner, "repoA"); err == nil {
			t.Errorf("another tenant pushed to repoA")
		}
		if err := push(t, dir, aOwner, "repoA"); err != nil {
			t.Errorf("owner could not push a branch: %v", err)
		}

		dir2, err := clone(t, aOwner, "repoA2")
		if err != nil {
			t.Fatalf("owner clone repoA2: %v", err)
		}
		if err := push(t, dir2, projKey, "repoA2"); err == nil {
			t.Errorf("project key pushed to a repository outside its project")
		}
	})
}

func errorWithOutput(err error, out string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %s", err, strings.TrimSpace(out))
}
