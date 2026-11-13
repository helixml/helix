package server

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Failed password sign-ins allowed per window before further attempts are
// refused with 429. The account limit stops guessing one account's password;
// the client limit stops spraying many accounts from one address, and is
// higher because several users can share an address.
const (
	loginFailureWindow     = 15 * time.Minute
	loginFailuresPerAcct   = 10
	loginFailuresPerClient = 100
	// loginThrottleMaxKeys bounds memory: past it, expired windows are swept
	// on the next failure.
	loginThrottleMaxKeys = 10000
)

// loginThrottle counts failed password sign-ins per account and per client
// address in fixed windows. It is in-memory and per API process: a modest
// brake on online guessing, not a distributed rate limiter.
type loginThrottle struct {
	mu        sync.Mutex
	now       func() time.Time
	window    time.Duration
	perAcct   int
	perClient int
	failures  map[string]*loginFailures
}

type loginFailures struct {
	start time.Time
	count int
}

func newLoginThrottle() *loginThrottle {
	return &loginThrottle{
		now:       time.Now,
		window:    loginFailureWindow,
		perAcct:   loginFailuresPerAcct,
		perClient: loginFailuresPerClient,
		failures:  make(map[string]*loginFailures),
	}
}

func loginAccountKey(email string) string {
	return "acct:" + strings.ToLower(strings.TrimSpace(email))
}

func loginClientKey(ip string) string { return "client:" + ip }

// retryAfter reports how long the caller must wait before another attempt
// for this account from this client, or 0 when an attempt is allowed.
func (t *loginThrottle) retryAfter(email, clientIP string) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	wait := t.blockedFor(loginAccountKey(email), t.perAcct, now)
	if clientIP != "" {
		if w := t.blockedFor(loginClientKey(clientIP), t.perClient, now); w > wait {
			wait = w
		}
	}
	return wait
}

func (t *loginThrottle) blockedFor(key string, limit int, now time.Time) time.Duration {
	f, ok := t.failures[key]
	if !ok || now.Sub(f.start) >= t.window || f.count < limit {
		return 0
	}
	return f.start.Add(t.window).Sub(now)
}

// recordFailure counts one failed attempt against the account and the client.
func (t *loginThrottle) recordFailure(email, clientIP string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	if len(t.failures) >= loginThrottleMaxKeys {
		for key, f := range t.failures {
			if now.Sub(f.start) >= t.window {
				delete(t.failures, key)
			}
		}
	}
	t.increment(loginAccountKey(email), now)
	if clientIP != "" {
		t.increment(loginClientKey(clientIP), now)
	}
}

func (t *loginThrottle) increment(key string, now time.Time) {
	f, ok := t.failures[key]
	if !ok || now.Sub(f.start) >= t.window {
		t.failures[key] = &loginFailures{start: now, count: 1}
		return
	}
	f.count++
}

// recordSuccess clears the account's failures after a successful sign-in.
// The client's count stands: one valid account must not reset spraying.
func (t *loginThrottle) recordSuccess(email string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.failures, loginAccountKey(email))
}

// writeLoginThrottled answers a refused attempt with 429 and Retry-After.
func writeLoginThrottled(w http.ResponseWriter, wait time.Duration) {
	seconds := int(wait.Round(time.Second) / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	http.Error(w, "Too many failed sign-in attempts. Try again later.", http.StatusTooManyRequests)
}

// loginClientIP is the address failed sign-ins are counted against: the
// end client named by a trusted proxy (HELIX_AUDIT_LOG_TRUSTED_PROXIES), or
// the TCP peer.
func (s *HelixAPIServer) loginClientIP(r *http.Request) string {
	host, _ := splitRemoteAddr(r.RemoteAddr)
	if s.requestAuditMiddleware != nil {
		if ip := s.requestAuditMiddleware.clientIP(r, host); ip != "" {
			return ip
		}
	}
	return host
}
