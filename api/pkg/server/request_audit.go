package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net"
	"net/http"
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
	auditMaxIPHeader      = 64
	auditMaxForwardedFor  = 1024
	auditMaxUserAgent     = 1024
	auditMaxRequestID     = 64
	auditMaxContentType   = 255
)

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

// requestAuditUser is the mutable holder the audit middleware puts in the
// request context. The auth middleware runs on a subrouter and attaches the
// user to a *copy* of the request, which the outer middleware cannot see, so
// setRequestUser writes through this holder instead.
type requestAuditUser struct {
	mu   sync.Mutex
	user *types.User
}

func (h *requestAuditUser) set(user *types.User) {
	if user == nil {
		return
	}
	copied := *user
	h.mu.Lock()
	defer h.mu.Unlock()
	h.user = &copied
}

func (h *requestAuditUser) get() *types.User {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.user == nil {
		return nil
	}
	copied := *h.user
	return &copied
}

const requestAuditUserKey contextKey = "request_audit_user"

// recordRequestAuditUser is called by setRequestUser. It is a no-op when the
// audit middleware is not installed (e.g. subrouter-only tests).
func recordRequestAuditUser(ctx context.Context, user types.User) {
	holder, ok := ctx.Value(requestAuditUserKey).(*requestAuditUser)
	if !ok {
		return
	}
	holder.set(&user)
}

type requestAuditMiddleware struct {
	recorder        *auditlog.Recorder
	skipRunnerReads bool
}

func newRequestAuditMiddleware(recorder *auditlog.Recorder, skipRunnerReads bool) *requestAuditMiddleware {
	return &requestAuditMiddleware{recorder: recorder, skipRunnerReads: skipRunnerReads}
}

// Handler records one event per request. It sits on the root router, outside
// auth, so authentication failures and anonymous calls are captured too.
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
		var body capturedRequestBody
		if isMutatingMethod(r.Method) {
			body = captureRequestBody(r)
		}

		holder := &requestAuditUser{}
		r = r.WithContext(context.WithValue(r.Context(), requestAuditUserKey, holder))

		writer := NewLoggingResponseWriter(w)
		next.ServeHTTP(writer, r)

		user := holder.get()
		if user == nil {
			// Socket requests carry no auth middleware; getRequestUser
			// synthesises the sandbox user for them.
			user = getRequestUser(r)
		}

		credentialSource, credentialFingerprint := requestCredential(r, user)

		event := buildRequestAuditEvent(r, writer, user, started, requestID, credentialSource, credentialFingerprint, query, body)
		if m.shouldSkip(event) {
			return
		}
		m.recorder.Record(event)
	})
}

func buildRequestAuditEvent(
	r *http.Request,
	writer *LoggingResponseWriter,
	user *types.User,
	started time.Time,
	requestID, credentialSource, credentialFingerprint, query string,
	body capturedRequestBody,
) *types.RequestAuditEvent {
	event := &types.RequestAuditEvent{
		ID:                    auditlog.NewEventID(),
		Created:               started.UTC(),
		DurationMS:            time.Since(started).Milliseconds(),
		Method:                r.Method,
		RouteTemplate:         currentRouteTemplate(r),
		Path:                  r.URL.Path,
		Query:                 query,
		Status:                writer.StatusCode(),
		ResponseBytes:         writer.BytesWritten(),
		CredentialSource:      credentialSource,
		CredentialFingerprint: credentialFingerprint,
		RemoteAddr:            r.RemoteAddr,
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

	if event.ContentType != "" && len(body.prefix) > 0 && !body.truncated && auditlog.IsJSONContentType(event.ContentType) {
		if redacted, err := auditlog.RedactJSON(body.prefix); err == nil {
			var cut bool
			event.Body, cut = auditlog.Truncate(string(redacted), auditlog.StoredBodyLimit)
			if cut {
				event.BodyTruncated = true
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
	event.CFConnectingIP, _ = auditlog.Truncate(event.CFConnectingIP, auditMaxIPHeader)
	event.XRealIP, _ = auditlog.Truncate(event.XRealIP, auditMaxIPHeader)
	event.XForwardedFor, _ = auditlog.Truncate(event.XForwardedFor, auditMaxForwardedFor)
	event.UserAgent, _ = auditlog.Truncate(event.UserAgent, auditMaxUserAgent)
	event.RequestID, _ = auditlog.Truncate(event.RequestID, auditMaxRequestID)
	event.ContentType, _ = auditlog.Truncate(event.ContentType, auditMaxContentType)
}

// shouldSkip drops machine polling and successful static-asset reads. Errors on
// non-API paths are always kept.
func (m *requestAuditMiddleware) shouldSkip(event *types.RequestAuditEvent) bool {
	if event.Method != http.MethodGet && event.Method != http.MethodHead {
		return false
	}
	if !strings.HasPrefix(event.Path, "/api/") && event.Status < http.StatusBadRequest {
		return true
	}
	if m.skipRunnerReads && (event.TokenType == string(types.TokenTypeRunner) || event.TokenType == string(types.TokenTypeSocket)) {
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

type capturedRequestBody struct {
	prefix    []byte
	size      int64
	truncated bool
}

// replayBody is the reconstructed request body handed to the handler: the bytes
// already read, followed by whatever remains on the wire.
type replayBody struct {
	io.Reader
	io.Closer
}

// captureRequestBody reads at most auditlog.BodyLimit and restores the body so
// the handler still receives every byte, including for streaming bodies.
func captureRequestBody(r *http.Request) capturedRequestBody {
	if r.Body == nil || r.Body == http.NoBody {
		return capturedRequestBody{size: max(r.ContentLength, 0)}
	}

	original := r.Body
	prefix, err := io.ReadAll(io.LimitReader(original, auditlog.BodyLimit))
	r.Body = &replayBody{Reader: io.MultiReader(bytes.NewReader(prefix), original), Closer: original}
	if err != nil {
		// The handler gets whatever could be read; the incident record keeps the
		// fact that the body was incomplete.
		return capturedRequestBody{prefix: prefix, size: int64(len(prefix))}
	}

	size := r.ContentLength
	if size < 0 {
		size = int64(len(prefix))
	}
	return capturedRequestBody{
		prefix:    prefix,
		size:      size,
		truncated: r.ContentLength > int64(auditlog.BodyLimit) || (r.ContentLength < 0 && len(prefix) == auditlog.BodyLimit),
	}
}

// requestCredential reports where the request's credential came from and a
// non-reversible fingerprint of it. The credential itself is never recorded.
//
// user is the authenticated user (nil when anonymous or when auth failed). It
// decides precedence: the auth middleware prefers the BFF session over a token
// unless the request names its own API key, so the credential that actually
// authenticated is the one recorded. A stale session cookie that fell through
// to a token must not hide the token.
func requestCredential(r *http.Request, user *types.User) (source, fingerprint string) {
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
		return "session", credentialFingerprint(sessionCookie)
	}
	if token != "" {
		return tokenSource, credentialFingerprint(token)
	}
	if sessionCookie != "" {
		return "session", credentialFingerprint(sessionCookie)
	}
	return "", ""
}

func credentialFingerprint(credential string) string {
	sum := sha256.Sum256([]byte(credential))
	return hex.EncodeToString(sum[:])[:12]
}

func isUnixSocketRequest(r *http.Request) bool {
	addr, ok := r.Context().Value(http.LocalAddrContextKey).(*net.UnixAddr)
	return ok && addr != nil
}
