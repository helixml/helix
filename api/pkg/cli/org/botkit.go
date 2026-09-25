package org

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/config"
	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/spf13/cobra"
)

// Shared helpers for building, driving and debugging bots from the CLI: bot
// documents for spec diffing, turns read the way a customer sees them, and the
// sandbox behind a session.

// botMap is a bot as a JSON-keyed map (REST field names), for diffing against
// a spec and for export.
func botMap(b orgapi.BotDTO) map[string]any {
	m := map[string]any{}
	bts, _ := json.Marshal(b)
	_ = json.Unmarshal(bts, &m)
	return m
}

// decodeStrict round-trips v (a spec map) into a typed request, reporting
// unknown fields and wrong types.
func decodeStrict(v any, out any) error {
	bts, err := json.Marshal(v)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(bts))
	dec.DisallowUnknownFields()
	return dec.Decode(out)
}

// callCtx bounds one API call; makeRequest honours the deadline instead of
// its 10 s default.
func callCtx(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, d)
}

// ---------------------------------------------------------------------------
// Turns

// entry is one element of an interaction's response_entries.
type entry struct {
	Type       string `json:"type"`
	Content    string `json:"content"`
	MessageID  string `json:"message_id"`
	ToolName   string `json:"tool_name"`
	ToolStatus string `json:"tool_status"`
}

func entriesOf(i *types.Interaction) []entry {
	var es []entry
	if i != nil && len(i.ResponseEntries) > 0 {
		_ = json.Unmarshal(i.ResponseEntries, &es)
	}
	return es
}

var thinkingRE = regexp.MustCompile(`(?s)<thinking>.*?</thinking>`)

func stripThinking(s string) string { return strings.TrimSpace(thinkingRE.ReplaceAllString(s, "")) }

// finalText is the bot's closing message as a customer sees it: every text
// entry after the last tool call, thinking removed. Harnesses often split one
// answer across several text entries; the blocking /sessions/chat reply is the
// whole turn (tool calls, page snapshots) and must not be shown to customers.
func finalText(i *types.Interaction) string {
	es := entriesOf(i)
	if len(es) == 0 {
		if i == nil {
			return ""
		}
		return stripThinking(i.ResponseMessage)
	}
	lastTool := -1
	for n, e := range es {
		if e.Type == "tool_call" {
			lastTool = n
		}
	}
	var parts []string
	for _, e := range es[lastTool+1:] {
		if t := stripThinking(e.Content); e.Type == "text" && t != "" {
			parts = append(parts, t)
		}
	}
	if len(parts) == 0 { // ended on a tool call: last non-empty text anywhere
		for n := len(es) - 1; n >= 0; n-- {
			if t := stripThinking(es[n].Content); es[n].Type == "text" && t != "" {
				return t
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

func toolCalls(i *types.Interaction) []entry {
	var out []entry
	for _, e := range entriesOf(i) {
		if e.Type == "tool_call" {
			out = append(out, e)
		}
	}
	return out
}

// toolSummary renders "name×n, …" most-used first.
func toolSummary(calls []entry) string {
	counts := map[string]int{}
	for _, c := range calls {
		counts[c.ToolName]++
	}
	names := make([]string, 0, len(counts))
	for n := range counts {
		names = append(names, n)
	}
	sort.Slice(names, func(a, b int) bool {
		if counts[names[a]] != counts[names[b]] {
			return counts[names[a]] > counts[names[b]]
		}
		return names[a] < names[b]
	})
	parts := make([]string, 0, len(names))
	for _, n := range names {
		parts = append(parts, fmt.Sprintf("%s×%d", truncate(n, 60), counts[n]))
	}
	return strings.Join(parts, ", ")
}

// interactions lists a session's newest turns (the first page; pages are
// 0-based, so page 1 would be the second page).
func interactions(ctx context.Context, c *client.HelixClient, sessionID string, perPage int, order string) ([]*types.Interaction, error) {
	page, err := c.ListInteractions(ctx, sessionID, &client.InteractionFilter{Page: 0, PerPage: perPage, Order: order})
	if err != nil {
		return nil, err
	}
	return page.Interactions, nil
}

// lastInteraction returns the newest turn. Ids are ULIDs (time ordered);
// `created` can tie across turns of the same session, so sort by id.
func lastInteraction(ctx context.Context, c *client.HelixClient, sessionID string) (*types.Interaction, error) {
	xs, err := interactions(ctx, c, sessionID, 5, "desc")
	if err != nil || len(xs) == 0 {
		return nil, err
	}
	sort.Slice(xs, func(a, b int) bool { return xs[a].ID < xs[b].ID })
	return xs[len(xs)-1], nil
}

// waitTurn polls until a turn newer than prevID has settled. The interaction
// list lags the blocking chat reply by a moment.
func waitTurn(ctx context.Context, c *client.HelixClient, sessionID, prevID string, maxWait time.Duration) (*types.Interaction, error) {
	deadline := time.Now().Add(maxWait)
	var last *types.Interaction
	for time.Now().Before(deadline) {
		i, err := lastInteraction(ctx, c, sessionID)
		if err != nil {
			return nil, err
		}
		if i != nil && i.ID != prevID {
			last = i
			if i.State == types.InteractionStateComplete || i.State == types.InteractionStateError {
				return i, nil
			}
		}
		select {
		case <-ctx.Done():
			return last, ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return last, nil
}

// turnResult is one chat turn: what the customer saw plus how it got there.
type turnResult struct {
	SessionID   string             `json:"session_id"`
	Reply       string             `json:"reply"`
	Seconds     float64            `json:"seconds"`
	Interaction *types.Interaction `json:"-"`
	ToolCalls   []entry            `json:"-"`
	Raw         string             `json:"-"`
}

// attachmentPart builds an inline data-URL message part (the only attachment
// form /sessions/chat accepts). Images → image_url, everything else → file.
func attachmentPart(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	mt := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if mt == "" {
		mt = http.DetectContentType(data)
	}
	if i := strings.Index(mt, ";"); i > 0 {
		mt = mt[:i]
	}
	dataURL := "data:" + mt + ";base64," + base64.StdEncoding.EncodeToString(data)
	if strings.HasPrefix(mt, "image/") {
		return map[string]any{"type": "image_url", "image_url": map[string]any{"url": dataURL}}, nil
	}
	return map[string]any{"type": "file", "file": map[string]any{"filename": filepath.Base(path), "file_data": dataURL}}, nil
}

// sendTurn posts one blocking /sessions/chat turn. sessionID may be empty with
// an app key (the gateway then creates a bot instance). key overrides the
// caller's key; app keys cannot read interactions, so for them Reply is cut
// from the raw blob best-effort.
func sendTurn(ctx context.Context, c *client.HelixClient, sessionID, message string, attach []string, key string, timeout time.Duration) (*turnResult, error) {
	var parts []any
	if len(attach) == 0 {
		parts = []any{message}
	} else {
		parts = []any{map[string]any{"type": "text", "text": message}}
		for _, p := range attach {
			part, err := attachmentPart(p)
			if err != nil {
				return nil, err
			}
			parts = append(parts, part)
		}
	}
	req := &types.SessionChatRequest{
		SessionID: sessionID,
		Type:      types.SessionTypeText,
		Stream:    false,
		Messages: []*types.Message{{
			Role:    "user",
			Content: types.MessageContent{ContentType: types.MessageContentTypeText, Parts: parts},
		}},
	}
	prevID := ""
	if sessionID != "" && key == "" {
		if last, err := lastInteraction(ctx, c, sessionID); err == nil && last != nil {
			prevID = last.ID
		}
	}
	chatClient := c
	if key != "" {
		cfg, err := config.LoadCliConfig()
		if err != nil {
			return nil, err
		}
		if chatClient, err = client.NewClient(cfg.URL, key, cfg.TLSSkipVerify); err != nil {
			return nil, err
		}
	}
	t0 := time.Now()
	chatCtx, cancel := callCtx(ctx, timeout)
	defer cancel()
	resp, err := chatClient.ChatSessionCompletion(chatCtx, req)
	if err != nil {
		return nil, err
	}
	res := &turnResult{SessionID: resp.ID, Seconds: time.Since(t0).Seconds()}
	if len(resp.Choices) > 0 {
		res.Raw = resp.Choices[0].Message.Content
	}
	if key == "" && res.SessionID != "" {
		if i, err := waitTurn(ctx, c, res.SessionID, prevID, 60*time.Second); err == nil && i != nil {
			res.Interaction = i
			res.ToolCalls = toolCalls(i)
			res.Reply = finalText(i)
		}
	}
	if res.Reply == "" {
		res.Reply = lastSegment(res.Raw)
	}
	return res, nil
}

// lastSegment cuts the closing message out of a raw turn blob (app-key and
// webhook callers): drop thinking and everything up to the last tool block.
func lastSegment(raw string) string {
	text := stripThinking(raw)
	const marker = "**Tool Call: "
	i := strings.LastIndex(text, marker)
	if i < 0 {
		return text
	}
	tail := text[i:]
	parts := regexp.MustCompile(`\n\s*\n`).Split(tail, -1)
	if len(parts) > 1 {
		return strings.TrimSpace(parts[len(parts)-1])
	}
	return strings.TrimSpace(tail)
}

// ---------------------------------------------------------------------------
// Sandboxes behind sessions

// sandboxForSession finds the sandbox row backing a session (bot main sessions
// and instances both have one). wait > 0 polls until it is running.
func sandboxForSession(ctx context.Context, c *client.HelixClient, orgID, sessionID string, wait time.Duration) (*types.Sandbox, error) {
	deadline := time.Now().Add(wait)
	for {
		resp, err := c.ListSandboxes(ctx, orgID, nil)
		if err != nil {
			return nil, err
		}
		for _, s := range resp.Sandboxes {
			if s.SessionID == sessionID && (wait == 0 || s.Status == types.SandboxStatusRunning) {
				return s, nil
			}
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("no running sandbox for session %s (stopped or idle-terminated sessions have none)", sessionID)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
}

// execInSession runs a bash script in the session's sandbox. The call waits
// for the command, so its deadline is the command timeout plus headroom.
func execInSession(ctx context.Context, c *client.HelixClient, orgID, sessionID, script string, timeout int) (*types.SandboxCommand, error) {
	sb, err := sandboxForSession(ctx, c, orgID, sessionID, 0)
	if err != nil {
		return nil, err
	}
	wait := timeout
	if wait <= 0 {
		wait = 60 // the server's default command timeout
	}
	runCtx, cancel := callCtx(ctx, time.Duration(wait+30)*time.Second)
	defer cancel()
	return c.RunSandboxCommand(runCtx, orgID, sb.ID, &types.RunSandboxCommandRequest{
		Cmd: "bash", Args: []string{"-lc", script}, TimeoutSeconds: timeout,
	})
}

// waitSandbox blocks until the session's sandbox is running, and fails fast
// when it reports failed (e.g. Hydra refusing to start: disk full, quota) —
// otherwise the first chat turn waits the full 5-minute readiness timeout and
// only says "external agent not ready".
func waitSandbox(ctx context.Context, c *client.HelixClient, orgID, sessionID string, maxWait time.Duration) error {
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		resp, err := c.ListSandboxes(ctx, orgID, nil)
		if err != nil {
			return err
		}
		for _, s := range resp.Sandboxes {
			if s.SessionID != sessionID {
				continue
			}
			switch s.Status {
			case types.SandboxStatusRunning:
				return nil
			case types.SandboxStatusFailed:
				return fmt.Errorf("sandbox %s failed to start: %s (the API log has the cause: `Failed to start bot instance sandbox` / hydra errors, e.g. disk space or desktop quota)", s.ID, s.StatusMessage)
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return fmt.Errorf("sandbox for %s not running after %s", sessionID, maxWait)
}

// silenceUsage stops cobra printing the whole usage text (and a second copy of
// the error — Execute prints it) on runtime errors.
func silenceUsage(cmd *cobra.Command) *cobra.Command {
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	for _, c := range cmd.Commands() {
		silenceUsage(c)
	}
	return cmd
}
