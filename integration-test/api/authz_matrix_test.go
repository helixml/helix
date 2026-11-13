package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	authpkg "github.com/helixml/helix/api/pkg/auth"
)

// The authorization matrix drives every row of the route tables below as
// every listed actor against one shared fixture (two organizations, their
// members and resources, and each kind of credential). See the README section
// "Authorization matrix" for how to add a row.

// actor names one credential in the fixture.
type actor string

const (
	// Organization A.
	aOwner       actor = "a-owner"         // personal API key of A's owner
	aOwnerOrgKey actor = "a-owner-org-key" // org API key minted by A's owner for org A
	// aOwnerBrowser is A's owner signed in to the web app: only the BFF
	// session cookie, no API key.
	aOwnerBrowser actor = "a-owner-browser"
	aMember       actor = "a-member" // personal key of a non-admin member of A
	// projKey is a spec-task session key as minted for a sandbox agent: A's
	// owner, scoped to project projA, session sessA and spec task taskA.
	projKey actor = "a-project-key"
	// sessKey is a session key with no project, as minted for a plain chat's
	// desktop: A's owner, scoped to session sessChat. The API middleware
	// confines it to that session's desktop routes and the LLM proxy.
	sessKey actor = "a-session-key"
	// projChatKey is the desktop key of sessProjChat, a projA session with
	// no spec task or desktop role, minted by GetOrCreateSessionAPIKey: it
	// carries projA.
	projChatKey actor = "a-project-chat-key"
	// sandboxKey is the key of a sandbox outside any project, as
	// ensureSandboxAPIToken mints it: A's owner, org A, bound to a sandbox id.
	sandboxKey actor = "a-sandbox-key"
	// appKey is an app key for agent appA minted by A's owner.
	appKey actor = "a-app-key"
	// workerKey is a helix-org worker's session key: A's owner, org A,
	// session sessWorker running as the chief-of-staff bot.
	workerKey actor = "a-worker-key"
	// exploreKey is the key of projA's exploratory desktop: A's owner,
	// scoped to projA and session sessExplore, no spec task
	// (services.sessionKeyScope).
	exploreKey actor = "a-exploratory-key"

	// Everyone else.
	bOwner     actor = "b-owner"    // owner of organization B
	outsider   actor = "outsider"   // verified user in no organization
	waitlisted actor = "waitlisted" // account still waiting for approval
	admin      actor = "admin"      // deployment administrator
	anonymous  actor = "anonymous"  // no credential at all
)

// want is what a row expects an actor to get.
type want int

const (
	// allow: a 2xx response.
	allow want = iota + 1
	// deny: 401, 403 or 404, and the body leaks no fixture marker.
	deny
	// reached: the request got past authorization but the handler could not
	// finish without infrastructure CI does not run (a sandbox, an LLM): a
	// 2xx-4xx other than 401/403/404. A 401/403/404 or a 5xx passes only
	// when its body carries the row's ReachedMarker, a message the handler
	// produces after authorization.
	reached
)

func (w want) String() string {
	switch w {
	case allow:
		return "allow"
	case deny:
		return "deny"
	case reached:
		return "reached"
	}
	return fmt.Sprintf("want(%d)", int(w))
}

// expect maps actors to outcomes for one route. Actors left out are not
// exercised for that route.
type expect map[actor]want

// route is one row of a table: a request template and what each actor gets.
//
// Path and Body may contain {placeholders}; each is replaced by the fixture
// value of that name (see authzFixture.vars). Rows that change state should
// set Fresh so allowed actors act on a throwaway copy, and Check so the
// shared resource is re-read after every denied attempt.
type route struct {
	Method string
	Path   string
	Body   any
	// Header adds request headers (values are templated too).
	Header map[string]string
	Expect expect
	// Fresh, when set, is called before each allowed or reached attempt and
	// returns placeholder overrides naming a resource created just for it.
	Fresh func(t *testing.T, f *authzFixture) map[string]string
	// Check, when set, is a GET path the fixture owner must still read with
	// the resource's marker after a denied attempt: nothing changed.
	Check string
	// CheckAs is the actor that performs Check (default aOwner).
	CheckAs actor
	// CheckMarker, for a Check that lists several resources, names the
	// fixture resource whose marker must still be in the list. Without it
	// the Check only asserts that no tamper marker appears.
	CheckMarker string
	// ReachedMarker lets a "reached" actor pass with a 403/404 or a 5xx
	// whose body contains this text: an answer produced after authorization,
	// such as "no Claude subscription found for session owner" or "Sandbox
	// not connected". Denied actors must never see it.
	ReachedMarker string
	// PositiveControl, for a row whose every actor is refused, names the
	// row that shows the same route working. Without it such a row fails.
	PositiveControl string
	// Custom, when set, replaces the standard status check for every actor
	// (the row's Expect still decides who runs it).
	Custom func(t *testing.T, f *authzFixture, who actor, resp authzResponse)
	// Skip leaves the row in the table but does not run it. Use
	// "tracked: <one-line description>" for a known authorization gap.
	Skip string
}

func (r route) name() string {
	return r.Method + " " + r.Path
}

// listRow asserts what a list endpoint returns to one actor: every Contains
// placeholder's id must appear in the body and no Excludes id may.
type listRow struct {
	Path     string
	Actor    actor
	Contains []string
	Excludes []string
	Skip     string
}

func requireAuthzServer(t *testing.T) {
	t.Helper()
	if os.Getenv("START_HELIX_TEST_SERVER") != "true" {
		t.Skip("set START_HELIX_TEST_SERVER=true to run API integration tests")
	}
	// Rows exercise the LLM proxy as several actors. With real provider
	// credentials in the server's environment, a row that regresses would
	// make a real, paid upstream call.
	if !providerEnvStripped() {
		for _, kv := range os.Environ() {
			if isProviderEnv(kv) {
				t.Fatalf("the authorization matrix must run with %s=true: %s would reach the test server",
					noProviderEnvVar, strings.SplitN(kv, "=", 2)[0])
			}
		}
	}
}

var placeholderPattern = regexp.MustCompile(`\{([a-zA-Z0-9_]+)\}`)

// expand substitutes {placeholders}. An unknown placeholder fails the test:
// a typo must not quietly turn into a request for a literal "{projA}".
func (f *authzFixture) expand(t *testing.T, s string, overrides map[string]string) string {
	t.Helper()
	return placeholderPattern.ReplaceAllStringFunc(s, func(m string) string {
		key := m[1 : len(m)-1]
		if v, ok := overrides[key]; ok {
			return v
		}
		v, ok := f.vars[key]
		if !ok {
			t.Fatalf("unknown placeholder %s in %q", m, s)
		}
		return v
	})
}

func (f *authzFixture) expandBody(t *testing.T, body any, overrides map[string]string) []byte {
	t.Helper()
	if body == nil {
		return nil
	}
	var raw []byte
	switch b := body.(type) {
	case string:
		raw = []byte(b)
	default:
		encoded, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		raw = encoded
	}
	return []byte(f.expand(t, string(raw), overrides))
}

type authzResponse struct {
	status int
	body   string
}

// do sends one row request as an actor. path is absolute (starts with
// /api/v1, /v1, /git ...).
func (f *authzFixture) do(t *testing.T, who actor, method, path string, body []byte, header map[string]string) authzResponse {
	t.Helper()
	return f.send(t, authzRequestTimeout, who, method, path, body, header)
}

// send is do with an explicit time limit; fixture setup (repositories,
// projects) uses the longer authzSetupTimeout.
func (f *authzFixture) send(t *testing.T, timeout time.Duration, who actor, method, path string, body []byte, header map[string]string) authzResponse {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, integrationServerURL()+path, reader)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	_, basic := header[basicAuthHeader]
	if who == aOwnerBrowser {
		req.AddCookie(&http.Cookie{Name: authpkg.SessionCookieName, Value: f.keys[who]})
	} else if who != anonymous {
		key, ok := f.keys[who]
		if !ok {
			t.Fatalf("fixture has no credential for actor %s", who)
		}
		if basic {
			req.SetBasicAuth("api", key)
		} else {
			req.Header.Set("Authorization", "Bearer "+key)
		}
	}
	for k, v := range header {
		if k != basicAuthHeader {
			req.Header.Set(k, v)
		}
	}
	resp, err := authzHTTPClient.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err) {
			t.Fatalf("%s %s as %s: no answer within %s. A matrix row must not depend on the network (an LLM provider, a sandbox); point it at something that fails fast and assert it with ReachedMarker: %v",
				method, path, who, timeout, err)
		}
		t.Fatalf("%s %s as %s: %v", method, path, who, err)
	}
	defer resp.Body.Close()
	// A completed WebSocket upgrade never ends its body; the status is the
	// whole answer.
	if resp.StatusCode == http.StatusSwitchingProtocols {
		return authzResponse{status: resp.StatusCode}
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return authzResponse{status: resp.StatusCode, body: string(data)}
}

// basicAuthHeader in a row's Header sends the actor's key as an HTTP basic
// auth password instead of a bearer token, the way git does. It is not sent.
const basicAuthHeader = "X-Authz-Basic-Auth"

// authzRequestTimeout bounds every row request, so a row that reaches a real
// upstream fails by name instead of eating the suite's time budget. It is
// well above what a working row needs, so a busy CI runner does not trip it.
const authzRequestTimeout = 20 * time.Second

// authzSetupTimeout bounds fixture setup requests, which do real work
// (creating git repositories and projects).
const authzSetupTimeout = 60 * time.Second

// authzHTTPClient never follows redirects: a 302 to a login page is an
// answer in its own right, not a 200. Each request carries its own deadline.
var authzHTTPClient = &http.Client{
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func isRefusal(status int) bool {
	return status == http.StatusUnauthorized || status == http.StatusForbidden || status == http.StatusNotFound
}

// leakedMarker returns the first fixture marker found in body, or "".
// Markers are unique strings embedded in every fixture resource's name or
// content, so a refusal that echoes one has disclosed the resource.
func (f *authzFixture) leakedMarker(body string) string {
	for name, marker := range f.markers {
		if strings.Contains(body, marker) {
			return name
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// runRoutes runs every row of a table as a subtest per (route, actor).
func runRoutes(t *testing.T, f *authzFixture, routes []route) {
	t.Helper()
	for _, r := range routes {
		r := r
		t.Run(r.name(), func(t *testing.T) {
			if r.Skip != "" {
				t.Skip(r.Skip)
			}
			// Positive control: a row nobody may use cannot tell a working
			// refusal from a route that is simply broken.
			hasPositive := false
			for _, w := range r.Expect {
				if w == allow || w == reached {
					hasPositive = true
				}
			}
			if !hasPositive && r.PositiveControl == "" {
				t.Fatalf("row %s has no allow/reached actor; every row needs a positive control", r.name())
			}

			// Denials run first, against the shared resource, so an allowed
			// mutation cannot make a later refusal pass vacuously.
			actors := make([]actor, 0, len(r.Expect))
			for a := range r.Expect {
				actors = append(actors, a)
			}
			sort.Slice(actors, func(i, j int) bool {
				wi, wj := r.Expect[actors[i]] == deny, r.Expect[actors[j]] == deny
				if wi != wj {
					return wi
				}
				return actors[i] < actors[j]
			})

			for _, who := range actors {
				expected := r.Expect[who]
				t.Run(string(who), func(t *testing.T) {
					var overrides map[string]string
					if expected != deny && r.Fresh != nil {
						overrides = r.Fresh(t, f)
					}
					path := f.expand(t, r.Path, overrides)
					body := f.expandBody(t, r.Body, overrides)
					header := map[string]string{}
					for k, v := range r.Header {
						header[k] = f.expand(t, v, overrides)
					}
					resp := f.do(t, who, r.Method, path, body, header)
					if r.Custom != nil {
						r.Custom(t, f, who, resp)
						return
					}
					f.assertOutcome(t, who, expected, r, path, resp)
					if expected == deny && r.Check != "" {
						checker := r.CheckAs
						if checker == "" {
							checker = aOwner
						}
						f.assertUnchanged(t, checker, f.expand(t, r.Check, nil), r.CheckMarker)
					}
				})
			}
		})
	}
}

func (f *authzFixture) assertOutcome(t *testing.T, who actor, expected want, r route, path string, resp authzResponse) {
	t.Helper()
	ctx := fmt.Sprintf("%s %s as %s: status %d body %q", r.Method, path, who, resp.status, truncate(resp.body, 400))
	pastAuthz := r.ReachedMarker != "" && strings.Contains(resp.body, r.ReachedMarker)
	switch expected {
	case allow:
		if resp.status < 200 || resp.status > 299 {
			t.Fatalf("expected 2xx; %s", ctx)
		}
	case reached:
		// A 5xx is never taken as proof of authorization on its own: a
		// crash looks the same. It passes only with the row's marker.
		if (isRefusal(resp.status) || resp.status >= 500) && !pastAuthz {
			t.Fatalf("expected the request to get past authorization (2xx-4xx other than 401/403/404, or the row's ReachedMarker); %s", ctx)
		}
	case deny:
		if !isRefusal(resp.status) {
			t.Fatalf("expected 401/403/404; %s", ctx)
		}
		if pastAuthz {
			t.Fatalf("expected a refusal before the handler ran; %s", ctx)
		}
		if leaked := f.leakedMarker(resp.body); leaked != "" {
			t.Fatalf("refusal leaked fixture resource %s; %s", leaked, ctx)
		}
	default:
		t.Fatalf("row has no expectation for %s", who)
	}
}

// assertUnchanged re-reads a resource after a refused mutation. It must still
// be there and still carry its marker (an update that "failed" but wrote the
// new name would lose it).
func (f *authzFixture) assertUnchanged(t *testing.T, who actor, path, marker string) {
	t.Helper()
	resp := f.do(t, who, http.MethodGet, path, nil, nil)
	if resp.status != http.StatusOK {
		t.Fatalf("resource changed after a refused request: GET %s as %s = %d %q", path, who, resp.status, truncate(resp.body, 300))
	}
	if marker != "" {
		if !strings.Contains(resp.body, f.markers[marker]) {
			t.Fatalf("refused request removed or renamed %s: GET %s returned %q", marker, path, truncate(resp.body, 300))
		}
		return
	}
	if strings.Contains(resp.body, tamperMarker) {
		t.Fatalf("refused request still modified the resource: GET %s returned %q", path, truncate(resp.body, 300))
	}
}

// tamperMarker is written by every mutation row. Seeing it after a refusal
// means the write happened.
const tamperMarker = "authz-tampered"

func runListRows(t *testing.T, f *authzFixture, rows []listRow) {
	t.Helper()
	positive := map[string]bool{}
	for _, r := range rows {
		if len(r.Contains) > 0 && r.Skip == "" {
			positive[r.Path] = true
		}
	}
	for _, r := range rows {
		r := r
		t.Run(fmt.Sprintf("GET %s as %s", r.Path, r.Actor), func(t *testing.T) {
			if r.Skip != "" {
				t.Skip(r.Skip)
			}
			if !positive[r.Path] {
				t.Fatalf("list %s has no row with Contains; add a positive control", r.Path)
			}
			path := f.expand(t, r.Path, nil)
			resp := f.do(t, r.Actor, http.MethodGet, path, nil, nil)
			if len(r.Contains) > 0 && resp.status != http.StatusOK {
				t.Fatalf("GET %s as %s: status %d %q", path, r.Actor, resp.status, truncate(resp.body, 300))
			}
			if resp.status != http.StatusOK && !isRefusal(resp.status) {
				t.Fatalf("GET %s as %s: unexpected status %d %q", path, r.Actor, resp.status, truncate(resp.body, 300))
			}
			for _, name := range r.Contains {
				if id := f.expand(t, "{"+name+"}", nil); !strings.Contains(resp.body, id) {
					t.Errorf("GET %s as %s: missing %s (%s)", path, r.Actor, name, id)
				}
			}
			for _, name := range r.Excludes {
				if id := f.expand(t, "{"+name+"}", nil); strings.Contains(resp.body, id) {
					t.Errorf("GET %s as %s: returned %s (%s), which it must not see", path, r.Actor, name, id)
				}
			}
		})
	}
}
