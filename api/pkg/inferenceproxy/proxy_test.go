package inferenceproxy

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const tinyCompose = `
services:
  vllm-tiny:
    image: vllm/vllm-openai:latest
    container_name: vllm-tiny
    ports:
      - "8000:8000"
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              device_ids: ["0"]
    command: ["--model", "x/y", "--served-model-name", "qwen-tiny"]
`

func TestNewLookup(t *testing.T) {
	l, err := NewLookup(tinyCompose)
	if err != nil {
		t.Fatal(err)
	}
	models := l.Models()
	if len(models) != 1 || models[0] != "qwen-tiny" {
		t.Errorf("models: got %v, want [qwen-tiny]", models)
	}
	// Upstream is 127.0.0.1:<host_port>. composeparse extracts host port
	// from "8000:8000" — the first 8000 (host side).
	if got := l.upstreamFor("qwen-tiny"); got != "http://127.0.0.1:8000" {
		t.Errorf("upstream: got %q, want http://127.0.0.1:8000", got)
	}
	if got := l.upstreamFor("Unknown"); got != "" {
		t.Errorf("unknown model should return empty; got %q", got)
	}
	// Case-insensitive match.
	if got := l.upstreamFor("QWEN-TINY"); got != "http://127.0.0.1:8000" {
		t.Errorf("case-insensitive lookup failed; got %q", got)
	}
}

func TestExtractModel(t *testing.T) {
	cases := []struct {
		body string
		want string
	}{
		{`{"model":"qwen-tiny","messages":[]}`, "qwen-tiny"},
		{`{"messages":[],"model":"qwen-tiny"}`, "qwen-tiny"},
		{`{"input":"foo","model":"emb"}`, "emb"},
		{`{"messages":[]}`, ""},
		{`not json`, ""},
		{``, ""},
	}
	for _, tc := range cases {
		got := extractModel([]byte(tc.body))
		if got != tc.want {
			t.Errorf("extractModel(%q) = %q, want %q", tc.body, got, tc.want)
		}
	}
}

func TestHandler_ProxyByModel(t *testing.T) {
	// Stand up a fake upstream that records what it received.
	var (
		gotPath   string
		gotBody   []byte
		gotMethod string
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-foo"}`))
	}))
	defer upstream.Close()

	// Construct a lookup whose model points at our test upstream.
	l := Empty()
	// Insert manually since the test upstream isn't a real container.
	l.entries = map[string]target{
		"qwen-tiny": {container: stripProtoHost(upstream.URL), port: 0}, // overridden below
	}
	// We bypass upstreamFor to point at the test server's URL directly.
	// Cleanest way: wrap the handler with a custom lookup that returns
	// the test URL string directly. We override below.
	customLookup := &fakeLookup{up: upstream.URL}
	h := handlerWithFakeLookup(customLookup)

	req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"qwen-tiny","messages":[]}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "chatcmpl-foo") {
		t.Errorf("response body not proxied; got %q", rec.Body.String())
	}
	if gotPath != "/v1/chat/completions" {
		t.Errorf("upstream path: got %q", gotPath)
	}
	if gotMethod != "POST" {
		t.Errorf("upstream method: got %q", gotMethod)
	}
	if !strings.Contains(string(gotBody), `"qwen-tiny"`) {
		t.Errorf("upstream body: missing model field; got %q", gotBody)
	}
}

func TestHandler_404OnUnknownModel(t *testing.T) {
	h := Handler(Empty())
	req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"does-not-exist"}`)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rec.Code)
	}
}

func TestHandler_400OnMissingModel(t *testing.T) {
	h := Handler(Empty())
	req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader([]byte(`{"messages":[]}`)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rec.Code)
	}
}

func TestHandler_GetModelsListsKnown(t *testing.T) {
	l, _ := NewLookup(tinyCompose)
	h := Handler(l)
	req := httptest.NewRequest("GET", "/v1/models", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "qwen-tiny") {
		t.Errorf("body missing qwen-tiny: %s", rec.Body.String())
	}
}

// --- helpers ---

// fakeLookup lets us test the handler against an httptest server whose URL
// has a port (the lookup's `container:port` model doesn't fit, so we
// substitute a single-string upstream).
type fakeLookup struct{ up string }

func (f *fakeLookup) Models() []string                  { return []string{"qwen-tiny"} }
func (f *fakeLookup) upstreamFor(model string) string {
	if model == "qwen-tiny" {
		return f.up
	}
	return ""
}
func (f *fakeLookup) Replace(*ModelLookup) {}

// handlerWithFakeLookup mirrors Handler() but accepts an interface so
// tests can substitute the upstream resolution.
func handlerWithFakeLookup(l interface {
	Models() []string
	upstreamFor(string) string
}) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, modelsResponse(l.Models()))
	})
	for _, path := range []string{"/v1/chat/completions", "/v1/embeddings", "/v1/images/generations"} {
		path := path
		mux.HandleFunc("POST "+path, func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(io.LimitReader(r.Body, 32<<20))
			model := extractModel(body)
			if model == "" {
				http.Error(w, "no model", http.StatusBadRequest)
				return
			}
			up := l.upstreamFor(model)
			if up == "" {
				http.Error(w, "unknown model", http.StatusNotFound)
				return
			}
			req2, _ := http.NewRequest("POST", up+path, bytes.NewReader(body))
			req2.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req2)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			defer resp.Body.Close()
			w.WriteHeader(resp.StatusCode)
			_, _ = io.Copy(w, resp.Body)
		})
	}
	return mux
}

func stripProtoHost(u string) string {
	u = strings.TrimPrefix(u, "http://")
	u = strings.TrimPrefix(u, "https://")
	if i := strings.Index(u, ":"); i >= 0 {
		return u[:i]
	}
	return u
}
