package server

// Authorization policies for the sandboxes routes. See route_auth_policy_test.go
// for the classes; routePolicyFile says which file a new route belongs in.
func init() {
	declareRoutePolicies(map[string]routePolicy{
		"ANY /api/v1/runner/{runner_id}/ws":                        handlerAuth("legacy NATS WebSocket proxy, off unless RUNNER_NATS_PROXY_ENABLED; NATS authenticates the CONNECT token (shared token only; not registered when RUNNER_REQUIRE_PER_HOST_TOKENS is set)"),
		"GET /api/v1/runner-profiles":                              admin(),
		"POST /api/v1/runner-profiles":                             admin(),
		"GET /api/v1/runner-profiles/{id}":                         admin(),
		"PUT /api/v1/runner-profiles/{id}":                         admin(),
		"DELETE /api/v1/runner-profiles/{id}":                      admin(),
		"GET /api/v1/runners/{runner_id}/compatible-profiles":      admin(),
		"GET /api/v1/runners/{runner_id}/assignment":               admin(),
		"POST /api/v1/runners/{runner_id}/assign-profile":          admin(),
		"POST /api/v1/runners/{runner_id}/clear-profile":           admin(),
		"GET /api/v1/sandboxes/{id}/containers/{session_id}/blkio": object("authorizeUserToSession").alsoRunner(),
		"GET /api/v1/sandbox-desktop-types":                        user(),
		"GET /api/v1/sandbox-runtimes":                             user(),
		"POST /api/v1/sandboxes/register":                          runner(),
		"POST /api/v1/sandboxes/{id}/heartbeat":                    runner(),
		"GET /api/v1/runner/{runner_id}/assignment":                runner(),
		"GET /api/v1/runner/profiles/{id}":                         runner(),
		"GET /api/v1/sandboxes/{id}/disk-history":                  adminOrRunner(),
		"GET /api/v1/sandboxes":                                    adminOrRunner(),
		"DELETE /api/v1/sandboxes/{id}":                            adminOrRunner(),
	})
}
