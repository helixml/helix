package server

// Authorization policies for the misc routes. See route_auth_policy_test.go
// for the classes; routePolicyFile says which file a new route belongs in.
func init() {
	declareRoutePolicies(map[string]routePolicy{
		"POST /api/v1/teams/webhook/{appID}": webhook("Bot Framework JWT validated by the Teams adapter"),
		"GET /api/v1/config":                 public("frontend bootstrap config; resolves the caller itself and returns no secrets"),
		"GET /api/v1/swagger":                public("OpenAPI document"),
		"GET /api/v1/organization-domains":   admin(),
		"GET /api/v1/llm_calls":              admin(),
		"GET /api/v1/system/settings":        admin(),
		"PUT /api/v1/system/settings":        admin(),
		"GET /api/v1/license":                admin(),
		"POST /api/v1/license":               admin(),
		"GET /api/v1/status":                 user(),
		"GET /api/v1/filestore/config":       user(),
		"GET /api/v1/filestore/list":         object("checkAppFilestoreAccess"),
		"GET /api/v1/filestore/get":          object("checkAppFilestoreAccess"),
		"POST /api/v1/filestore/folder":      object("checkAppFilestoreAccess"),
		"POST /api/v1/filestore/upload":      object("checkAppFilestoreAccess"),
		"PUT /api/v1/filestore/rename":       object("checkAppFilestoreAccess"),
		"DELETE /api/v1/filestore/delete":    object("checkAppFilestoreAccess"),
		"POST /api/v1/interactions/{interaction_id}/questions/{request_id}/respond": object("loadAuthorizedPendingQuestion"),
		"POST /api/v1/interactions/{interaction_id}/questions/{request_id}/cancel":  object("loadAuthorizedPendingQuestion"),
		"GET /api/v1/search":                  user(),
		"POST /api/v1/resource-search":        object("authorizeOrgMember"),
		"GET /api/v1/bandwidth-probe":         user(),
		"GET /api/v1/revdial":                 user().alsoRunner(),
		"GET /api/v1/mcp/{server}":            noAuthzHelper("MCP gateway: each backend authorizes its own resources"),
		"POST /api/v1/mcp/{server}":           noAuthzHelper("MCP gateway: each backend authorizes its own resources"),
		"GET /api/v1/mcp/{server}/{path:.*}":  noAuthzHelper("MCP gateway: each backend authorizes its own resources"),
		"POST /api/v1/mcp/{server}/{path:.*}": noAuthzHelper("MCP gateway: each backend authorizes its own resources"),
		"POST /api/v1/stripe/webhook":         webhook("Stripe-Signature verified with the webhook signing secret"),
		"ANY /api/v1/filestore/viewer/":       handlerAuth("extractMiddleware; isFilestoreRouteAuthorized checks the path belongs to the caller").authz("isFilestoreRouteAuthorized"),
	})
}
