package api

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/types"
)

// peerRoutes are member-to-member probes: aPeer, a plain member of A, owns
// projP, repoP, sessP, taskP, appP and their secrets, artifact and knowledge,
// and shares none of them. aMember belongs to the same organization and must
// be refused every verb on every one of them, with nothing changed.
func peerRoutes() []route {
	peer := func(e expect) expect { return strangers(e) }
	return []route{
		// Project.
		{
			Method: http.MethodGet, Path: "/api/v1/projects/{projP}",
			Expect: peer(expect{aPeer: allow, aMember: deny}),
		},
		{
			Method: http.MethodPut, Path: "/api/v1/projects/{projP}", Body: tamper("description"),
			Expect: peer(expect{aPeer: allow, aMember: deny}),
			Fresh:  freshPeerProject, Check: "/api/v1/projects/{projP}", CheckAs: aPeer,
		},
		{
			Method: http.MethodDelete, Path: "/api/v1/projects/{projP}",
			Expect: peer(expect{aPeer: allow, aMember: deny}),
			Fresh:  freshPeerProject, Check: "/api/v1/projects/{projP}", CheckAs: aPeer,
		},
		{
			Method: http.MethodGet, Path: "/api/v1/projects/{projP}/repositories",
			Expect: peer(expect{aPeer: allow, aMember: deny}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/projects/{projP}/secrets",
			Expect: peer(expect{aPeer: allow, aMember: deny}),
		},
		{
			Method: http.MethodPost, Path: "/api/v1/projects/{projP}/secrets",
			Body:   map[string]string{"name": "AUTHZ_NEW", "value": "v"},
			Expect: peer(expect{aPeer: allow, aMember: deny}),
			Fresh:  freshPeerProject, Check: "/api/v1/projects/{projP}/secrets", CheckAs: aPeer, CheckMarker: "secretP",
		},
		{
			Method: http.MethodGet, Path: "/api/v1/projects/{projP}/access-grants",
			Expect: peer(expect{aPeer: allow, aMember: deny}),
		},
		{
			// Granting yourself admin on a colleague's project.
			Method: http.MethodPost, Path: "/api/v1/projects/{projP}/access-grants",
			Body:   map[string]any{"user_reference": "{aMemberID}", "roles": []string{"admin"}},
			Expect: peer(expect{aOwner: allow, aMember: deny}),
			Fresh:  freshPeerProject, Check: "/api/v1/projects/{projP}/access-grants", CheckAs: aPeer,
		},
		{
			Method: http.MethodGet, Path: "/api/v1/projects/{projP}/artifacts",
			Expect: peer(expect{aPeer: allow, aMember: deny}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/projects/{projP}/usage",
			Expect: peer(expect{aPeer: allow, aMember: deny}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/projects/{projP}/tasks-progress",
			Expect: peer(expect{aPeer: allow, aMember: deny}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/projects/{projP}/labels",
			Expect: peer(expect{aPeer: allow, aMember: deny}),
		},
		{
			Method: http.MethodPost, Path: "/api/v1/projects/{projP}/pin",
			Expect: peer(expect{aPeer: allow, aMember: deny}),
		},

		// Repository.
		{
			Method: http.MethodGet, Path: "/api/v1/git/repositories/{repoP}",
			Expect: peer(expect{aPeer: allow, aMember: deny}),
		},
		{
			Method: http.MethodPut, Path: "/api/v1/git/repositories/{repoP}", Body: tamper("description"),
			Expect: peer(expect{aPeer: allow, aMember: deny}),
			Fresh:  freshPeerRepo, Check: "/api/v1/git/repositories/{repoP}", CheckAs: aPeer,
		},
		{
			Method: http.MethodDelete, Path: "/api/v1/git/repositories/{repoP}",
			Expect: peer(expect{aPeer: allow, aMember: deny}),
			Fresh:  freshPeerRepo, Check: "/api/v1/git/repositories/{repoP}", CheckAs: aPeer,
		},
		{
			Method: http.MethodGet, Path: "/api/v1/git/repositories/{repoP}/contents?path=README.md&branch=main",
			Expect: peer(expect{aPeer: allow, aMember: deny}),
		},
		{
			Method: http.MethodPut, Path: "/api/v1/git/repositories/{repoP}/contents",
			Body:   map[string]string{"path": "README.md", "branch": "main", "message": "authz", "content": tamperMarker, "author": "a", "email": "a@example.com"},
			Expect: peer(expect{aPeer: allow, aMember: deny}),
			Fresh:  freshPeerRepo, Check: "/api/v1/git/repositories/{repoP}/contents?path=README.md&branch=main", CheckAs: aPeer, CheckMarker: "repoP",
		},
		{
			Method: http.MethodGet, Path: "/api/v1/git/repositories/{repoP}/access-grants",
			Expect: peer(expect{aPeer: allow, aMember: deny}),
		},
		{
			Method: http.MethodGet, Path: "/git/{repoP}/info/refs?service=git-upload-pack", Header: map[string]string{basicAuthHeader: "1"},
			Expect: peer(expect{aPeer: allow, aMember: deny}),
		},
		{
			Method: http.MethodGet, Path: "/git/{repoP}/info/refs?service=git-receive-pack", Header: map[string]string{basicAuthHeader: "1"},
			Expect: peer(expect{aPeer: allow, aMember: deny}),
		},

		// Session.
		{
			Method: http.MethodGet, Path: "/api/v1/sessions/{sessP}",
			Expect: peer(expect{aPeer: allow, aMember: deny}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/sessions/{sessP}/interactions",
			Expect: peer(expect{aPeer: allow, aMember: deny}),
		},
		{
			Method: http.MethodPut, Path: "/api/v1/sessions/{sessP}", Body: tamper("name"),
			Expect: peer(expect{aPeer: allow, aMember: deny}),
			Fresh:  freshPeerSession, Check: "/api/v1/sessions/{sessP}", CheckAs: aPeer,
		},
		{
			Method: http.MethodDelete, Path: "/api/v1/sessions/{sessP}",
			Expect: peer(expect{aPeer: allow, aMember: deny}),
			Fresh:  freshPeerSession, Check: "/api/v1/sessions/{sessP}", CheckAs: aPeer,
		},

		// Spec task.
		{
			Method: http.MethodGet, Path: "/api/v1/spec-tasks/{taskP}",
			Expect: peer(expect{aPeer: allow, aMember: deny}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/spec-tasks/{taskP}/specs",
			Expect: peer(expect{aPeer: allow, aMember: deny}),
		},
		{
			Method: http.MethodPut, Path: "/api/v1/spec-tasks/{taskP}", Body: tamper("description"),
			Expect: peer(expect{aPeer: allow, aMember: deny}),
			Fresh:  freshPeerTask, Check: "/api/v1/spec-tasks/{taskP}", CheckAs: aPeer,
		},
		{
			Method: http.MethodPatch, Path: "/api/v1/spec-tasks/{taskP}/archive",
			Body:   map[string]bool{"archived": true},
			Expect: peer(expect{aPeer: allow, aMember: deny}),
			Fresh:  freshPeerTask, Check: "/api/v1/spec-tasks/{taskP}", CheckAs: aPeer,
		},
		{
			Method: http.MethodDelete, Path: "/api/v1/spec-tasks/{taskP}",
			Expect: peer(expect{aPeer: allow, aMember: deny}),
			Fresh:  freshPeerArchivedTask, Check: "/api/v1/spec-tasks/{taskP}", CheckAs: aPeer,
		},

		// Agent.
		{
			Method: http.MethodGet, Path: "/api/v1/apps/{appP}",
			Expect: peer(expect{aPeer: allow, aMember: deny}),
		},
		{
			Method: http.MethodPut, Path: "/api/v1/apps/{appP}",
			Body:   map[string]any{"organization_id": "{orgA}", "config": map[string]any{"helix": map[string]any{"name": tamperMarker}}},
			Expect: peer(expect{aPeer: allow, aMember: deny}),
			Fresh:  freshPeerApp, Check: "/api/v1/apps/{appP}", CheckAs: aPeer,
		},
		{
			Method: http.MethodDelete, Path: "/api/v1/apps/{appP}",
			Expect: peer(expect{aPeer: allow, aMember: deny}),
			Fresh:  freshPeerApp, Check: "/api/v1/apps/{appP}", CheckAs: aPeer,
		},
		{
			Method: http.MethodGet, Path: "/api/v1/apps/{appP}/access-grants",
			Expect: peer(expect{aPeer: allow, aMember: deny}),
		},
		{
			Method: http.MethodPost, Path: "/api/v1/apps/{appP}/access-grants",
			Body:   map[string]any{"user_reference": "{aMemberID}", "roles": []string{"admin"}},
			Expect: peer(expect{aOwner: allow, aMember: deny}),
			Fresh:  freshPeerApp, Check: "/api/v1/apps/{appP}/access-grants", CheckAs: aPeer,
		},
		{
			Method: http.MethodPost, Path: "/api/v1/apps/{appP}/duplicate",
			Expect: peer(expect{aPeer: allow, aMember: deny}),
		},
		{
			Method: http.MethodPost, Path: "/v1/chat/completions?app_id={appP}",
			Body:          map[string]any{"model": "global/helix/authz-matrix-missing-model", "messages": []map[string]string{{"role": "user", "content": "hi"}}, "max_tokens": 1},
			Expect:        peer(expect{aPeer: reached, aMember: deny}),
			ReachedMarker: "authz-matrix.invalid",
		},
		{
			// An app key is minted only for an agent the caller may use.
			Method: http.MethodPost, Path: "/api/v1/api_keys",
			Body:   map[string]any{"name": "authz", "type": "app", "app_id": "{appP}"},
			Expect: peer(expect{aPeer: allow, aMember: deny}),
		},

		// Secrets.
		{
			Method: http.MethodPut, Path: "/api/v1/secrets/{secretP}", Body: tamper("name"),
			Expect: peer(expect{aPeer: allow, aMember: deny}),
			Fresh:  freshPeerProjectSecret, Check: "/api/v1/projects/{projP}/secrets", CheckAs: aPeer, CheckMarker: "secretP",
		},
		{
			Method: http.MethodDelete, Path: "/api/v1/secrets/{secretP}",
			Expect: peer(expect{aPeer: allow, aMember: deny}),
			Fresh:  freshPeerProjectSecret, Check: "/api/v1/projects/{projP}/secrets", CheckAs: aPeer, CheckMarker: "secretP",
		},
		{
			Method: http.MethodPut, Path: "/api/v1/secrets/{secretPeerPersonal}", Body: tamper("name"),
			Expect: peer(expect{aPeer: allow, aMember: deny}),
			Fresh:  freshPeerPersonalSecret, Check: "/api/v1/secrets", CheckAs: aPeer, CheckMarker: "secretPeerPersonal",
		},
		{
			Method: http.MethodDelete, Path: "/api/v1/secrets/{secretPeerPersonal}",
			Expect: peer(expect{aPeer: allow, aMember: deny}),
			Fresh:  freshPeerPersonalSecret, Check: "/api/v1/secrets", CheckAs: aPeer, CheckMarker: "secretPeerPersonal",
		},
		{
			// A secret created into a colleague's project.
			Method: http.MethodPost, Path: "/api/v1/secrets",
			Body:   map[string]string{"name": "AUTHZ_PEER_NEW", "value": "v", "project_id": "{projP}"},
			Expect: peer(expect{aPeer: allow, aMember: deny}),
			Fresh:  freshPeerProject, Check: "/api/v1/projects/{projP}/secrets", CheckAs: aPeer, CheckMarker: "secretP",
		},

		// Artifact.
		{
			Method: http.MethodGet, Path: "/api/v1/artifacts/{artifactP}",
			Expect: peer(expect{aPeer: allow, aMember: deny}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/artifacts/{artifactP}/download",
			Expect: peer(expect{aPeer: allow, aMember: deny}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/artifacts/{artifactP}/versions",
			Expect: peer(expect{aPeer: allow, aMember: deny}),
		},
		{
			Method: http.MethodDelete, Path: "/api/v1/artifacts/{artifactP}",
			Expect: peer(expect{aPeer: allow, aMember: deny}),
			Fresh:  freshPeerArtifact, Check: "/api/v1/artifacts/{artifactP}", CheckAs: aPeer,
		},

		// Knowledge and provider endpoint.
		{
			Method: http.MethodGet, Path: "/api/v1/knowledge/{knowledgeP}",
			Expect: peer(expect{aPeer: allow, aMember: deny}),
		},
		{
			Method: http.MethodDelete, Path: "/api/v1/knowledge/{knowledgeP}",
			Expect: peer(expect{aPeer: allow, aMember: deny}),
			Fresh:  freshPeerKnowledge, Check: "/api/v1/knowledge/{knowledgeP}", CheckAs: aPeer,
		},
		{
			Method: http.MethodDelete, Path: "/api/v1/provider-endpoints/{providerPeer}",
			Expect: peer(expect{aPeer: allow, aMember: deny}),
			Fresh:  freshPeerProvider,
		},
		{
			Method: http.MethodGet, Path: "/api/v1/provider-endpoints/{providerPeer}/users-daily-usage",
			Expect: peer(expect{aPeer: allow, aMember: deny}),
		},
	}
}

// peerListRows: aMember's lists never show aPeer's unshared objects.
func peerListRows() []listRow {
	return []listRow{
		{Path: "/api/v1/projects?organization_id={orgA}", Actor: aPeer, Contains: []string{"projP"}},
		{Path: "/api/v1/projects?organization_id={orgA}", Actor: aMember, Excludes: []string{"projP"}},
		{Path: "/api/v1/apps?organization_id={orgA}", Actor: aPeer, Contains: []string{"appP"}},
		{Path: "/api/v1/apps?organization_id={orgA}", Actor: aMember, Excludes: []string{"appP"}},
		{Path: "/api/v1/sessions?org_id={orgA}", Actor: aPeer, Contains: []string{"sessP"}},
		{Path: "/api/v1/sessions?org_id={orgA}", Actor: aMember, Excludes: []string{"sessP"}},
		{Path: "/api/v1/git/repositories?organization_id={orgA}", Actor: aPeer, Contains: []string{"repoP"}},
		{Path: "/api/v1/git/repositories?organization_id={orgA}", Actor: aMember, Excludes: []string{"repoP"}},
		{Path: "/api/v1/spec-tasks?organization_id={orgA}", Actor: aPeer, Contains: []string{"taskP"}},
		{Path: "/api/v1/spec-tasks?organization_id={orgA}", Actor: aMember, Excludes: []string{"taskP"}},
		{Path: "/api/v1/secrets", Actor: aPeer, Contains: []string{"secretPeerPersonal"}},
		{Path: "/api/v1/secrets", Actor: aMember, Excludes: []string{"secretPeerPersonal", "secretP"}},
		{Path: "/api/v1/knowledge", Actor: aPeer, Contains: []string{"knowledgeP"}},
		{Path: "/api/v1/knowledge", Actor: aMember, Excludes: []string{"knowledgeP"}},
		{Path: "/api/v1/provider-endpoints", Actor: aPeer, Contains: []string{"providerPeer"}},
		{Path: "/api/v1/provider-endpoints", Actor: aMember, Excludes: []string{"providerPeer"}},
		{Path: "/api/v1/search?q=authzmark", Actor: aPeer, Contains: []string{"projP"}},
		{Path: "/api/v1/search?q=authzmark", Actor: aMember, Excludes: []string{"projP", "appP", "sessP", "taskP", "repoP"}},
	}
}

func freshPeerProject(t *testing.T, f *authzFixture) map[string]string {
	t.Helper()
	name := "fresh-peer-project-" + uuid.NewString()[:8]
	repo := f.createRepoAs(t, aPeer, name, f.vars["orgA"])
	return map[string]string{"projP": f.createProjectAs(t, aPeer, name, repo, f.vars["orgAgentP"])}
}

func freshPeerRepo(t *testing.T, f *authzFixture) map[string]string {
	t.Helper()
	return map[string]string{"repoP": f.createRepoAs(t, aPeer, "fresh-peer-repo-"+uuid.NewString()[:8], f.vars["orgA"])}
}

func freshPeerSession(t *testing.T, f *authzFixture) map[string]string {
	t.Helper()
	return map[string]string{"sessP": f.createSessionAs(t, aPeer, "fresh-peer-session-"+uuid.NewString()[:8], f.vars["orgA"], f.vars["projP"], f.vars["taskP"])}
}

func freshPeerTask(t *testing.T, f *authzFixture) map[string]string {
	t.Helper()
	return map[string]string{"taskP": f.createSpecTaskAs(t, aPeer, "fresh-peer-task-"+uuid.NewString()[:8], f.vars["projP"])}
}

func freshPeerArchivedTask(t *testing.T, f *authzFixture) map[string]string {
	t.Helper()
	return map[string]string{"taskP": f.archivedTaskAs(t, aPeer, f.vars["projP"])}
}

func freshPeerApp(t *testing.T, f *authzFixture) map[string]string {
	t.Helper()
	return map[string]string{"appP": f.createAppAs(t, aPeer, "fresh-peer-app-"+uuid.NewString()[:8], f.vars["orgA"], types.AgentKindHelix)}
}

func freshPeerProjectSecret(t *testing.T, f *authzFixture) map[string]string {
	t.Helper()
	return map[string]string{"secretP": f.createSecretAs(t, aPeer, "fresh-peer-secret-"+uuid.NewString()[:8], f.vars["projP"])}
}

func freshPeerPersonalSecret(t *testing.T, f *authzFixture) map[string]string {
	t.Helper()
	return map[string]string{"secretPeerPersonal": f.createSecretAs(t, aPeer, "fresh-peer-secret-"+uuid.NewString()[:8], "")}
}

func freshPeerArtifact(t *testing.T, f *authzFixture) map[string]string {
	t.Helper()
	return map[string]string{"artifactP": f.createArtifactAs(t, aPeer, "fresh-peer-artifact-"+uuid.NewString()[:8], f.vars["projP"])}
}

func freshPeerKnowledge(t *testing.T, f *authzFixture) map[string]string {
	t.Helper()
	return map[string]string{"knowledgeP": f.createKnowledgeAs(t, aPeer, "fresh-peer-knowledge-"+uuid.NewString()[:8], f.vars["orgA"], f.vars["appP"])}
}

func freshPeerProvider(t *testing.T, f *authzFixture) map[string]string {
	t.Helper()
	return map[string]string{"providerPeer": f.createProvider(t, "fresh-peer-provider-"+uuid.NewString()[:8], types.OwnerTypeUser, f.users[aPeer].ID)}
}
