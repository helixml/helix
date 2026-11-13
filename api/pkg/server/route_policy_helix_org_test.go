package server

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/gorilla/mux"

	helixorgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
)

// Authorization policies for the helix-org routes served behind the
// "ANY /api/v1/orgs/{org}/" prefix (registerHelixOrgAuthenticatedRoutes).
// The outer router mounts them as one prefix, so routeAuthPolicies cannot see
// them; this table declares each route the org-graph API serves, keyed by its
// own pattern (the path after /api/v1/orgs/{org}).
//
// Every route first passes withHelixOrgIdentity: the caller must be a member
// of the org in the URL (authorizeOrgMember, which also confines an org- or
// project-scoped key), and isHelixOrgPrivilegedMutation reserves org-wide
// configuration to owners and administrators. The tests below check that the
// table names every served route, that the owner gate covers exactly the
// routes declared owner-only, and that every route declared as checked by its
// handler really calls the helper it names.

type helixOrgAccess string

const (
	// helixOrgMemberRead: any member of the organization.
	helixOrgMemberRead helixOrgAccess = "member-read"
	// helixOrgOwnerGate: owners and administrators only, refused by
	// withHelixOrgIdentity before the handler runs.
	helixOrgOwnerGate helixOrgAccess = "owner-gate"
	// helixOrgBotEditor: a member, and the handler authorizes the bot it
	// changes (Helper).
	helixOrgBotEditor helixOrgAccess = "bot-editor"
	// helixOrgMemberAction: any member may do this; Reason says why.
	helixOrgMemberAction helixOrgAccess = "member-action"
	// helixOrgSigned: the handler verifies a signature; Reason says which.
	helixOrgSigned helixOrgAccess = "signed"
)

type helixOrgRoutePolicy struct {
	Access helixOrgAccess
	Helper string
	Reason string
}

func orgRead() helixOrgRoutePolicy  { return helixOrgRoutePolicy{Access: helixOrgMemberRead} }
func orgOwner() helixOrgRoutePolicy { return helixOrgRoutePolicy{Access: helixOrgOwnerGate} }
func orgBotEdit(helper string) helixOrgRoutePolicy {
	return helixOrgRoutePolicy{Access: helixOrgBotEditor, Helper: helper}
}
func orgMemberAction(reason string) helixOrgRoutePolicy {
	return helixOrgRoutePolicy{Access: helixOrgMemberAction, Reason: reason}
}

var helixOrgRoutePolicies = map[string]helixOrgRoutePolicy{
	"GET /overview":                                 orgRead(),
	"GET /bots":                                     orgRead(),
	"POST /bots":                                    orgBotEdit("authorizeLoadedBotEdit"),
	"GET /bots/{id}":                                orgRead(),
	"PATCH /bots/{id}":                              orgBotEdit("authorizeLoadedBotEdit"),
	"DELETE /bots/{id}":                             orgBotEdit("authorizeBotEdit"),
	"POST /bots/{id}/chat":                          orgMemberAction("opens the caller's own chat with an org bot; members talk to the org's bots"),
	"POST /bots/{id}/activate":                      orgMemberAction("wakes an org bot for a turn; any member may wake one (stop and restart need edit rights)"),
	"POST /bots/{id}/stop":                          orgBotEdit("authorizeLoadedBotEdit"),
	"POST /bots/{id}/restart":                       orgBotEdit("authorizeLoadedBotEdit"),
	"POST /bots/{id}/apply-config":                  orgBotEdit("authorizeLoadedBotEdit"),
	"GET /bots/{id}/instances":                      orgRead(),
	"POST /bots/{id}/instances":                     orgBotEdit("authorizeBotEdit"),
	"DELETE /bots/{id}/instances/{session_id}":      orgBotEdit("authorizeBotEdit"),
	"POST /bots/{id}/parents":                       orgBotEdit("authorizeBotEdit"),
	"DELETE /bots/{id}/parents/{parent_id}":         orgBotEdit("authorizeBotEdit"),
	"GET /bots/{id}/secrets":                        orgRead(),
	"GET /bots/{id}/available-secrets":              orgRead(),
	"PUT /bots/{id}/secrets/{name}":                 orgOwner(),
	"DELETE /bots/{id}/secrets/{name}":              orgOwner(),
	"GET /tools":                                    orgRead(),
	"GET /settings":                                 orgRead(),
	"PUT /settings/{key}":                           orgOwner(),
	"DELETE /settings/{key}":                        orgOwner(),
	"GET /trigger-kinds":                            orgRead(),
	"GET /triggers":                                 orgRead(),
	"POST /triggers":                                orgOwner(),
	"GET /triggers/{id}":                            orgRead(),
	"PUT /triggers/{id}":                            orgOwner(),
	"DELETE /triggers/{id}":                         orgOwner(),
	"GET /triggers/{id}/events":                     orgRead(),
	"GET /bots/{id}/attachments":                    orgRead(),
	"POST /bots/{id}/attachments":                   orgBotEdit("authorizeBotEdit"),
	"DELETE /bots/{id}/attachments/{attachment_id}": orgBotEdit("authorizeBotEdit"),
	"GET /processors":                               orgRead(),
	"POST /processors":                              orgOwner(),
	"GET /processors/{id}":                          orgRead(),
	"PUT /processors/{id}":                          orgOwner(),
	"DELETE /processors/{id}":                       orgOwner(),
	"GET /assets":                                   orgRead(),
	"POST /assets":                                  orgOwner(),
	"GET /assets/{id}":                              orgRead(),
	"PATCH /assets/{id}":                            orgOwner(),
	"DELETE /assets/{id}":                           orgOwner(),
	"GET /assets/{id}/health":                       orgRead(),
	"GET /assets/{id}/links":                        orgRead(),
	"POST /assets/{id}/links":                       orgOwner(),
	"DELETE /assets/{id}/links/{bot_id}":            orgOwner(),
	"GET /chart/positions":                          orgRead(),
	"PUT /chart/positions":                          orgMemberAction("the org chart's on-screen layout; no data or capability changes"),
	"DELETE /chart/positions":                       orgMemberAction("resets the org chart's on-screen layout"),
	"POST /github/webhook": {Access: helixOrgSigned,
		Reason: "GitHub deliveries; the transport verifies the per-org webhook secret's HMAC whoever sends it"},
	"GET /github/repos":                          orgRead(),
	"GET /github/app-installation":               orgRead(),
	"POST /github/app-manifest":                  orgOwner(),
	"POST /triggers/{id}/github/install-webhook": orgOwner(),
	"GET /triggers/{id}/github/webhook-status":   orgRead(),
	"POST /triggers/{id}/gitlab/install-webhook": orgOwner(),
	"GET /triggers/{id}/gitlab/webhook-status":   orgRead(),
}

func helixOrgServedPatterns() []string {
	var patterns []string
	for _, r := range helixorgapi.Routes(helixorgapi.Deps{}) {
		patterns = append(patterns, r.Pattern)
	}
	return patterns
}

func TestHelixOrgRoutePolicy_EveryRouteIsDeclared(t *testing.T) {
	served := map[string]bool{}
	var missing []string
	for _, pattern := range helixOrgServedPatterns() {
		served[pattern] = true
		if _, ok := helixOrgRoutePolicies[pattern]; !ok {
			missing = append(missing, pattern)
		}
	}
	var stale []string
	for pattern, p := range helixOrgRoutePolicies {
		if !served[pattern] {
			stale = append(stale, pattern)
		}
		if (p.Access == helixOrgMemberAction || p.Access == helixOrgSigned) && strings.TrimSpace(p.Reason) == "" {
			t.Errorf("%s: %s routes need a Reason", pattern, p.Access)
		}
		if p.Access == helixOrgBotEditor && p.Helper == "" {
			t.Errorf("%s: bot-editor routes name the helper the handler calls", pattern)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	if len(missing) > 0 {
		t.Errorf("helix-org serves %d route(s) without a policy in helixOrgRoutePolicies:\n\t%s", len(missing), strings.Join(missing, "\n\t"))
	}
	if len(stale) > 0 {
		t.Errorf("helixOrgRoutePolicies declares %d route(s) helix-org no longer serves:\n\t%s", len(stale), strings.Join(stale, "\n\t"))
	}
}

var helixOrgPathVarRE = regexp.MustCompile(`\{[^}]+\}`)

// TestHelixOrgRoutePolicy_OwnerGateMatches: the routes declared owner-only are
// exactly those withHelixOrgIdentity refuses to members.
func TestHelixOrgRoutePolicy_OwnerGateMatches(t *testing.T) {
	for pattern, p := range helixOrgRoutePolicies {
		method, path, _ := strings.Cut(pattern, " ")
		full := APIPrefix + "/orgs/acme" + helixOrgPathVarRE.ReplaceAllString(path, "x1")
		r := mux.SetURLVars(httptest.NewRequest(method, full, nil), map[string]string{"org": "acme"})
		gated := isHelixOrgPrivilegedMutation(r)
		if want := p.Access == helixOrgOwnerGate; gated != want {
			t.Errorf("%s is declared %s, but the owner gate %s it", pattern, p.Access, map[bool]string{true: "covers", false: "does not cover"}[gated])
		}
		if p.Access == helixOrgMemberRead && method != http.MethodGet {
			t.Errorf("%s is declared member-read but is not a GET", pattern)
		}
	}
}

// TestHelixOrgRoutePolicy_BotEditorsCallTheirHelper parses the org-graph API
// package: each bot-editor route's handler must reach the helper it names.
func TestHelixOrgRoutePolicy_BotEditorsCallTheirHelper(t *testing.T) {
	dir := filepath.Join("..", "org", "interfaces", "server", "api")
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	methods := map[string]*ast.FuncDecl{}
	handlers := map[string]string{} // pattern -> apiHandler method
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range file.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			if fd.Recv != nil {
				methods[fd.Name.Name] = fd
			}
			if fd.Name.Name == "Routes" && fd.Recv == nil {
				collectHelixOrgHandlers(fd, handlers)
			}
		}
	}
	reaches := func(start, helper string) bool {
		seen := map[string]bool{}
		var walk func(string) bool
		walk = func(name string) bool {
			if name == helper {
				return true
			}
			fd, ok := methods[name]
			if !ok || seen[name] {
				return false
			}
			seen[name] = true
			found := false
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				if sel, ok := n.(*ast.SelectorExpr); ok && !found && walk(sel.Sel.Name) {
					found = true
				}
				return !found
			})
			return found
		}
		return walk(start)
	}
	for pattern, p := range helixOrgRoutePolicies {
		if p.Access != helixOrgBotEditor {
			continue
		}
		handler, ok := handlers[pattern]
		if !ok {
			t.Errorf("%s: no handler found in helixorgapi.Routes", pattern)
			continue
		}
		if !reaches(handler, p.Helper) {
			t.Errorf("%s: handler %s does not call %s", pattern, handler, p.Helper)
		}
	}
}

// collectHelixOrgHandlers reads `{Pattern: "...", Handler: http.HandlerFunc(a.x)}`
// entries from the Routes function.
func collectHelixOrgHandlers(fd *ast.FuncDecl, out map[string]string) {
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		var pattern, handler string
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, _ := kv.Key.(*ast.Ident)
			if key == nil {
				continue
			}
			switch key.Name {
			case "Pattern":
				if bl, ok := kv.Value.(*ast.BasicLit); ok {
					pattern = strings.Trim(bl.Value, `"`)
				}
			case "Handler":
				if call, ok := kv.Value.(*ast.CallExpr); ok && len(call.Args) == 1 {
					if sel, ok := call.Args[0].(*ast.SelectorExpr); ok {
						handler = sel.Sel.Name
					}
				}
			}
		}
		if pattern != "" && handler != "" {
			out[pattern] = handler
		}
		return true
	})
}
