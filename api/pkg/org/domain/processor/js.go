package processor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dop251/goja"

	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// KindJS runs a user-authored JavaScript function against each inbound
// Message. Scripts can transform the event, drop it, fan it out across
// labeled outputs, and issue HTTP requests via a built-in client.
//
// # Script contract
//
// The config field `code` is a JavaScript program that must define:
//
//	function process(event, ctx) { … }
//
// process is invoked once per inbound Message.
//
// ## event
//
// A plain object with the canonical Message wire keys (always present):
//
//	{
//	  from, to, subject, body, body_content_type,
//	  thread_id, in_reply_to, message_id,
//	  attachments,   // array of {filename, content_type, url, size_bytes}
//	  extra,         // object (parsed JSON) or null
//	  reply_hint
//	}
//
// Mutating event and returning it rewrites the published Message.
//
// ## ctx
//
//	ctx.http     — HTTP client (same as the global `http`; see below)
//	ctx.outputs  — [{index, label, topic_id}, …] for multi-branch routing
//
// ## return value
//
//	null / undefined          → drop (no publish)
//	event object              → publish to the single/default output
//	{ out: label|index, event?: event }
//	                          → publish to the named/numbered branch
//	[ … ]                     → fan-out; each element is one of the above
//
// When there is exactly one output, returning the event is enough.
// With multiple outputs, either route via `{ out: "label" }` (or
// numeric index) or return a bare event to hit the first output.
//
// ## HTTP client
//
//	const res = http.get(url, options?)
//	const res = http.post(url, options?)
//	const res = http.put(url, options?)
//	const res = http.patch(url, options?)
//	const res = http.delete(url, options?)
//	const res = http.request(method, url, options?)
//
// options (all optional):
//
//	headers:    { "Name": "value" }
//	query:      { key: "value" }          // appended to the URL
//	body:       string | object           // objects JSON-encoded
//	json:       object                    // alias for body + JSON Content-Type
//	timeout_ms: number                    // default 10000, max 30000
//
// response:
//
//	status   number
//	ok       boolean   // 200–299
//	headers  object    // lowercase keys
//	body     string
//	json()   any       // parse body as JSON (throws on invalid JSON)
//
// JSON.parse / JSON.stringify are provided. There is no filesystem,
// process, or require — the sandbox is host functions only.
//
// # Example — enrich and rewrite
//
//	function process(event, ctx) {
//	  const res = http.get("https://api.example.com/lookup", {
//	    query: { email: event.from },
//	    headers: { "Authorization": "Bearer …" },
//	  });
//	  if (!res.ok) return null; // drop on upstream failure
//	  const data = res.json();
//	  event.subject = `[${data.tier}] ${event.subject}`;
//	  event.extra = Object.assign({}, event.extra || {}, { tier: data.tier });
//	  return event;
//	}
//
// # Example — route by HTTP status of a webhook call
//
//	function process(event, ctx) {
//	  const res = http.post("https://hooks.example.com/ingest", {
//	    json: { from: event.from, body: event.body },
//	  });
//	  if (res.ok) return { out: "ok", event: event };
//	  return { out: "failed", event: event };
//	}
const KindJS Kind = "js"

// Defaults and hard caps for KindJS HTTP.
const (
	jsDefaultHTTPTimeout = 10 * time.Second
	jsMaxHTTPTimeout     = 30 * time.Second
	jsMaxResponseBytes   = 1 << 20 // 1 MiB
	jsMaxScriptBytes     = 256 << 10
)

// jsConfig is the parsed config for KindJS.
type jsConfig struct {
	Code string `json:"code"`
}

// Validate requires non-empty code that parses and defines process, and
// at least one output. Match predicates on outputs are ignored by js
// (routing is decided by the script return value); they may be non-empty
// for chart labeling but are not evaluated here.
func (c jsConfig) Validate(out []Output) error {
	if len(out) < 1 {
		return errors.New("js processor needs at least 1 output")
	}
	if err := c.checkCode(); err != nil {
		return err
	}
	return nil
}

// checkCode parses the script and ensures a process function is defined.
func (c jsConfig) checkCode() error {
	code := strings.TrimSpace(c.Code)
	if code == "" {
		return errors.New("js code is empty")
	}
	if len(code) > jsMaxScriptBytes {
		return fmt.Errorf("js code exceeds %d bytes", jsMaxScriptBytes)
	}
	vm := goja.New()
	installJSBuiltins(vm, nil) // no HTTP during validate
	if _, err := vm.RunString(code); err != nil {
		return fmt.Errorf("parse js code: %w", err)
	}
	fn, ok := goja.AssertFunction(vm.Get("process"))
	if !ok || fn == nil {
		return errors.New(`js code must define function process(event, ctx) { … }`)
	}
	return nil
}

// Process runs the script's process(event, ctx) against the inbound
// Message and maps the return value to Results.
func (c jsConfig) Process(ctx context.Context, in streaming.Message, out []Output) ([]Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	vm := goja.New()
	// Interrupt the VM if the parent context is cancelled (e.g. hop
	// timeout or shutdown) so a runaway script cannot block the runner.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			vm.Interrupt("context cancelled")
		case <-stop:
		}
	}()

	client := &http.Client{Timeout: jsDefaultHTTPTimeout}
	installJSBuiltins(vm, &jsHTTP{ctx: ctx, client: client})

	if _, err := vm.RunString(c.Code); err != nil {
		return nil, fmt.Errorf("js load: %w", err)
	}
	fn, ok := goja.AssertFunction(vm.Get("process"))
	if !ok || fn == nil {
		return nil, errors.New("js process function missing at runtime")
	}

	eventVal := messageToJS(vm, in)
	ctxVal := vm.NewObject()
	_ = ctxVal.Set("http", vm.Get("http"))
	_ = ctxVal.Set("outputs", outputsToJS(vm, out))

	ret, err := fn(goja.Undefined(), eventVal, ctxVal)
	if err != nil {
		return nil, fmt.Errorf("js process: %w", err)
	}
	return mapJSReturn(vm, ret, in, out)
}

// js is the Strategy for KindJS.
type js struct{}

// ParseConfig decodes the raw blob into a jsConfig.
func (js) ParseConfig(raw json.RawMessage) (Config, error) {
	if len(raw) == 0 {
		return jsConfig{}, nil
	}
	var c jsConfig
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("parse js config: %w", err)
	}
	return c, nil
}

// ---------------------------------------------------------------------------
// Host builtins: JSON + http
// ---------------------------------------------------------------------------

// installJSBuiltins installs JSON.parse/stringify and the http client
// on the VM. httpClient may be nil (validate path) — http methods then
// throw if called.
func installJSBuiltins(vm *goja.Runtime, httpClient *jsHTTP) {
	jsonObj := vm.NewObject()
	_ = jsonObj.Set("parse", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			panic(vm.ToValue("JSON.parse: missing argument"))
		}
		s := call.Argument(0).String()
		var v any
		if err := json.Unmarshal([]byte(s), &v); err != nil {
			panic(vm.ToValue(fmt.Sprintf("JSON.parse: %v", err)))
		}
		return vm.ToValue(v)
	})
	_ = jsonObj.Set("stringify", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return vm.ToValue("null")
		}
		exported := call.Argument(0).Export()
		b, err := json.Marshal(exported)
		if err != nil {
			panic(vm.ToValue(fmt.Sprintf("JSON.stringify: %v", err)))
		}
		return vm.ToValue(string(b))
	})
	_ = vm.Set("JSON", jsonObj)

	httpObj := vm.NewObject()
	do := func(method string) func(call goja.FunctionCall) goja.Value {
		return func(call goja.FunctionCall) goja.Value {
			if httpClient == nil {
				panic(vm.ToValue("http is not available during validation"))
			}
			if len(call.Arguments) < 1 {
				panic(vm.ToValue(method + ": url is required"))
			}
			u := call.Argument(0).String()
			var opts map[string]any
			if len(call.Arguments) >= 2 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
				exported := call.Argument(1).Export()
				m, ok := exported.(map[string]any)
				if !ok {
					panic(vm.ToValue(method + ": options must be an object"))
				}
				opts = m
			}
			res, err := httpClient.do(method, u, opts)
			if err != nil {
				panic(vm.ToValue(err.Error()))
			}
			return httpResponseToJS(vm, res)
		}
	}
	_ = httpObj.Set("get", do(http.MethodGet))
	_ = httpObj.Set("post", do(http.MethodPost))
	_ = httpObj.Set("put", do(http.MethodPut))
	_ = httpObj.Set("patch", do(http.MethodPatch))
	_ = httpObj.Set("delete", do(http.MethodDelete))
	_ = httpObj.Set("request", func(call goja.FunctionCall) goja.Value {
		if httpClient == nil {
			panic(vm.ToValue("http is not available during validation"))
		}
		if len(call.Arguments) < 2 {
			panic(vm.ToValue("http.request: method and url are required"))
		}
		method := strings.ToUpper(call.Argument(0).String())
		u := call.Argument(1).String()
		var opts map[string]any
		if len(call.Arguments) >= 3 && !goja.IsUndefined(call.Argument(2)) && !goja.IsNull(call.Argument(2)) {
			exported := call.Argument(2).Export()
			m, ok := exported.(map[string]any)
			if !ok {
				panic(vm.ToValue("http.request: options must be an object"))
			}
			opts = m
		}
		res, err := httpClient.do(method, u, opts)
		if err != nil {
			panic(vm.ToValue(err.Error()))
		}
		return httpResponseToJS(vm, res)
	})
	_ = vm.Set("http", httpObj)
}

// jsHTTP is the host-side HTTP implementation for scripts.
type jsHTTP struct {
	ctx    context.Context
	client *http.Client
}

type jsHTTPResponse struct {
	Status  int
	Headers map[string]string
	Body    string
}

func (h *jsHTTP) do(method, rawURL string, opts map[string]any) (*jsHTTPResponse, error) {
	if h == nil || h.client == nil {
		return nil, errors.New("http client not configured")
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		return nil, errors.New("http: method is empty")
	}
	if strings.TrimSpace(rawURL) == "" {
		return nil, errors.New("http: url is empty")
	}

	timeout := jsDefaultHTTPTimeout
	if opts != nil {
		if v, ok := opts["timeout_ms"]; ok {
			ms, err := asInt64(v)
			if err != nil {
				return nil, fmt.Errorf("http: timeout_ms: %w", err)
			}
			if ms <= 0 {
				return nil, errors.New("http: timeout_ms must be > 0")
			}
			timeout = time.Duration(ms) * time.Millisecond
			if timeout > jsMaxHTTPTimeout {
				timeout = jsMaxHTTPTimeout
			}
		}
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("http: bad url: %w", err)
	}
	if opts != nil {
		if q, ok := opts["query"]; ok && q != nil {
			qm, ok := q.(map[string]any)
			if !ok {
				return nil, errors.New("http: query must be an object")
			}
			vals := u.Query()
			for k, v := range qm {
				vals.Set(k, fmt.Sprint(v))
			}
			u.RawQuery = vals.Encode()
		}
	}

	var bodyReader io.Reader
	setJSONContentType := false
	if opts != nil {
		if j, ok := opts["json"]; ok && j != nil {
			b, err := json.Marshal(j)
			if err != nil {
				return nil, fmt.Errorf("http: marshal json: %w", err)
			}
			bodyReader = bytes.NewReader(b)
			setJSONContentType = true
		} else if b, ok := opts["body"]; ok && b != nil {
			switch v := b.(type) {
			case string:
				bodyReader = strings.NewReader(v)
			default:
				encoded, err := json.Marshal(v)
				if err != nil {
					return nil, fmt.Errorf("http: marshal body: %w", err)
				}
				bodyReader = bytes.NewReader(encoded)
				setJSONContentType = true
			}
		}
	}

	reqCtx, cancel := context.WithTimeout(h.ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, method, u.String(), bodyReader)
	if err != nil {
		return nil, fmt.Errorf("http: build request: %w", err)
	}
	if setJSONContentType {
		req.Header.Set("Content-Type", "application/json")
	}
	if opts != nil {
		if hdrs, ok := opts["headers"]; ok && hdrs != nil {
			hm, ok := hdrs.(map[string]any)
			if !ok {
				return nil, errors.New("http: headers must be an object")
			}
			for k, v := range hm {
				req.Header.Set(k, fmt.Sprint(v))
			}
		}
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, jsMaxResponseBytes+1)
	bodyBytes, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("http: read body: %w", err)
	}
	if len(bodyBytes) > jsMaxResponseBytes {
		return nil, fmt.Errorf("http: response body exceeds %d bytes", jsMaxResponseBytes)
	}

	headers := make(map[string]string, len(resp.Header))
	for k, vs := range resp.Header {
		if len(vs) > 0 {
			headers[strings.ToLower(k)] = vs[0]
		}
	}
	return &jsHTTPResponse{
		Status:  resp.StatusCode,
		Headers: headers,
		Body:    string(bodyBytes),
	}, nil
}

func httpResponseToJS(vm *goja.Runtime, res *jsHTTPResponse) goja.Value {
	obj := vm.NewObject()
	_ = obj.Set("status", res.Status)
	_ = obj.Set("ok", res.Status >= 200 && res.Status < 300)
	_ = obj.Set("headers", res.Headers)
	_ = obj.Set("body", res.Body)
	_ = obj.Set("json", func(call goja.FunctionCall) goja.Value {
		var v any
		if err := json.Unmarshal([]byte(res.Body), &v); err != nil {
			panic(vm.ToValue(fmt.Sprintf("response.json: %v", err)))
		}
		return vm.ToValue(v)
	})
	return obj
}

// ---------------------------------------------------------------------------
// Message ↔ JS value mapping
// ---------------------------------------------------------------------------

func messageToJS(vm *goja.Runtime, m streaming.Message) goja.Value {
	obj := vm.NewObject()
	_ = obj.Set("from", m.From)
	to := make([]any, len(m.To))
	for i, t := range m.To {
		to[i] = t
	}
	_ = obj.Set("to", to)
	_ = obj.Set("subject", m.Subject)
	_ = obj.Set("body", m.Body)
	_ = obj.Set("body_content_type", m.BodyContentType)
	_ = obj.Set("thread_id", m.ThreadID)
	_ = obj.Set("in_reply_to", m.InReplyTo)
	_ = obj.Set("message_id", m.MessageID)
	_ = obj.Set("reply_hint", m.ReplyHint)

	atts := make([]any, 0, len(m.Attachments))
	for _, a := range m.Attachments {
		atts = append(atts, map[string]any{
			"filename":     a.Filename,
			"content_type": a.ContentType,
			"url":          a.URL,
			"size_bytes":   a.SizeBytes,
		})
	}
	_ = obj.Set("attachments", atts)

	var extra any
	if len(m.Extra) > 0 {
		if err := json.Unmarshal(m.Extra, &extra); err != nil {
			extra = string(m.Extra)
		}
	}
	_ = obj.Set("extra", extra)
	return obj
}

func outputsToJS(vm *goja.Runtime, out []Output) goja.Value {
	arr := make([]any, len(out))
	for i, o := range out {
		arr[i] = map[string]any{
			"index":    i,
			"label":    o.Label,
			"topic_id": string(o.TopicID),
		}
	}
	return vm.ToValue(arr)
}

// messageFromJS overlays fields from a JS-exported map onto base.
// Unknown keys are ignored; only canonical Message fields are applied.
func messageFromJS(base streaming.Message, v any) (streaming.Message, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return base, fmt.Errorf("event must be an object, got %T", v)
	}
	out := base
	if x, ok := m["from"]; ok {
		out.From = fmt.Sprint(x)
	}
	if x, ok := m["subject"]; ok {
		out.Subject = fmt.Sprint(x)
	}
	if x, ok := m["body"]; ok {
		out.Body = fmt.Sprint(x)
	}
	if x, ok := m["body_content_type"]; ok {
		out.BodyContentType = fmt.Sprint(x)
	}
	if x, ok := m["thread_id"]; ok {
		out.ThreadID = fmt.Sprint(x)
	}
	if x, ok := m["in_reply_to"]; ok {
		out.InReplyTo = fmt.Sprint(x)
	}
	if x, ok := m["message_id"]; ok {
		out.MessageID = fmt.Sprint(x)
	}
	if x, ok := m["reply_hint"]; ok {
		out.ReplyHint = fmt.Sprint(x)
	}
	if x, ok := m["to"]; ok && x != nil {
		switch arr := x.(type) {
		case []any:
			to := make([]string, 0, len(arr))
			for _, e := range arr {
				to = append(to, fmt.Sprint(e))
			}
			out.To = to
		case []string:
			out.To = arr
		default:
			return base, fmt.Errorf("event.to must be an array")
		}
	}
	if x, ok := m["extra"]; ok {
		if x == nil {
			out.Extra = nil
		} else {
			b, err := json.Marshal(x)
			if err != nil {
				return base, fmt.Errorf("event.extra: %w", err)
			}
			out.Extra = b
		}
	}
	if x, ok := m["attachments"]; ok && x != nil {
		arr, ok := x.([]any)
		if !ok {
			return base, fmt.Errorf("event.attachments must be an array")
		}
		atts := make([]streaming.Attachment, 0, len(arr))
		for i, e := range arr {
			em, ok := e.(map[string]any)
			if !ok {
				return base, fmt.Errorf("event.attachments[%d] must be an object", i)
			}
			a := streaming.Attachment{}
			if v, ok := em["filename"]; ok {
				a.Filename = fmt.Sprint(v)
			}
			if v, ok := em["content_type"]; ok {
				a.ContentType = fmt.Sprint(v)
			}
			if v, ok := em["url"]; ok {
				a.URL = fmt.Sprint(v)
			}
			if v, ok := em["size_bytes"]; ok {
				n, err := asInt64(v)
				if err != nil {
					return base, fmt.Errorf("event.attachments[%d].size_bytes: %w", i, err)
				}
				a.SizeBytes = n
			}
			atts = append(atts, a)
		}
		out.Attachments = atts
	}
	return out, nil
}

// mapJSReturn converts the process() return value into Results.
func mapJSReturn(vm *goja.Runtime, ret goja.Value, base streaming.Message, out []Output) ([]Result, error) {
	if ret == nil || goja.IsUndefined(ret) || goja.IsNull(ret) {
		return nil, nil // drop
	}
	exported := ret.Export()

	// Array → fan-out.
	if arr, ok := exported.([]any); ok {
		var results []Result
		for i, item := range arr {
			rs, err := mapOneReturn(item, base, out)
			if err != nil {
				return nil, fmt.Errorf("return[%d]: %w", i, err)
			}
			results = append(results, rs...)
		}
		return results, nil
	}
	return mapOneReturn(exported, base, out)
}

// mapOneReturn handles a single return element: an event object or a
// { out, event } route object.
func mapOneReturn(v any, base streaming.Message, out []Output) ([]Result, error) {
	if v == nil {
		return nil, nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("return value must be an object or array, got %T", v)
	}

	// Route form: { out: label|index, event?: … }
	if routeKey, hasOut := m["out"]; hasOut {
		topic, err := resolveOutput(routeKey, out)
		if err != nil {
			return nil, err
		}
		msg := base
		if ev, ok := m["event"]; ok && ev != nil {
			msg, err = messageFromJS(base, ev)
			if err != nil {
				return nil, err
			}
		} else {
			// Allow shorthand: put message fields alongside `out`.
			// If only `out` is set, pass the original message through.
			onlyOut := true
			for k := range m {
				if k != "out" {
					onlyOut = false
					break
				}
			}
			if !onlyOut {
				// Treat the whole object (minus out) as the event.
				ev := make(map[string]any, len(m))
				for k, val := range m {
					if k == "out" {
						continue
					}
					ev[k] = val
				}
				// Prefer nested event if both somehow present (already handled).
				if _, hasEvent := m["event"]; !hasEvent {
					msg, err = messageFromJS(base, ev)
					if err != nil {
						return nil, err
					}
				}
			}
		}
		return []Result{{TopicID: topic, Message: msg}}, nil
	}

	// Plain event → first output (transform default).
	if len(out) == 0 {
		return nil, errors.New("js processor has no outputs")
	}
	msg, err := messageFromJS(base, m)
	if err != nil {
		return nil, err
	}
	return []Result{{TopicID: out[0].TopicID, Message: msg}}, nil
}

// resolveOutput picks an Output by label (string) or index (number).
func resolveOutput(key any, out []Output) (streaming.TopicID, error) {
	switch k := key.(type) {
	case string:
		if k == "" {
			if len(out) == 0 {
				return "", errors.New("no outputs")
			}
			return out[0].TopicID, nil
		}
		for _, o := range out {
			if o.Label == k {
				return o.TopicID, nil
			}
		}
		// Also accept topic_id directly for convenience.
		for _, o := range out {
			if string(o.TopicID) == k {
				return o.TopicID, nil
			}
		}
		return "", fmt.Errorf("unknown output %q (labels: %s)", k, outputLabels(out))
	case int64:
		i := int(k)
		if i < 0 || i >= len(out) {
			return "", fmt.Errorf("output index %d out of range [0,%d)", i, len(out))
		}
		return out[i].TopicID, nil
	case float64:
		i := int(k)
		if float64(i) != k {
			return "", fmt.Errorf("output index must be an integer, got %v", k)
		}
		if i < 0 || i >= len(out) {
			return "", fmt.Errorf("output index %d out of range [0,%d)", i, len(out))
		}
		return out[i].TopicID, nil
	default:
		// goja may export numbers as int/int32 depending on value.
		if n, err := asInt64(key); err == nil {
			i := int(n)
			if i < 0 || i >= len(out) {
				return "", fmt.Errorf("output index %d out of range [0,%d)", i, len(out))
			}
			return out[i].TopicID, nil
		}
		return "", fmt.Errorf("out must be a label string or numeric index, got %T", key)
	}
}

func outputLabels(out []Output) string {
	parts := make([]string, len(out))
	for i, o := range out {
		if o.Label != "" {
			parts[i] = o.Label
		} else {
			parts[i] = fmt.Sprintf("#%d", i)
		}
	}
	return strings.Join(parts, ", ")
}

func asInt64(v any) (int64, error) {
	switch n := v.(type) {
	case int:
		return int64(n), nil
	case int32:
		return int64(n), nil
	case int64:
		return n, nil
	case float64:
		if float64(int64(n)) != n {
			return 0, fmt.Errorf("not an integer: %v", n)
		}
		return int64(n), nil
	case json.Number:
		return n.Int64()
	default:
		return 0, fmt.Errorf("not a number: %T", v)
	}
}
