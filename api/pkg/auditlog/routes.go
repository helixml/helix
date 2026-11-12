package auditlog

import "strings"

// sensitiveBodyPaths lists the request paths whose bodies carry credentials in
// a shape redaction cannot make safe: the payload *is* the secret, or it is a
// credential document (provider headers, OAuth client config, agent secrets).
// For these paths only the size and content type are recorded.
//
// `*` matches exactly one path segment, `**` matches any remaining segments.
// Paths are matched both with and without the /api/v1 prefix so the vhost and
// socket handlers are covered too.
var sensitiveBodyPaths = []string{
	// User and project secrets: POST /secrets {"name": ..., "value": ...} has
	// no sensitive key name at all.
	"/secrets",
	"/secrets/*",
	"/projects/*/secrets",

	// Login, registration, password reset and desktop auto-login secrets.
	"/auth/**",

	// OAuth provider and connection configuration (client secrets, tokens).
	"/oauth/**",

	// Git provider, service connection and provider endpoint credentials,
	// including endpoint headers (`x-api-key`).
	"/git-provider-connections",
	"/git-provider-connections/**",
	"/service-connections",
	"/service-connections/**",
	"/provider-endpoints",
	"/provider-endpoints/**",

	// Agent/app config carries secrets, API headers, query parameters, MCP
	// environments and personal access tokens.
	"/agents",
	"/agents/*",
	"/apps",
	"/apps/*",

	// Webhook endpoint signing secrets.
	"/organizations/*/webhook-endpoints",
	"/organizations/*/webhook-endpoints/**",

	// Sandbox create carries an env map.
	"/organizations/*/sandboxes",

	// Secret intake: the whole point of the form is collecting credentials.
	"/projects/*/secret-intakes",
	"/projects/*/secret-intakes/**",
	"/connect/intake/redeem",
	"/connect/intake/submit",
}

// IsSensitiveBodyRoute reports whether the body of a request for this path must
// not be recorded, even redacted.
func IsSensitiveBodyRoute(path string) bool {
	normalized := strings.TrimSuffix(path, "/")
	normalized = strings.TrimPrefix(normalized, "/api/v1")
	if normalized == "" {
		return false
	}
	segments := strings.Split(strings.TrimPrefix(normalized, "/"), "/")

	for _, pattern := range sensitiveBodyPaths {
		if matchSegments(strings.Split(strings.TrimPrefix(pattern, "/"), "/"), segments) {
			return true
		}
	}
	return false
}

func matchSegments(pattern, segments []string) bool {
	for i, segment := range pattern {
		if segment == "**" {
			return len(segments) > i
		}
		if i >= len(segments) {
			return false
		}
		if segment != "*" && segment != segments[i] {
			return false
		}
	}
	return len(pattern) == len(segments)
}
