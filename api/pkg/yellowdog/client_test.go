package yellowdog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCredentialsAuthHeader(t *testing.T) {
	c := Credentials{KeyID: "ABC123", Secret: "supersecret"}
	got := c.authHeader()
	want := "yd-key ABC123:supersecret"
	if got != want {
		t.Errorf("authHeader() = %q, want %q", got, want)
	}
}

func TestCredentialsValid(t *testing.T) {
	cases := []struct {
		name  string
		creds Credentials
		want  bool
	}{
		{"both empty", Credentials{}, false},
		{"only key", Credentials{KeyID: "k"}, false},
		{"only secret", Credentials{Secret: "s"}, false},
		{"both set", Credentials{KeyID: "k", Secret: "s"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.creds.Valid(); got != tc.want {
				t.Errorf("Valid() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNewClientRejectsInvalidCreds(t *testing.T) {
	_, err := NewClient(Credentials{})
	if err == nil {
		t.Fatal("expected error from NewClient with empty credentials, got nil")
	}
}

// fakeServer stands in for portal.yellowdog.co. The test cases assert
// against the request the client emitted (headers, path, body) and
// reply with whatever shape the test wants.
//
// httptest.NewServer serves on http:// (no TLS), so we must pass
// WithInsecureBaseURL to bypass NewClient's HTTPS enforcement.
func fakeServer(t *testing.T, h http.HandlerFunc) (*httptest.Server, *Client) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := NewClient(
		Credentials{KeyID: "k", Secret: "s"},
		WithBaseURL(srv.URL),
		WithInsecureBaseURL(),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return srv, c
}

func TestListNamespacesSendsAuthHeader(t *testing.T) {
	var gotAuth string
	_, c := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{}, "nextSliceId": nil})
	})

	if _, err := c.ListNamespaces(context.Background()); err != nil {
		t.Fatalf("ListNamespaces: %v", err)
	}
	want := "yd-key k:s"
	if gotAuth != want {
		t.Errorf("Authorization header = %q, want %q", gotAuth, want)
	}
}

func TestListNamespacesHitsCorrectPath(t *testing.T) {
	var gotPath string
	_, c := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[],"nextSliceId":null}`))
	})
	if _, err := c.ListNamespaces(context.Background()); err != nil {
		t.Fatalf("ListNamespaces: %v", err)
	}
	if gotPath != "/namespaces" {
		t.Errorf("path = %q, want %q", gotPath, "/namespaces")
	}
}

func TestListNamespacesDecodesItems(t *testing.T) {
	_, c := fakeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"items":[
				{"id":"18048B","namespace":"yd-demo","deletable":false},
				{"id":"5C0252","namespace":"development","deletable":false}
			],
			"nextSliceId":null
		}`))
	})

	page, err := c.ListNamespaces(context.Background())
	if err != nil {
		t.Fatalf("ListNamespaces: %v", err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("len(Items) = %d, want 2", len(page.Items))
	}
	if page.Items[0].ID != "18048B" || page.Items[0].Namespace != "yd-demo" {
		t.Errorf("first item = %+v, want id=18048B namespace=yd-demo", page.Items[0])
	}
	if page.HasMore() {
		t.Errorf("HasMore() = true, want false when nextSliceId is null")
	}
}

func TestAPIErrorDecodedOn4xx(t *testing.T) {
	_, c := fakeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{
			"type":"about:blank",
			"title":"Unauthorized",
			"status":401,
			"detail":"key not recognised",
			"instance":"/api/namespaces"
		}`))
	})

	_, err := c.ListNamespaces(context.Background())
	if err == nil {
		t.Fatal("expected error on 401, got nil")
	}
	if !IsUnauthorized(err) {
		t.Errorf("IsUnauthorized(err) = false, want true (err = %v)", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err type = %T, want *APIError", err)
	}
	if apiErr.Status != 401 {
		t.Errorf("Status = %d, want 401", apiErr.Status)
	}
	if !strings.Contains(apiErr.Error(), "key not recognised") {
		t.Errorf("Error() missing detail; got %q", apiErr.Error())
	}
}

func TestAPIErrorOnNon4xxNonJSONResponse(t *testing.T) {
	_, c := fakeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("upstream is sad")) // not JSON
	})

	_, err := c.ListNamespaces(context.Background())
	if err == nil {
		t.Fatal("expected error on 502, got nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err type = %T, want *APIError", err)
	}
	if apiErr.Status != http.StatusBadGateway {
		t.Errorf("Status = %d, want %d", apiErr.Status, http.StatusBadGateway)
	}
}

// --- S2: Credentials redaction --------------------------------------

func TestCredentialsStringRedacts(t *testing.T) {
	c := Credentials{KeyID: "ABCDEF1234567890", Secret: "topsecret-value-12345"}
	got := c.String()
	if strings.Contains(got, "topsecret") {
		t.Errorf("String() leaked Secret: %q", got)
	}
	if strings.Contains(got, "1234567890") {
		t.Errorf("String() leaked full KeyID: %q", got)
	}
	if !strings.Contains(got, "ABCDEF") {
		t.Errorf("String() should still hint at KeyID prefix; got %q", got)
	}
	if !strings.Contains(got, "<redacted>") {
		t.Errorf("String() should include <redacted> marker for the Secret; got %q", got)
	}
}

func TestCredentialsFmtVerbsRedact(t *testing.T) {
	c := Credentials{KeyID: "k123456", Secret: "supersecret"}
	for _, verb := range []string{"%v", "%+v", "%#v", "%s"} {
		out := fmt.Sprintf(verb, c)
		if strings.Contains(out, "supersecret") {
			t.Errorf("fmt %s leaked Secret: %q", verb, out)
		}
	}
}

func TestCredentialsJSONMarshalDoesNotLeakSecret(t *testing.T) {
	c := Credentials{KeyID: "k123456", Secret: "supersecret"}
	wrapper := struct {
		Name string      `json:"name"`
		C    Credentials `json:"c"`
	}{Name: "ctx", C: c}
	raw, err := json.Marshal(wrapper)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(raw), "supersecret") {
		t.Errorf("Marshalled output leaked Secret: %s", raw)
	}
	if strings.Contains(string(raw), "k123456") {
		t.Errorf("Marshalled output leaked KeyID: %s", raw)
	}
}

func TestClientStringRedacts(t *testing.T) {
	c, err := NewClient(Credentials{KeyID: "ABCDEF1234567890", Secret: "supersecret"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	out := fmt.Sprintf("%+v", c)
	if strings.Contains(out, "supersecret") {
		t.Errorf("Client %%+v leaked Secret: %q", out)
	}
}

// --- S3: Non-HTTPS rejection ----------------------------------------

func TestNewClientRejectsHTTPBaseURL(t *testing.T) {
	_, err := NewClient(
		Credentials{KeyID: "k", Secret: "s"},
		WithBaseURL("http://example.com/api"),
	)
	if err == nil {
		t.Fatal("expected error from http:// base URL, got nil")
	}
	if !strings.Contains(err.Error(), "https") {
		t.Errorf("error should mention https; got %q", err.Error())
	}
}

func TestNewClientAllowsHTTPWithInsecureOpt(t *testing.T) {
	_, err := NewClient(
		Credentials{KeyID: "k", Secret: "s"},
		WithBaseURL("http://example.com/api"),
		WithInsecureBaseURL(),
	)
	if err != nil {
		t.Errorf("WithInsecureBaseURL should permit http://; got %v", err)
	}
}

func TestNewClientAcceptsHTTPSByDefault(t *testing.T) {
	_, err := NewClient(
		Credentials{KeyID: "k", Secret: "s"},
		WithBaseURL("https://example.com/api"),
	)
	if err != nil {
		t.Errorf("https:// base URL should be accepted by default; got %v", err)
	}
}

// --- S1: Response body size bound -----------------------------------

func TestResponseBodySizeBound(t *testing.T) {
	// Send a body larger than maxResponseBodyBytes (8 MB) consisting of
	// a giant JSON string literal. The LimitReader should truncate;
	// json.Decoder hits unexpected EOF and the call fails cleanly with
	// no OOM.
	_, c := fakeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// 9 MB of 'x' inside a JSON string. Total response ~9 MB.
		_, _ = w.Write([]byte(`{"items":[{"id":"`))
		buf := strings.Repeat("x", 1024)
		for i := 0; i < 9*1024; i++ {
			_, _ = w.Write([]byte(buf))
		}
		_, _ = w.Write([]byte(`","namespace":"x","deletable":false}],"nextSliceId":null}`))
	})

	_, err := c.ListNamespaces(context.Background())
	if err == nil {
		t.Fatal("expected error from oversized response, got nil")
	}
	// We don't care which specific decode error; we care that we got
	// SOMETHING and did not blow up. Sanity check the call returned in
	// finite time, which it has since we're here.
}

// --- C1: Retry behaviour --------------------------------------------

func TestRetryOn5xxThenSuccess(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 3 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"status":502,"title":"Bad Gateway"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[],"nextSliceId":null}`))
	}))
	t.Cleanup(srv.Close)

	c, err := NewClient(
		Credentials{KeyID: "k", Secret: "s"},
		WithBaseURL(srv.URL),
		WithInsecureBaseURL(),
		WithRetry(3, 1*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	if _, err := c.ListNamespaces(context.Background()); err != nil {
		t.Fatalf("ListNamespaces with retry: %v", err)
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Errorf("attempt count = %d, want 3", got)
	}
}

func TestNoRetryOn4xx(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"status":401,"title":"Unauthorized"}`))
	}))
	t.Cleanup(srv.Close)

	c, err := NewClient(
		Credentials{KeyID: "k", Secret: "s"},
		WithBaseURL(srv.URL),
		WithInsecureBaseURL(),
		WithRetry(3, 1*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = c.ListNamespaces(context.Background())
	if err == nil {
		t.Fatal("expected error on 401, got nil")
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Errorf("attempts on 4xx = %d, want 1 (no retry)", got)
	}
}

func TestRetryExhausted(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"status":502}`))
	}))
	t.Cleanup(srv.Close)

	c, err := NewClient(
		Credentials{KeyID: "k", Secret: "s"},
		WithBaseURL(srv.URL),
		WithInsecureBaseURL(),
		WithRetry(2, 1*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = c.ListNamespaces(context.Background())
	if err == nil {
		t.Fatal("expected error after retries exhausted, got nil")
	}
	if !errors.As(err, new(*APIError)) {
		t.Errorf("error should still be *APIError after exhausting retries; got %T", err)
	}
	if got := atomic.LoadInt32(&attempts); got != 2 {
		t.Errorf("attempts = %d, want 2", got)
	}
}

func TestRetryContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)

	c, err := NewClient(
		Credentials{KeyID: "k", Secret: "s"},
		WithBaseURL(srv.URL),
		WithInsecureBaseURL(),
		// Large backoff to give cancellation a chance to win.
		WithRetry(5, 5*time.Second),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after the first attempt completes; the retry sleep
	// should be interrupted.
	time.AfterFunc(50*time.Millisecond, cancel)

	_, err = c.ListNamespaces(ctx)
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err should wrap context.Canceled; got %v", err)
	}
}

func TestRetryNotAppliedToPOST(t *testing.T) {
	// POST is not idempotent, so the retry loop should never fire.
	// We exercise the code path via Client.do directly, since none of
	// our endpoints today POST.
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)

	c, err := NewClient(
		Credentials{KeyID: "k", Secret: "s"},
		WithBaseURL(srv.URL),
		WithInsecureBaseURL(),
		WithRetry(3, 1*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	body := map[string]string{"hello": "world"}
	err = c.do(context.Background(), http.MethodPost, "/anything", body, nil)
	if err == nil {
		t.Fatal("expected error on 503, got nil")
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Errorf("POST attempts = %d, want 1 (no retry on non-idempotent)", got)
	}
}

func TestNoRetryByDefault(t *testing.T) {
	// Without WithRetry, a single 5xx surfaces immediately.
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)

	c, err := NewClient(
		Credentials{KeyID: "k", Secret: "s"},
		WithBaseURL(srv.URL),
		WithInsecureBaseURL(),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	_, err = c.ListNamespaces(context.Background())
	if err == nil {
		t.Fatal("expected error on 502, got nil")
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Errorf("attempts = %d, want 1 (retry disabled by default)", got)
	}
}
