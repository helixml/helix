package server

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/janitor"
	"github.com/helixml/helix/api/pkg/sandbox"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/stripe"
	"github.com/helixml/helix/api/pkg/types"
	"go.uber.org/mock/gomock"
)

// Route authorization policy.
//
// Every route the API serves is declared in routeAuthPolicies (one
// route_policy_<area>_test.go file per area, see routePolicyFile) with the
// class of caller it admits. The
// tests in this file walk the router that production builds (buildRouter) and
// check that:
//
//   - every served route is declared, and every declaration is still served
//     (TestRouteAuthPolicy_EveryRouteIsDeclared);
//   - the declared class matches the auth middleware the route really sits
//     behind (TestRouteAuthPolicy_DeclaredClassMatchesRouter);
//   - each class refuses callers without credentials and callers holding the
//     wrong kind of credential (TestRouteAuthPolicy_Enforced);
//   - object routes on the user router call the authorization helper their
//     declaration names, before any dependency call other than loading the
//     object (TestRouteAuthPolicy_ObjectRoutesCallAuthzHelper).

type routeAuthClass string

const (
	// classPublic: no credentials required. Reason says why.
	classPublic routeAuthClass = "public"
	// classSignedWebhook: no Helix credentials; the handler verifies a
	// signature or per-object secret carried by the request. Reason says which.
	classSignedWebhook routeAuthClass = "signed-webhook"
	// classHandlerAuth: mounted outside the auth routers; the handler (or a
	// wrapper around it) authenticates the caller itself. Reason says how.
	classHandlerAuth routeAuthClass = "handler-auth"
	// classUser: requireUser; the handler acts on the caller's own data or on
	// objects it filters by the caller in the store query (no object id in
	// the path, or one that the handler scopes by the caller).
	classUser routeAuthClass = "user"
	// classUserObject: requireUser plus an object check. Authz names the
	// helper(s) the handler calls before touching the object.
	classUserObject routeAuthClass = "user-object"
	// classAdmin: requireAdmin (global admin).
	classAdmin routeAuthClass = "admin"
	// classRunner: requireRunner (deployment runner token).
	classRunner routeAuthClass = "runner"
	// classAdminOrRunner: requireAdminOrRunner.
	classAdminOrRunner routeAuthClass = "admin-or-runner"
)

// gateUserOrRunner is the requireUserOrRunner gate. It is not a declarable
// class: a user or user-object route marked alsoRunner() sits behind it.
const gateUserOrRunner routeAuthClass = "user-or-runner"

type routePolicy struct {
	Class routeAuthClass
	// Authz lists authorization helpers the route's handler calls directly.
	// Required for classUserObject.
	Authz []string
	// Reason documents a public, signed-webhook or handler-auth route.
	Reason string
	// RunnerAllowed marks user routes that sandbox hosts call with the runner
	// token. They must be mounted behind requireUserOrRunner (requireUser
	// refuses the runner token), and the wrong-class probe does not send the
	// runner token there.
	RunnerAllowed bool
}

func public(reason string) routePolicy { return routePolicy{Class: classPublic, Reason: reason} }
func webhook(reason string) routePolicy {
	return routePolicy{Class: classSignedWebhook, Reason: reason}
}
func handlerAuth(reason string) routePolicy {
	return routePolicy{Class: classHandlerAuth, Reason: reason}
}
func user() routePolicy          { return routePolicy{Class: classUser} }
func admin() routePolicy         { return routePolicy{Class: classAdmin} }
func runner() routePolicy        { return routePolicy{Class: classRunner} }
func adminOrRunner() routePolicy { return routePolicy{Class: classAdminOrRunner} }
func object(helpers ...string) routePolicy {
	return routePolicy{Class: classUserObject, Authz: helpers}
}

// authz names the authorization helper(s) a handler-auth route's handler
// calls on the object in its path.
func (p routePolicy) authz(helpers ...string) routePolicy { p.Authz = helpers; return p }

// gap records a route whose handler does not authorize the object today.
// It is a defect to fix, kept visible here rather than hidden in the table.
func gap(reason string) routePolicy {
	return routePolicy{Class: classUser, Reason: "KNOWN GAP: " + reason}
}

// alsoRunner marks a user route that sandbox hosts call with the runner token.
func (p routePolicy) alsoRunner() routePolicy { p.RunnerAllowed = true; return p }

// noAuthzHelper documents a user-router route with a path id whose handler
// scopes the object some other way (store query filtered by the caller, or
// the id is the caller's own). Reason is required.
func noAuthzHelper(reason string) routePolicy {
	return routePolicy{Class: classUser, Reason: reason}
}

// routeAuthPolicies is assembled from the per-domain route_policy_*_test.go
// files, each of which declares its routes from init(). One file per area of
// the API keeps unrelated route additions from conflicting.
var routeAuthPolicies = map[string]routePolicy{}

// routePolicyDeclaredIn records which file declared each entry.
var routePolicyDeclaredIn = map[string]string{}

func declareRoutePolicies(policies map[string]routePolicy) {
	_, file, _, _ := runtime.Caller(1)
	file = filepath.Base(file)
	for key, p := range policies {
		if prev, dup := routePolicyDeclaredIn[key]; dup {
			panic(fmt.Sprintf("route policy %q declared in both %s and %s", key, prev, file))
		}
		routeAuthPolicies[key] = p
		routePolicyDeclaredIn[key] = file
	}
}

// routePolicyFile names the file a route's policy belongs in.
func routePolicyFile(key string) string {
	path := key
	if i := strings.IndexByte(key, ' '); i >= 0 {
		path = key[i+1:]
	}
	domain := "root"
	if rest, ok := strings.CutPrefix(path, APIPrefix+"/"); ok {
		segment, _, _ := strings.Cut(rest, "/")
		domain = routePolicyDomains[segment]
		if domain == "" {
			domain = "misc"
		}
	}
	return "route_policy_" + domain + "_test.go"
}

// routePolicyDomains groups the first /api/v1 path segment into policy files.
var routePolicyDomains = map[string]string{
	"agents": "agents", "apps": "agents", "agent-tools": "agents", "triggers": "agents", "knowledge": "agents",
	"skills": "agents", "context-menu": "agents",
	"admin": "admin",
	"orgs":  "orgs", "organizations": "orgs", "wallet": "orgs", "subscription": "orgs", "top-ups": "orgs",
	"usage": "orgs", "quotas": "orgs", "slack": "orgs",
	"git": "git", "git-provider-connections": "git", "repositories": "git", "kodit": "git",
	"service-connections": "git", "webhooks": "git",
	"sessions": "sessions", "external-agents": "sessions", "zed": "sessions", "work-sessions": "sessions",
	"ws": "sessions", "prompt-history": "sessions", "claude-subscriptions": "sessions",
	"codex-subscriptions": "sessions", "attention-events": "sessions",
	"spec-tasks": "spectasks", "clone-groups": "spectasks", "specs": "spectasks",
	"projects": "projects", "samples": "projects", "sample-projects": "projects", "artifacts": "projects",
	"public": "projects",
	"auth":   "identity", "users": "identity", "api_keys": "identity", "oauth": "identity", "invitations": "identity",
	"secrets":            "identity",
	"provider-endpoints": "providers", "providers": "providers", "helix-models": "providers", "model-info": "providers",
	"v1":        "providers",
	"sandboxes": "sandboxes", "sandbox-desktop-types": "sandboxes", "sandbox-runtimes": "sandboxes",
	"runner-profiles": "sandboxes", "runners": "sandboxes", "runner": "sandboxes",
}

// servedRoute is one (method, path template) the router serves.
type servedRoute struct {
	Method   string // "ANY" when the route has no method matcher
	Template string
	Gate     routeAuthClass // derived from the middleware chain; "" when none
	Chain    []string       // middleware function names, outermost first
	route    *mux.Route
}

func (r servedRoute) Key() string { return r.Method + " " + r.Template }

const (
	routeTestRunnerToken = "route-test-runner-token"
	routeTestUserKey     = types.APIKeyPrefix + "routetestuserkey"
	routeTestUserID      = "usr_route_test"
)

// newRouteTestServer builds the HelixAPIServer routes with the same
// registration code production runs. Subsystems that need live
// infrastructure are left nil; helix-org is mounted with empty handlers so
// its routes are present without its Postgres-backed store.
func newRouteTestServer(t *testing.T, st store.Store) *HelixAPIServer {
	t.Helper()
	cfg := &config.ServerConfig{}
	cfg.WebServer.URL = "http://localhost:8080"
	cfg.WebServer.Host = "localhost"
	cfg.WebServer.Port = 8080
	cfg.WebServer.RunnerToken = routeTestRunnerToken
	cfg.WebServer.FrontendURL = "http://127.0.0.1:1" // SPA catch-all; never reached by the probes
	// Mount every optional route and turn on optional features so the
	// probes reach the handlers' own checks.
	cfg.WebServer.RunnerNATSProxyEnabled = true
	cfg.ConnectPortal.SecretIntakeEnabled = true
	t.Setenv("HELIX_ENCRYPTION_KEY", "route-auth-policy-test-key")

	workDir := t.TempDir()
	s := &HelixAPIServer{
		Cfg:                  cfg,
		Store:                st,
		Controller:           &controller.Controller{Options: controller.Options{Store: st, Config: cfg}},
		Janitor:              janitor.NewJanitor(config.Janitor{}),
		mcpGateway:           NewMCPGateway(),
		helixOrg:             &helixOrgHandlers{},
		gitRepositoryService: services.NewGitRepositoryService(st, workDir, cfg.WebServer.URL, "test", "test@example.com"),
		sandboxController:    sandbox.New(st, nil, nil, cfg.WebServer.URL, workDir),
		Stripe:               stripe.NewStripe(config.Stripe{}, st),
	}
	execCtrl := gomock.NewController(t)
	executor := external_agent.NewMockExecutor(execCtrl)
	stubAllMethods(execCtrl, executor, reflect.TypeOf((*external_agent.Executor)(nil)).Elem())
	s.externalAgentExecutor = executor
	s.gitHTTPServer = services.NewGitHTTPServer(st, nil, services.GitHTTPServerConfig{
		ServerBaseURL: cfg.WebServer.URL, AuthTokenHeader: "Authorization", EnablePush: true, EnablePull: true,
	}, s.authorizeUserToRepository, nil)
	s.authMiddleware = newAuthMiddleware(nil, nil, st, authMiddlewareConfig{runnerToken: routeTestRunnerToken}, cfg, nil)
	if err := s.buildRouter(context.Background()); err != nil {
		t.Fatalf("build router: %v", err)
	}
	return s
}

// errNotWired is what stubbed subsystems answer in route tests.
var errNotWired = errors.New("not available in the route test server")

// stubAllMethods makes every method of iface on a gomock mock answer zero
// values and errNotWired, so handlers that reach the subsystem (often from a
// background goroutine) fail cleanly instead of dereferencing nil.
func stubAllMethods(ctrl *gomock.Controller, mock any, iface reflect.Type) {
	errT := reflect.TypeOf((*error)(nil)).Elem()
	for i := 0; i < iface.NumMethod(); i++ {
		m := iface.Method(i)
		mt := m.Type
		matchers := make([]any, mt.NumIn())
		for j := range matchers {
			matchers[j] = gomock.Any()
		}
		fn := reflect.MakeFunc(mt, func([]reflect.Value) []reflect.Value {
			outs := make([]reflect.Value, mt.NumOut())
			for k := range outs {
				if mt.Out(k) == errT {
					outs[k] = reflect.ValueOf(&errNotWired).Elem()
				} else {
					outs[k] = reflect.Zero(mt.Out(k))
				}
			}
			return outs
		})
		ctrl.RecordCallWithMethodType(mock, m.Name, mt, matchers...).DoAndReturn(fn.Interface()).AnyTimes()
	}
}

var (
	servedRoutesOnce sync.Once
	servedRoutesVal  []servedRoute
	servedRoutesErr  error
)

// walkServedRoutes lists every (method, template) the production router
// serves, with the auth gate derived from its middleware chain.
func walkServedRoutes(t *testing.T, router *mux.Router) []servedRoute {
	t.Helper()
	var out []servedRoute
	err := router.Walk(func(route *mux.Route, r *mux.Router, ancestors []*mux.Route) error {
		h := route.GetHandler()
		if h == nil {
			return nil
		}
		if _, isRouter := h.(*mux.Router); isRouter {
			return nil // subrouter mount; its routes are walked on their own
		}
		tmpl, err := route.GetPathTemplate()
		if err != nil {
			tmpl = "" // MatcherFunc-only route (e.g. /debug)
		}
		if tmpl == "" {
			tmpl = "<matcher>"
		}
		chain := routeMiddlewareChain(router, r, ancestors)
		gate := gateFromChain(chain)
		methods, err := route.GetMethods()
		if err != nil || len(methods) == 0 {
			methods = []string{"ANY"}
		}
		for _, m := range methods {
			if m == http.MethodOptions {
				continue // CORS preflight; requireUser passes it through by design
			}
			out = append(out, servedRoute{Method: m, Template: tmpl, Gate: gate, Chain: chain, route: route})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk router: %v", err)
	}
	return out
}

// routeMiddlewareChain returns the names of the middleware wrapping a route:
// the root router's, then each enclosing subrouter's.
func routeMiddlewareChain(root, owner *mux.Router, ancestors []*mux.Route) []string {
	routers := []*mux.Router{root}
	for _, a := range ancestors {
		if sub, ok := a.GetHandler().(*mux.Router); ok && sub != root {
			routers = append(routers, sub)
		}
	}
	if owner != nil && owner != routers[len(routers)-1] {
		routers = append(routers, owner)
	}
	var names []string
	for _, rt := range routers {
		mws := reflect.ValueOf(rt).Elem().FieldByName("middlewares")
		for i := 0; i < mws.Len(); i++ {
			fn := mws.Index(i)
			for fn.Kind() == reflect.Interface {
				fn = fn.Elem()
			}
			if fn.Kind() != reflect.Func {
				continue
			}
			names = append(names, shortFuncName(runtime.FuncForPC(fn.Pointer()).Name()))
		}
	}
	return names
}

func shortFuncName(full string) string {
	full = strings.TrimSuffix(full, "-fm")
	if i := strings.LastIndex(full, "/"); i >= 0 {
		full = full[i+1:]
	}
	return full
}

func gateFromChain(chain []string) routeAuthClass {
	has := func(name string) bool {
		for _, c := range chain {
			if c == "server."+name {
				return true
			}
		}
		return false
	}
	switch {
	case has("requireAdmin"):
		return classAdmin
	case has("requireAdminOrRunner"):
		return classAdminOrRunner
	case has("requireRunner"):
		return classRunner
	case has("requireUserOrRunner"):
		return gateUserOrRunner
	case has("requireUser"):
		return classUser
	}
	return ""
}

// gate reports which router gate a declared policy must sit behind.
func (p routePolicy) gate() routeAuthClass {
	switch p.Class {
	case classUser, classUserObject:
		if p.RunnerAllowed {
			return gateUserOrRunner
		}
		return classUser
	case classAdmin, classRunner, classAdminOrRunner:
		return p.Class
	}
	return ""
}

func routeTestServedRoutes(t *testing.T) []servedRoute {
	t.Helper()
	servedRoutesOnce.Do(func() {
		ctrl := gomock.NewController(t)
		s := newRouteTestServer(t, store.NewMockStore(ctrl))
		servedRoutesVal = walkServedRoutes(t, s.router)
	})
	if servedRoutesErr != nil {
		t.Fatal(servedRoutesErr)
	}
	return servedRoutesVal
}

func suggestedPolicy(r servedRoute) string {
	switch r.Gate {
	case classAdmin:
		return "admin()"
	case classRunner:
		return "runner()"
	case classAdminOrRunner:
		return "adminOrRunner()"
	case gateUserOrRunner:
		if len(pathVars(r.Template)) > 0 {
			return `object("authorizeUserToX").alsoRunner()`
		}
		return "user().alsoRunner()"
	case classUser:
		if len(pathVars(r.Template)) > 0 {
			return `object("authorizeUserToX")`
		}
		return "user()"
	}
	return `public("why this needs no credentials") / webhook("which signature the handler checks") / handlerAuth("how the handler authenticates")`
}

var pathVarRE = regexp.MustCompile(`\{([^}:]+)(?::[^}]*)?\}`)

func pathVars(tmpl string) []string {
	var vars []string
	for _, m := range pathVarRE.FindAllStringSubmatch(tmpl, -1) {
		vars = append(vars, m[1])
	}
	return vars
}

// --- Test A -----------------------------------------------------------------

func TestRouteAuthPolicy_EveryRouteIsDeclared(t *testing.T) {
	served := routeTestServedRoutes(t)
	seen := map[string]bool{}
	var missing []string
	for _, r := range served {
		if seen[r.Key()] {
			continue // same method+template mounted twice: first one wins at runtime
		}
		seen[r.Key()] = true
		if _, ok := routeAuthPolicies[r.Key()]; !ok {
			missing = append(missing, fmt.Sprintf("\t%s:\t%q: %s,", routePolicyFile(r.Key()), r.Key(), suggestedPolicy(r)))
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf("%d route(s) are served without an authorization policy.\n"+
			"Declare each one, with the class of caller it admits, in the api/pkg/server file named before it.\n"+
			"Suggested entries (pick the right class and authz helper):\n%s",
			len(missing), strings.Join(missing, "\n"))
	}

	var stale []string
	for key := range routeAuthPolicies {
		if !seen[key] {
			stale = append(stale, "\t"+routePolicyDeclaredIn[key]+": "+strconv.Quote(key))
		}
	}
	if len(stale) > 0 {
		sort.Strings(stale)
		t.Errorf("%d policy entr(ies) name a route the router no longer serves; delete them:\n%s",
			len(stale), strings.Join(stale, "\n"))
	}
}

func TestRouteAuthPolicy_DeclarationsAreComplete(t *testing.T) {
	for key, p := range routeAuthPolicies {
		if want := routePolicyFile(key); routePolicyDeclaredIn[key] != want {
			t.Errorf("%s is declared in %s; move it to %s", key, routePolicyDeclaredIn[key], want)
		}
		switch p.Class {
		case classPublic, classSignedWebhook, classHandlerAuth:
			if strings.TrimSpace(p.Reason) == "" {
				t.Errorf("%s: %s routes need a Reason saying how the caller is trusted", key, p.Class)
			}
		case classUserObject:
			if len(p.Authz) == 0 {
				t.Errorf("%s: user-object routes must name the authorization helper the handler calls", key)
			}
		case classUser, classAdmin, classRunner, classAdminOrRunner:
		default:
			t.Errorf("%s: unknown class %q", key, p.Class)
		}
	}
}

func TestRouteAuthPolicy_DeclaredClassMatchesRouter(t *testing.T) {
	for _, r := range routeTestServedRoutes(t) {
		p, ok := routeAuthPolicies[r.Key()]
		if !ok {
			continue // reported by TestRouteAuthPolicy_EveryRouteIsDeclared
		}
		if want := p.gate(); want != r.Gate {
			got := string(r.Gate)
			if got == "" {
				got = "no auth middleware"
			}
			t.Errorf("%s is declared %q, which must be mounted behind the %q gate, but the router serves it with %s (middleware: %s).\n"+
				"Either mount it on the router the declaration names or correct the declaration in %s.",
				r.Key(), p.Class, orNone(want), got, strings.Join(r.Chain, " -> "), routePolicyFile(r.Key()))
		}
	}
}

func orNone(c routeAuthClass) string {
	if c == "" {
		return "none (public/handler-auth)"
	}
	return string(c)
}

// --- handler source index (used by Test C) ---------------------------------

// routeRegistration is one HandleFunc/Handle call found in the package source.
type routeRegistration struct {
	Path     string   // the literal path argument
	Methods  []string // from a chained .Methods(...)
	Handlers []string // package functions/methods referenced by the handler expression
	Body     *ast.BlockStmt
}

type packageSource struct {
	fset         *token.FileSet
	funcs        map[string]*ast.FuncDecl // by bare name; methods and functions
	regs         []routeRegistration
	serverFields map[string]bool // HelixAPIServer struct fields: the server's dependencies
}

var (
	pkgSourceOnce sync.Once
	pkgSourceVal  *packageSource
	pkgSourceErr  error
)

func loadPackageSource(t *testing.T) *packageSource {
	t.Helper()
	pkgSourceOnce.Do(func() { pkgSourceVal, pkgSourceErr = parsePackageSource(".") })
	if pkgSourceErr != nil {
		t.Fatal(pkgSourceErr)
	}
	return pkgSourceVal
}

func parsePackageSource(dir string) (*packageSource, error) {
	fset := token.NewFileSet()
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, err
	}
	src := &packageSource{fset: fset, funcs: map[string]*ast.FuncDecl{}, serverFields: map[string]bool{}}
	var parsed []*ast.File
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			return nil, err
		}
		parsed = append(parsed, file)
		ast.Inspect(file, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok || ts.Name.Name != "HelixAPIServer" {
				return true
			}
			if st, ok := ts.Type.(*ast.StructType); ok {
				for _, f := range st.Fields.List {
					for _, name := range f.Names {
						src.serverFields[name.Name] = true
					}
				}
			}
			return false
		})
		for _, d := range file.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Body != nil {
				// Methods on different receivers can share a name; keep the
				// HelixAPIServer one, which is what route handlers are.
				if prev, dup := src.funcs[fd.Name.Name]; dup && receiverName(prev) == "HelixAPIServer" {
					continue
				}
				src.funcs[fd.Name.Name] = fd
			}
		}
	}
	stringVars := packageStringLists(parsed)
	for _, file := range parsed {
		for _, d := range file.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			src.scanRegistrations(fd, stringVars)
		}
	}
	return src, nil
}

// packageStringLists collects package-level `var x = []string{"a", "b"}`.
func packageStringLists(files []*ast.File) map[string][]string {
	out := map[string][]string{}
	for _, file := range files {
		for _, d := range file.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, spec := range gd.Specs {
				vs := spec.(*ast.ValueSpec)
				for i, name := range vs.Names {
					if i < len(vs.Values) {
						if lits := stringListLiteral(vs.Values[i]); lits != nil {
							out[name.Name] = lits
						}
					}
				}
			}
		}
	}
	return out
}

func stringListLiteral(e ast.Expr) []string {
	cl, ok := e.(*ast.CompositeLit)
	if !ok {
		return nil
	}
	var out []string
	for _, elt := range cl.Elts {
		lit, ok := elt.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return nil
		}
		v, _ := strconv.Unquote(lit.Value)
		out = append(out, v)
	}
	return out
}

// scanRegistrations records every HandleFunc/Handle call in fd. Paths built
// from local variables (`base+"/{id}"` inside a range over a string list)
// are expanded to every value they can take.
func (src *packageSource) scanRegistrations(fd *ast.FuncDecl, pkgLists map[string][]string) {
	strVals := map[string][]string{} // local ident -> possible string values
	exprs := map[string]ast.Expr{}   // local ident -> assigned expression
	chained := map[*ast.CallExpr]bool{}
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.RangeStmt:
			if v, ok := x.Value.(*ast.Ident); ok {
				vals := stringListLiteral(x.X)
				if id, ok := x.X.(*ast.Ident); ok && vals == nil {
					vals = pkgLists[id.Name]
				}
				if vals != nil {
					strVals[v.Name] = vals
				}
			}
		case *ast.AssignStmt:
			if len(x.Lhs) == 1 && len(x.Rhs) == 1 {
				if id, ok := x.Lhs[0].(*ast.Ident); ok {
					exprs[id.Name] = x.Rhs[0]
					if vals := evalStrings(x.Rhs[0], strVals); vals != nil {
						strVals[id.Name] = vals
					}
				}
			}
		case *ast.CallExpr:
			src.recordRegistration(x, strVals, exprs, chained)
		}
		return true
	})
}

func evalStrings(e ast.Expr, vars map[string][]string) []string {
	switch x := e.(type) {
	case *ast.BasicLit:
		if x.Kind != token.STRING {
			return nil
		}
		v, _ := strconv.Unquote(x.Value)
		return []string{v}
	case *ast.Ident:
		return vars[x.Name]
	case *ast.ParenExpr:
		return evalStrings(x.X, vars)
	case *ast.BinaryExpr:
		if x.Op != token.ADD {
			return nil
		}
		l, r := evalStrings(x.X, vars), evalStrings(x.Y, vars)
		if l == nil || r == nil {
			return nil
		}
		var out []string
		for _, a := range l {
			for _, b := range r {
				out = append(out, a+b)
			}
		}
		return out
	}
	return nil
}

func (src *packageSource) recordRegistration(call *ast.CallExpr, strVals map[string][]string, exprs map[string]ast.Expr, chained map[*ast.CallExpr]bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}
	var reg *ast.CallExpr
	var methods []string
	switch sel.Sel.Name {
	case "Methods":
		inner, ok := sel.X.(*ast.CallExpr)
		if !ok {
			return
		}
		isel, ok := inner.Fun.(*ast.SelectorExpr)
		if !ok || (isel.Sel.Name != "HandleFunc" && isel.Sel.Name != "Handle") {
			return
		}
		reg = inner
		chained[inner] = true
		for _, a := range call.Args {
			methods = append(methods, methodLiteral(a))
		}
	case "Handler": // router.PathPrefix("/x/").Handler(h)
		inner, ok := sel.X.(*ast.CallExpr)
		if !ok || len(inner.Args) != 1 || len(call.Args) != 1 {
			return
		}
		if isel, ok := inner.Fun.(*ast.SelectorExpr); !ok || isel.Sel.Name != "PathPrefix" {
			return
		}
		reg = &ast.CallExpr{Fun: inner.Fun, Args: []ast.Expr{inner.Args[0], call.Args[0]}}
	case "HandleFunc", "Handle":
		if chained[call] {
			return // recorded with its methods by the enclosing .Methods() call
		}
		reg = call
	default:
		return
	}
	if len(reg.Args) != 2 {
		return
	}
	paths := evalStrings(reg.Args[0], strVals)
	if paths == nil {
		return
	}
	var handlers []string
	var body *ast.BlockStmt
	seen := map[string]bool{}
	var visit func(e ast.Node)
	visit = func(e ast.Node) {
		ast.Inspect(e, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.SelectorExpr:
				if fd, ok := src.funcs[x.Sel.Name]; ok && receiverName(fd) != "authMiddleware" {
					handlers = append(handlers, x.Sel.Name)
				}
			case *ast.Ident:
				if fd, ok := src.funcs[x.Name]; ok && fd.Recv == nil {
					handlers = append(handlers, x.Name)
				} else if assigned, ok := exprs[x.Name]; ok && !seen[x.Name] {
					seen[x.Name] = true
					visit(assigned) // handler built into a local variable first
				}
			case *ast.FuncLit:
				body = x.Body
				return false
			}
			return true
		})
	}
	visit(reg.Args[1])
	for _, p := range paths {
		src.regs = append(src.regs, routeRegistration{Path: p, Methods: methods, Handlers: handlers, Body: body})
	}
}

func receiverName(fd *ast.FuncDecl) string {
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return ""
	}
	expr := fd.Recv.List[0].Type
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if id, ok := expr.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

func methodLiteral(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.BasicLit:
		s, _ := strconv.Unquote(x.Value)
		return s
	case *ast.SelectorExpr: // http.MethodGet
		return strings.ToUpper(strings.TrimPrefix(x.Sel.Name, "Method"))
	}
	return ""
}

// registrationsFor finds the source registrations for a served route: the
// template is the literal path under one of the API's mount prefixes and the
// methods match.
func (src *packageSource) registrationsFor(r servedRoute) []routeRegistration {
	var best []routeRegistration
	bestLen := -1
	for _, reg := range src.regs {
		if reg.Path == "" || !strings.HasSuffix(r.Template, reg.Path) {
			continue
		}
		switch strings.TrimSuffix(r.Template, reg.Path) { // the subrouter the literal was mounted on
		case "", APIPrefix, githubCompatPrefix:
		default:
			continue
		}
		if r.Method != "ANY" && len(reg.Methods) > 0 && !routeListHas(reg.Methods, r.Method) {
			continue
		}
		if r.Method == "ANY" && len(reg.Methods) > 0 {
			continue
		}
		switch {
		case len(reg.Path) > bestLen:
			best, bestLen = []routeRegistration{reg}, len(reg.Path)
		case len(reg.Path) == bestLen:
			best = append(best, reg)
		}
	}
	return best
}

func routeListHas(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// references reports the package functions a function body calls or names.
func (src *packageSource) references(body *ast.BlockStmt) map[string]bool {
	refs := map[string]bool{}
	if body == nil {
		return refs
	}
	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.SelectorExpr:
			// Only HelixAPIServer methods: a bare-name match on another
			// type's method (summary.State()) is not a call into this package.
			if fd, ok := src.funcs[x.Sel.Name]; ok && receiverName(fd) == "HelixAPIServer" {
				refs[x.Sel.Name] = true
			}
		case *ast.Ident:
			if fd, ok := src.funcs[x.Name]; ok && fd.Recv == nil {
				refs[x.Name] = true
			}
		}
		return true
	})
	return refs
}

// isAuthzHelper reports whether name is an authorize* function, or a package
// function that (transitively) calls one.
func (src *packageSource) isAuthzHelper(name string) bool {
	return src.reachesAuthorize(name, map[string]bool{})
}

func (src *packageSource) reachesAuthorize(name string, visiting map[string]bool) bool {
	if strings.HasPrefix(name, "authorize") {
		return true
	}
	if visiting[name] {
		return false
	}
	visiting[name] = true
	fd, ok := src.funcs[name]
	if !ok {
		return false
	}
	for callee := range src.references(fd.Body) {
		if src.reachesAuthorize(callee, visiting) {
			return true
		}
	}
	return false
}

// --- Test C -----------------------------------------------------------------

// TestRouteAuthPolicy_ObjectRoutesCallAuthzHelper: a user-router route that
// takes an object id in its path must name, in its policy, the authorization
// helper its handler calls, and the handler must call it.
func TestRouteAuthPolicy_ObjectRoutesCallAuthzHelper(t *testing.T) {
	src := loadPackageSource(t)
	for _, r := range routeTestServedRoutes(t) {
		p, ok := routeAuthPolicies[r.Key()]
		if !ok {
			continue
		}
		if (r.Gate == classUser || r.Gate == gateUserOrRunner) && len(pathVars(r.Template)) > 0 && p.Class == classUser && strings.TrimSpace(p.Reason) == "" {
			t.Errorf("%s takes an object id in its path but is declared user() with no authorization helper.\n"+
				"Declare it object(\"<helper>\") naming the authorize* helper the handler calls before touching the object,\n"+
				"or noAuthzHelper(\"<how the handler scopes the object to the caller>\") in %s.",
				r.Key(), routePolicyFile(r.Key()))
			continue
		}
		if len(p.Authz) == 0 {
			continue
		}
		regs := src.registrationsFor(r)
		if len(regs) == 0 {
			t.Errorf("%s: cannot find the HandleFunc/Handle call that registers it, so its handler cannot be checked for %v. "+
				"Register it with a literal path, or declare it noAuthzHelper(...) with the reason.", r.Key(), p.Authz)
			continue
		}
		refs := map[string]bool{}
		var handlers []string
		for _, reg := range regs {
			for k := range src.references(reg.Body) {
				refs[k] = true
			}
			for _, h := range reg.Handlers {
				handlers = append(handlers, h)
				refs[h] = true
				if fd, ok := src.funcs[h]; ok {
					for k := range src.references(fd.Body) {
						refs[k] = true
					}
				}
			}
		}
		for _, helper := range p.Authz {
			if !src.isAuthzHelper(helper) {
				t.Errorf("%s: %q is not an authorization helper (neither authorize* nor a function that calls one). Fix the declaration in %s.",
					r.Key(), helper, routePolicyFile(r.Key()))
				continue
			}
			if !refs[helper] {
				t.Errorf("%s: declared to authorize with %s, but its handler (%s) never calls it.\n"+
					"Call %s before reading or changing the object, or correct the declaration in %s.",
					r.Key(), helper, strings.Join(handlers, ", "), helper, routePolicyFile(r.Key()))
				continue
			}
			for _, body := range handlerBodies(src, regs) {
				if call, ok := src.dataCallBeforeHelper(body, helper); ok {
					t.Errorf("%s: the handler calls %s before %s.\n"+
						"Only loads (Get/Load/Lookup/Find/List/Count/...) of the object may come before the authorization helper; "+
						"move %s after it.", r.Key(), call, helper, call)
				}
			}
		}
	}
}

// handlerBodies returns the bodies of a route's handler functions.
func handlerBodies(src *packageSource, regs []routeRegistration) []*ast.BlockStmt {
	var bodies []*ast.BlockStmt
	for _, reg := range regs {
		if reg.Body != nil {
			bodies = append(bodies, reg.Body)
		}
		for _, h := range reg.Handlers {
			if fd, ok := src.funcs[h]; ok {
				bodies = append(bodies, fd.Body)
			}
		}
	}
	return bodies
}

// readOnlyCall matches dependency methods that only load data, which a
// handler may call to find the object it then authorizes.
var readOnlyCall = regexp.MustCompile(`^(Get|Load|Lookup|Find|List|Count|Has|Is|Exists|Resolve)`)

// dataCallBeforeHelper reports the first call into a server dependency
// (server.Store.X, server.Controller.X, server.someService.X) that
// is not a load and comes before the first reference to helper in body.
// It reports nothing when body does not reference helper.
func (src *packageSource) dataCallBeforeHelper(body *ast.BlockStmt, helper string) (string, bool) {
	helperPos := token.NoPos
	ast.Inspect(body, func(n ast.Node) bool {
		var name string
		switch x := n.(type) {
		case *ast.SelectorExpr:
			name = x.Sel.Name
		case *ast.Ident:
			name = x.Name
		}
		if name == helper && (helperPos == token.NoPos || n.Pos() < helperPos) {
			helperPos = n.Pos()
		}
		return true
	})
	if helperPos == token.NoPos {
		return "", false
	}
	var first string
	firstPos := token.NoPos
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || call.Pos() >= helperPos {
			return true
		}
		method, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		field, ok := method.X.(*ast.SelectorExpr) // receiver.<field>.<method>
		if !ok {
			return true
		}
		if _, isRecv := field.X.(*ast.Ident); !isRecv {
			return true
		}
		if !src.serverFields[field.Sel.Name] || field.Sel.Name == "Cfg" {
			return true // only the server's dependencies (Store, Controller, services)
		}
		if readOnlyCall.MatchString(method.Sel.Name) {
			return true
		}
		if firstPos == token.NoPos || call.Pos() < firstPos {
			first, firstPos = field.Sel.Name+"."+method.Sel.Name, call.Pos()
		}
		return true
	})
	return first, firstPos != token.NoPos
}

// TestRouteAuthPolicy_Draft prints declarations for every served route with
// the authz helpers each handler calls. It is a tool for writing the table,
// not a check: HELIX_ROUTE_POLICY_DRAFT=1 go test -run Draft ./pkg/server/
func TestRouteAuthPolicy_Draft(t *testing.T) {
	if os.Getenv("HELIX_ROUTE_POLICY_DRAFT") == "" {
		t.Skip("set HELIX_ROUTE_POLICY_DRAFT=1 to print a draft policy table")
	}
	src := loadPackageSource(t)
	seen := map[string]bool{}
	for _, r := range routeTestServedRoutes(t) {
		if seen[r.Key()] {
			continue
		}
		seen[r.Key()] = true
		var handlers, helpers []string
		for _, reg := range src.registrationsFor(r) {
			refs := src.references(reg.Body)
			for _, h := range reg.Handlers {
				handlers = append(handlers, h)
				if fd, ok := src.funcs[h]; ok {
					for k := range src.references(fd.Body) {
						refs[k] = true
					}
				}
			}
			for k := range refs {
				if src.isAuthzHelper(k) && !routeListHas(helpers, k) {
					helpers = append(helpers, k)
				}
			}
		}
		sort.Strings(helpers)
		fmt.Printf("DRAFT\t%s\t%s\t%s\t%s\n", r.Key(), r.Gate, strings.Join(handlers, ","), strings.Join(helpers, ","))
	}
}
