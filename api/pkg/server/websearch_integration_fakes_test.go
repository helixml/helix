package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"

	"github.com/helixml/helix/api/pkg/websearch/native"
)

// The network edges of the web search stack, faked: SearXNG, the web
// (two sites), the document extraction service and an OpenAI-compatible
// LLM provider. Everything between them and the HTTP handlers is real.

// wsWeb serves two sites on two loopback hosts, so results span domains:
// a news site on 127.0.0.1 and a blog on localhost.
type wsWeb struct {
	news, blog *httptest.Server
	mu         sync.Mutex
	hits       map[string]int // "host:port/path" → GETs served
}

func newWSWeb(t *testing.T) *wsWeb {
	w := &wsWeb{hits: map[string]int{}}
	w.news = httptest.NewServer(http.HandlerFunc(w.serveNews))
	w.blog = httptest.NewServer(http.HandlerFunc(w.serveBlog))
	// httptest listens on 127.0.0.1; name the blog by localhost instead.
	w.blog.URL = strings.Replace(w.blog.URL, "127.0.0.1", "localhost", 1)
	t.Cleanup(w.news.Close)
	t.Cleanup(w.blog.Close)
	return w
}

func (w *wsWeb) newsURL(path string) string { return w.news.URL + path }
func (w *wsWeb) blogURL(path string) string { return w.blog.URL + path }

// hitCount is how many times the page at a normalised URL was served.
func (w *wsWeb) hitCount(pageURL string) int {
	u, err := url.Parse(pageURL)
	if err != nil {
		panic(err)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.hits[u.Host+u.Path]
}

func (w *wsWeb) count(r *http.Request) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.hits[r.Host+r.URL.Path]++
}

func (w *wsWeb) serveNews(rw http.ResponseWriter, r *http.Request) {
	w.count(r)
	switch r.URL.Path {
	case "/articles/go-generics":
		writeHTML(rw, newsArticle(w, "Go 1.18 brings generics to the language", "2026-10-03T09:30:00Z",
			`<p>Go 1.18 is the first release of Go with generics. Generic functions and types take type parameters, written in square brackets, and type parameters are constrained by interfaces.</p>
			<p>Before generics, Go programmers wrote the same container code once per element type or fell back to the empty interface and reflection. Type parameters remove that duplication while keeping static type checking.</p>
			<p>The release took more than ten years of design work. Ian Lance Taylor and Robert Griesemer wrote the type parameters proposal, which went through several public drafts before it was accepted.</p>
			<p>Read part 2 for constraints, type sets and the comparable interface, and see how the Rust community compares the two approaches.</p>`,
			`<li><a href="/articles/go-generics/part-2">Part 2: constraints and type sets</a></li>
			<li><a href="/about">About us</a></li>
			<li><a href="`+w.blogURL("/posts/rust-vs-go")+`">Rust traits versus Go generics</a></li>`))
	case "/articles/go-generics/part-2":
		writeHTML(rw, newsArticle(w, "Go generics part 2: constraints and type sets", "2026-10-05T09:30:00Z",
			`<p>A constraint is an interface that lists the types a type parameter may be instantiated with. Since Go 1.18 interfaces can contain type sets written with the tilde operator, such as ~int | ~float64.</p>
			<p>The predeclared comparable constraint permits any type that supports the equality operators, which makes it the natural constraint for map keys in generic code.</p>
			<p>The constraints package in golang.org/x/exp collects common constraints such as Ordered, for types that support the less-than operator.</p>`,
			`<li><a href="/articles/go-generics">Part 1: introducing generics</a></li>`))
	case "/about":
		writeHTML(rw, newsArticle(w, "About Example News", "",
			`<p>Example News covers programming languages, compilers and developer tools. Our reporters have followed the Go project since its first public release in 2009 and write about every major release.</p>
			<p>We are funded by our readers. We do not run advertising, and we publish corrections at the bottom of every article.</p>`,
			""))
	case "/articles/go-errors":
		writeHTML(rw, newsArticle(w, "Error handling in Go, revisited", "2025-01-10T08:00:00Z",
			`<p>Go reports errors as values. A function that can fail returns an error as its last result, and callers check it explicitly with an if statement.</p>
			<p>Errors can wrap other errors with fmt.Errorf and the %w verb, and errors.Is and errors.As inspect the chain. Proposals to add a try operator were withdrawn after community feedback.</p>`,
			""))
	case "/report.pdf":
		rw.Header().Set("Content-Type", "application/pdf")
		_, _ = rw.Write([]byte(wsFakePDF))
	case "/slow":
		select {
		case <-r.Context().Done():
		case <-time.After(10 * time.Second):
			writeHTML(rw, newsArticle(w, "Too late", "", "<p>This page is very slow.</p>", ""))
		}
	case "/favicon.ico", "/img/generics.png", "/img/diagram.png":
		rw.Header().Set("Content-Type", "image/png")
		_, _ = rw.Write([]byte("\x89PNG\r\n\x1a\n"))
	default:
		http.NotFound(rw, r)
	}
}

func (w *wsWeb) serveBlog(rw http.ResponseWriter, r *http.Request) {
	w.count(r)
	switch r.URL.Path {
	case "/posts/rust-vs-go":
		writeHTML(rw, blogPost("Rust traits versus Go generics", `,"datePublished":"2026-09-15","author":{"@type":"Person","name":"Grace Hopper"}`,
			`<p>Rust has had generics with trait bounds since 1.0. Go added type parameters in Go 1.18, constrained by interfaces rather than traits.</p>
			<p>Rust monomorphizes every instantiation, which costs compile time and binary size but makes generic code as fast as hand-written code. Go uses GC shape stenciling with dictionaries, a hybrid that compiles faster but can be slower at run time.</p>
			<p>Both languages check generic code once, at the definition, rather than at each use as C++ templates do.</p>`))
	case "/", "/posts":
		writeHTML(rw, blogPost("Notes on programming languages", "",
			`<p>This blog compares programming languages by building the same small programs in each of them, and writes up what was easy and what was hard.</p>
			<ul><li><a href="/posts/rust-vs-go">Rust traits versus Go generics</a></li>
			<li><a href="/posts/go-concurrency">Go concurrency patterns</a></li></ul>`))
	case "/posts/go-concurrency":
		writeHTML(rw, blogPost("Go concurrency patterns", "",
			`<p>Goroutines are cheap: a new goroutine starts with a few kilobytes of stack. Channels connect them, and the select statement waits on several channel operations at once.</p>
			<p>A worker pool is a fixed set of goroutines reading jobs from one channel and writing results to another. The errgroup package adds cancellation and error propagation to the pattern.</p>`))
	default:
		http.NotFound(rw, r)
	}
}

func writeHTML(rw http.ResponseWriter, body string) {
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(rw, body)
}

// newsArticle is a page in the style of a news CMS: OpenGraph and article
// meta tags, JSON-LD, site navigation, a cookie banner and a footer around
// the article.
func newsArticle(w *wsWeb, title, published, body, related string) string {
	meta := ""
	if published != "" {
		meta = `<meta property="article:published_time" content="` + published + `">`
	}
	return `<!doctype html>
<html lang="en"><head>
<meta charset="utf-8">
<title>` + title + ` | Example News</title>
<meta property="og:title" content="` + title + `">
<meta property="og:type" content="article">
<meta property="og:image" content="/img/generics.png">
<meta property="og:site_name" content="Example News">
` + meta + `
<link rel="icon" href="/favicon.ico">
<script type="application/ld+json">{"@context":"https://schema.org","@type":"NewsArticle","headline":"` + title + `","author":{"@type":"Person","name":"Ada Lovelace"}}</script>
</head><body>
<header><a href="/">Example News</a>
<nav><a href="/">Home</a> <a href="/topics/go">Go</a> <a href="/topics/rust">Rust</a> <a href="/subscribe">Subscribe</a></nav></header>
<div role="banner" class="cookie-banner">We use cookies to measure readership. <a href="/privacy">Privacy policy</a></div>
<main><article>
<h1>` + title + `</h1>
<p class="byline">By Ada Lovelace</p>
` + body + `
<figure><img src="/img/diagram.png" alt="Type parameter diagram"><figcaption>How type parameters are checked.</figcaption></figure>
<ul class="related">` + related + `</ul>
</article></main>
<aside class="sidebar"><h2>Most read</h2><a href="/articles/go-errors">Error handling in Go, revisited</a></aside>
<footer><p>Copyright 2026 Example News Ltd. All rights reserved.</p><a href="/contact">Contact</a></footer>
</body></html>`
}

// blogPost is a personal blog page with JSON-LD metadata only.
func blogPost(title, ldExtra, body string) string {
	return `<!doctype html>
<html><head>
<meta charset="utf-8">
<title>` + title + `</title>
<script type="application/ld+json">{"@context":"https://schema.org","@type":"BlogPosting","headline":"` + title + `"` + ldExtra + `}</script>
</head><body>
<nav><a href="/">Blog home</a> <a href="/posts">All posts</a></nav>
<article><h1>` + title + `</h1>` + body + `</article>
<footer>Written with care. Comments are closed.</footer>
</body></html>`
}

// wsFakePDF is enough of a PDF for content sniffing; the fake extraction
// service turns it into text.
const wsFakePDF = "%PDF-1.7\n1 0 obj << /Type /Catalog >> endobj\ntrailer << /Root 1 0 R >>\n%%EOF\n"

const wsPDFText = "Go Developer Survey 2026\n\nGenerics are used by 60 percent of respondents, up from 30 percent two years earlier. Most use them for container types and utility functions."

// wsExtractor is the document extraction service pkg/extract talks to.
type wsExtractor struct {
	srv *httptest.Server
}

func newWSExtractor(t *testing.T) *wsExtractor {
	e := &wsExtractor{}
	e.srv = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		var req struct {
			URL     string `json:"url"`
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(rw, err.Error(), http.StatusBadRequest)
			return
		}
		// The web search extractor must send content only: a URL would
		// make this service fetch it, bypassing the guarded fetcher.
		if req.URL != "" {
			http.Error(rw, "unexpected url "+req.URL, http.StatusBadRequest)
			return
		}
		content, err := base64.StdEncoding.DecodeString(req.Content)
		if err != nil || !strings.HasPrefix(string(content), "%PDF-") {
			http.Error(rw, "not a PDF", http.StatusUnprocessableEntity)
			return
		}
		_ = json.NewEncoder(rw).Encode(map[string]string{"text": wsPDFText})
	}))
	t.Cleanup(e.srv.Close)
	return e
}

// wsSearXNG is a SearXNG instance's JSON search API over a fixed index of
// the fake web. Published dates use each format SearXNG emits.
type wsSearXNG struct {
	srv     *httptest.Server
	web     *wsWeb
	now     time.Time
	mu      sync.Mutex
	queries []url.Values
}

func newWSSearXNG(t *testing.T, web *wsWeb) *wsSearXNG {
	s := &wsSearXNG{web: web, now: time.Now().UTC()}
	s.srv = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.srv.Close)
	return s
}

// requests returns the queries SearXNG received.
func (s *wsSearXNG) requests() []url.Values {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]url.Values(nil), s.queries...)
}

func (s *wsSearXNG) serve(rw http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if r.URL.Path != "/search" || q.Get("format") != "json" || r.UserAgent() == "" {
		http.Error(rw, "Too Many Requests", http.StatusTooManyRequests) // SearXNG's bot detection
		return
	}
	s.mu.Lock()
	s.queries = append(s.queries, q)
	s.mu.Unlock()

	resp := map[string]any{
		"query":                q.Get("q"),
		"number_of_results":    0,
		"results":              []any{},
		"answers":              []any{},
		"corrections":          []any{},
		"infoboxes":            []any{},
		"suggestions":          []any{"golang generics tutorial"},
		"unresponsive_engines": []any{},
	}
	if page, _ := strconv.Atoi(q.Get("pageno")); page <= 1 {
		resp["results"] = s.results(q)
		if strings.Contains(q.Get("q"), "flaky") {
			resp["unresponsive_engines"] = []any{[]any{"brave", "timeout"}}
		}
	}
	rw.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(rw).Encode(resp)
}

func (s *wsSearXNG) results(q url.Values) []any {
	day := 24 * time.Hour
	naive := func(t time.Time) string { return t.Format("2006-01-02T15:04:05.000000") }
	dateOnly := func(t time.Time) string { return t.Format("2006-01-02") }
	offset := func(t time.Time) string { return t.Format("2006-01-02T15:04:05") + "+00:00" }

	generics := wsSearXNGResult(s.web.newsURL("/articles/go-generics"), "Go 1.18 brings generics to the language",
		"Go 1.18 is the first release of Go with generics. Generic functions and types take type parameters…",
		naive(s.now.Add(-10*day)), "general", 1)
	errorsPage := wsSearXNGResult(s.web.newsURL("/articles/go-errors"), "Error handling in Go, revisited",
		"Go reports errors as values. A function that can fail returns an error as its last result…",
		offset(s.now.Add(-400*day)), "general", 3)
	// Upper-case host and a fragment, as engines sometimes return them;
	// the result id must be the normalised URL.
	rust := wsSearXNGResult(strings.Replace(s.web.blogURL("/posts/rust-vs-go"), "localhost", "LOCALHOST", 1)+"#comments",
		"Rust traits versus Go generics", "Rust has had generics with trait bounds since 1.0. Go added type parameters…",
		dateOnly(s.now.Add(-60*day)), "general", 2)
	concurrency := wsSearXNGResult(s.web.blogURL("/posts/go-concurrency"), "Go concurrency patterns",
		"Goroutines are cheap: a new goroutine starts with a few kilobytes of stack…", "", "general", 4)

	if q.Get("categories") == "news" {
		generics["category"], errorsPage["category"] = "news", "news"
		return []any{generics, errorsPage}
	}
	return []any{generics, rust, errorsPage, concurrency}
}

func wsSearXNGResult(pageURL, title, content, published, category string, position int) map[string]any {
	u, _ := url.Parse(pageURL)
	r := map[string]any{
		"url":           pageURL,
		"title":         title,
		"content":       content,
		"engine":        "duckduckgo",
		"engines":       []string{"duckduckgo", "brave"},
		"positions":     []int{position},
		"score":         2.0 / float64(position),
		"category":      category,
		"parsed_url":    []string{u.Scheme, u.Host, u.Path, "", "", u.Fragment},
		"template":      "default.html",
		"thumbnail":     "",
		"img_src":       "",
		"priority":      "",
		"publishedDate": nil,
	}
	if published != "" {
		r["publishedDate"] = published
	}
	return r
}

// wsPageCache is an in-memory native.PageCache.
type wsPageCache struct {
	mu    sync.Mutex
	pages map[string]native.RawPage
}

var _ native.PageCache = (*wsPageCache)(nil)

func newWSPageCache() *wsPageCache { return &wsPageCache{pages: map[string]native.RawPage{}} }

func (c *wsPageCache) Get(_ context.Context, pageURL string) (*native.RawPage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	p, ok := c.pages[pageURL]
	if !ok {
		return nil, nil
	}
	return &p, nil
}

func (c *wsPageCache) Put(_ context.Context, page *native.RawPage) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pages[page.URL] = *page
	return nil
}

// age makes the cached copy of pageURL older by d.
func (c *wsPageCache) age(pageURL string, d time.Duration) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	p, ok := c.pages[pageURL]
	if ok {
		p.FetchedAt = p.FetchedAt.Add(-d)
		c.pages[pageURL] = p
	}
	return ok
}

// Model names the fake LLM provider serves.
const (
	wsEmbeddingModel = "fake-embed-1"
	wsFastModel      = "fake-fast-1"
	wsDefaultModel   = "fake-default-1"
	wsStrongModel    = "fake-strong-1"
	wsLLMAPIKey      = "sk-fake"
)

// wsLLM is an OpenAI-compatible provider: /embeddings returns bag-of-words
// hash vectors, so cosine similarity tracks shared words and ranking is
// meaningful; /chat/completions returns canned text for each synthesis task,
// streamed when asked, and JSON built from the response_format schema.
type wsLLM struct {
	srv   *httptest.Server
	mu    sync.Mutex
	calls []wsLLMCall
}

type wsLLMCall struct {
	Path       string
	Model      string
	Stream     bool
	SchemaName string
	Strict     bool
}

func newWSLLM(t *testing.T) *wsLLM {
	l := &wsLLM{}
	l.srv = httptest.NewServer(http.HandlerFunc(l.serve))
	t.Cleanup(l.srv.Close)
	return l
}

func (l *wsLLM) baseURL() string { return l.srv.URL + "/v1" }

func (l *wsLLM) record(c wsLLMCall) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, c)
}

func (l *wsLLM) recorded() []wsLLMCall {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]wsLLMCall(nil), l.calls...)
}

func (l *wsLLM) serve(rw http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+wsLLMAPIKey {
		openAIError(rw, http.StatusUnauthorized, "invalid api key")
		return
	}
	// Generous limits, so the client's rate limiter never waits.
	rw.Header().Set("x-ratelimit-limit-requests", "100000")
	rw.Header().Set("x-ratelimit-remaining-requests", "100000")
	rw.Header().Set("x-ratelimit-limit-tokens", "100000000")
	rw.Header().Set("x-ratelimit-remaining-tokens", "100000000")
	switch r.URL.Path {
	case "/v1/embeddings":
		l.embeddings(rw, r)
	case "/v1/chat/completions":
		l.chat(rw, r)
	default:
		openAIError(rw, http.StatusNotFound, "no route "+r.URL.Path)
	}
}

func openAIError(rw http.ResponseWriter, status int, msg string) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(status)
	_ = json.NewEncoder(rw).Encode(map[string]any{"error": map[string]any{"message": msg, "type": "invalid_request_error"}})
}

func (l *wsLLM) embeddings(rw http.ResponseWriter, r *http.Request) {
	var req struct {
		Model          string   `json:"model"`
		Input          []string `json:"input"`
		EncodingFormat string   `json:"encoding_format"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		openAIError(rw, http.StatusBadRequest, err.Error())
		return
	}
	l.record(wsLLMCall{Path: "embeddings", Model: req.Model})
	if req.Model != wsEmbeddingModel || len(req.Input) == 0 || (req.EncodingFormat != "" && req.EncodingFormat != "float") {
		openAIError(rw, http.StatusBadRequest, fmt.Sprintf("bad embeddings request: model %q, %d inputs, format %q", req.Model, len(req.Input), req.EncodingFormat))
		return
	}
	data := make([]map[string]any, len(req.Input))
	for i, in := range req.Input {
		if strings.TrimSpace(in) == "" {
			openAIError(rw, http.StatusBadRequest, fmt.Sprintf("input[%d] is empty", i))
			return
		}
		data[i] = map[string]any{"object": "embedding", "index": i, "embedding": bagOfWords(in)}
	}
	rw.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(rw).Encode(map[string]any{
		"object": "list", "data": data, "model": req.Model,
		"usage": map[string]int{"prompt_tokens": 10 * len(req.Input), "total_tokens": 10 * len(req.Input)},
	})
}

// bagOfWords hashes the words of s (lower-cased, 3+ letters) into a unit
// vector.
func bagOfWords(s string) []float32 {
	const dims = 128
	v := make([]float64, dims)
	for _, word := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if len(word) < 3 {
			continue
		}
		h := fnv.New32a()
		_, _ = h.Write([]byte(word))
		v[h.Sum32()%dims]++
	}
	var norm float64
	for _, x := range v {
		norm += x * x
	}
	out := make([]float32, dims)
	if norm == 0 {
		out[0] = 1
		return out
	}
	norm = math.Sqrt(norm)
	for i, x := range v {
		out[i] = float32(x / norm)
	}
	return out
}

type wsChatRequest struct {
	Model    string `json:"model"`
	Stream   bool   `json:"stream"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
	ResponseFormat *struct {
		Type       string `json:"type"`
		JSONSchema *struct {
			Name   string          `json:"name"`
			Schema json.RawMessage `json:"schema"`
			Strict bool            `json:"strict"`
		} `json:"json_schema"`
	} `json:"response_format"`
}

func (l *wsLLM) chat(rw http.ResponseWriter, r *http.Request) {
	var req wsChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		openAIError(rw, http.StatusBadRequest, err.Error())
		return
	}
	call := wsLLMCall{Path: "chat", Model: req.Model, Stream: req.Stream}
	if rf := req.ResponseFormat; rf != nil && rf.JSONSchema != nil {
		call.SchemaName, call.Strict = rf.JSONSchema.Name, rf.JSONSchema.Strict
	}
	l.record(call)
	switch {
	case req.Model != wsFastModel && req.Model != wsDefaultModel && req.Model != wsStrongModel:
		openAIError(rw, http.StatusNotFound, "unknown model "+req.Model)
		return
	case len(req.Messages) < 2 || req.Messages[0].Role != "system" || req.Messages[1].Role != "user":
		openAIError(rw, http.StatusBadRequest, "expected a system and a user message")
		return
	}

	content, err := cannedCompletion(&req)
	if err != nil {
		openAIError(rw, http.StatusBadRequest, err.Error())
		return
	}
	if req.Stream {
		streamCompletion(rw, req.Model, content, strings.Contains(req.Messages[1].Content, wsInterruptMarker))
		return
	}
	rw.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(rw).Encode(map[string]any{
		"id": "chatcmpl-fake", "object": "chat.completion", "created": time.Now().Unix(), "model": req.Model,
		"choices": []any{map[string]any{
			"index": 0, "finish_reason": "stop",
			"message": map[string]any{"role": "assistant", "content": content},
		}},
		"usage": map[string]int{"prompt_tokens": 100, "completion_tokens": 20, "total_tokens": 120},
	})
}

// wsInterruptMarker in a prompt makes a streamed completion fail after its
// first words, with the in-band error event OpenAI-compatible servers send.
const wsInterruptMarker = "interrupted"

// streamCompletion sends content as chat.completion.chunk events, a word at
// a time, then a usage chunk and [DONE].
func streamCompletion(rw http.ResponseWriter, model, content string, interrupt bool) {
	rw.Header().Set("Content-Type", "text/event-stream")
	flusher, _ := rw.(http.Flusher)
	send := func(v any) {
		b, _ := json.Marshal(v)
		fmt.Fprintf(rw, "data: %s\n\n", b)
		if flusher != nil {
			flusher.Flush()
		}
	}
	chunk := func(delta map[string]any, finish any) map[string]any {
		return map[string]any{
			"id": "chatcmpl-fake", "object": "chat.completion.chunk", "created": time.Now().Unix(), "model": model,
			"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}},
		}
	}
	send(chunk(map[string]any{"role": "assistant", "content": ""}, nil))
	words := strings.SplitAfter(content, " ")
	for i, w := range words {
		if interrupt && i == 2 {
			send(map[string]any{"error": map[string]any{"message": "upstream model overloaded", "type": "server_error"}})
			return
		}
		send(chunk(map[string]any{"content": w}, nil))
	}
	send(chunk(map[string]any{}, "stop"))
	send(map[string]any{
		"id": "chatcmpl-fake", "object": "chat.completion.chunk", "created": time.Now().Unix(), "model": model,
		"choices": []any{}, "usage": map[string]int{"prompt_tokens": 100, "completion_tokens": len(words), "total_tokens": 100 + len(words)},
	})
	fmt.Fprint(rw, "data: [DONE]\n\n")
}

// Canned completions, chosen by the task the system prompt sets.
const (
	wsDescribeReply  = "golang generics type parameters"
	wsSummaryReply   = "Go 1.18 introduced generics: functions and types with type parameters constrained by interfaces."
	wsAnswerReply    = "Go added generics in version 1.18 [1], and they are often compared with Rust traits [2]."
	wsSynthesisReply = "Generics arrived in Go 1.18 [1]; Rust has had them since 1.0 [2]."
)

// wsExpandedQueries are what the fake returns for deep search's query
// expansion; the duplicate must be dropped.
var wsExpandedQueries = []string{"golang type parameters tutorial", "go generic constraints", "golang type parameters tutorial"}

func cannedCompletion(req *wsChatRequest) (string, error) {
	system := req.Messages[0].Content
	if rf := req.ResponseFormat; rf != nil {
		if rf.Type != "json_schema" || rf.JSONSchema == nil {
			return "", fmt.Errorf("unsupported response_format %q", rf.Type)
		}
		var schema map[string]any
		if err := json.Unmarshal(rf.JSONSchema.Schema, &schema); err != nil {
			return "", fmt.Errorf("response_format schema: %w", err)
		}
		var out any
		switch rf.JSONSchema.Name {
		case "queries":
			out = map[string]any{"queries": wsExpandedQueries}
		case "search_output":
			out = searchOutputFor(schema)
		default:
			out = valueFor(schema, rf.JSONSchema.Name)
		}
		b, err := json.Marshal(out)
		return string(b), err
	}
	switch {
	case strings.HasPrefix(system, "Write one web search query"):
		return wsDescribeReply, nil
	case strings.HasPrefix(system, "Summarize"):
		return wsSummaryReply, nil
	case strings.HasPrefix(system, "Answer the user's question"):
		return wsAnswerReply, nil
	case strings.HasPrefix(system, "Research the user's query"):
		return wsSynthesisReply, nil
	}
	return "", fmt.Errorf("unrecognised task: %.60q", system)
}

// searchOutputFor fills {content, grounding}: content from its schema, and
// one grounding entry per field, citing the first two sources.
func searchOutputFor(schema map[string]any) map[string]any {
	props, _ := schema["properties"].(map[string]any)
	content, _ := props["content"].(map[string]any)
	grounding, _ := props["grounding"].(map[string]any)
	item, _ := grounding["items"].(map[string]any)
	itemProps, _ := item["properties"].(map[string]any)
	field, _ := itemProps["field"].(map[string]any)
	sources, _ := itemProps["sources"].(map[string]any)
	sourceItems, _ := sources["items"].(map[string]any)
	sourceNums, _ := sourceItems["enum"].([]any)
	cite := sourceNums[:min(2, len(sourceNums))]

	var entries []any
	fields, _ := field["enum"].([]any)
	for _, f := range fields {
		entries = append(entries, map[string]any{"field": f, "sources": cite, "confidence": "high"})
	}
	return map[string]any{"content": valueFor(content, "content"), "grounding": entries}
}

// valueFor builds a value matching a JSON schema.
func valueFor(schema map[string]any, name string) any {
	if enum, ok := schema["enum"].([]any); ok && len(enum) > 0 {
		return enum[0]
	}
	typ, _ := schema["type"].(string)
	if typ == "" && schema["properties"] != nil {
		typ = "object"
	}
	switch typ {
	case "object":
		props, _ := schema["properties"].(map[string]any)
		names := make([]string, 0, len(props))
		for n := range props {
			names = append(names, n)
		}
		sort.Strings(names)
		out := map[string]any{}
		for _, n := range names {
			p, _ := props[n].(map[string]any)
			out[n] = valueFor(p, n)
		}
		return out
	case "array":
		items, _ := schema["items"].(map[string]any)
		return []any{valueFor(items, name)}
	case "integer":
		return 2026
	case "number":
		return 1.18
	case "boolean":
		return true
	default:
		return "Go 1.18 (" + name + ")"
	}
}
