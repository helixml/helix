// Package inferenceproxy is the body-aware reverse proxy that runs inside
// the Helix sandbox. It listens for OpenAI-compatible inference requests,
// reads the `model` field from the request body, and forwards to the
// matching container in the inner dockerd via Docker's built-in DNS.
//
// The proxy does NOT validate auth or rate-limit — it sits behind the API
// server's existing auth path and is only reachable via the established
// runner connection. Treat unauthenticated requests as a misconfiguration,
// not a security threat.
package inferenceproxy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"

	"github.com/helixml/helix/api/pkg/runner/composeparse"
)

// ModelLookup maps a request model name to a target container/port. Built
// fresh from the active profile's compose YAML each time the profile
// changes (cheap — ~5 entries).
type ModelLookup struct {
	mu      sync.RWMutex
	entries map[string]target // model name (lowercased) -> target
}

type target struct {
	container string
	port      int
}

// NewLookup parses the compose YAML and builds a routing table.
func NewLookup(composeYAML string) (*ModelLookup, error) {
	parsed, err := composeparse.Parse([]byte(composeYAML))
	if err != nil {
		return nil, err
	}
	l := &ModelLookup{entries: map[string]target{}}
	for _, m := range parsed.Models {
		l.entries[strings.ToLower(m.Name)] = target{container: m.ContainerName, port: m.InternalPort}
	}
	return l, nil
}

// Empty returns a routing table that knows about no models — used as the
// initial state before a profile is applied. Any request returns 404.
func Empty() *ModelLookup { return &ModelLookup{entries: map[string]target{}} }

// Models lists all model names known to this lookup, lowercased.
func (l *ModelLookup) Models() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	out := make([]string, 0, len(l.entries))
	for k := range l.entries {
		out = append(out, k)
	}
	return out
}

// Replace swaps the routing table atomically. Called when the active
// profile changes.
func (l *ModelLookup) Replace(other *ModelLookup) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = other.entries
}

// upstreamFor returns the http://localhost:port URL for a given model
// name, or an empty string if not found.
//
// The inference-proxy runs in the outer sandbox network namespace; it
// can't resolve the inner dockerd's container DNS. Compose `ports:`
// mappings expose the inner container on 127.0.0.1:<host_port> in the
// sandbox network — that's what we route to. composeparse extracts the
// host port (not the container port) into the `port` field.
func (l *ModelLookup) upstreamFor(model string) string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	t, ok := l.entries[strings.ToLower(model)]
	if !ok || t.port == 0 {
		return ""
	}
	return fmt.Sprintf("http://127.0.0.1:%d", t.port)
}

// Handler returns an HTTP handler that proxies OpenAI-compatible requests.
// On each request it reads + buffers the body, JSON-decodes only the
// `model` field, looks up the target, and forwards (with the original
// body) to the upstream's matching path.
//
// Endpoints handled:
//   - POST /v1/chat/completions
//   - POST /v1/embeddings
//   - POST /v1/images/generations
//   - GET  /v1/models  (returns the union from the lookup)
//
// Anything else returns 404.
func Handler(lookup *ModelLookup) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, _ *http.Request) {
		models := lookup.Models()
		writeJSON(w, http.StatusOK, modelsResponse(models))
	})
	for _, path := range []string{"/v1/chat/completions", "/v1/embeddings", "/v1/images/generations"} {
		path := path
		mux.HandleFunc("POST "+path, func(w http.ResponseWriter, r *http.Request) {
			proxyByModel(w, r, lookup, path)
		})
	}
	return mux
}

func proxyByModel(w http.ResponseWriter, r *http.Request, lookup *ModelLookup, upstreamPath string) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 32<<20)) // 32 MiB cap
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	model := extractModel(body)
	if model == "" {
		http.Error(w, "request body has no `model` field", http.StatusBadRequest)
		return
	}
	upstream := lookup.upstreamFor(model)
	if upstream == "" {
		http.Error(w, fmt.Sprintf("model %q not in active profile", model), http.StatusNotFound)
		return
	}
	target, err := url.Parse(upstream)
	if err != nil {
		http.Error(w, "bad upstream URL: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Build a single-shot reverse proxy. We could cache these per upstream
	// to save the ~microsecond setup, but proxy creation is cheap and the
	// upstream set changes on profile switches, so caching adds risk.
	rp := httputil.NewSingleHostReverseProxy(target)
	rp.Director = func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.URL.Path = upstreamPath
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.ContentLength = int64(len(body))
		req.Header.Set("Host", target.Host)
	}
	rp.ServeHTTP(w, r)
}

// extractModel reads the `model` field from a JSON body without decoding
// the whole thing. Cheap for big chat completion payloads with embedded
// images / long contexts.
func extractModel(body []byte) string {
	type modelOnly struct {
		Model string `json:"model"`
	}
	var m modelOnly
	if err := json.Unmarshal(body, &m); err != nil {
		return ""
	}
	return m.Model
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// modelsResponse mirrors the shape OpenAI's /v1/models returns.
func modelsResponse(names []string) any {
	type entry struct {
		ID     string `json:"id"`
		Object string `json:"object"`
	}
	out := struct {
		Object string  `json:"object"`
		Data   []entry `json:"data"`
	}{Object: "list"}
	for _, n := range names {
		out.Data = append(out.Data, entry{ID: n, Object: "model"})
	}
	return out
}

// ErrNoModel is returned when a request body has no `model` field.
var ErrNoModel = errors.New("request body has no `model` field")
