package server

// Authorization policies for the providers routes. See route_auth_policy_test.go
// for the classes; routePolicyFile says which file a new route belongs in.
func init() {
	declareRoutePolicies(map[string]routePolicy{
		"GET /api/v1/providers/detect-local":                       admin(),
		"POST /api/v1/helix-models":                                admin(),
		"PUT /api/v1/helix-models/{id:.*}":                         admin(),
		"DELETE /api/v1/helix-models/{id:.*}":                      admin(),
		"GET /api/v1/model-info":                                   admin(),
		"POST /api/v1/model-info":                                  admin(),
		"GET /api/v1/model-info/{id:.*}":                           admin(),
		"PUT /api/v1/model-info/{id:.*}":                           admin(),
		"DELETE /api/v1/model-info/{id:.*}":                        admin(),
		"GET /api/v1/providers":                                    user(),
		"POST /api/v1/provider-endpoints":                          object("authorizeOrgOwner"),
		"PUT /api/v1/provider-endpoints/{id}":                      object("authorizeOrgOwner"),
		"DELETE /api/v1/provider-endpoints/{id}":                   object("authorizeOrgOwner"),
		"GET /api/v1/provider-endpoints/{id}/available-models":     object("loadEditableProviderEndpoint"),
		"PUT /api/v1/provider-endpoints/{id}/models":               object("loadEditableProviderEndpoint"),
		"GET /api/v1/provider-endpoints/{id}/local-models":         object("loadEditableProviderEndpoint"),
		"POST /api/v1/provider-endpoints/{id}/local-models/load":   object("loadEditableProviderEndpoint"),
		"POST /api/v1/provider-endpoints/{id}/local-models/unload": object("loadEditableProviderEndpoint"),
		"GET /api/v1/provider-endpoints/{id}/daily-usage":          object("providerVisible"),
		"GET /api/v1/provider-endpoints/{id}/throughput-usage":     object("providerVisible"),
		"GET /api/v1/provider-endpoints/{id}/users-daily-usage":    object("providerVisible"),
		"GET /api/v1/v1/models":                                    user(),
		"GET /api/v1/helix-models":                                 user(),
		"GET /api/v1/provider-endpoints":                           public("anonymous callers see global endpoints only; ?org_id= needs membership").authz("authorizeOrgMember"),
	})
}
