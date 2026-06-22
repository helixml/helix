package briefing

import (
	"fmt"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
)

// BuildPrompt assembles the per-activation prompt: identity hint +
// mandate + the triggers that woke the Worker up. The dispatcher
// coalesces bursts, so a single activation may carry multiple triggers
// — the prompt frames them as a numbered list when that happens, so
// the agent can read all of them before deciding what to do (often the
// most recent supersedes the earlier ones). Tools are exposed natively
// via MCP under the "helix" server (tool names appear as
// mcp__helix__<name>); Claude figures the rest out from tools/list.
//
// `mandate` is the static text the agent reads first — for the Helix
// runtime it's a short pointer at the helix-specs branch, which carries
// the real policy text the Worker reads before acting.
func BuildPrompt(workerID orgchart.WorkerID, mandate string, triggers []activation.Trigger) string {
	var ctx strings.Builder

	if len(triggers) > 1 {
		fmt.Fprintf(&ctx, "%d triggers have queued for you since your last activation. They are listed below in arrival order. Read all of them before deciding what to do — often the latest supersedes earlier ones, and most cascades resolve to a single response or to silence.\n\n", len(triggers))
	}

	for i, t := range triggers {
		if len(triggers) > 1 {
			fmt.Fprintf(&ctx, "[%d/%d]\n", i+1, len(triggers))
		}
		switch t.Kind {
		case activation.TriggerHire:
			ctx.WriteString("You have just been hired. This is your first activation. Complete any one-time setup your role describes, then exit. The runtime will re-activate you when an event arrives on a Topic you subscribe to.\n")
		case activation.TriggerEvent:
			ctx.WriteString(renderTrigger(t))
		case activation.TriggerManual:
			ctx.WriteString("An operator manually woke you up from the worker UI. Re-read your role and identity (per the mandate above), summarise your current state, and stand by — the operator will follow up with instructions on the next message.\n")
		default:
			fmt.Fprintf(&ctx, "Activation kind: %q.\n", t.Kind)
		}
		if len(triggers) > 1 && i < len(triggers)-1 {
			ctx.WriteByte('\n')
		}
	}

	return fmt.Sprintf(`You are Worker %s, running inside helix-org. Your environment is
the current working directory. Each activation is a single turn — do
the work and exit.

%s

=== Trigger ===
%s=== end trigger ===

Act now. No preamble.
`, workerID, mandate, ctx.String())
}

// renderTrigger formats an event-kind activation.Trigger for the activation
// prompt. Every populated field of the canonical Message envelope is
// rendered so the Worker can branch on Subject, From, ThreadID, Extra,
// etc. directly — no separate read_events round-trip needed for the
// trigger event itself. Empty fields are omitted to keep the prompt
// tight.
//
// Header keys are aligned for legibility but the parser the Worker is
// going to apply (Claude reading the prompt) is robust to spacing, so
// "neat" is for humans tailing the prompt.
func renderTrigger(t activation.Trigger) string {
	var b strings.Builder
	b.WriteString("A new event arrived on a Topic you subscribe to.\n\n")
	fmt.Fprintf(&b, "  topic:      %s\n", t.TopicID)
	fmt.Fprintf(&b, "  event:       %s\n", t.EventID)
	fmt.Fprintf(&b, "  time:        %s\n", t.CreatedAt.Format(time.RFC3339))
	if t.Source != "" {
		fmt.Fprintf(&b, "  source:      %s\n", t.Source)
	}
	// source_kind drives the agent.md priority rule: AI-origin events
	// are low-priority by default. Always emit when known (even when
	// Source itself is empty — a future inbound transport that can
	// classify origin without resolving a Worker still needs to flag
	// AI vs human here).
	if t.SourceKind != "" {
		fmt.Fprintf(&b, "  source_kind: %s\n", t.SourceKind)
	}
	m := t.Message
	if m.From != "" {
		fmt.Fprintf(&b, "  from:        %s\n", m.From)
	}
	if len(m.To) > 0 {
		fmt.Fprintf(&b, "  to:          %s\n", strings.Join(m.To, ", "))
	}
	if m.Subject != "" {
		fmt.Fprintf(&b, "  subject:     %s\n", m.Subject)
	}
	if m.ThreadID != "" {
		fmt.Fprintf(&b, "  thread_id:   %s\n", m.ThreadID)
	}
	if m.InReplyTo != "" {
		fmt.Fprintf(&b, "  in_reply_to: %s\n", m.InReplyTo)
	}
	if m.MessageID != "" {
		fmt.Fprintf(&b, "  message_id:  %s\n", m.MessageID)
	}
	if m.Body != "" {
		b.WriteString("  body:\n")
		b.WriteString(indentBlock(m.Body, "    "))
		b.WriteByte('\n')
	}
	if len(m.Extra) > 0 {
		b.WriteString("  extra:\n")
		b.WriteString(indentBlock(string(m.Extra), "    "))
		b.WriteByte('\n')
	}
	return b.String()
}

// indentBlock prefixes every line of s with prefix. Used so multi-line
// event bodies render readably inside the prompt.
func indentBlock(s, prefix string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

// DescribeTrigger returns a short label for one activation.Trigger — used for
// transcript markers and structured logging.
func DescribeTrigger(t activation.Trigger) string {
	switch t.Kind {
	case activation.TriggerHire:
		return "hire"
	case activation.TriggerEvent:
		return fmt.Sprintf("event %s on %s from %s", t.EventID, t.TopicID, t.Source)
	case activation.TriggerManual:
		return "manual"
	default:
		return string(t.Kind)
	}
}

// DescribeTriggers labels the activation marker that gets published
// to the worker's transcript. A single trigger reuses
// DescribeTrigger verbatim so observers see no change for the common
// case; a coalesced batch summarises as "batch of N" with each
// trigger's individual description joined by "; ".
func DescribeTriggers(triggers []activation.Trigger) string {
	if len(triggers) == 1 {
		return DescribeTrigger(triggers[0])
	}
	parts := make([]string, len(triggers))
	for i, t := range triggers {
		parts[i] = DescribeTrigger(t)
	}
	return fmt.Sprintf("batch of %d: %s", len(triggers), strings.Join(parts, "; "))
}

// OneLine collapses whitespace and clips to max runes for readability.
// Shared by both runtimes' transcript renderers.
func OneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if max > 0 && len(s) > max {
		return s[:max] + "…"
	}
	return s
}
