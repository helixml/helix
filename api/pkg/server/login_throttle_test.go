package server

import (
	"net/http"
	"net/http/httptest"
	"strconv"
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
	t.perAcct = 3
	t.perClient = 5
	return t
}

func TestLoginThrottleAccountLimit(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	th := newTestLoginThrottle(&now)

	for i := 0; i < 3; i++ {
		require.Zero(t, th.retryAfter("User@Example.com", "198.51.100.1"))
		th.recordFailure("User@Example.com", "198.51.100.1")
	}
	// The account is locked whatever the client and the address casing.
	require.Equal(t, loginFailureWindow, th.retryAfter("user@example.com ", "203.0.113.9"))
	// Other accounts are unaffected.
	require.Zero(t, th.retryAfter("other@example.com", "203.0.113.9"))

	now = now.Add(loginFailureWindow)
	require.Zero(t, th.retryAfter("user@example.com", "198.51.100.1"), "the window expires")
}

func TestLoginThrottleClientLimitAndSuccessReset(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	th := newTestLoginThrottle(&now)

	th.recordFailure("a@example.com", "198.51.100.1")
	th.recordFailure("a@example.com", "198.51.100.1")
	th.recordSuccess("a@example.com")
	th.recordFailure("a@example.com", "198.51.100.1")
	require.Zero(t, th.retryAfter("a@example.com", "198.51.100.1"), "a success clears the account count")

	// Spraying different accounts from one client hits the client limit.
	for _, email := range []string{"b@example.com", "c@example.com"} {
		th.recordFailure(email, "198.51.100.1")
	}
	require.Positive(t, th.retryAfter("fresh@example.com", "198.51.100.1"))
	require.Zero(t, th.retryAfter("fresh@example.com", "198.51.100.2"))
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
