package server

import (
	"net/http"
	"strings"

	"github.com/helixml/helix/api/pkg/types"
)

// Authorization for embed API keys.
//
// An embed key (types.APIkeytypeEmbed) is handed to a BROWSER on an untrusted
// page — a candidate or client viewing their own agent chat in an iframe on a
// public site. It must therefore be far weaker than any other credential we
// issue: it may touch a fixed list of paths, and only ever the ONE spec task and
// ONE session recorded on the key.
//
// Why a distinct key type rather than enforcing on the existing SessionID /
// SpecTaskID fields: those are already set on ordinary ephemeral keys
// ("for metrics/attribution", spec_driven_task_service.go), which agent sandboxes
// use for broad API access. Enforcing on them retroactively would break every
// running agent. Opting in with a new type keeps existing behaviour byte for byte.
//
// FAIL CLOSED. Anything not explicitly matched is denied. A missing entry costs
// a visibly broken panel in the embed, which we fix by adding it; the opposite
// default would cost a silent cross-tenant read.

// embedScope is what a matched path is allowed to address.
type embedScope int

const (
	// scopeGlobal: no per-resource id in the path. Safe because the response is
	// about the key's own (service) identity or is public config.
	scopeGlobal embedScope = iota
	// scopeTask: the path segment after /spec-tasks/ must equal key.SpecTaskID.
	scopeTask
	// scopeSession: the path segment after /sessions/ or /external-agents/ must
	// equal key.SessionID.
	scopeSession
)

// embedRule matches a concrete request path.
//
// prefix/suffix rather than a router pattern because this runs inside the auth
// middleware, before mux has parsed vars. `idAfter` names the prefix the scoped
// id follows.
type embedRule struct {
	methods []string // empty = any
	prefix  string
	suffix  string // "" = prefix must match the whole remaining path
	scope   embedScope
}

// embedRules is the complete allowlist for an embed key.
//
// Deliberately ABSENT, and each for a reason:
//   - /api/v1/spec-tasks (list) — returns every task in the project, i.e. every
//     other candidate's chat. This is the single most important omission.
//   - /api/v1/agents, /api/v1/organizations, /api/v1/projects/... — tenant-wide
//     inventory the embed does not need to render a conversation.
//   - /api/v1/external-agents/{id}/ws/stream and the desktop endpoints — the
//     video/desktop surface. Headless chat does not need it.
//   - Every mutation of the task itself (PUT /spec-tasks/{id}, labels, archive,
//     execution-config PATCH, start-planning) — an end user must not be able to
//     re-point or reconfigure the agent run they are talking to.
var embedRules = []embedRule{
	// Bootstrap. The embed page redirects to /login unless /auth/authenticated
	// and /auth/user answer, so these are load-bearing, not optional. They
	// describe the key's own service identity — not any candidate's.
	{methods: []string{"GET"}, prefix: "/api/v1/config", scope: scopeGlobal},
	{methods: []string{"GET"}, prefix: "/api/v1/status", scope: scopeGlobal},
	{methods: []string{"GET"}, prefix: "/api/v1/auth/authenticated", scope: scopeGlobal},
	{methods: []string{"GET"}, prefix: "/api/v1/auth/user", scope: scopeGlobal},

	// The one task this key may read.
	{methods: []string{"GET"}, prefix: "/api/v1/spec-tasks/", scope: scopeTask},
	{methods: []string{"GET"}, prefix: "/api/v1/spec-tasks/", suffix: "/execution-config", scope: scopeTask},
	{methods: []string{"GET"}, prefix: "/api/v1/spec-tasks/", suffix: "/zed-threads", scope: scopeTask},
	{methods: []string{"GET"}, prefix: "/api/v1/spec-tasks/", suffix: "/clone-groups", scope: scopeTask},
	{methods: []string{"GET"}, prefix: "/api/v1/spec-tasks/", suffix: "/attachments", scope: scopeTask},

	// The one conversation this key may read and post to.
	{methods: []string{"GET"}, prefix: "/api/v1/sessions/", scope: scopeSession},
	{methods: []string{"GET"}, prefix: "/api/v1/sessions/", suffix: "/interactions", scope: scopeSession},
	{methods: []string{"GET"}, prefix: "/api/v1/sessions/", suffix: "/step-info", scope: scopeSession},
	{methods: []string{"POST"}, prefix: "/api/v1/sessions/", suffix: "/cancel", scope: scopeSession},
	{methods: []string{"GET"}, prefix: "/api/v1/external-agents/", suffix: "/file", scope: scopeSession},

	// Sending a message. The session is named in the BODY, not the path, so this
	// rule cannot bound it here — sessions/chat is bounded by the handler, which
	// resolves the session and authorizes the owning user.
	{methods: []string{"POST"}, prefix: "/api/v1/sessions/chat", scope: scopeGlobal},

	// Streaming updates. The session id arrives as a query param; checked below.
	{methods: []string{"GET"}, prefix: "/api/v1/ws/user", scope: scopeGlobal},
}

// embedNeutered maps paths the embed SPA *requires* to the empty response it
// should get instead of the real data.
//
// NEUTER, DON'T DENY. The spec-task page is an internal UI: it eagerly loads
// tenant inventory (organizations, agents, the project's task list) before it
// will render anything. Returning 403 breaks the page — the account context
// gives up and routes away, which is what a first attempt at this actually did.
// Returning an empty success satisfies the page and leaks nothing.
//
// `/spec-tasks` matters most: it is the enumeration path that would otherwise
// hand one visitor every other visitor's conversation. An empty list is strictly
// better than a denial here, because it is both safe AND lets the page work.
//
// Keys are exact paths (query strings are ignored); prefixes ending in "/"
// match any single trailing segment.
var embedNeutered = map[string]string{
	"/api/v1/spec-tasks":             "[]",
	"/api/v1/organizations":          "[]",
	"/api/v1/agents":                 "[]",
	"/api/v1/claude-subscriptions":   "[]",
	"/api/v1/oauth/providers":        "[]",
	"/api/v1/oauth/connections":      "[]",
	"/api/v1/provider-endpoints":     "[]",
	"/api/v1/providers/detect-local": "[]",
	"/api/v1/prompt-history":         "[]",
}

// embedNeuteredPrefixes covers id-bearing paths, which cannot be exact-matched.
// The response is an empty object because the SPA reads fields off these.
var embedNeuteredPrefixes = []string{
	"/api/v1/projects/",
	"/api/v1/users/",
}

// embedNeuteredResponse returns the body to serve instead of real data, if any.
func embedNeuteredResponse(r *http.Request) (string, bool) {
	path := strings.TrimSuffix(r.URL.Path, "/")
	if body, ok := embedNeutered[path]; ok {
		return body, true
	}
	for _, p := range embedNeuteredPrefixes {
		if strings.HasPrefix(path, p) {
			// Nested reads under these (labels, repositories, …) are equally
			// inventory; an empty list keeps list-shaped consumers happy.
			if strings.Count(strings.TrimPrefix(path, p), "/") > 0 {
				return "[]", true
			}
			return "{}", true
		}
	}
	return "", false
}

// embedKeyAllows reports whether an embed-key request may proceed.
func embedKeyAllows(user *types.User, r *http.Request) bool {
	path := strings.TrimSuffix(r.URL.Path, "/")
	if path == "" {
		return false
	}
	// A key with no task bound to it can address nothing.
	if user.SpecTaskID == "" {
		return false
	}

	// The websocket carries its subject as a query parameter rather than a path
	// segment, so bound it explicitly. Without this an embed key could subscribe
	// to another candidate's session stream.
	if path == "/api/v1/ws/user" {
		if sid := r.URL.Query().Get("session_id"); sid != "" && sid != user.SessionID {
			return false
		}
	}

	for _, rule := range embedRules {
		if !rule.matchesMethod(r.Method) {
			continue
		}
		id, ok := rule.match(path)
		if !ok {
			continue
		}
		switch rule.scope {
		case scopeGlobal:
			return true
		case scopeTask:
			if id != "" && id == user.SpecTaskID {
				return true
			}
		case scopeSession:
			if id != "" && user.SessionID != "" && id == user.SessionID {
				return true
			}
		}
	}
	return false
}

func (rule embedRule) matchesMethod(method string) bool {
	if len(rule.methods) == 0 {
		return true
	}
	for _, m := range rule.methods {
		if strings.EqualFold(m, method) {
			return true
		}
	}
	return false
}

// match reports whether path fits the rule, returning the scoped id segment.
//
// The id must be a single segment: a rule for /api/v1/spec-tasks/{id} must not
// match /api/v1/spec-tasks/{id}/something-else, or a suffix rule's protection
// would be bypassable by appending a path.
func (rule embedRule) match(path string) (string, bool) {
	if !strings.HasPrefix(path, rule.prefix) {
		return "", false
	}
	rest := strings.TrimPrefix(path, rule.prefix)

	if rule.suffix == "" {
		// Whole remaining path is the id (or empty for a fixed path).
		if strings.Contains(rest, "/") {
			return "", false
		}
		return rest, true
	}
	if !strings.HasSuffix(rest, rule.suffix) {
		return "", false
	}
	id := strings.TrimSuffix(rest, rule.suffix)
	if id == "" || strings.Contains(id, "/") {
		return "", false
	}
	return id, true
}
