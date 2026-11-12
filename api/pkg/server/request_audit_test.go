package server

import (
	"bufio"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/auditlog"
	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

var testAuditFingerprintKey = []byte("0123456789abcdef0123456789abcdef")

func newTestAuditRecorder() (*auditlog.Recorder, *bytes.Buffer) {
	stdout := &bytes.Buffer{}
	recorder := auditlog.NewRecorder(auditlog.Options{
		Enabled:      true,
		Stdout:       true,
		Database:     false,
		StdoutWriter: stdout,
	}, nil)
	return recorder, stdout
}

func newTestAuditMiddleware(recorder *auditlog.Recorder) *requestAuditMiddleware {
	return newRequestAuditMiddleware(recorder, requestAuditOptions{
		skipRunnerReads: true,
		fingerprintKey:  testAuditFingerprintKey,
	})
}

func newTrustedProxyAuditMiddleware(recorder *auditlog.Recorder, cidrs ...string) *requestAuditMiddleware {
	proxies, err := parseTrustedProxies(cidrs)
	if err != nil {
		panic(err)
	}
	return newRequestAuditMiddleware(recorder, requestAuditOptions{
		skipRunnerReads: true,
		fingerprintKey:  testAuditFingerprintKey,
		trustedProxies:  proxies,
	})
}

// auditFingerprint mirrors the middleware's HMAC so the test can assert the
// recorded value is derived from the credential without storing it.
func auditFingerprint(t *testing.T, credential string) string {
	t.Helper()
	mac := hmac.New(sha256.New, testAuditFingerprintKey)
	mac.Write([]byte(auditFingerprintContext))
	mac.Write([]byte(credential))
	return hex.EncodeToString(mac.Sum(nil))[:12]
}

// lastAuditEvent decodes the last JSON line the recorder wrote. The middleware
// writes synchronously, so no waiting is needed.
func lastAuditEvent(t *testing.T, stdout *bytes.Buffer) *types.RequestAuditEvent {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	require.NotEmpty(t, lines)
	var event types.RequestAuditEvent
	require.NoError(t, json.Unmarshal([]byte(lines[len(lines)-1]), &event))
	return &event
}

func TestRequestAuditMiddlewareCapturesAuthenticatedRequest(t *testing.T) {
	recorder, stdout := newTestAuditRecorder()

	router := mux.NewRouter()
	router.Use(newTestAuditMiddleware(recorder).Handler)
	// Mirror production nesting: root router → /api/v1 subrouter → auth subrouter.
	api := router.PathPrefix("/api/v1").Subrouter()
	auth := api.MatcherFunc(func(*http.Request, *mux.RouteMatch) bool { return true }).Subrouter()
	auth.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Inner middleware authenticates and attaches the user to a copy of
			// the request, exactly like authMiddleware.extractMiddleware.
			r = r.WithContext(setRequestUser(r.Context(), types.User{
				ID:             "usr_1",
				Email:          "operator@example.com",
				Admin:          true,
				OrganizationID: "org_1",
				TokenType:      types.TokenTypeSession,
			}))
			next.ServeHTTP(w, r)
		})
	})

	const requestBody = `{"email":"operator@example.com","password":"hunter2","api_key_file":"/run/helix.key"}`
	var handlerBody string
	auth.HandleFunc("/things/{id}", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		handlerBody = string(body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("hello"))
	}).Methods(http.MethodPost)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/things/thing_1?access_token=secret-token&page=2", strings.NewReader(requestBody))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer fixture-value")
	request.Header.Set("X-Request-Id", "req_test_1")
	request.Header.Set("User-Agent", "forensics-test/1.0")
	request.Header.Set("CF-Connecting-IP", "198.51.100.7")
	request.Header.Set("X-Forwarded-For", "198.51.100.7, 10.0.0.1")
	request.Header.Set("X-Real-IP", "198.51.100.7")
	request.RemoteAddr = "203.0.113.9:1234"

	router.ServeHTTP(httptest.NewRecorder(), request)

	require.Equal(t, requestBody, handlerBody, "handler must receive the full body")

	event := lastAuditEvent(t, stdout)
	require.Equal(t, http.MethodPost, event.Method)
	require.Equal(t, "/api/v1/things/{id}", event.RouteTemplate)
	require.Equal(t, http.StatusCreated, event.Status)
	require.EqualValues(t, len("hello"), event.ResponseBytes)
	require.Equal(t, "access_token=[REDACTED]&page=2", event.Query)

	// The user comes from the inner middleware via the audit holder.
	require.Equal(t, "usr_1", event.UserID)
	require.Equal(t, "operator@example.com", event.UserEmail)
	require.True(t, event.UserAdmin)
	require.Equal(t, "org_1", event.OrganizationID)
	require.Equal(t, string(types.TokenTypeSession), event.TokenType)

	// Client address inputs, recorded as sent. The peer address is also split so
	// it can be filtered on without the ephemeral port.
	require.Equal(t, "203.0.113.9:1234", event.RemoteAddr)
	require.Equal(t, "203.0.113.9", event.RemoteIP)
	require.Equal(t, 1234, event.RemotePort)
	require.Empty(t, event.ClientIP, "no trusted proxy is configured by default")
	require.Equal(t, "198.51.100.7", event.CFConnectingIP)
	require.Equal(t, "198.51.100.7, 10.0.0.1", event.XForwardedFor)
	require.Equal(t, "198.51.100.7", event.XRealIP)
	require.Equal(t, "forensics-test/1.0", event.UserAgent)
	require.Equal(t, "req_test_1", event.RequestID)

	// Credential: source + non-reversible fingerprint, never the token.
	require.Equal(t, "bearer", event.CredentialSource)
	require.Equal(t, auditFingerprint(t, "fixture-value"), event.CredentialFingerprint)

	// Body is redacted: the password is gone, the email and api_key_file stay.
	require.Contains(t, event.Body, "operator@example.com")
	require.Contains(t, event.Body, "/run/helix.key")
	require.NotContains(t, event.Body, "hunter2")
	require.NotContains(t, stdout.String(), "fixture-value")
	require.NotContains(t, stdout.String(), "secret-token")
}

func TestRequestAuditMiddlewareRestoresBodyOverLimit(t *testing.T) {
	recorder, stdout := newTestAuditRecorder()

	payload := bytes.Repeat([]byte("x"), 100*1024)
	wantHash := sha256.Sum256(payload)

	var receivedSize int
	var receivedHash string
	router := mux.NewRouter()
	router.Use(newTestAuditMiddleware(recorder).Handler)
	router.HandleFunc("/api/v1/upload", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		receivedSize = len(body)
		sum := sha256.Sum256(body)
		receivedHash = hex.EncodeToString(sum[:])
		w.WriteHeader(http.StatusOK)
	}).Methods(http.MethodPost)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/upload", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/octet-stream")
	router.ServeHTTP(httptest.NewRecorder(), request)

	require.Equal(t, len(payload), receivedSize, "handler must receive the whole body")
	require.Equal(t, hex.EncodeToString(wantHash[:]), receivedHash, "body bytes must be unchanged")

	event := lastAuditEvent(t, stdout)
	require.EqualValues(t, len(payload), event.BodySize)
	require.True(t, event.BodyTruncated)
	require.Empty(t, event.Body, "bodies over the capture limit are not parsed")
	require.Equal(t, "application/octet-stream", event.ContentType)
}

// The body is captured by a tee as the handler reads it, so a handler that
// never reads the body costs nothing and cannot block before auth.
func TestRequestAuditMiddlewareDoesNotPreReadBody(t *testing.T) {
	recorder, stdout := newTestAuditRecorder()

	body := &countingBody{reader: strings.NewReader(`{"unused":"payload"}`)}
	router := mux.NewRouter()
	router.Use(newTestAuditMiddleware(recorder).Handler)
	router.HandleFunc("/api/v1/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}).Methods(http.MethodPost)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/ping", nil)
	request.Body = body
	request.ContentLength = int64(len(`{"unused":"payload"}`))
	router.ServeHTTP(httptest.NewRecorder(), request)

	require.Zero(t, body.reads.Load(), "the middleware must not read the body itself")

	event := lastAuditEvent(t, stdout)
	require.Equal(t, http.StatusAccepted, event.Status)
	require.Zero(t, event.BodySize)
	require.Empty(t, event.Body)
}

type countingBody struct {
	reader io.Reader
	reads  atomic.Int64
}

func (c *countingBody) Read(p []byte) (int, error) {
	c.reads.Add(1)
	return c.reader.Read(p)
}

func (c *countingBody) Close() error { return nil }

func TestRequestAuditMiddlewareSkipRules(t *testing.T) {
	serve := func(t *testing.T, method, path string, runner bool, status int) (*types.RequestAuditEvent, bool) {
		t.Helper()
		recorder, stdout := newTestAuditRecorder()

		router := mux.NewRouter()
		router.Use(newTestAuditMiddleware(recorder).Handler)

		api := router.PathPrefix("/api/v1").Subrouter()
		api.HandleFunc("/poll", func(w http.ResponseWriter, r *http.Request) {
			if runner {
				r = r.WithContext(setRequestUser(r.Context(), types.User{
					ID: "runner", TokenType: types.TokenTypeRunner, Token: "runner-token-secret",
				}))
			}
			w.WriteHeader(status)
		}).Methods(http.MethodGet, http.MethodHead)

		// Production registers an SPA catch-all on the root router, so frontend
		// asset requests are routed (and therefore audited).
		router.PathPrefix("/").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
		})

		router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(method, path, nil))

		if stdout.Len() == 0 {
			return nil, false
		}
		return lastAuditEvent(t, stdout), true
	}

	t.Run("runner polling is skipped", func(t *testing.T) {
		_, recorded := serve(t, http.MethodGet, "/api/v1/poll", true, http.StatusOK)
		require.False(t, recorded)
	})

	t.Run("runner HEAD polling is skipped", func(t *testing.T) {
		_, recorded := serve(t, http.MethodHead, "/api/v1/poll", true, http.StatusOK)
		require.False(t, recorded)
	})

	t.Run("anonymous API reads are recorded", func(t *testing.T) {
		event, recorded := serve(t, http.MethodGet, "/api/v1/poll", false, http.StatusUnauthorized)
		require.True(t, recorded)
		require.Equal(t, http.StatusUnauthorized, event.Status)
		require.Empty(t, event.UserID)
	})

	t.Run("successful static assets are skipped", func(t *testing.T) {
		_, recorded := serve(t, http.MethodGet, "/logo.png", false, http.StatusOK)
		require.False(t, recorded)
	})

	t.Run("failed static requests are recorded", func(t *testing.T) {
		event, recorded := serve(t, http.MethodGet, "/missing", false, http.StatusNotFound)
		require.True(t, recorded)
		require.Equal(t, http.StatusNotFound, event.Status)
	})

	t.Run("mutating runner calls are recorded", func(t *testing.T) {
		recorder, stdout := newTestAuditRecorder()
		router := mux.NewRouter()
		router.Use(newTestAuditMiddleware(recorder).Handler)
		router.HandleFunc("/api/v1/jobs", func(w http.ResponseWriter, r *http.Request) {
			r = r.WithContext(setRequestUser(r.Context(), types.User{
				ID: "runner", TokenType: types.TokenTypeRunner,
			}))
		}).Methods(http.MethodPost)

		router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v1/jobs", strings.NewReader("{}")))
		event, recorded := func() (*types.RequestAuditEvent, bool) {
			if stdout.Len() == 0 {
				return nil, false
			}
			return lastAuditEvent(t, stdout), true
		}()
		require.True(t, recorded)
		require.Equal(t, string(types.TokenTypeRunner), event.TokenType)
	})
}

// Production installs the audit middleware outside the mux (around the vhost
// proxy) and relies on the route stamp for the template, so these two paths
// must both be recorded.
func TestRequestAuditMiddlewareRecordsOutsideTheRouter(t *testing.T) {
	t.Run("routed request keeps its route template", func(t *testing.T) {
		recorder, stdout := newTestAuditRecorder()
		middleware := newTestAuditMiddleware(recorder)

		router := mux.NewRouter()
		router.Use(stampRequestAuditRoute)
		router.HandleFunc("/api/v1/things/{id}", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		handler := middleware.Handler(router)
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/things/thing_1", nil))

		event := lastAuditEvent(t, stdout)
		require.Equal(t, "/api/v1/things/{id}", event.RouteTemplate, "the mux stamps the template for the outer middleware")
	})

	t.Run("proxied customer traffic is recorded", func(t *testing.T) {
		recorder, stdout := newTestAuditRecorder()
		middleware := newTestAuditMiddleware(recorder)

		// A vhost-proxied request never reaches the Helix router.
		proxy := middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("proxied"))
		}))

		proxy.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/some-page", nil))

		event := lastAuditEvent(t, stdout)
		require.Equal(t, "/some-page", event.Path)
		require.Empty(t, event.RouteTemplate)
		require.Equal(t, http.StatusOK, event.Status)
	})
}

func TestRequestCredentialSources(t *testing.T) {
	middleware := newTestAuditMiddleware(nil)

	cases := []struct {
		name       string
		user       *types.User
		prepare    func(*http.Request)
		wantSource string
	}{
		{
			name: "bearer",
			prepare: func(r *http.Request) {
				r.Header.Set("Authorization", "Bearer some-token-value")
			},
			wantSource: "bearer",
		},
		{
			name: "api key header",
			prepare: func(r *http.Request) {
				r.Header.Set("x-api-key", "fixture-value")
			},
			wantSource: "api-key",
		},
		{
			name: "cookie",
			prepare: func(r *http.Request) {
				r.AddCookie(&http.Cookie{Name: "access_token", Value: "fixture-value"})
			},
			wantSource: "cookie",
		},
		{
			name: "query",
			prepare: func(r *http.Request) {
				r.URL.RawQuery = "access_token=some-token-value"
			},
			wantSource: "query",
		},
		{
			name: "bff session",
			prepare: func(r *http.Request) {
				r.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "fixture-value"})
			},
			wantSource: "session",
		},
		{
			name: "session user with a stale cookie",
			user: &types.User{TokenType: types.TokenTypeSession},
			prepare: func(r *http.Request) {
				r.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "fixture-value"})
			},
			wantSource: "session",
		},
		{
			// A stale session cookie falls through to the token in the auth
			// middleware; the audit must fingerprint the token that authenticated.
			name: "api key user with a stale session cookie",
			user: &types.User{TokenType: types.TokenTypeAPIKey},
			prepare: func(r *http.Request) {
				r.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "fixture-value"})
				r.Header.Set("Authorization", "Bearer some-token-value")
			},
			wantSource: "bearer",
		},
		{
			name:       "anonymous",
			prepare:    func(r *http.Request) {},
			wantSource: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/things", nil)
			tc.prepare(request)

			source, fingerprint := middleware.requestCredential(request, tc.user)
			require.Equal(t, tc.wantSource, source)
			if source == "" {
				require.Empty(t, fingerprint)
				return
			}
			require.Len(t, fingerprint, 12)
			require.NotContains(t, fingerprint, "fixture-value")
		})
	}
}

func TestRequestAuditMiddlewareSupportsHijackAndFlush(t *testing.T) {
	recorder, _ := newTestAuditRecorder()

	router := mux.NewRouter()
	router.Use(newTestAuditMiddleware(recorder).Handler)
	router.HandleFunc("/api/v1/stream", func(w http.ResponseWriter, r *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		require.True(t, ok, "websocket handlers need Hijack")
		_, _, _ = hijacker.Hijack()

		flusher, ok := w.(http.Flusher)
		require.True(t, ok, "streaming handlers need Flush")
		flusher.Flush()

		w.WriteHeader(http.StatusSwitchingProtocols)
	}).Methods(http.MethodGet)

	writer := &hijackableWriter{ResponseRecorder: httptest.NewRecorder()}
	router.ServeHTTP(writer, httptest.NewRequest(http.MethodGet, "/api/v1/stream", nil))

	require.True(t, writer.hijacked)
	require.True(t, writer.flushed)
}

type hijackableWriter struct {
	*httptest.ResponseRecorder
	hijacked bool
	flushed  bool
}

func (h *hijackableWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h.hijacked = true
	return nil, nil, nil
}

func (h *hijackableWriter) Flush() { h.flushed = true }

func TestListRequestAuditEventsFilters(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	var captured *types.RequestAuditEventFilters
	mockStore.EXPECT().ListRequestAuditEvents(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, filters *types.RequestAuditEventFilters) (*types.RequestAuditEventResponse, error) {
			captured = filters
			total := int64(1)
			return &types.RequestAuditEventResponse{
				Events: []*types.RequestAuditEvent{{ID: "rau_1"}},
				Total:  &total,
				Limit:  filters.Limit,
			}, nil
		})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit-events?user_id=usr_1&remote_ip=203.0.113.9&client_ip=198.51.100.7&path=/api/v1/sessions&method=post&status=403&since=2026-11-01T00:00:00Z&until=2026-11-02T00:00:00Z&limit=10&cursor=abc&include_total=true", nil)
	server.listRequestAuditEvents(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.NotNil(t, captured)
	require.Equal(t, "usr_1", captured.UserID)
	require.Equal(t, "203.0.113.9", captured.RemoteIP)
	require.Equal(t, "198.51.100.7", captured.ClientIP)
	require.Equal(t, "/api/v1/sessions", captured.PathPrefix)
	require.Equal(t, "post", captured.Method)
	require.NotNil(t, captured.Status)
	require.Equal(t, 403, *captured.Status)
	require.Equal(t, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), captured.Since.UTC())
	require.Equal(t, time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC), captured.Until.UTC())
	require.Equal(t, 10, captured.Limit)
	require.Equal(t, "abc", captured.Cursor)
	require.True(t, captured.IncludeTotal)

	var response types.RequestAuditEventResponse
	require.NoError(t, json.NewDecoder(recorder.Body).Decode(&response))
	require.Len(t, response.Events, 1)
}

func TestListRequestAuditEventsRejectsBadFilters(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}

	for _, query := range []string{"status=not-a-number", "since=yesterday", "until=yesterday", "limit=-1", "include_total=maybe"} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit-events?"+query, nil)
		server.listRequestAuditEvents(recorder, request)
		require.Equal(t, http.StatusBadRequest, recorder.Code, "query %q", query)
	}
}

func TestRequestAuditMiddlewareNeverRecordsRawBodySecrets(t *testing.T) {
	recorder, stdout := newTestAuditRecorder()

	router := mux.NewRouter()
	router.Use(newTestAuditMiddleware(recorder).Handler)
	router.HandleFunc("/api/v1/register", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}).Methods(http.MethodPost)

	body := `{"email":"new@example.com","password":"hunter2","token":"tok-1"}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/register", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(httptest.NewRecorder(), request)

	require.NotContains(t, stdout.String(), "hunter2")
	require.NotContains(t, stdout.String(), "tok-1")
	require.Contains(t, stdout.String(), "new@example.com", "login/register email must stay readable")
}

// A route whose payload is itself a credential document records only its size
// and content type: no amount of key-based redaction can make it safe.
func TestRequestAuditMiddlewareDropsSensitiveRouteBodies(t *testing.T) {
	paths := []string{
		"/api/v1/secrets",
		"/api/v1/projects/prj_1/secrets",
		"/api/v1/auth/login",
		"/api/v1/auth/password-reset",
		"/api/v1/oauth/providers",
		"/api/v1/oauth/connections/oauth_1",
		"/api/v1/git-provider-connections",
		"/api/v1/service-connections",
		"/api/v1/provider-endpoints",
		"/api/v1/agents",
		"/api/v1/apps/app_1",
		"/api/v1/organizations/org_1/webhook-endpoints",
		"/api/v1/organizations/org_1/sandboxes",
		"/api/v1/projects/prj_1/secret-intakes",
		"/connect/intake/submit",
	}

	const body = `{"name":"prod-key","value":"plaintext-value-here"}`

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			recorder, stdout := newTestAuditRecorder()
			router := mux.NewRouter()
			router.Use(newTestAuditMiddleware(recorder).Handler)
			router.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
			}).Methods(http.MethodPost)

			request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(httptest.NewRecorder(), request)

			require.NotContains(t, stdout.String(), "plaintext-value-here")
			event := lastAuditEvent(t, stdout)
			require.Empty(t, event.Body, "sensitive route bodies are dropped entirely")
			require.EqualValues(t, len(body), event.BodySize)
			require.Equal(t, "application/json", event.ContentType)
		})
	}
}

func TestRequestAuditMiddlewareKeepsNonSensitiveRouteBodies(t *testing.T) {
	recorder, stdout := newTestAuditRecorder()

	router := mux.NewRouter()
	router.Use(newTestAuditMiddleware(recorder).Handler)
	router.HandleFunc("/api/v1/sessions", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
	}).Methods(http.MethodPost)

	body := `{"name":"my-session","model":"claude"}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(httptest.NewRecorder(), request)

	event := lastAuditEvent(t, stdout)
	require.Contains(t, event.Body, "my-session")
}

// The actor is whoever authenticated, not whoever the request was dispatched
// for: in-process dispatch calls setRequestUser a second time.
func TestSetRequestUserDoesNotOverwriteAuditActor(t *testing.T) {
	recorder, stdout := newTestAuditRecorder()

	router := mux.NewRouter()
	router.Use(newTestAuditMiddleware(recorder).Handler)
	api := router.PathPrefix("/api/v1").Subrouter()
	api.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Auth middleware authenticates the caller.
			r = r.WithContext(setRequestUser(r.Context(), types.User{
				ID: "usr_caller", Email: "caller@example.com",
			}))
			next.ServeHTTP(w, r)
		})
	})
	api.HandleFunc("/compat/dispatch", func(w http.ResponseWriter, r *http.Request) {
		// In-process dispatch re-authenticates as another identity.
		r = r.WithContext(setRequestUser(r.Context(), types.User{
			ID: "usr_dispatched", Email: "dispatched@example.com",
		}))
		w.WriteHeader(http.StatusOK)
	}).Methods(http.MethodPost)

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v1/compat/dispatch", strings.NewReader("{}")))

	event := lastAuditEvent(t, stdout)
	require.Equal(t, "usr_caller", event.UserID)
	require.Equal(t, "caller@example.com", event.UserEmail)
}

// A panicking handler still leaves an audit row, recorded as a 500, and the
// panic continues so the server's recovery handler still runs.
func TestRequestAuditMiddlewareRecordsPanics(t *testing.T) {
	recorder, stdout := newTestAuditRecorder()

	router := mux.NewRouter()
	router.Use(newTestAuditMiddleware(recorder).Handler)
	router.HandleFunc("/api/v1/boom", func(w http.ResponseWriter, r *http.Request) {
		panic("handler exploded")
	}).Methods(http.MethodPost)

	require.PanicsWithValue(t, "handler exploded", func() {
		router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v1/boom", strings.NewReader("{}")))
	})

	event := lastAuditEvent(t, stdout)
	require.Equal(t, http.StatusInternalServerError, event.Status)
	require.Equal(t, "/api/v1/boom", event.Path)
}

func TestRequestAuditMiddlewareTrustedProxyClientIP(t *testing.T) {
	t.Run("trusted peer derives client ip from cloudflare", func(t *testing.T) {
		recorder, stdout := newTestAuditRecorder()
		middleware := newTrustedProxyAuditMiddleware(recorder, "10.0.0.0/8")

		router := mux.NewRouter()
		router.Use(middleware.Handler)
		router.HandleFunc("/api/v1/things", func(w http.ResponseWriter, r *http.Request) {}).Methods(http.MethodGet)

		request := httptest.NewRequest(http.MethodGet, "/api/v1/things", nil)
		request.RemoteAddr = "10.1.2.3:5555"
		request.Header.Set("CF-Connecting-IP", "198.51.100.7")
		request.Header.Set("X-Forwarded-For", "203.0.113.50, 10.1.2.3")
		router.ServeHTTP(httptest.NewRecorder(), request)

		event := lastAuditEvent(t, stdout)
		require.Equal(t, "10.1.2.3", event.RemoteIP)
		require.Equal(t, 5555, event.RemotePort)
		require.Equal(t, "198.51.100.7", event.ClientIP, "cloudflare header wins when the peer is trusted")
		require.Equal(t, "203.0.113.50, 10.1.2.3", event.XForwardedFor, "raw header is still recorded as sent")
	})

	t.Run("untrusted peer ignores proxy headers", func(t *testing.T) {
		recorder, stdout := newTestAuditRecorder()
		middleware := newTrustedProxyAuditMiddleware(recorder, "10.0.0.0/8")

		router := mux.NewRouter()
		router.Use(middleware.Handler)
		router.HandleFunc("/api/v1/things", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		request := httptest.NewRequest(http.MethodGet, "/api/v1/things", nil)
		request.RemoteAddr = "203.0.113.9:1234"
		request.Header.Set("CF-Connecting-IP", "198.51.100.7")
		request.Header.Set("X-Forwarded-For", "198.51.100.7")
		router.ServeHTTP(httptest.NewRecorder(), request)

		event := lastAuditEvent(t, stdout)
		require.Equal(t, "203.0.113.9", event.RemoteIP)
		require.Empty(t, event.ClientIP)
	})

	t.Run("forwarded for is used when cloudflare header is absent", func(t *testing.T) {
		recorder, stdout := newTestAuditRecorder()
		middleware := newTrustedProxyAuditMiddleware(recorder, "10.0.0.0/8")

		router := mux.NewRouter()
		router.Use(middleware.Handler)
		router.HandleFunc("/api/v1/things", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Methods(http.MethodGet)

		request := httptest.NewRequest(http.MethodGet, "/api/v1/things", nil)
		request.RemoteAddr = "10.1.2.3:5555"
		request.Header.Set("X-Forwarded-For", "203.0.113.50, 10.1.2.3")
		router.ServeHTTP(httptest.NewRecorder(), request)

		event := lastAuditEvent(t, stdout)
		require.Equal(t, "203.0.113.50", event.ClientIP)
	})
}
