package server

import (
	"container/list"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Failed password sign-ins allowed per window before further attempts are
// refused with 429:
//
//   - per account and client: stops guessing one account's password from one
//     place, without letting anyone elsewhere lock the owner out;
//   - per account, from all clients: a much higher ceiling against guessing
//     distributed over many addresses;
//   - per client, across accounts: stops spraying many accounts from one
//     address. Higher, because several users can share an address.
const (
	loginFailureWindow         = 15 * time.Minute
	loginFailuresPerAcctClient = 10
	loginFailuresPerAcct       = 150
	loginFailuresPerClient     = 100
	// loginThrottleMaxKeys bounds memory per counter kind; past it the
	// oldest window is evicted.
	loginThrottleMaxKeys = 10000
)

// loginThrottle counts password sign-in attempts in fixed windows. An
// attempt is reserved (counted) before the password is checked, under one
// lock, so a parallel burst cannot exceed the limits; a successful sign-in
// releases it. It is in-memory and per API process: a brake on online
// guessing, not a distributed rate limiter.
type loginThrottle struct {
	mu            sync.Mutex
	now           func() time.Time
	window        time.Duration
	perAcctClient int
	perAcct       int
	perClient     int
	maxKeys       int
	accounts      *windowCounter // account
	pairs         *windowCounter // account + "|" + client
	clients       *windowCounter // client
	// pairsByAccount indexes pairs so an account can be cleared without
	// walking every counter.
	pairsByAccount map[string]map[string]struct{}
}

func newLoginThrottle() *loginThrottle {
	t := &loginThrottle{
		now:            time.Now,
		window:         loginFailureWindow,
		perAcctClient:  loginFailuresPerAcctClient,
		perAcct:        loginFailuresPerAcct,
		perClient:      loginFailuresPerClient,
		accounts:       newWindowCounter(loginThrottleMaxKeys, nil),
		clients:        newWindowCounter(loginThrottleMaxKeys, nil),
		pairsByAccount: make(map[string]map[string]struct{}),
	}
	t.pairs = newWindowCounter(loginThrottleMaxKeys, t.forgetPair)
	return t
}

// forgetPair drops an evicted or deleted pair from the account index.
func (t *loginThrottle) forgetPair(pair string) {
	account, _, _ := strings.Cut(pair, "|")
	if keys, ok := t.pairsByAccount[account]; ok {
		delete(keys, pair)
		if len(keys) == 0 {
			delete(t.pairsByAccount, account)
		}
	}
}

// loginAttempt is a reserved attempt. Succeeded must be called when the
// password was right; a failed attempt simply stays counted.
type loginAttempt struct {
	throttle *loginThrottle
	account  string
	client   string
}

func normalizeLoginAccount(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// reserve counts an attempt for this account from this client, or reports
// how long to wait when a limit is already reached. client may be empty when
// no client address is known; then only the account-wide limit applies.
func (t *loginThrottle) reserve(email, client string) (*loginAttempt, time.Duration) {
	account := normalizeLoginAccount(email)
	pair := account + "|" + client

	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()

	wait := t.accounts.blockedFor(account, t.perAcct, t.window, now)
	if client != "" {
		wait = max(wait,
			t.pairs.blockedFor(pair, t.perAcctClient, t.window, now),
			t.clients.blockedFor(client, t.perClient, t.window, now))
	}
	if wait > 0 {
		return nil, wait
	}

	t.accounts.increment(account, t.window, now)
	if client != "" {
		t.pairs.increment(pair, t.window, now)
		keys, ok := t.pairsByAccount[account]
		if !ok {
			keys = make(map[string]struct{})
			t.pairsByAccount[account] = keys
		}
		keys[pair] = struct{}{}
		t.clients.increment(client, t.window, now)
	}
	return &loginAttempt{throttle: t, account: account, client: client}, 0
}

// Succeeded clears the account's failures (from every client: the owner has
// proved the password) and returns this attempt to the client's budget, so
// valid sign-ins from a shared address do not count as spraying.
func (a *loginAttempt) Succeeded() {
	t := a.throttle
	t.mu.Lock()
	defer t.mu.Unlock()
	t.clearAccountLocked(a.account)
	if a.client != "" {
		t.clients.decrement(a.client)
	}
}

// clearAccount forgets an account's failures, e.g. after its password was
// reset: whoever was guessing the old password no longer locks it.
func (t *loginThrottle) clearAccount(email string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.clearAccountLocked(normalizeLoginAccount(email))
}

func (t *loginThrottle) clearAccountLocked(account string) {
	t.accounts.delete(account)
	for pair := range t.pairsByAccount[account] {
		t.pairs.delete(pair)
	}
}

// windowCounter holds fixed-window counts with a hard size cap. Keys are
// kept in insertion order (a window restart moves its key to the back), so
// the oldest is evicted in O(1) when the cap is reached; expired windows are
// simply replaced when next touched, with no sweeps.
type windowCounter struct {
	max     int
	entries map[string]*list.Element
	order   *list.List       // of *windowEntry, oldest first
	onDrop  func(key string) // called when a key is evicted or deleted
}

type windowEntry struct {
	key   string
	start time.Time
	count int
}

func newWindowCounter(maxKeys int, onDrop func(key string)) *windowCounter {
	return &windowCounter{max: maxKeys, entries: make(map[string]*list.Element), order: list.New(), onDrop: onDrop}
}

func (c *windowCounter) blockedFor(key string, limit int, window time.Duration, now time.Time) time.Duration {
	el, ok := c.entries[key]
	if !ok {
		return 0
	}
	e := el.Value.(*windowEntry)
	if now.Sub(e.start) >= window || e.count < limit {
		return 0
	}
	return e.start.Add(window).Sub(now)
}

func (c *windowCounter) increment(key string, window time.Duration, now time.Time) {
	if el, ok := c.entries[key]; ok {
		e := el.Value.(*windowEntry)
		if now.Sub(e.start) < window {
			e.count++
			return
		}
		e.start, e.count = now, 1
		c.order.MoveToBack(el)
		return
	}
	if len(c.entries) >= c.max {
		c.delete(c.order.Front().Value.(*windowEntry).key)
	}
	c.entries[key] = c.order.PushBack(&windowEntry{key: key, start: now, count: 1})
}

func (c *windowCounter) decrement(key string) {
	if el, ok := c.entries[key]; ok {
		if e := el.Value.(*windowEntry); e.count > 0 {
			e.count--
		}
	}
}

func (c *windowCounter) delete(key string) {
	if el, ok := c.entries[key]; ok {
		c.order.Remove(el)
		delete(c.entries, key)
		if c.onDrop != nil {
			c.onDrop(key)
		}
	}
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

// loginClientKey is the client that failed sign-ins are counted against, or
// "" when no client can be told apart:
//
//   - a peer in HELIX_LOGIN_TRUSTED_PROXIES: the first X-Forwarded-For hop,
//     from the right, outside those proxies (the left part of the header is
//     whatever the client sent);
//   - a public peer otherwise: the peer itself;
//   - a private, loopback or link-local peer that is not a configured proxy:
//     "". It is almost certainly an unlisted reverse proxy, and keying on it
//     would put every user behind it in one bucket.
//
// IPv6 clients are keyed on their /64, which one host usually controls whole.
func loginClientKey(r *http.Request, trustedProxies []*net.IPNet) string {
	host, _ := splitRemoteAddr(r.RemoteAddr)
	peer := net.ParseIP(host)
	if peer == nil {
		return ""
	}
	if ipInNetworks(peer, trustedProxies) {
		client := forwardedClientIP(r.Header.Values("X-Forwarded-For"), trustedProxies)
		if client == nil || ipInNetworks(client, trustedProxies) {
			return ""
		}
		return clientBucket(client)
	}
	if peer.IsLoopback() || peer.IsPrivate() || peer.IsLinkLocalUnicast() || peer.IsUnspecified() {
		return ""
	}
	return clientBucket(peer)
}

func clientBucket(ip net.IP) string {
	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}
	return ip.Mask(net.CIDRMask(64, 128)).String() + "/64"
}
