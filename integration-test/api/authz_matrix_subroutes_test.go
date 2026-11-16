package api

import "net/http"

// projectSubRoutes cover the project routes projectRoutes leaves out: the
// rest of /api/v1/projects/{id}/..., project creation into an organization,
// artifact writes and secret creation. Most need infrastructure CI does not
// run (a sandbox, Kodit, a web-service host), so the owner only has to get
// past authorization.
func projectSubRoutes() []route {
	return []route{
		{
			Method: http.MethodPost, Path: "/api/v1/projects",
			Body:   map[string]string{"organization_id": "{orgA}", "name": "authz-new-project", "default_repo_id": "{repoA}"},
			Expect: strangers(expect{aOwner: reached}),
			Fresh:  freshRepo,
		},
		{
			Method: http.MethodPut, Path: "/api/v1/projects/apply",
			Body:   map[string]any{"organization_id": "{orgA}", "name": "authz-apply"},
			Expect: strangers(expect{aOwner: reached}),
		},
		{
			Method: http.MethodPost, Path: "/api/v1/projects/quick-create",
			Body:   map[string]any{"organization_id": "{orgA}", "repo_id": "{repoA}", "prompt": "authz"},
			Expect: strangers(expect{aOwner: reached}),
			Fresh:  freshRepo,
		},
		{
			Method: http.MethodGet, Path: "/api/v1/projects/{projA}/vcs-connections",
			Expect: strangers(expect{aOwner: reached, aMember: reached}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/projects/{projA}/goose-recipes",
			Expect: strangers(expect{aOwner: reached}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/projects/{projA}/startup-script/history",
			Expect: strangers(expect{aOwner: reached}),
		},
		{
			Method: http.MethodGet, Path: "/api/v1/projects/{projA}/tasks-usage",
			Expect: strangers(expect{aOwner: reached, aMember: reached}),
		},
		{
			Method: http.MethodPut, Path: "/api/v1/projects/{projA}/repositories/{repoA2}/attach",
			Expect: strangers(expect{aOwner: reached, aMember: deny, projKey: deny}),
			Fresh:  freshProject, Check: "/api/v1/projects/{projA}/repositories", CheckMarker: "repoA",
		},
		{
			Method: http.MethodPut, Path: "/api/v1/projects/{projA}/repositories/{repoA}/detach",
			Expect: strangers(expect{aOwner: reached, aMember: deny}),
			Fresh:  freshProject, Check: "/api/v1/projects/{projA}/repositories", CheckMarker: "repoA",
		},
		{
			Method: http.MethodPut, Path: "/api/v1/projects/{projA}/repositories/{repoA}/primary",
			Expect: strangers(expect{aOwner: reached, aMember: deny}),
			Fresh:  freshProject, Check: "/api/v1/projects/{projA}/repositories", CheckMarker: "repoA",
		},
		{
			// Re-parenting an organization's project into another tenant.
			Method: http.MethodPost, Path: "/api/v1/projects/{projA}/move",
			Body:   map[string]string{"organization_id": "{orgB}"},
			Expect: strangers(expect{aOwner: reached, aMember: deny}),
			Fresh:  freshProject, Check: "/api/v1/projects/{projA}",
		},
		{
			// A personal project moves only with its owner's own credential:
			// a key scoped to a project, or to an organization the owner
			// belongs to, may not carry it into that organization.
			Method: http.MethodPost, Path: "/api/v1/projects/{projPersonal}/move",
			Body:   map[string]string{"organization_id": "{orgA}"},
			Expect: expect{aOwner: allow, projKey: deny, aOwnerOrgKey: deny, workerKey: deny, sandboxKey: deny, bOwner: deny},
			Fresh:  freshPersonalProject, Check: "/api/v1/projects/{projPersonal}",
		},
		{
			Method: http.MethodPost, Path: "/api/v1/projects/{projPersonal}/move",
			Body:   map[string]string{"organization_id": "{orgB}"},
			Expect: expect{aOwner: allow, aOwnerOrgBKey: deny, bOwner: deny},
			Fresh:  freshPersonalProject, Check: "/api/v1/projects/{projPersonal}",
		},
		{
			Method: http.MethodPost, Path: "/api/v1/projects/{projPersonal}/move/preview",
			Body:   map[string]string{"organization_id": "{orgB}"},
			Expect: expect{aOwner: allow, aOwnerOrgBKey: deny, projKey: deny, bOwner: deny},
		},
		{
			Method: http.MethodDelete, Path: "/api/v1/projects/{projA}/exploratory-session",
			Expect:        strangers(expect{aOwner: reached, aMember: deny}),
			ReachedMarker: "failed to stop exploratory session",
		},
		{
			Method: http.MethodPut, Path: "/api/v1/projects/{projA}/web-service",
			Body:   map[string]any{"enabled": false},
			Expect: strangers(expect{aOwner: reached, aMember: deny}),
			Fresh:  freshProject,
		},
		{
			Method: http.MethodPost, Path: "/api/v1/projects/{projA}/web-service/deploy",
			Expect: strangers(expect{aOwner: reached, aMember: deny}),
			Fresh:  freshProject,
		},
		{
			Method: http.MethodPost, Path: "/api/v1/projects/{projA}/web-service/domains",
			Body:   map[string]string{"domain": "authz.authz-matrix.invalid"},
			Expect: strangers(expect{aOwner: reached, aMember: deny}),
			Fresh:  freshProject,
		},
		{
			Method: http.MethodGet, Path: "/api/v1/projects/{projA}/docker-cache/zfs-tree",
			Expect: strangers(expect{aOwner: reached}),
		},
		{
			Method: http.MethodDelete, Path: "/api/v1/projects/{projA}/docker-cache",
			Expect: strangers(expect{aOwner: reached, aMember: deny}),
		},
		{
			Method: http.MethodPost, Path: "/api/v1/projects/{projA}/artifacts",
			Body:   map[string]string{"name": tamperMarker},
			Expect: strangers(expect{aOwner: reached, aMember: deny}),
		},
		{
			Method: http.MethodPut, Path: "/api/v1/artifacts/{artifactA}",
			Body:   map[string]string{"name": tamperMarker},
			Expect: strangers(expect{aOwner: reached, aMember: deny}),
			Fresh:  freshArtifact, Check: "/api/v1/artifacts/{artifactA}",
		},
		{
			Method: http.MethodHead, Path: "/api/v1/artifacts/{artifactA}/download",
			Expect: strangers(expect{aOwner: allow, aMember: allow}),
		},
		{
			// Secrets created through the top-level route into a project.
			Method: http.MethodPost, Path: "/api/v1/secrets",
			Body:   map[string]string{"name": "AUTHZ_TOP_NEW", "value": "v", "project_id": "{projA}"},
			Expect: strangers(expect{aOwner: allow, aMember: deny}),
			Fresh:  freshProject, Check: "/api/v1/projects/{projA}/secrets", CheckMarker: "secretProj",
		},
		{
			// A personal secret for the key's owner: a desktop key may not
			// plant one.
			Method: http.MethodPost, Path: "/api/v1/secrets",
			Body:   map[string]string{"name": "AUTHZ_PERSONAL_NEW", "value": "v"},
			Expect: expect{aOwner: allow, sessKey: deny, sandboxKey: deny, waitlisted: deny, anonymous: deny},
			Check:  "/api/v1/secrets", CheckMarker: "secretPersonal",
		},
		{
			Method: http.MethodGet, Path: "/api/v1/secrets",
			Expect: expect{aOwner: allow, sessKey: deny, sandboxKey: deny, waitlisted: deny, anonymous: deny},
		},
	}
}

// repositorySubRoutes cover the repository routes repositoryRoutes leaves
// out. Kodit, remotes and pull requests are not configured in CI, so the
// owner only has to get past authorization.
func repositorySubRoutes() []route {
	const koditOff = "Kodit service is not enabled"
	reads := []struct{ sub, marker string }{
		{"enrichments", koditOff}, {"kodit-commits", koditOff}, {"search-snippets?query=authz", koditOff},
		{"kodit-status", koditOff}, {"wiki", koditOff}, {"wiki-page?path=index", koditOff},
		{"semantic-search?query=authz", koditOff}, {"keyword-search?keywords=authz", koditOff},
		{"visual-search?query=authz", koditOff}, {"page-image?path=x&page=1", koditOff},
		{"grep?pattern=authz", koditOff}, {"files?branch=main", koditOff},
		{"file-content?path=README.md&branch=main", koditOff}, {"pulls", ""},
		// Not a number: the owner is answered 400 after authorization.
		{"pulls/x", ""},
	}
	var rows []route
	for _, r := range reads {
		rows = append(rows, route{
			Method: http.MethodGet, Path: "/api/v1/git/repositories/{repoA}/" + r.sub,
			Expect: strangers(expect{aOwner: reached}), ReachedMarker: r.marker,
		})
		rows = append(rows, route{
			Method: http.MethodGet, Path: "/api/v1/git/repositories/{repoA2}/" + r.sub,
			Expect: expect{aOwner: reached, aMember: deny, projKey: deny}, ReachedMarker: r.marker,
		})
	}
	const notExternal = "repository is not external"
	writes := []struct {
		sub    string
		method string
		body   any
		marker string
	}{
		{"branches", http.MethodPost, map[string]string{"branch_name": "authz-branch", "base_branch": "main"}, ""},
		{"kodit-rescan?commit_sha=0000000000000000000000000000000000000000", http.MethodPost, nil, koditOff},
		{"push-pull", http.MethodPost, nil, notExternal},
		{"pull", http.MethodPost, nil, ""},
		{"push", http.MethodPost, nil, ""},
		{"sync-all", http.MethodPost, nil, ""},
		{"pull-requests", http.MethodPost, map[string]string{"title": tamperMarker, "source_branch": "main", "target_branch": "main"}, notExternal},
		{"pulls/x", http.MethodPatch, map[string]string{"title": tamperMarker}, ""},
		{"pulls/x/merge", http.MethodPost, nil, ""},
		{"pulls/x/request-rebase", http.MethodPost, nil, ""},
		{"access-grants", http.MethodPost, map[string]any{"user_reference": "{aMemberID}", "roles": []string{"admin"}}, ""},
		{"access-grants/authz-missing-grant", http.MethodDelete, nil, "access grant not found"},
	}
	for _, w := range writes {
		rows = append(rows, route{
			Method: w.method, Path: "/api/v1/git/repositories/{repoA}/" + w.sub, Body: w.body,
			Expect: strangers(expect{aOwner: reached, aMember: deny}), ReachedMarker: w.marker,
			Fresh: freshRepo, Check: "/api/v1/git/repositories/{repoA}",
		})
	}
	return rows
}
