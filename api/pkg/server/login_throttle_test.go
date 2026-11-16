package server

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func newTestLoginThrottle(now *time.Time) *loginThrottle {
	t := newLoginThrottle()
	t.now = func() time.Time { return *now }
	t.perAcctClient = 3
	t.perAcct = 6
	t.perClient = 5
	return t
}

// fail reserves an attempt and leaves it counted, as a wrong password does.
func fail(t *testing.T, th *loginThrottle, email, client string) {
	t.Helper()
	attempt, wait := th.reserve(email, client)
	require.NotNil(t, attempt, "attempt for %s from %s refused (wait %s)", email, client, wait)
}

func blocked(th *loginThrottle, email, client string) bool {
	attempt, wait := th.reserve(email, client)
	if attempt != nil {
		// Probing must not change the counts the test is about.
		attempt.Succeeded()
	}
	return attempt == nil && wait > 0
}

func TestLoginThrottleLocksAccountPerClient(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	th := newTestLoginThrottle(&now)

	for i := 0; i < 3; i++ {
		fail(t, th, "User@Example.com", "198.51.100.1")
	}
	attempt, wait := th.reserve("user@example.com ", "198.51.100.1")
	require.Nil(t, attempt, "the guessing client is locked out of the account")
	require.Equal(t, loginFailureWindow, wait)

	// The owner, elsewhere, can still sign in.
	owner, _ := th.reserve("user@example.com", "203.0.113.9")
	require.NotNil(t, owner)
	owner.Succeeded()

	// The success cleared the account, including the guessing client's lock.
	require.False(t, blocked(th, "user@example.com", "198.51.100.1"))

	now = now.Add(loginFailureWindow)
	require.False(t, blocked(th, "user@example.com", "198.51.100.1"), "the window expires")
}

func TestLoginThrottleAccountWideCap(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	th := newTestLoginThrottle(&now)

	// Distributed guessing: two failures each from many clients.
	for i := 0; i < 3; i++ {
		client := "198.51.100." + strconv.Itoa(i+1)
		fail(t, th, "victim@example.com", client)
		fail(t, th, "victim@example.com", client)
	}
	require.True(t, blocked(th, "victim@example.com", "203.0.113.200"), "account-wide cap reached")
	require.False(t, blocked(th, "other@example.com", "203.0.113.200"))

	// Without a known client only the account-wide cap applies.
	th2 := newTestLoginThrottle(&now)
	for i := 0; i < 6; i++ {
		fail(t, th2, "a@example.com", "")
	}
	require.True(t, blocked(th2, "a@example.com", ""))
}

func TestLoginThrottleClientSprayLimit(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	th := newTestLoginThrottle(&now)
	for i := 0; i < 5; i++ {
		fail(t, th, "acct"+strconv.Itoa(i)+"@example.com", "198.51.100.1")
	}
	require.True(t, blocked(th, "fresh@example.com", "198.51.100.1"))
	require.False(t, blocked(th, "fresh@example.com", "198.51.100.2"))
}

func TestLoginThrottleClearAccount(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	th := newTestLoginThrottle(&now)
	for i := 0; i < 3; i++ {
		fail(t, th, "user@example.com", "198.51.100.1")
	}
	require.True(t, blocked(th, "user@example.com", "198.51.100.1"))

	th.clearAccount("USER@example.com")

	require.False(t, blocked(th, "user@example.com", "198.51.100.1"))
	require.Empty(t, th.pairsByAccount)
}

func TestLoginThrottleParallelBurstIsBounded(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	th := newTestLoginThrottle(&now)

	var admitted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if attempt, _ := th.reserve("user@example.com", "198.51.100.1"); attempt != nil {
				admitted.Add(1)
			}
		}()
	}
	wg.Wait()
	require.EqualValues(t, 3, admitted.Load())
}

func TestLoginThrottleSizeCapEvictsOldest(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	th := newTestLoginThrottle(&now)
	th.accounts = newWindowCounter(2, nil)
	th.clients = newWindowCounter(2, nil)
	th.pairs = newWindowCounter(2, th.forgetPair)

	for i := 0; i < 5; i++ {
		fail(t, th, "acct"+strconv.Itoa(i)+"@example.com", "198.51.100."+strconv.Itoa(i))
	}
	require.Len(t, th.accounts.entries, 2)
	require.Len(t, th.pairs.entries, 2)
	require.Len(t, th.clients.entries, 2)
	require.Len(t, th.pairsByAccount, 2, "evicted pairs leave the account index")
	require.Contains(t, th.accounts.entries, "acct4@example.com")
	require.NotContains(t, th.accounts.entries, "acct0@example.com")
}

func mustCIDRs(t *testing.T, cidrs ...string) []*net.IPNet {
	t.Helper()
	nets, err := parseTrustedProxies(cidrs)
	require.NoError(t, err)
	return nets
}

func loginRequestFrom(peer string, xff ...string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	r.RemoteAddr = peer
	for _, v := range xff {
		r.Header.Add("X-Forwarded-For", v)
	}
	return r
}

func TestLoginClientKey(t *testing.T) {
	proxies := mustCIDRs(t, "10.0.0.0/8")

	// No trusted proxy configured, private peer (an unlisted reverse proxy):
	// no per-client bucket, whatever the headers say.
	require.Empty(t, loginClientKey(loginRequestFrom("10.1.2.3:5555", "203.0.113.50"), nil))
	require.Empty(t, loginClientKey(loginRequestFrom("127.0.0.1:5555"), nil))

	// A public peer is the client.
	require.Equal(t, "198.51.100.7", loginClientKey(loginRequestFrom("198.51.100.7:1234", "203.0.113.50"), nil))

	// Behind a trusted proxy the rightmost untrusted hop is the client; what
	// the client put on the left of the header is ignored.
	require.Equal(t, "203.0.113.50", loginClientKey(loginRequestFrom("10.1.2.3:5555", "203.0.113.50, 10.9.9.9"), proxies))
	for _, spoof := range []string{"1.1.1.1", "2.2.2.2", "3.3.3.3"} {
		require.Equal(t, "203.0.113.50",
			loginClientKey(loginRequestFrom("10.1.2.3:5555", spoof+", 203.0.113.50"), proxies),
			"spoofed hop %s must not create a new bucket", spoof)
	}
	// Multiple header lines are one list.
	require.Equal(t, "203.0.113.50", loginClientKey(loginRequestFrom("10.1.2.3:5555", "9.9.9.9", "203.0.113.50"), proxies))

	// A trusted peer with no forwarded client gets no bucket.
	require.Empty(t, loginClientKey(loginRequestFrom("10.1.2.3:5555"), proxies))

	// IPv6 clients are keyed on their /64.
	a := loginClientKey(loginRequestFrom("[2001:db8:1:2::1]:443"), nil)
	b := loginClientKey(loginRequestFrom("[2001:db8:1:2:ffff::9]:443"), nil)
	require.Equal(t, "2001:db8:1:2::/64", a)
	require.Equal(t, a, b)
}

func TestForwardedClientIPRightmostUntrusted(t *testing.T) {
	proxies := mustCIDRs(t, "10.0.0.0/8")
	require.Equal(t, "203.0.113.50", forwardedClientIP([]string{"198.51.100.1, 203.0.113.50, 10.0.0.2"}, proxies).String())
	require.Equal(t, "10.0.0.1", forwardedClientIP([]string{"10.0.0.1, 10.0.0.2"}, proxies).String(), "all trusted: leftmost")
	require.Nil(t, forwardedClientIP([]string{"garbage, 10.0.0.2"}, proxies))
	require.Nil(t, forwardedClientIP(nil, proxies))
}

// Once an address has failed too often, the next attempt is refused before
// any lookup, and the answer is the same for known and unknown accounts.
func TestLoginRefusesAfterRepeatedFailures(t *testing.T) {
	s, st, authn := newRegularAuthServer(t)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.loginThrottle = newTestLoginThrottle(&now)

	st.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{Email: "nobody@example.com"}).Return(nil, store.ErrNotFound).Times(3)
	authn.EXPECT().ValidatePassword(gomock.Any(), gomock.Any(), "guess").Return(assert.AnError).Times(3)

	attempt := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
			jsonBody(t, types.LoginRequest{Email: "nobody@example.com", Password: "guess"}))
		req.RemoteAddr = "198.51.100.1:1234"
		rr := httptest.NewRecorder()
		s.login(rr, req)
		return rr
	}
	for i := 0; i < 3; i++ {
		require.Equal(t, http.StatusUnauthorized, attempt().Code)
	}

	rr := attempt()
	require.Equal(t, http.StatusTooManyRequests, rr.Code)
	retry, err := strconv.Atoi(rr.Header().Get("Retry-After"))
	require.NoError(t, err)
	require.Equal(t, int(loginFailureWindow/time.Second), retry)
}

// Completing a password reset unlocks the account.
func TestPasswordResetCompleteClearsLoginFailures(t *testing.T) {
	s, _, authn := newRegularAuthServer(t)
	s.sessionManager = nil
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.loginThrottle = newTestLoginThrottle(&now)
	for i := 0; i < 3; i++ {
		fail(t, s.loginThrottle, "user@example.com", "198.51.100.1")
	}
	require.True(t, blocked(s.loginThrottle, "user@example.com", "198.51.100.1"))

	user := &types.User{ID: "usr_1", Email: "user@example.com"}
	authn.EXPECT().PasswordResetComplete(gomock.Any(), "reset-token", "new-password-123").Return(user, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password-reset-complete",
		jsonBody(t, types.PasswordResetCompleteRequest{AccessToken: "reset-token", NewPassword: "new-password-123"}))
	s.passwordResetComplete(httptest.NewRecorder(), req)

	require.False(t, blocked(s.loginThrottle, "user@example.com", "198.51.100.1"))
}
