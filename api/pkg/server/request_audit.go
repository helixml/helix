package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/auditlog"
	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// Column widths mirrored from types.RequestAuditEvent. Oversized values are cut
// here so one hostile request cannot fail the batch insert that carries other
// users' audit rows.
const (
	auditMaxPath          = 2048
	auditMaxRouteTemplate = 512
	auditMaxEmail         = 320
	auditMaxUserID        = 255
	auditMaxRemoteAddr    = 255
	auditMaxRemoteIP      = 128
	auditMaxIPHeader      = 64
	auditMaxForwardedFor  = 1024
	auditMaxUserAgent     = 1024
	auditMaxRequestID     = 64
	auditMaxContentType   = 255
)

// auditFingerprintContext keeps the audit fingerprint key separate from the
// deployment key it is derived from.
const auditFingerprintContext = "helix request audit credential fingerprint\x00"

// newRequestAuditRecorder builds the audit pipeline from config. A nil result
// means the trail is disabled and the middleware is a no-op.
func newRequestAuditRecorder(cfg config.AuditLog, st store.Store) *auditlog.Recorder {
	if !cfg.Enabled {
		return nil
	}
	return auditlog.NewRecorder(auditlog.Options{
		Enabled:       cfg.Enabled,
		Stdout:        cfg.Stdout,
		Database:      cfg.Database,
		RetentionDays: cfg.RetentionDays,
		BufferSize:    cfg.BufferSize,
	}, st)
}

// requestAuditState is the mutable holder the audit middleware puts in the
// request context. The auth middleware runs on a subrouter and attaches the
// user to a *copy* of the request, which the outer middleware cannot see, so
// setRequestUser writes through this holder instead. The router likewise stamps
// the matched route template here, because the outer middleware sits outside
// the mux and cannot resolve it itself.
type requestAuditState struct {
	mu            sync.Mutex
	user          *types.User
	routeTemplate string
}

// setUser is first-writer-wins. Authentication is attempted once per request
// and then followed by in-process dispatch (github_compat.go,
// internal_pull_requests.go, helix_org_inproc.go), which calls setRequestUser
// again with a different identity. The actor recorded must be the one the
// caller authenticated as, not whoever the request was dispatched for.
func (s *requestAuditState) setUser(user *types.User) {
	if user == nil {
		return
	}
	copied := *user
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.user != nil {
		return
	}
	s.user = &copied
}

func (s *requestAuditState) getUser() *types.User {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.user == nil {
		return nil
	}
	copied := *s.user
	return &copied
}

func (s *requestAuditState) setRouteTemplate(template string) {
	if template == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.routeTemplate = template
}

func (s *requestAuditState) getRouteTemplate() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.routeTemplate
}

const requestAuditUserKey contextKey = "request_audit_user"

// recordRequestAuditUser is called by setRequestUser. It is a no-op when the
// audit middleware is not installed (e.g. subrouter-only tests).
func recordRequestAuditUser(ctx context.Context, user types.User) {
	state, ok := ctx.Value(requestAuditUserKey).(*requestAuditState)
	if !ok {
		return
	}
	state.setUser(&user)
}

// stampRequestAuditRoute hands the matched route template to the audit
// middleware that wraps the whole server. It must run inside the mux, after
// matching.
func stampRequestAuditRoute(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if state, ok := r.Context().Value(requestAuditUserKey).(*requestAuditState); ok {
			state.setRouteTemplate(currentRouteTemplate(r))
		}
		next.ServeHTTP(w, r)
	})
}

type requestAuditOptions struct {
	skipRunnerReads bool
	fingerprintKey  []byte
	trustedProxies  []*net.IPNet
}

func newRequestAuditOptions(cfg config.AuditLog, fingerprintKey []byte) (requestAuditOptions, error) {
	proxies, err := parseTrustedProxies(cfg.TrustedProxies)
	if err != nil {
		return requestAuditOptions{}, err
	}
	return requestAuditOptions{
		skipRunnerReads: cfg.SkipRunnerReads,
		fingerprintKey:  fingerprintKey,
		trustedProxies:  proxies,
	}, nil
}

// parseTrustedProxies turns the operator's CIDR allowlist into networks. An
// invalid entry is a startup error: silently trusting nothing would make
// client_ip mysteriously empty.
func parseTrustedProxies(entries []string) ([]*net.IPNet, error) {
	proxies := make([]*net.IPNet, 0, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		_, network, err := net.ParseCIDR(entry)
		if err != nil {
			return nil, fmt.Errorf("invalid HELIX_AUDIT_LOG_TRUSTED_PROXIES entry %q: %w", entry, err)
		}
		proxies = append(proxies, network)
	}
	return proxies, nil
}

type requestAuditMiddleware struct {
	recorder *auditlog.Recorder
	opts     requestAuditOptions
}

func newRequestAuditMiddleware(recorder *auditlog.Recorder, opts requestAuditOptions) *requestAuditMiddleware {
	return &requestAuditMiddleware{recorder: recorder, opts: opts}
}

// Handler records one event per request. It wraps the outermost handler -
// outside the vhost proxy and outside the mux - so authentication failures,
// anonymous calls, routed traffic and proxied customer traffic are all
// captured.
func (m *requestAuditMiddleware) Handler(next http.Handler) http.Handler {
	if m == nil || !m.recorder.Enabled() {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()

		requestID := r.Header.Get("X-Request-Id")
		if requestID == "" {
			requestID = system.GenerateUUID()
		}

		query, _ := auditlog.Truncate(auditlog.RedactQuery(r.URL.RawQuery), auditlog.StoredQueryLimit)

		state := &requestAuditState{}
		r = r.WithContext(context.WithValue(r.Context(), requestAuditUserKey, state))

		// Tee the body as the handler reads it. Reading it here would block
		// mutating requests before auth with no read timeout to bound them.
		var body *requestBodyCapture
		if isMutatingMethod(r.Method) {
			if body = newRequestBodyCapture(r); body != nil {
				r.Body = body
			}
		}

		writer := NewLoggingResponseWriter(w)

		// A panicking request must still leave a row: record it as a 500, then
		// let the panic continue to the server's recovery handler.
		defer func() {
			recovered := recover()
			status := writer.StatusCode()
			if recovered != nil {
				status = http.StatusInternalServerError
			}
			m.record(r, writer, state, started, requestID, query, body, status)
			if recovered != nil {
				panic(recovered)
			}
		}()

		next.ServeHTTP(writer, r)
	})
}

func (m *requestAuditMiddleware) record(
	r *http.Request,
	writer *LoggingResponseWriter,
	state *requestAuditState,
	started time.Time,
	requestID, query string,
	body *requestBodyCapture,
	status int,
) {
	user := state.getUser()
	if user == nil {
		// Socket requests carry no auth middleware; getRequestUser
		// synthesises the sandbox user for them.
		user = getRequestUser(r)
	}

	credentialSource, fingerprint := m.requestCredential(r, user)

	var captured capturedRequestBody
	if body != nil {
		captured = body.snapshot(r.ContentLength)
	}

	event := m.buildRequestAuditEvent(r, writer, state, user, started, requestID, credentialSource, fingerprint, query, captured)
	// The writer status is what the client saw; the passed status only differs
	// when the handler panicked before writing anything.
	if status != event.Status {
		event.Status = status
	}
	if m.shouldSkip(event) {
		return
	}
	m.recorder.Record(event)
}

func (m *requestAuditMiddleware) buildRequestAuditEvent(
	r *http.Request,
	writer *LoggingResponseWriter,
	state *requestAuditState,
	user *types.User,
	started time.Time,
	requestID, credentialSource, credentialFingerprint, query string,
	body capturedRequestBody,
) *types.RequestAuditEvent {
	routeTemplate := state.getRouteTemplate()
	if routeTemplate == "" {
		routeTemplate = currentRouteTemplate(r)
	}

	remoteIP, remotePort := splitRemoteAddr(r.RemoteAddr)
	event := &types.RequestAuditEvent{
		ID:                    auditlog.NewEventID(),
		Created:               started.UTC(),
		DurationMS:            time.Since(started).Milliseconds(),
		Method:                r.Method,
		RouteTemplate:         routeTemplate,
		Path:                  auditlog.RedactPath(r.URL.Path),
		Query:                 query,
		Status:                writer.StatusCode(),
		ResponseBytes:         writer.BytesWritten(),
		CredentialSource:      credentialSource,
		CredentialFingerprint: credentialFingerprint,
		RemoteAddr:            r.RemoteAddr,
		RemoteIP:              remoteIP,
		RemotePort:            remotePort,
		ClientIP:              m.clientIP(r, remoteIP),
		CFConnectingIP:        r.Header.Get("CF-Connecting-IP"),
		XForwardedFor:         r.Header.Get("X-Forwarded-For"),
		XRealIP:               r.Header.Get("X-Real-IP"),
		UserAgent:             r.UserAgent(),
		RequestID:             requestID,
		ContentType:           r.Header.Get("Content-Type"),
		BodySize:              body.size,
		BodyTruncated:         body.truncated,
	}

	if user != nil {
		event.UserID = user.ID
		event.UserEmail = user.Email
		event.UserAdmin = user.Admin
		event.OrganizationID = user.OrganizationID
		event.TokenType = string(user.TokenType)
		event.APIKeyType = string(user.APIKeyType)
	}

	if len(body.prefix) > 0 && !body.truncated && auditlog.IsJSONContentType(event.ContentType) {
		// Routes whose payload is itself a credential document keep only their
		// size and content type.
		if !auditlog.IsSensitiveBodyRoute(r.URL.Path) {
			if redacted, err := auditlog.RedactJSON(body.prefix); err == nil {
				var cut bool
				event.Body, cut = auditlog.Truncate(string(redacted), auditlog.StoredBodyLimit)
				if cut {
					event.BodyTruncated = true
				}
			}
		}
	}

	truncateAuditEvent(event)
	return event
}

func truncateAuditEvent(event *types.RequestAuditEvent) {
	event.Path, _ = auditlog.Truncate(event.Path, auditMaxPath)
	event.RouteTemplate, _ = auditlog.Truncate(event.RouteTemplate, auditMaxRouteTemplate)
	event.UserID, _ = auditlog.Truncate(event.UserID, auditMaxUserID)
	event.UserEmail, _ = auditlog.Truncate(event.UserEmail, auditMaxEmail)
	event.OrganizationID, _ = auditlog.Truncate(event.OrganizationID, auditMaxUserID)
	event.RemoteAddr, _ = auditlog.Truncate(event.RemoteAddr, auditMaxRemoteAddr)
	event.RemoteIP, _ = auditlog.Truncate(event.RemoteIP, auditMaxRemoteIP)
	event.ClientIP, _ = auditlog.Truncate(event.ClientIP, auditMaxRemoteIP)
	event.CFConnectingIP, _ = auditlog.Truncate(event.CFConnectingIP, auditMaxIPHeader)
	event.XRealIP, _ = auditlog.Truncate(event.XRealIP, auditMaxIPHeader)
	event.XForwardedFor, _ = auditlog.Truncate(event.XForwardedFor, auditMaxForwardedFor)
	event.UserAgent, _ = auditlog.Truncate(event.UserAgent, auditMaxUserAgent)
	event.RequestID, _ = auditlog.Truncate(event.RequestID, auditMaxRequestID)
	event.ContentType, _ = auditlog.Truncate(event.ContentType, auditMaxContentType)
}

// shouldSkip drops machine polling and successful static-asset reads. Errors on
// non-API paths are always kept, and so is proxied customer traffic (which has
// no route template because it never reached the Helix router).
func (m *requestAuditMiddleware) shouldSkip(event *types.RequestAuditEvent) bool {
	if event.Method != http.MethodGet && event.Method != http.MethodHead {
		return false
	}
	if !strings.HasPrefix(event.Path, "/api/") && event.Status < http.StatusBadRequest && event.RouteTemplate != "" {
		return true
	}
	if m.opts.skipRunnerReads && (event.TokenType == string(types.TokenTypeRunner) || event.TokenType == string(types.TokenTypeSocket)) {
		return true
	}
	return false
}

// currentRouteTemplate resolves the route that matched, e.g.
// /api/v1/sessions/{id}. It is empty when nothing matched (404, method
// mismatch, static fallback).
func currentRouteTemplate(r *http.Request) string {
	route := mux.CurrentRoute(r)
	if route == nil {
		return ""
	}
	template, err := route.GetPathTemplate()
	if err != nil {
		return ""
	}
	return template
}

func isMutatingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// splitRemoteAddr separates the TCP peer into address and port so the address
// is usable as a filter. Unix sockets and odd peers keep the raw address.
func splitRemoteAddr(remoteAddr string) (string, int) {
	host, portValue, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr, 0
	}
	port, err := strconv.Atoi(portValue)
	if err != nil {
		port = 0
	}
	return host, port
}

// clientIP derives the end-client address from a proxy header, but only when
// the TCP peer is in the operator's trusted proxy allowlist. Without that,
// header values are recorded as sent and never treated as the client.
func (m *requestAuditMiddleware) clientIP(r *http.Request, remoteIP string) string {
	if len(m.opts.trustedProxies) == 0 {
		return ""
	}
	peer := net.ParseIP(remoteIP)
	if peer == nil || !ipInNetworks(peer, m.opts.trustedProxies) {
		return ""
	}

	if cf := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); cf != "" {
		if ip := net.ParseIP(cf); ip != nil {
			return ip.String()
		}
	}
	for _, candidate := range strings.Split(r.Header.Get("X-Forwarded-For"), ",") {
		if ip := net.ParseIP(strings.TrimSpace(candidate)); ip != nil {
			return ip.String()
		}
	}
	return ""
}

func ipInNetworks(ip net.IP, networks []*net.IPNet) bool {
	for _, network := range networks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

type capturedRequestBody struct {
	prefix    []byte
	size      int64
	truncated bool
}

// requestBodyCapture tees the request body into a bounded buffer as the
// handler reads it, so a request that never reads its body is not delayed by
// the audit trail.
type requestBodyCapture struct {
	body io.ReadCloser

	mu        sync.Mutex
	prefix    []byte
	read      int64
	truncated bool
}

func newRequestBodyCapture(r *http.Request) *requestBodyCapture {
	if r.Body == nil || r.Body == http.NoBody {
		return nil
	}
	return &requestBodyCapture{body: r.Body, prefix: make([]byte, 0, 512)}
}

func (c *requestBodyCapture) Read(p []byte) (int, error) {
	n, err := c.body.Read(p)
	if n > 0 {
		c.mu.Lock()
		c.read += int64(n)
		if remaining := auditlog.BodyLimit - len(c.prefix); remaining > 0 {
			if n < remaining {
				c.prefix = append(c.prefix, p[:n]...)
			} else {
				c.prefix = append(c.prefix, p[:remaining]...)
				c.truncated = true
			}
		} else {
			c.truncated = true
		}
		c.mu.Unlock()
	}
	return n, err
}

func (c *requestBodyCapture) Close() error { return c.body.Close() }

func (c *requestBodyCapture) snapshot(contentLength int64) capturedRequestBody {
	c.mu.Lock()
	defer c.mu.Unlock()

	prefix := make([]byte, len(c.prefix))
	copy(prefix, c.prefix)

	truncated := c.truncated || contentLength > int64(auditlog.BodyLimit)
	if contentLength >= 0 && c.read < contentLength {
		// The handler did not consume the whole body, so a partial capture must
		// not be parsed as if it were complete.
		truncated = true
	}
	return capturedRequestBody{prefix: prefix, size: c.read, truncated: truncated}
}

// requestCredential reports where the request's credential came from and a
// non-reversible fingerprint of it. The credential itself is never recorded.
//
// user is the authenticated user (nil when anonymous or when auth failed). It
// decides precedence: the auth middleware prefers the BFF session over a token
// unless the request names its own API key, so the credential that actually
// authenticated is the one recorded. A stale session cookie that fell through
// to a token must not hide the token.
func (m *requestAuditMiddleware) requestCredential(r *http.Request, user *types.User) (source, fingerprint string) {
	if isUnixSocketRequest(r) {
		return "socket", ""
	}

	sessionCookie := ""
	if cookie, err := r.Cookie(auth.SessionCookieName); err == nil {
		sessionCookie = cookie.Value
	}
	token, tokenSource := getRequestTokenWithSource(r)

	usedSession := false
	switch {
	case user != nil && user.TokenType == types.TokenTypeSession:
		usedSession = sessionCookie != ""
	case user != nil && user.TokenType != "":
		usedSession = false
	default:
		// Unauthenticated request: mirror what auth would have tried first.
		usedSession = sessionCookie != "" && !namesOwnAPIKey(r)
	}

	if usedSession {
		return "session", m.credentialFingerprint(sessionCookie)
	}
	if token != "" {
		return tokenSource, m.credentialFingerprint(token)
	}
	if sessionCookie != "" {
		return "session", m.credentialFingerprint(sessionCookie)
	}
	return "", ""
}

// credentialFingerprint is HMAC-SHA256 under the deployment's existing
// encryption key, truncated. A bare hash of a bearer token is brute-forceable
// offline (the token space is small enough), while the HMAC key is not in the
// database the audit rows live in.
func (m *requestAuditMiddleware) credentialFingerprint(credential string) string {
	mac := hmac.New(sha256.New, m.opts.fingerprintKey)
	mac.Write([]byte(auditFingerprintContext))
	mac.Write([]byte(credential))
	return hex.EncodeToString(mac.Sum(nil))[:12]
}

func isUnixSocketRequest(r *http.Request) bool {
	addr, ok := r.Context().Value(http.LocalAddrContextKey).(*net.UnixAddr)
	return ok && addr != nil
}
