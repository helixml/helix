package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"regexp"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"go.uber.org/mock/gomock"
)

// Enforcement smoke test (Test B).
//
// Every non-public route is called through the real router with
//   - no credentials, and
//   - a credential of the wrong class: a plain user API key on admin and
//     runner routes, the runner token on user routes;
//   - and, on user-object routes, a plain user key against an object that
//     belongs to somebody else.
//
// The store answers every lookup with an object owned by another user in
// another organization, and every membership/grant lookup with "not found",
// so an object route that serves the request has skipped its authorization.
// Only 401, 403 and 404 count as refused.

const (
	victimUserID = "usr_route_victim"
	victimOrgID  = "org_route_victim"
)

type probeKind string

const (
	probeNoCredentials probeKind = "no credentials"
	probeUserKey       probeKind = "non-admin user API key"
	probeRunnerToken   probeKind = "runner token"
	probeOtherTenant   probeKind = "user key on another tenant's object"
)

// probeCtxKey marks the context of a probe request, so store calls made by
// background goroutines are not attributed to it.
type probeCtxKey struct{}

// storeCallLog records the store methods a probe reached.
// Only calls whose context carries the current probe's id are recorded:
// goroutines an earlier probe left running (detached contexts keep values)
// must not be blamed on the next route.
type storeCallLog struct {
	mu      sync.Mutex
	current int
	calls   []string
	// asOwner switches the store from "every object belongs to another
	// tenant" to "every object belongs to the caller" for the owner probe.
	asOwner atomic.Bool
}

func (l *storeCallLog) add(ctx context.Context, name string) {
	id, _ := ctx.Value(probeCtxKey{}).(int)
	l.mu.Lock()
	if id != 0 && id == l.current {
		l.calls = append(l.calls, name)
	}
	l.mu.Unlock()
}

// start begins a new probe and returns its id.
func (l *storeCallLog) start() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.current++
	l.calls = nil
	return l.current
}

func (l *storeCallLog) take() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := l.calls
	l.calls = nil
	return out
}

// newVictimStore returns a MockStore that answers every method:
//   - the API key and user lookups that authenticate routeTestUserKey;
//   - membership, grant and role lookups: not found;
//   - other getters: an object owned by victimUserID in victimOrgID;
//   - everything else: zero values and no error.
func newVictimStore(t *testing.T) (*store.MockStore, *storeCallLog) {
	t.Helper()
	ctrl := gomock.NewController(t)
	mock := store.NewMockStore(ctrl)
	log := &storeCallLog{}
	iface := reflect.TypeOf((*store.Store)(nil)).Elem()
	for i := 0; i < iface.NumMethod(); i++ {
		m := iface.Method(i)
		name, mt := m.Name, m.Type
		matchers := make([]any, mt.NumIn())
		for j := range matchers {
			matchers[j] = gomock.Any()
		}
		fn := reflect.MakeFunc(mt, func(args []reflect.Value) []reflect.Value {
			if ctx, ok := args[0].Interface().(context.Context); ok && ctx != nil {
				log.add(ctx, name)
			}
			return victimStoreResult(name, mt, args, log.asOwner.Load())
		})
		ctrl.RecordCallWithMethodType(mock, name, mt, matchers...).DoAndReturn(fn.Interface()).AnyTimes()
	}
	return mock, log
}

var (
	errType           = reflect.TypeOf((*error)(nil)).Elem()
	denyingLookupName = regexp.MustCompile(`Membership|AccessGrant|Role|Permission|Grant|Member`)
)

func victimStoreResult(name string, mt reflect.Type, args []reflect.Value, asOwner bool) []reflect.Value {
	switch name {
	case "GetAPIKey":
		q := args[1].Interface().(*types.ApiKey)
		if q != nil && q.Key == routeTestUserKey {
			return values(&types.ApiKey{Key: routeTestUserKey, Owner: routeTestUserID, OwnerType: types.OwnerTypeUser, Type: types.APIkeytypeAPI}, nil)
		}
		return values((*types.ApiKey)(nil), store.ErrNotFound)
	case "GetUser":
		q := args[1].Interface().(*store.GetUserQuery)
		if q != nil && q.ID == routeTestUserID {
			return values(&types.User{ID: routeTestUserID, Type: types.OwnerTypeUser, Email: "caller@example.com"}, nil)
		}
		return values((*types.User)(nil), store.ErrNotFound)
	case "ListAPIKeys":
		// Controller.GetAPIKeys creates a key and lists again until one exists.
		return values([]*types.ApiKey{{Key: routeTestUserKey, Owner: routeTestUserID, OwnerType: types.OwnerTypeUser, Type: types.APIkeytypeAPI}}, nil)
	}
	if asOwner && ctxProbeID(args[0]) != 0 && !ownerReadOnlyCall.MatchString(name) {
		// The owner got past authorization and is about to change state:
		// that is all the positive control needs. Stop the handler here,
		// before it starts background work the test server cannot run.
		panic(ownerReachedWrite{method: name})
	}
	switch name {
	case "GetOrganizationMembership":
		if asOwner {
			return values(&types.OrganizationMembership{OrganizationID: victimOrgID, UserID: routeTestUserID, Role: types.OrganizationRoleOwner}, nil)
		}
	}

	outs := make([]reflect.Value, mt.NumOut())
	deny := !asOwner && denyingLookupName.MatchString(name) && (strings.HasPrefix(name, "Get") || strings.HasPrefix(name, "List") ||
		strings.HasPrefix(name, "Find") || strings.HasPrefix(name, "Has") || strings.HasPrefix(name, "Is") || strings.HasPrefix(name, "Count"))
	for i := range outs {
		ot := mt.Out(i)
		switch {
		case ot == errType:
			if deny && strings.HasPrefix(name, "Get") {
				outs[i] = reflect.ValueOf(&store.ErrNotFound).Elem()
			} else {
				outs[i] = reflect.Zero(errType)
			}
		case ot.Kind() == reflect.Ptr && ot.Elem().Kind() == reflect.Struct && !deny:
			outs[i] = victimObject(ot.Elem(), asOwner)
		default:
			outs[i] = reflect.Zero(ot)
		}
	}
	return outs
}

// ownerReachedWrite aborts an owner-probe request at its first store write.
type ownerReachedWrite struct{ method string }

// ownerReadOnlyCall matches store methods that only read, plus the auth
// bookkeeping every authenticated request does.
var ownerReadOnlyCall = regexp.MustCompile(`^(Get|List|Find|Count|Has|Is|Exists|Search|EnsureUserMeta|Lookup)`)

func ctxProbeID(v reflect.Value) int {
	ctx, ok := v.Interface().(context.Context)
	if !ok || ctx == nil {
		return 0
	}
	id, _ := ctx.Value(probeCtxKey{}).(int)
	return id
}

func values(v any, err error) []reflect.Value {
	e := reflect.Zero(errType)
	if err != nil {
		e = reflect.ValueOf(&err).Elem()
	}
	return []reflect.Value{reflect.ValueOf(v), e}
}

// victimObject builds a *T whose ownership fields name the victim tenant,
// or the caller when asOwner (the org stays the victim org, of which the
// owner-mode store makes the caller an owner).
func victimObject(t reflect.Type, asOwner bool) reflect.Value {
	ownerID := victimUserID
	if asOwner {
		ownerID = routeTestUserID
	}
	v := reflect.New(t)
	e := v.Elem()
	for i := 0; i < t.NumField(); i++ {
		f := e.Field(i)
		if !f.CanSet() || f.Kind() != reflect.String {
			continue
		}
		switch t.Field(i).Name {
		case "ID":
			f.SetString("obj_route_victim")
		case "Owner", "OwnerID", "UserID", "CreatedBy", "CreatorID":
			f.SetString(ownerID)
		case "OrganizationID", "OrgID":
			f.SetString(victimOrgID)
		case "ProjectID": // project-scoped objects (spec tasks) authorize through their project
			f.SetString("obj_route_victim")
		case "OwnerType":
			f.SetString(string(types.OwnerTypeUser))
		case "LocalPath": // git repositories are stat'ed on load
			f.SetString(os.TempDir())
		}
	}
	return v
}

// probeRequest builds a request for the route with dummy path parameters and
// checks the router really dispatches it to that route.
func probeRequest(router *mux.Router, r servedRoute) (*http.Request, error) {
	method := r.Method
	if method == "ANY" {
		method = http.MethodGet
	}
	if r.Template == "<matcher>" {
		return nil, errors.New("matcher-only route has no template")
	}
	candidates := []string{"obj_route_victim", "1", "sci_route_victim"}
	vars := pathVars(r.Template)
	for _, val := range candidates {
		pairs := make([]string, 0, 2*len(vars))
		for _, v := range vars {
			pairs = append(pairs, v, val)
		}
		u, err := r.route.URLPath(pairs...)
		if err != nil {
			continue
		}
		path := u.Path
		if re, err := r.route.GetPathRegexp(); err == nil && !strings.HasSuffix(re, "$") {
			if !strings.HasSuffix(path, "/") {
				path += "/"
			}
			path += "probe" // PathPrefix: reach something under it
		}
		if queries, err := r.route.GetQueriesTemplates(); err == nil && len(queries) > 0 {
			q := make([]string, 0, len(queries))
			for _, kv := range queries {
				q = append(q, strings.SplitN(kv, "=", 2)[0]+"=probe")
			}
			path += "?" + strings.Join(q, "&")
		}
		req := httptest.NewRequest(method, "http://localhost:8080"+path, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		var match mux.RouteMatch
		if router.Match(req, &match) && match.Route == r.route {
			return req, nil
		}
	}
	return nil, fmt.Errorf("no dummy path reaches this route (shadowed by an earlier route?)")
}

// probeTimeout bounds one probe. A handler that blocks is a harness
// artefact (a dependency the test server does not wire), not a verdict.
const probeTimeout = time.Second

// statusOwnerReachedWrite marks an owner probe stopped at its first store
// write: it was let through.
const statusOwnerReachedWrite = 299

type probeOutcome struct {
	status int
	panic  string
	hung   bool
	calls  []string
}

func serveProbe(router http.Handler, req *http.Request, probeID int) probeOutcome {
	ctx, cancel := context.WithTimeout(context.WithValue(req.Context(), probeCtxKey{}, probeID), probeTimeout)
	defer cancel()
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	done := make(chan probeOutcome, 1)
	go func() {
		out := probeOutcome{}
		defer func() {
			if p := recover(); p != nil {
				if w, ok := p.(ownerReachedWrite); ok {
					out.status = statusOwnerReachedWrite
					out.calls = append(out.calls, w.method)
					done <- out
					return
				}
				out.panic = fmt.Sprintf("%v\n%s", p, firstFrames(string(debug.Stack()), 12))
			}
			out.status = rec.Code
			done <- out
		}()
		router.ServeHTTP(rec, req)
	}()
	select {
	case out := <-done:
		return out
	case <-time.After(probeTimeout):
		return probeOutcome{hung: true}
	}
}

func firstFrames(stack string, n int) string {
	lines := strings.Split(stack, "\n")
	var keep []string
	for _, l := range lines {
		if strings.Contains(l, "helix/api/pkg/") && !strings.Contains(l, "_test.go") {
			keep = append(keep, strings.TrimSpace(l))
		}
		if len(keep) >= n {
			break
		}
	}
	return strings.Join(keep, "\n")
}

// refused reports whether a probe was turned away. 401/403/404 are the
// expected answers; another 4xx (a 400 for the probe's empty body, a 405)
// also means nothing was served or changed. 2xx and 5xx never count.
func refused(status int) bool {
	return status >= 400 && status < 500
}

// probesFor lists the wrong-class callers to try for a declared policy.
func probesFor(p routePolicy, hasPathObject bool) []probeKind {
	switch p.Class {
	case classPublic:
		return nil
	case classSignedWebhook:
		return []probeKind{probeNoCredentials}
	case classHandlerAuth:
		if len(p.Authz) > 0 && hasPathObject {
			return []probeKind{probeNoCredentials, probeOtherTenant}
		}
		return []probeKind{probeNoCredentials}
	case classAdmin, classRunner, classAdminOrRunner:
		return []probeKind{probeNoCredentials, probeUserKey}
	case classUser:
		if p.RunnerAllowed {
			return []probeKind{probeNoCredentials}
		}
		return []probeKind{probeNoCredentials, probeRunnerToken}
	case classUserObject:
		probes := []probeKind{probeNoCredentials}
		if hasPathObject {
			probes = append(probes, probeOtherTenant)
		}
		if !p.RunnerAllowed {
			probes = append(probes, probeRunnerToken)
		}
		return probes
	}
	return nil
}

func TestRouteAuthPolicy_Enforced(t *testing.T) {
	st, calls := newVictimStore(t)
	s := newRouteTestServer(t, st)
	s.authMiddleware.lastSeenCache.Store(routeTestUserID, time.Now().Add(time.Hour)) // no background last-seen write

	send := func(r servedRoute, probe probeKind, asOwner bool) (probeOutcome, error) {
		req, err := probeRequest(s.router, r)
		if err != nil {
			return probeOutcome{}, err
		}
		switch probe {
		case probeUserKey, probeOtherTenant:
			req.Header.Set("Authorization", "Bearer "+routeTestUserKey)
		case probeRunnerToken:
			req.Header.Set("Authorization", "Bearer "+routeTestRunnerToken)
		}
		calls.asOwner.Store(asOwner)
		defer calls.asOwner.Store(false)
		out := serveProbe(s.router, req, calls.start())
		out.calls = calls.take()
		return out, nil
	}

	seen := map[string]bool{}
	var problems, inconclusive []string
	conclusiveObjectRoutes := 0
	stillGap := map[string]bool{}
	for _, r := range walkServedRoutes(t, s.router) {
		if seen[r.Key()] {
			continue
		}
		seen[r.Key()] = true
		if r.Template != "<matcher>" {
			if _, err := probeRequest(s.router, r); err != nil {
				problems = append(problems, fmt.Sprintf("%s is never reached: an earlier route matches every request for it. "+
					"Register it before the route that shadows it.", r.Key()))
				continue
			}
		}
		p, ok := routeAuthPolicies[r.Key()]
		if !ok {
			continue // reported by TestRouteAuthPolicy_EveryRouteIsDeclared
		}
		for _, probe := range probesFor(p, len(pathVars(r.Template)) > 0) {
			gapKey := r.Key() + " | " + string(probe)
			if _, skip := probeSkips[gapKey]; skip {
				continue
			}
			out, err := send(r, probe, false)
			if err != nil {
				break // matcher-only route
			}

			var why string
			switch {
			case out.hung || out.panic != "":
				// The handler needs a subsystem the test server leaves nil.
				// Nothing was decided either way.
				inconclusive = append(inconclusive, gapKey+": handler hit an unwired dependency")
				continue
			case !refused(out.status):
				why = fmt.Sprintf("answered %d", out.status)
			case probe == probeNoCredentials && p.Class != classSignedWebhook && p.Class != classHandlerAuth && len(out.calls) > 0:
				why = fmt.Sprintf("refused, but reached the store first (%s)", strings.Join(uniq(out.calls), ", "))
			case probe == probeOtherTenant:
				// A 400 says the body was bad, not that the caller was
				// refused, unless an authorization lookup already ran.
				if out.status != http.StatusUnauthorized && out.status != http.StatusForbidden && out.status != http.StatusNotFound &&
					!callsInclude(out.calls, denyingLookupName) {
					inconclusive = append(inconclusive, fmt.Sprintf("%s: answered %d before any authorization lookup", gapKey, out.status))
					continue
				}
				// Positive control: the owner must get a different answer,
				// or the refusal proves nothing about authorization.
				owner, _ := send(r, probeOtherTenant, true)
				switch {
				case owner.hung || owner.panic != "":
					inconclusive = append(inconclusive, gapKey+": owner control hit an unwired dependency")
					continue
				case owner.status == out.status:
					inconclusive = append(inconclusive, fmt.Sprintf("%s: owner control was refused too (%d)", gapKey, owner.status))
					continue
				}
				conclusiveObjectRoutes++
			}
			if why == "" {
				continue
			}
			if _, known := knownEnforcementGaps[gapKey]; known {
				stillGap[gapKey] = true
				continue
			}
			problems = append(problems, fmt.Sprintf("%s with %s: %s (store calls: %s)",
				r.Key(), probe, why, strings.Join(uniq(out.calls), ", ")))
		}
	}
	for key := range knownEnforcementGaps {
		if !stillGap[key] {
			problems = append(problems, fmt.Sprintf("%s is listed in knownEnforcementGaps but is now refused (or inconclusive): delete the entry", key))
		}
	}
	sort.Strings(inconclusive)
	t.Logf("%d object routes refused a stranger and served their owner; %d probes were inconclusive:\n%s",
		conclusiveObjectRoutes, len(inconclusive), strings.Join(inconclusive, "\n"))
	if conclusiveObjectRoutes < minConclusiveObjectRoutes {
		problems = append(problems, fmt.Sprintf("only %d object routes passed the stranger/owner probe pair (floor %d): "+
			"the harness stopped reaching handlers' authorization, so the probe is no longer evidence. Run with -v for the inconclusive list.",
			conclusiveObjectRoutes, minConclusiveObjectRoutes))
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		t.Errorf("%d route(s) did not refuse a caller their policy excludes.\n"+
			"Only 401/403/404 count as refused. Fix the handler (authorize before loading or acting), or correct the policy in its route_policy_*_test.go file.\n%s",
			len(problems), strings.Join(problems, "\n"))
	}
}

// minConclusiveObjectRoutes is a floor on how many object routes the
// stranger/owner probe pair can tell apart. Raise it when wiring more of the
// test server makes more routes conclusive; never lower it to pass.
const minConclusiveObjectRoutes = 257

func callsInclude(calls []string, re *regexp.Regexp) bool {
	for _, c := range calls {
		if re.MatchString(c) {
			return true
		}
	}
	return false
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// probeSkips lists probes the route test server cannot run because the
// handler needs a subsystem it does not wire. Keep it short: wiring the
// dependency is better than skipping.
var probeSkips = map[string]string{
	"POST /api/v1/teams/webhook/{appID} | no credentials": "needs the trigger manager's Teams bot registry",
}
