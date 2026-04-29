package chat

import (
	"encoding/json"
	"fmt"
	"html"
	"strings"

	"github.com/helixml/helix-org/prompts"
)

// streamEvent captures the parts of claude's stream-json format the
// chat surface needs to render. The shape is shared between the
// live SSE bridge (chat.go) and the historical-replay reader
// (sessions.go) — both parse the same events out of either claude's
// stdout or its on-disk session jsonl.
type streamEvent struct {
	Type    string          `json:"type"`
	Subtype string          `json:"subtype,omitempty"`
	Message json.RawMessage `json:"message,omitempty"`
	Result  string          `json:"result,omitempty"`
	IsError bool            `json:"is_error,omitempty"`
}

// messagePayload mirrors the message envelope inside both stream-json
// (live) and session-jsonl (history) lines. content can be a string
// (raw user prompt) or an array of contentSegment (everything else).
type messagePayload struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type contentSegment struct {
	Type    string          `json:"type"`
	Text    string          `json:"text,omitempty"`
	Name    string          `json:"name,omitempty"`
	Input   json.RawMessage `json:"input,omitempty"`
	Content json.RawMessage `json:"content,omitempty"`
	IsError bool            `json:"is_error,omitempty"`
}

// renderFragments turns one parsed stream-json event into zero or
// more HTML fragments, one per atomic visual unit (user bubble,
// assistant text bubble, tool-use chip, tool-result chip, error
// banner). Used by both the live SSE bridge and historical replay.
//
// Returns []string rather than a single string so the caller can
// broadcast each fragment as its own SSE message and the browser can
// stream them in one at a time.
func renderFragments(ev streamEvent) []string {
	switch ev.Type {
	case "user":
		return renderUserEvent(ev.Message)
	case "assistant":
		return renderAssistantEvent(ev.Message)
	case "result":
		if ev.IsError {
			return []string{renderTurnError(ev.Result)}
		}
	}
	return nil
}

// renderUserEvent decodes a user-event message and returns the HTML
// fragments it produces. content is either a raw string (the user's
// prompt) or an array of segments (where the only renderable segment
// is tool_result — the live stream-json sometimes wraps tool results
// in a user envelope). text segments inside an array body are also
// surfaced so resumed-history user messages with multipart content
// render correctly.
//
// CLI metadata blocks (<local-command-caveat>, <command-name>,
// <system-reminder>) are silently dropped — those are scaffolding
// claude wrote into the transcript, not actual user prompts, and
// rendering them as bubbles would clutter the resumed view.
func renderUserEvent(messageJSON json.RawMessage) []string {
	var msg messagePayload
	if err := json.Unmarshal(messageJSON, &msg); err != nil {
		return nil
	}
	// Try string-shaped content first.
	var asString string
	if err := json.Unmarshal(msg.Content, &asString); err == nil {
		if asString = strings.TrimSpace(asString); asString != "" && !isMetaPrompt(asString) {
			return []string{renderUserBubble(asString)}
		}
		return nil
	}
	// Otherwise treat as array of segments.
	var segs []contentSegment
	if err := json.Unmarshal(msg.Content, &segs); err != nil {
		return nil
	}
	var out []string
	for _, seg := range segs {
		switch seg.Type {
		case "text":
			t := strings.TrimSpace(seg.Text)
			if t == "" || isMetaPrompt(t) {
				continue
			}
			out = append(out, renderUserBubble(t))
		case "tool_result":
			out = append(out, renderToolResult(string(seg.Content), seg.IsError))
		}
	}
	return out
}

// renderAssistantEvent decodes an assistant-event message and returns
// the HTML fragments. text segments become assistant bubbles;
// tool_use becomes a tool-use chip; thinking is silently dropped
// (internal scratchpad — not for the chat surface).
func renderAssistantEvent(messageJSON json.RawMessage) []string {
	var msg messagePayload
	if err := json.Unmarshal(messageJSON, &msg); err != nil {
		return nil
	}
	var segs []contentSegment
	if err := json.Unmarshal(msg.Content, &segs); err != nil {
		return nil
	}
	var out []string
	for _, seg := range segs {
		switch seg.Type {
		case "text":
			if seg.Text != "" {
				out = append(out, renderAssistantText(seg.Text))
			}
		case "tool_use":
			out = append(out, renderToolUse(seg.Name, string(seg.Input)))
		}
	}
	return out
}

// renderSlashSuggestion renders one row in the slash-command dropdown.
// Clicking the row fills the textarea with `/<name> ` (trailing space
// so the user can keep typing arguments) and clears the dropdown.
//
// The inline onclick is the smallest thing that works — it does the
// two DOM ops that have no reasonable server-rendered equivalent
// (mutating the textarea value, hiding the suggestion list). Anything
// fancier than this would mean adopting a JS framework, which we don't
// need.
func renderSlashSuggestion(p prompts.Prompt) string {
	name := html.EscapeString(string(p.Name()))
	title := html.EscapeString(p.Title())
	desc := html.EscapeString(p.Description())
	return fmt.Sprintf(
		`<button type="button"
		         class="block w-full text-left px-4 py-2 hover:bg-[var(--surface-elev)] border-b border-[var(--line)] last:border-b-0"
		         onclick="var t=document.querySelector('textarea[name=&quot;message&quot;]');t.value='/%s ';t.focus();var s=document.getElementById('slash-suggestions');if(s){s.innerHTML='';}">
		   <span class="font-mono text-[14px]" style="color: var(--accent);">/%s</span>
		   <span class="text-[13px]" style="color: var(--ink);"> — %s</span>
		   <div class="text-[12px] mt-0.5" style="color: var(--ink-muted);">%s</div>
		 </button>`,
		name, name, title, desc,
	)
}

func renderUserBubble(text string) string {
	return fmt.Sprintf(
		`<div class="msg msg-user my-2 ml-auto max-w-[78%%] px-4 py-3 rounded-2xl text-[14px] whitespace-pre-wrap" style="background: var(--accent); color: var(--accent-on);">%s</div>`,
		html.EscapeString(text),
	)
}

func renderAssistantText(text string) string {
	return fmt.Sprintf(
		`<div class="msg msg-asst my-2 mr-auto max-w-[78%%] px-4 py-3 rounded-2xl text-[14px] whitespace-pre-wrap" style="background: var(--surface-elev); border: 1px solid var(--line); color: var(--ink);">%s</div>`,
		html.EscapeString(text),
	)
}

func renderToolUse(name, input string) string {
	return fmt.Sprintf(
		`<div class="msg msg-tool my-1 mr-auto max-w-[78%%] px-3 py-1 font-mono text-[12px] rounded" style="color: var(--ink-muted);">▸ <strong>%s</strong> <span class="opacity-70">%s</span></div>`,
		html.EscapeString(name),
		html.EscapeString(oneLine(input, 220)),
	)
}

func renderToolResult(content string, isErr bool) string {
	color := "var(--ink-muted)"
	arrow := "◂"
	if isErr {
		color = "#A0432F"
		arrow = "⚠"
	}
	return fmt.Sprintf(
		`<div class="msg msg-tool-result my-1 mr-auto max-w-[78%%] px-3 py-1 font-mono text-[12px] rounded" style="color: %s;">%s %s</div>`,
		color, arrow, html.EscapeString(oneLine(content, 220)),
	)
}

func renderTurnError(msg string) string {
	return fmt.Sprintf(
		`<div class="msg msg-error my-2 mr-auto max-w-[78%%] px-4 py-3 rounded-2xl text-[13px] font-mono" style="background: #FBE9E2; border: 1px solid #E8B7A6; color: #6E2A1A;">⚠ %s</div>`,
		html.EscapeString(oneLine(msg, 500)),
	)
}

func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if max > 0 && len(s) > max {
		return s[:max] + "…"
	}
	return s
}
