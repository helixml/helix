package server

// Authorization policies for the admin routes. See route_auth_policy_test.go
// for the classes; routePolicyFile says which file a new route belongs in.
func init() {
	declareRoutePolicies(map[string]routePolicy{
		"GET /api/v1/admin/agent-sandboxes/debug":                    admin(),
		"GET /api/v1/admin/agent-sandboxes/events":                   admin(),
		"PUT /api/v1/admin/users/{id}/password":                      admin(),
		"POST /api/v1/admin/users/{id}/api-keys":                     admin(),
		"DELETE /api/v1/admin/users/{id}":                            admin(),
		"POST /api/v1/admin/users/{id}/approve":                      admin(),
		"POST /api/v1/admin/users/{id}/trial-activate":               admin(),
		"DELETE /api/v1/admin/users/{id}/trial-activate":             admin(),
		"POST /api/v1/admin/users/{id}/credits":                      admin(),
		"GET /api/v1/admin/users/{id}/owned-orgs":                    admin(),
		"GET /api/v1/admin/orgs":                                     admin(),
		"POST /api/v1/admin/orgs/{id}/plan":                          admin(),
		"POST /api/v1/admin/service-accounts":                        admin(),
		"GET /api/v1/admin/audit-events":                             admin(),
		"GET /api/v1/admin/runners/{runner_id}/logs":                 admin(),
		"GET /api/v1/admin/kodit/stats":                              admin(),
		"GET /api/v1/admin/kodit/repositories":                       admin(),
		"POST /api/v1/admin/kodit/repositories/batch/delete":         admin(),
		"POST /api/v1/admin/kodit/repositories/batch/rescan":         admin(),
		"GET /api/v1/admin/kodit/repositories/{koditRepoId}":         admin(),
		"POST /api/v1/admin/kodit/repositories/{koditRepoId}/sync":   admin(),
		"POST /api/v1/admin/kodit/repositories/{koditRepoId}/rescan": admin(),
		"GET /api/v1/admin/kodit/repositories/{koditRepoId}/tasks":   admin(),
		"DELETE /api/v1/admin/kodit/repositories/{koditRepoId}":      admin(),
		"GET /api/v1/admin/kodit/queue":                              admin(),
		"DELETE /api/v1/admin/kodit/queue/{taskId}":                  admin(),
		"PUT /api/v1/admin/kodit/queue/{taskId}/priority":            admin(),
	})
}
