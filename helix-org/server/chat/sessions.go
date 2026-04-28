package chat

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SessionInfo is one row in the Recents list — a per-cwd claude
// session jsonl summarized for the sidebar.
type SessionInfo struct {
	SessionID string    // sid extracted from the first line
	Title     string    // best-effort title (custom-title, else first user prompt)
	ModTime   time.Time // file mtime, used for ordering
}

// ListSessions returns the claude session jsonls under
// ~/.claude/projects/<cwd-as-hyphens>/ ordered most-recent first.
// Sessions whose first line cannot be decoded are skipped silently —
// a corrupt log shouldn't break the sidebar render. Files containing
// no user-visible turns (only meta events) are also skipped.
func ListSessions(cwd string) []SessionInfo {
	dir := claudeProjectsDir(cwd)
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []SessionInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		path := filepath.Join(dir, e.Name())
		s, ok := summarize(path, info.ModTime())
		if !ok {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ModTime.After(out[j].ModTime)
	})
	return out
}

// ReadHistory streams the claude session jsonl for sid (or the latest
// in cwd if sid is empty) and returns rendered HTML fragments — same
// format the live SSE bridge emits — so the chat page can mount the
// existing conversation on load.
//
// Lines that fail to decode or are meta events (custom-title,
// attachment, file-history-snapshot, system, etc.) are silently
// skipped. Returns nil if the session can't be located.
func ReadHistory(cwd, sid string) []string {
	dir := claudeProjectsDir(cwd)
	if dir == "" {
		return nil
	}
	path := ""
	if sid != "" {
		candidate := filepath.Join(dir, sid+".jsonl")
		if _, err := os.Stat(candidate); err == nil {
			path = candidate
		}
	}
	if path == "" {
		path = newestJSONL(dir)
	}
	if path == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var out []string
	for scanner.Scan() {
		var ev streamEvent
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			continue
		}
		out = append(out, renderFragments(ev)...)
	}
	return out
}

// summarize reads the head of a session jsonl just deeply enough to
// extract sid + a display title. ok=false means this file should be
// skipped (no usable sid, or no human-visible turn). The scanner
// stops once both fields are populated to keep the cost ~O(first
// user message).
func summarize(path string, mtime time.Time) (SessionInfo, bool) {
	f, err := os.Open(path) //nolint:gosec // path is built from claudeProjectsDir + a known suffix
	if err != nil {
		return SessionInfo{}, false
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var (
		sid   string
		title string
	)
	for scanner.Scan() {
		var head struct {
			SessionID   string          `json:"sessionId"`
			Type        string          `json:"type"`
			CustomTitle string          `json:"customTitle,omitempty"`
			Message     json.RawMessage `json:"message,omitempty"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &head); err != nil {
			continue
		}
		if sid == "" && head.SessionID != "" {
			sid = head.SessionID
		}
		switch head.Type {
		case "custom-title":
			if t := strings.TrimSpace(head.CustomTitle); t != "" && title == "" {
				title = t
			}
		case "user":
			if title == "" {
				if t := firstUserText(head.Message); t != "" {
					title = t
				}
			}
		}
		if sid != "" && title != "" {
			break
		}
	}
	if sid == "" {
		return SessionInfo{}, false
	}
	if title == "" {
		// Skip jsonls with no user-visible content — they're
		// almost always transient bookkeeping (custom-title only,
		// abandoned spawns, etc.).
		return SessionInfo{}, false
	}
	return SessionInfo{SessionID: sid, Title: shortenTitle(title), ModTime: mtime}, true
}

// firstUserText extracts a user-visible string from a session jsonl
// "user" event's message envelope. It transparently handles the two
// shapes claude writes: plain string content, or an array of
// segments where the first text segment is the prompt. Tool-result
// segments and CLI metadata blocks (anything starting with "<") are
// skipped — those are scaffolding, not user prompts.
func firstUserText(messageJSON json.RawMessage) string {
	if len(messageJSON) == 0 {
		return ""
	}
	var msg messagePayload
	if err := json.Unmarshal(messageJSON, &msg); err != nil {
		return ""
	}
	var asString string
	if err := json.Unmarshal(msg.Content, &asString); err == nil {
		t := strings.TrimSpace(asString)
		if isMetaPrompt(t) {
			return ""
		}
		return t
	}
	var segs []contentSegment
	if err := json.Unmarshal(msg.Content, &segs); err != nil {
		return ""
	}
	for _, seg := range segs {
		if seg.Type != "text" {
			continue
		}
		t := strings.TrimSpace(seg.Text)
		if t == "" || isMetaPrompt(t) {
			continue
		}
		return t
	}
	return ""
}

// isMetaPrompt reports whether the given user-message body is a CLI
// metadata block rather than a real prompt — e.g. /clear, /reload,
// the local-command-caveat preamble. Recents should ignore these
// when picking a title.
func isMetaPrompt(s string) bool {
	if s == "" {
		return true
	}
	if strings.HasPrefix(s, "<command-name>") || strings.HasPrefix(s, "<local-command-") || strings.HasPrefix(s, "<system-reminder>") {
		return true
	}
	return false
}

func shortenTitle(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	const max = 60
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}

func claudeProjectsDir(cwd string) string {
	if cwd == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "projects", strings.ReplaceAll(cwd, "/", "-"))
}

func newestJSONL(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var (
		newestPath string
		newestTime time.Time
	)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(newestTime) {
			newestTime = info.ModTime()
			newestPath = filepath.Join(dir, e.Name())
		}
	}
	return newestPath
}

// latestClaudeSessionID returns the sid of the most recently
// modified .jsonl in claude's per-cwd session store, or "" if none.
// Mirrors cmd/helix-org/chat.go's resolver. Used by the bridge to
// decide what to pass to claude --resume on lazy spawn.
func latestClaudeSessionID(cwd string) string {
	dir := claudeProjectsDir(cwd)
	if dir == "" {
		return ""
	}
	path := newestJSONL(dir)
	if path == "" {
		return ""
	}
	f, err := os.Open(path) //nolint:gosec // path is built from a known prefix and a directory entry name
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	if !scanner.Scan() {
		return ""
	}
	var record struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
		return ""
	}
	return record.SessionID
}
