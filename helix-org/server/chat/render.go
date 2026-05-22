package chat

import (
	"bytes"
	"fmt"
	"html"
	"strings"

	"github.com/yuin/goldmark"

	"github.com/helixml/helix/helix-org/prompts"
)

// markdown is the shared parser/renderer for assistant text. Default
// goldmark options escape any raw HTML the LLM emits — we never opt
// into WithUnsafe because the assistant text is not trusted input.
var markdown = goldmark.New()

// renderMarkdown turns assistant-emitted markdown into safe HTML.
// On parse error (which the default goldmark cannot really produce
// for arbitrary text input) we fall back to escaped plaintext so the
// bubble still renders something legible.
func renderMarkdown(src string) string {
	var buf bytes.Buffer
	if err := markdown.Convert([]byte(src), &buf); err != nil {
		return html.EscapeString(src)
	}
	return buf.String()
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
		`<div class="msg msg-asst md my-2 mr-auto max-w-[78%%] px-4 py-3 rounded-2xl text-[14px]" style="background: var(--surface-elev); border: 1px solid var(--line); color: var(--ink);">%s</div>`,
		renderMarkdown(text),
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
