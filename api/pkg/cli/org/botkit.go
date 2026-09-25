package org

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/spf13/cobra"
)

// Shared helpers for building, driving and debugging bots from the CLI:
// bot documents that tolerate fields newer than this binary's DTOs, turns read
// the way a customer sees them, and the sandbox behind a session.

// botDoc is a bot as returned by GET /orgs/{org}/bots/{id}, kept as a raw map
// so fields newer than orgapi.BotDTO (e.g. instance_profile) round-trip.
type botDoc map[string]any

func (b botDoc) str(k string) string {
	if v, ok := b[k].(string); ok {
		return v
	}
	return ""
}

func (b botDoc) strs(k string) []string {
	raw, _ := b[k].([]any)
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// instanceProfile mirrors types.BotInstanceProfile (PR #3293) locally.
type instanceProfile struct {
	SandboxRuntime string   `json:"sandbox_runtime,omitempty"`
	MCPServers     []string `json:"mcp_servers"`
	Tools          []string `json:"tools"`
	HelixSkills    bool     `json:"helix_skills,omitempty"`
}

func (b botDoc) profile() instanceProfile {
	var p instanceProfile
	if raw, ok := b["instance_profile"]; ok && raw != nil {
		bts, _ := json.Marshal(raw)
		_ = json.Unmarshal(bts, &p)
	}
	return p
}

func (c *httpClient) getBot(ctx context.Context, orgID, botID string) (botDoc, error) {
	var detail struct {
		Bot         botDoc `json:"bot"`
		LegacyAppID string `json:"legacy_app_id"`
		ProjectID   string `json:"project_id"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/orgs/"+orgID+"/bots/"+botID, nil, &detail, 30*time.Second); err != nil {
		return nil, err
	}
	if detail.Bot == nil {
		return nil, fmt.Errorf("bot %s: empty response", botID)
	}
	if detail.Bot.str("legacy_app_id") == "" && detail.LegacyAppID != "" {
		detail.Bot["legacy_app_id"] = detail.LegacyAppID
	}
	if detail.Bot.str("project_id") == "" && detail.ProjectID != "" {
		detail.Bot["project_id"] = detail.ProjectID
	}
	return detail.Bot, nil
}

// botInstance is one element of GET /orgs/{org}/bots/{id}/instances.
type botInstance struct {
	SessionID      string `json:"session_id"`
	BotID          string `json:"bot_id"`
	Name           string `json:"name"`
	SandboxRuntime string `json:"sandbox_runtime"`
	SandboxStatus  string `json:"sandbox_status"`
	Owner          string `json:"owner"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

func (c *httpClient) listInstances(ctx context.Context, orgID, botID string) ([]botInstance, error) {
	var out []botInstance
	err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/orgs/%s/bots/%s/instances", orgID, botID), nil, &out, 30*time.Second)
	return out, err
}

func (c *httpClient) createInstance(ctx context.Context, orgID, botID, name, runtime, message string) (*botInstance, error) {
	body := map[string]any{}
	if name != "" {
		body["name"] = name
	}
	if runtime != "" {
		body["sandbox_runtime"] = runtime
	}
	if message != "" {
		body["message"] = message
	}
	var out botInstance
	err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/orgs/%s/bots/%s/instances", orgID, botID), body, &out, 60*time.Second)
	return &out, err
}

func (c *httpClient) deleteInstance(ctx context.Context, orgID, botID, sessionID string) error {
	return c.doJSON(ctx, http.MethodDelete, fmt.Sprintf("/orgs/%s/bots/%s/instances/%s", orgID, botID, sessionID), nil, nil, 60*time.Second)
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

// interactions lists a session's turns. Pages are 0-based: page=1 is the second page.
func (c *httpClient) interactions(ctx context.Context, sessionID string, perPage int, order string) ([]*types.Interaction, error) {
	var out types.PaginatedInteractions
	q := url.Values{"per_page": {fmt.Sprint(perPage)}, "order": {order}, "page": {"0"}}
	err := c.doJSON(ctx, http.MethodGet, "/sessions/"+sessionID+"/interactions?"+q.Encode(), nil, &out, 30*time.Second)
	return out.Interactions, err
}

// lastInteraction returns the newest turn. Ids are ULIDs (time ordered);
// `created` can tie across turns of the same session, so sort by id.
func (c *httpClient) lastInteraction(ctx context.Context, sessionID string) (*types.Interaction, error) {
	xs, err := c.interactions(ctx, sessionID, 5, "desc")
	if err != nil || len(xs) == 0 {
		return nil, err
	}
	sort.Slice(xs, func(a, b int) bool { return xs[a].ID < xs[b].ID })
	return xs[len(xs)-1], nil
}

// waitTurn polls until a turn newer than prevID has settled. The interaction
// list lags the blocking chat reply by a moment.
func (c *httpClient) waitTurn(ctx context.Context, sessionID, prevID string, maxWait time.Duration) (*types.Interaction, error) {
	deadline := time.Now().Add(maxWait)
	var last *types.Interaction
	for time.Now().Before(deadline) {
		i, err := c.lastInteraction(ctx, sessionID)
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
func (c *httpClient) sendTurn(ctx context.Context, sessionID, message string, attach []string, key string, timeout time.Duration) (*turnResult, error) {
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
	body := map[string]any{
		"stream": false, "type": "text",
		"messages": []any{map[string]any{"role": "user", "content": map[string]any{"content_type": "text", "parts": parts}}},
	}
	if sessionID != "" {
		body["session_id"] = sessionID
	}
	prevID := ""
	if sessionID != "" && key == "" {
		if last, err := c.lastInteraction(ctx, sessionID); err == nil && last != nil {
			prevID = last.ID
		}
	}
	cc := c
	if key != "" {
		cp := *c
		cp.apiKey = key
		cc = &cp
	}
	t0 := time.Now()
	var resp struct {
		ID      string `json:"id"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := cc.doJSON(ctx, http.MethodPost, "/sessions/chat", body, &resp, timeout); err != nil {
		return nil, err
	}
	res := &turnResult{SessionID: resp.ID, Seconds: time.Since(t0).Seconds()}
	if len(resp.Choices) > 0 {
		res.Raw = resp.Choices[0].Message.Content
	}
	if key == "" && res.SessionID != "" {
		if i, err := c.waitTurn(ctx, res.SessionID, prevID, 60*time.Second); err == nil && i != nil {
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
func sandboxForSession(ctx context.Context, orgID, sessionID string, wait time.Duration) (*types.Sandbox, error) {
	apiClient, err := client.NewClientFromEnv()
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(wait)
	for {
		resp, err := apiClient.ListSandboxes(ctx, orgID, nil)
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

// execInSession runs a bash script in the session's sandbox.
func execInSession(ctx context.Context, orgID, sessionID, script string, timeout int) (*types.SandboxCommand, error) {
	sb, err := sandboxForSession(ctx, orgID, sessionID, 0)
	if err != nil {
		return nil, err
	}
	apiClient, err := client.NewClientFromEnv()
	if err != nil {
		return nil, err
	}
	return apiClient.RunSandboxCommand(ctx, orgID, sb.ID, &types.RunSandboxCommandRequest{
		Cmd: "bash", Args: []string{"-lc", script}, TimeoutSeconds: timeout,
	})
}

// waitSandbox blocks until the session's sandbox is running, and fails fast
// when it reports failed (e.g. Hydra refusing to start: disk full, quota) —
// otherwise the first chat turn waits the full 5-minute readiness timeout and
// only says "external agent not ready".
func waitSandbox(ctx context.Context, orgID, sessionID string, maxWait time.Duration) error {
	apiClient, err := client.NewClientFromEnv()
	if err != nil {
		return err
	}
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		resp, err := apiClient.ListSandboxes(ctx, orgID, nil)
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
