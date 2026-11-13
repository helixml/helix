//go:build !nokodit

package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// koditScopeGuardMaxBody bounds the JSON-RPC body the guard reads.
const koditScopeGuardMaxBody = 1 << 20

// koditScopeGuard checks MCP requests before the scoped Kodit handler sees
// them and refuses any file URI (tool call or resources/read, every read
// mode) whose repository is outside the session scope. Kodit v1.4.0 enforces
// the scope itself on every read path; this keeps a second, independent check
// on the one input that names a repository directly.
func koditScopeGuard(allowed map[int64]bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Body == nil {
			next.ServeHTTP(w, r)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, koditScopeGuardMaxBody+1))
		if err != nil {
			http.Error(w, "could not read request", http.StatusBadRequest)
			return
		}
		if len(body) > koditScopeGuardMaxBody {
			http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
			return
		}
		if reason := koditScopeViolation(body, allowed); reason != "" {
			http.Error(w, reason, http.StatusForbidden)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
		next.ServeHTTP(w, r)
	})
}

type koditRPCMessage struct {
	Method string `json:"method"`
	Params struct {
		URI       string          `json:"uri"`
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"params"`
}

// koditScopeViolation returns why a JSON-RPC body (one message or a batch)
// may not be served, or "" when it may.
func koditScopeViolation(body []byte, allowed map[int64]bool) string {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return ""
	}
	var messages []koditRPCMessage
	if trimmed[0] == '[' {
		if err := json.Unmarshal(trimmed, &messages); err != nil {
			return "malformed request"
		}
	} else {
		var one koditRPCMessage
		if err := json.Unmarshal(trimmed, &one); err != nil {
			return "malformed request"
		}
		messages = []koditRPCMessage{one}
	}
	for _, m := range messages {
		switch m.Method {
		case "resources/read":
			if !koditURIInScope(m.Params.URI, allowed) {
				return "resource is outside this session's repositories"
			}
		case "tools/call":
			if m.Params.Name == "kodit_read_resource" {
				uri, ok := koditExactStringArg(m.Params.Arguments, "uri")
				if !ok || !koditURIInScope(uri, allowed) {
					return "resource is outside this session's repositories"
				}
			}
		}
	}
	return ""
}

// koditURIInScope parses file://{id}/{blob}/{path} the way kodit does and
// reports whether {id} is in the allowed set. Anything unparseable is out.
func koditURIInScope(uri string, allowed map[int64]bool) bool {
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme != "file" {
		return false
	}
	rest := parsed.Host + parsed.Path
	id, _, _ := strings.Cut(rest, "/")
	repoID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return false
	}
	return allowed[repoID]
}

// koditExactStringArg reads arguments[key] the way mcp-go hands tool
// arguments to kodit: a map keyed by the exact name. Decoding into a struct
// would match keys case-insensitively and could check a different value
// from the one kodit reads.
func koditExactStringArg(raw json.RawMessage, key string) (string, bool) {
	var args map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &args) != nil {
		return "", false
	}
	value, ok := args[key]
	if !ok {
		return "", false
	}
	var s string
	if json.Unmarshal(value, &s) != nil {
		return "", false
	}
	return s, true
}
