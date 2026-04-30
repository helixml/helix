// Package chat bridges the browser chat surface to a long-lived
// `claude` subprocess running in the helix-org server's working
// directory. The bridge owns one subprocess per Bridge instance —
// today there is exactly one Owner, so one global session is enough —
// and fans claude's stream-json stdout out to any number of SSE
// listeners as ready-to-swap HTML fragments. User input arrives via
// HTTP POST and is written to claude's stdin as a stream-json frame.
//
// The subprocess runs in the server's cwd so the conversation is
// shared with terminal `helix-org chat` invoked from the same
// directory: claude's per-cwd session store handles persistence, and
// the bridge resumes the most recent session by ID at startup.
package chat

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/helixml/helix-org/prompts"
)

// Bridge owns the chat subprocess and the SSE fan-out. Construct one
// per server, mount StreamHandler() and SendHandler() under /ui/chat/.
type Bridge struct {
	claudeBin string
	cwd       string
	mcpURL    string
	logger    *slog.Logger
	// prompts is the optional MCP-prompt registry. When set, SendHandler
	// intercepts inputs that start with `/<name>` and replaces them with
	// the prompt's rendered seed text before forwarding to claude.
	// Reason: claude in stream-json mode does not process slash commands —
	// it just wraps the raw text as a user message — so MCP prompts
	// (which Claude Code's interactive TUI handles natively) are dead on
	// arrival here unless we expand them server-side.
	prompts *prompts.Registry

	mu             sync.Mutex // guards sess, forceNew, resumeSID, freshFromPath
	sess           *session
	forceNew       bool   // next start() spawns claude with no --resume
	overrideResume string // next start() resumes this sid; "" = use latest
	// freshFromPath is the path of the latest jsonl at the moment the
	// user clicked "New chat". Until a *different* file becomes
	// latest (i.e. the new claude process has produced its own jsonl),
	// the UI suppresses history rendering. Empty means "no New chat
	// pending — render normally". Path-based, not time-based, because
	// a sibling claude process (e.g. dev's Claude Code) may keep
	// updating its own jsonl after the click and would otherwise look
	// like fresh content.
	freshFromPath string
}

// CWD returns the working directory the bridge launches `claude` in.
// The UI uses it to read claude's per-cwd session jsonls for history
// rendering and the Recents list.
func (b *Bridge) CWD() string { return b.cwd }

// New returns a Bridge configured to spawn `claude` from claudeBin in
// the given cwd, wired to a single MCP server at mcpURL named "helix".
// Sessions are spawned lazily on the first request.
func New(claudeBin, cwd, mcpURL string, logger *slog.Logger) *Bridge {
	if logger == nil {
		logger = slog.Default()
	}
	return &Bridge{claudeBin: claudeBin, cwd: cwd, mcpURL: mcpURL, logger: logger}
}

// WithPrompts attaches a prompts.Registry so the bridge can resolve
// `/<name>` inputs in the chat textarea into MCP-prompt seed text
// before handing the message to claude. Returns the same Bridge so the
// call can be chained off New. nil is equivalent to no prompts —
// slash commands fall through to claude unchanged.
func (b *Bridge) WithPrompts(reg *prompts.Registry) *Bridge {
	b.prompts = reg
	return b
}

// session is one running claude subprocess plus its SSE listeners. It
// is created once and reused for the life of the process; if the
// subprocess exits, the next request creates a fresh session that
// resumes the same claude conversation by ID.
type session struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser

	mu        sync.Mutex
	listeners map[chan string]struct{}
	dead      chan struct{}
}

// ensure returns the live session, lazily starting it if there isn't
// one or if the previous one exited. ctx is the request context — only
// used to bound startup, not to bound the subprocess lifetime.
func (b *Bridge) ensure(ctx context.Context) (*session, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.sess != nil {
		select {
		case <-b.sess.dead:
			b.sess = nil
		default:
			return b.sess, nil
		}
	}
	s, err := b.start(ctx)
	if err != nil {
		return nil, err
	}
	b.sess = s
	return s, nil
}

func (b *Bridge) start(ctx context.Context) (*session, error) {
	mcpJSON, err := json.Marshal(map[string]any{
		"mcpServers": map[string]any{
			"helix": map[string]string{"type": "http", "url": b.mcpURL},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal mcp config: %w", err)
	}
	args := []string{
		"-p",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--verbose",
		"--permission-mode", "bypassPermissions",
		"--strict-mcp-config",
		"--mcp-config", string(mcpJSON),
	}
	switch {
	case b.forceNew:
		// no --resume — fresh session
	case b.overrideResume != "":
		args = append(args, "--resume", b.overrideResume)
	default:
		if sid := latestClaudeSessionID(b.cwd); sid != "" {
			args = append(args, "--resume", sid)
		}
	}
	// ctx is only used for cancellation during start (e.g. the request
	// going away mid-spawn). We deliberately do NOT bind the subprocess
	// to ctx — the subprocess outlives the request.
	cmd := exec.CommandContext(context.Background(), b.claudeBin, args...) //nolint:gosec // claudeBin is operator-supplied
	cmd.Dir = b.cwd
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start claude: %w", err)
	}
	// Intents consumed only on successful spawn — if start() fails we
	// preserve "user wanted X" for the retry.
	b.forceNew = false
	b.overrideResume = ""

	s := &session{
		cmd:       cmd,
		stdin:     stdin,
		listeners: make(map[chan string]struct{}),
		dead:      make(chan struct{}),
	}

	go b.readLoop(s, stdout)
	go b.drainStderr(stderr)
	go func() {
		_ = cmd.Wait()
		close(s.dead)
		b.logger.Info("chat session exited", "pid", cmd.Process.Pid)
	}()

	b.logger.Info("chat session started", "pid", cmd.Process.Pid, "cwd", b.cwd)
	_ = ctx // explicit: ctx not retained; subprocess outlives the request
	return s, nil
}

func (b *Bridge) readLoop(s *session, r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var ev streamEvent
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			b.logger.Warn("chat parse stream-json", "err", err, "line", oneLine(scanner.Text(), 200))
			continue
		}
		for _, frag := range renderFragments(ev) {
			s.broadcast(frag)
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		b.logger.Warn("chat scanner error", "err", err)
	}
}

func (b *Bridge) drainStderr(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		b.logger.Warn("chat claude stderr", "line", scanner.Text())
	}
}

func (s *session) broadcast(frag string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for ch := range s.listeners {
		select {
		case ch <- frag:
		default:
			// Drop on slow listener — better than blocking the read
			// loop on one stuck browser tab.
		}
	}
}

func (s *session) subscribe() chan string {
	ch := make(chan string, 64)
	s.mu.Lock()
	s.listeners[ch] = struct{}{}
	s.mu.Unlock()
	return ch
}

func (s *session) unsubscribe(ch chan string) {
	s.mu.Lock()
	delete(s.listeners, ch)
	s.mu.Unlock()
}

// send writes one user message frame to claude's stdin in the
// stream-json format claude expects with --input-format stream-json.
func (s *session) send(text string) error {
	frame := map[string]any{
		"type": "user",
		"message": map[string]any{
			"role":    "user",
			"content": text,
		},
	}
	data, err := json.Marshal(frame)
	if err != nil {
		return fmt.Errorf("marshal user frame: %w", err)
	}
	data = append(data, '\n')
	if _, err := s.stdin.Write(data); err != nil {
		return fmt.Errorf("write user frame: %w", err)
	}
	return nil
}

// StreamHandler serves the SSE channel at GET /ui/chat/stream. Each
// browser tab opens one of these long-lived connections and receives
// pre-rendered HTML fragments as `data: …` lines, which htmx swaps
// straight into #chat-log.
func (b *Bridge) StreamHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		s, err := b.ensure(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		ch := s.subscribe()
		defer s.unsubscribe(ch)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		ping := time.NewTicker(15 * time.Second)
		defer ping.Stop()

		for {
			select {
			case frag := <-ch:
				// SSE forbids raw `\n` inside a `data:` line — `\n\n`
				// terminates the event. The spec's own answer is to
				// split multi-line payloads across repeated `data:`
				// lines, which the browser's EventSource rejoins with
				// `\n`. Markdown-rendered fragments contain real
				// newlines inside `<pre>` blocks (fenced code), so we
				// must preserve them; flattening to spaces collapsed
				// fenced markdown into a single visual line.
				_, _ = fmt.Fprint(w, "event: message\n")
				for _, line := range strings.Split(frag, "\n") {
					_, _ = fmt.Fprintf(w, "data: %s\n", line)
				}
				_, _ = fmt.Fprint(w, "\n")
				flusher.Flush()
			case <-ping.C:
				_, _ = fmt.Fprint(w, ": keepalive\n\n")
				flusher.Flush()
			case <-r.Context().Done():
				return
			case <-s.dead:
				return
			}
		}
	})
}

// NewHandler wipes the active session at POST /ui/chat/new. Closing
// stdin lets the subprocess exit cleanly (it might still finish a
// turn that's already in flight, which is fine — there are no
// listeners on the old session). The HX-Redirect header tells htmx to
// navigate the browser to /ui/, which re-renders with an empty
// #chat-log and lazily spawns a fresh `claude` (no --resume) on the
// next request.
func (b *Bridge) NewHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b.newSession()
		w.Header().Set("HX-Redirect", "/ui/")
		w.WriteHeader(http.StatusOK)
	})
}

// newSession kills the active session (if any) and flags the next
// ensure() to spawn claude with no --resume. Idempotent: with no
// active session, sets the flag and returns. Captures the path of
// the current latest jsonl so the UI can tell when a *different*
// file becomes latest — the signal that the new claude has written
// its first event and the chat page can stop suppressing history.
func (b *Bridge) newSession() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.forceNew = true
	b.overrideResume = ""
	b.freshFromPath = newestJSONL(claudeProjectsDir(b.cwd))
	if b.sess != nil {
		_ = b.sess.stdin.Close()
		b.sess = nil
		b.logger.Info("chat session reset by user", "marker", b.freshFromPath)
	}
}

// HistoryStartsFresh reports whether the chat page should render
// nothing as initial history because the user clicked New chat and
// no different jsonl has yet become the latest in the cwd. Returns
// false when no New chat has happened, or when a different file
// (the freshly-spawned claude's new jsonl) is now the latest —
// meaning the new conversation has begun and its history is safe
// to render.
func (b *Bridge) HistoryStartsFresh() bool {
	b.mu.Lock()
	marker := b.freshFromPath
	b.mu.Unlock()
	if marker == "" {
		return false
	}
	current := newestJSONL(claudeProjectsDir(b.cwd))
	if current == "" {
		return true
	}
	if current == marker {
		return true
	}
	// A different file is now latest — clear the marker so we don't
	// keep paying for stat() on every page load.
	b.mu.Lock()
	if b.freshFromPath == marker {
		b.freshFromPath = ""
	}
	b.mu.Unlock()
	return false
}

// SwitchHandler kills the active session and flags the next ensure()
// to resume the requested session ID. Form field "sid" carries the
// target. HX-Redirect bounces the browser to /ui/?sid=<sid> so the
// chat handler renders that session's history.
func (b *Bridge) SwitchHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		sid := strings.TrimSpace(r.PostFormValue("sid"))
		if sid == "" {
			http.Error(w, "sid required", http.StatusBadRequest)
			return
		}
		b.switchSession(sid)
		w.Header().Set("HX-Redirect", "/ui/?sid="+sid)
		w.WriteHeader(http.StatusOK)
	})
}

// switchSession kills the active session and flags the next ensure()
// to spawn claude with --resume <sid>.
func (b *Bridge) switchSession(sid string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.forceNew = false
	b.overrideResume = sid
	if b.sess != nil {
		_ = b.sess.stdin.Close()
		b.sess = nil
		b.logger.Info("chat session switched", "sid", sid)
	}
}

// SendHandler accepts a user message at POST /ui/chat/send. The form
// posts with the field name "message"; the response is the rendered
// user-bubble HTML which htmx swaps into #chat-log immediately so the
// user sees their message before the assistant streams its reply.
// Assistant chunks land on the SSE channel asynchronously.
//
// The body is capped at 64 KiB — chat messages are short, and a hard
// cap protects the form parser from a hostile client streaming
// unbounded form data.
func (b *Bridge) SendHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		msg := strings.TrimSpace(r.PostFormValue("message"))
		if msg == "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		// User-facing bubble shows the original input — if they typed
		// `/role marketing director`, that's what they expect to see in
		// their own conversation, not the expanded interview text.
		bubble := msg
		if expanded, ok := b.expandSlashCommand(r.Context(), msg); ok {
			msg = expanded
		}
		s, err := b.ensure(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := s.send(msg); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, renderUserBubble(bubble))
	})
}

// expandSlashCommand intercepts inputs of the form `/<name> <rest>` and
// replaces them with the rendered text of an MCP prompt registered
// under <name>. Returns (expanded, true) on a hit, ("", false) if the
// input isn't a slash command, the registry isn't attached, or the
// prompt name isn't known. The fall-through case lets the message reach
// claude unchanged so that anything we don't recognise (e.g. claude's
// own built-ins like `/clear`) keeps working as far as it ever did.
//
// Argument convention: this is the smallest thing that works for
// today's prompts — it threads any text after the command name into
// the prompt's *first declared argument*. That's enough for `/role`
// (one optional `hint` arg) and any other single-arg prompt; multi-arg
// prompts will need real parsing when we have one.
func (b *Bridge) expandSlashCommand(ctx context.Context, msg string) (string, bool) {
	if b.prompts == nil || !strings.HasPrefix(msg, "/") {
		return "", false
	}
	name, rest, _ := strings.Cut(msg[1:], " ")
	if name == "" {
		return "", false
	}
	p, err := b.prompts.Get(prompts.Name(name))
	if err != nil {
		return "", false
	}
	args := map[string]string{}
	rest = strings.TrimSpace(rest)
	if rest != "" {
		if a := p.Arguments(); len(a) > 0 {
			args[a[0].Name] = rest
		}
	}
	rendered, err := p.Render(ctx, args)
	if err != nil {
		b.logger.Info("chat slash command render failed", "name", name, "err", err)
		return "", false
	}
	parts := make([]string, 0, len(rendered))
	for _, m := range rendered {
		parts = append(parts, m.Text)
	}
	return strings.Join(parts, "\n\n"), true
}

// CommandsHandler renders the slash-command typeahead at POST
// /ui/chat/commands. The textarea fires this on keyup; the body is the
// current value, keyed `message`. We respond with a (possibly empty)
// HTML fragment that htmx swaps into #slash-suggestions: an empty
// response hides the dropdown, a list of buttons exposes each matching
// prompt with its title and description.
//
// We don't filter by which Worker holds which grant here — the chat
// surface today is the owner's, and the SendHandler intercepts and
// expands locally without going through the per-worker MCP visibility
// pipeline. If we open the chat to non-owner Workers later, this
// endpoint should call into the same gating logic the per-worker MCP
// server uses.
func (b *Bridge) CommandsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if b.prompts == nil {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
		if err := r.ParseForm(); err != nil {
			return
		}
		msg := r.PostFormValue("message")
		if !strings.HasPrefix(msg, "/") {
			return
		}
		// Match against the first whitespace-delimited token (minus
		// leading slash). Once the user types past `/<name> ` they're
		// composing an argument, not picking a command — keep the
		// chosen prompt highlighted but stop filtering further.
		token, _, _ := strings.Cut(msg[1:], " ")
		prefix := strings.ToLower(token)

		all := b.prompts.All()
		matches := make([]prompts.Prompt, 0, len(all))
		for _, p := range all {
			if strings.HasPrefix(strings.ToLower(string(p.Name())), prefix) {
				matches = append(matches, p)
			}
		}
		sort.Slice(matches, func(i, j int) bool { return matches[i].Name() < matches[j].Name() })
		for _, p := range matches {
			_, _ = fmt.Fprint(w, renderSlashSuggestion(p))
		}
	})
}

// rendering helpers and stream-json shapes live in render.go,
// shared with the historical-replay reader in sessions.go.
