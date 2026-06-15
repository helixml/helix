package agent

import (
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// TestRenderTriggerGitHub: a github-shaped event (issue.opened) must
// surface every populated envelope field in the prompt.
func TestRenderTriggerGitHub(t *testing.T) {
	t.Parallel()

	extra := []byte(`{"action":"opened","event":"issues","issue":{"id":12345,"number":42,"title":"x","body":"y"},"sender":{"login":"philwinder"},"repository":{"full_name":"helixml/helix-org"}}`)
	tr := activation.Trigger{
		Kind:      activation.TriggerEvent,
		EventID:   "e-abc",
		StreamID:  "s-github",
		Source:    "",
		CreatedAt: time.Date(2026, 4, 28, 12, 27, 23, 0, time.UTC),
		Message: streaming.Message{
			From:      "philwinder",
			Subject:   "README setup steps mention an env var that no longer exists",
			Body:      "Step 3 references HELIX_FOO; the code reads HELIX_BAR now.",
			ThreadID:  "#42",
			MessageID: "delivery-uuid-1",
			Extra:     extra,
		},
	}

	got := renderTrigger(tr)

	wants := []string{
		"stream:      s-github",
		"event:       e-abc",
		"time:        2026-04-28T12:27:23Z",
		"from:        philwinder",
		"subject:     README setup steps mention an env var that no longer exists",
		"thread_id:   #42",
		"message_id:  delivery-uuid-1",
		"Step 3 references HELIX_FOO",
		`"event":"issues"`,
		`"action":"opened"`,
		`"sender":{"login":"philwinder"}`,
		`"repository":{"full_name":"helixml/helix-org"}`,
	}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("renderTrigger output missing %q\n--- output ---\n%s", w, got)
		}
	}

	for _, omit := range []string{"to:", "in_reply_to:", "source:"} {
		if strings.Contains(got, omit) {
			t.Errorf("renderTrigger output should omit empty %q\n--- output ---\n%s", omit, got)
		}
	}
}

func TestRenderTriggerEmail(t *testing.T) {
	t.Parallel()

	tr := activation.Trigger{
		Kind:      activation.TriggerEvent,
		EventID:   "e-1",
		StreamID:  "s-support",
		Source:    "",
		CreatedAt: time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC),
		Message: streaming.Message{
			From:      "alice@example.com",
			To:        []string{"abc123+sam@inbound.postmarkapp.com"},
			Subject:   "[eng] Re: Webhook stream isn't firing",
			Body:      "Most webhook flow issues are config or subscription mismatches.",
			ThreadID:  "<root@example.com>",
			InReplyTo: "<original@example.com>",
			MessageID: "<msg-2@example.com>",
		},
	}

	got := renderTrigger(tr)

	wants := []string{
		"from:        alice@example.com",
		"to:          abc123+sam@inbound.postmarkapp.com",
		"subject:     [eng] Re: Webhook stream isn't firing",
		"thread_id:   <root@example.com>",
		"in_reply_to: <original@example.com>",
		"message_id:  <msg-2@example.com>",
		"Most webhook flow issues",
	}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("renderTrigger output missing %q\n--- output ---\n%s", w, got)
		}
	}
}

func TestRenderTriggerWorkerPublished(t *testing.T) {
	t.Parallel()

	tr := activation.Trigger{
		Kind:      activation.TriggerEvent,
		EventID:   "e-1",
		StreamID:  "s-general",
		Source:    "w-alice",
		CreatedAt: time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC),
		Message: streaming.Message{
			From: "w-alice",
			Body: "hello",
		},
	}
	got := renderTrigger(tr)

	for _, w := range []string{"source:      w-alice", "from:        w-alice", "hello"} {
		if !strings.Contains(got, w) {
			t.Errorf("renderTrigger output missing %q\n--- output ---\n%s", w, got)
		}
	}
	for _, omit := range []string{"to:", "subject:", "thread_id:", "in_reply_to:", "message_id:", "extra:"} {
		if strings.Contains(got, omit) {
			t.Errorf("renderTrigger output should omit empty %q\n--- output ---\n%s", omit, got)
		}
	}
}

// TestBuildPromptIncludesEnvelope checks the integration: a activation.Trigger
// with full envelope fields produces a prompt whose === Trigger ===
// section carries all of them.
func TestBuildPromptIncludesEnvelope(t *testing.T) {
	t.Parallel()

	tr := activation.Trigger{
		Kind:      activation.TriggerEvent,
		EventID:   "e-abc",
		StreamID:  "s-github",
		CreatedAt: time.Date(2026, 4, 28, 12, 27, 23, 0, time.UTC),
		Message: streaming.Message{
			From:    "philwinder",
			Subject: "Confusing example in the docs",
			Body:    "The README has an install command that doesn't run as written.",
			Extra:   []byte(`{"event":"issues","action":"opened"}`),
		},
	}
	prompt := BuildPrompt("w-doc-engineer", "[role.md contents]", []activation.Trigger{tr})

	if !strings.Contains(prompt, "=== Trigger ===") || !strings.Contains(prompt, "=== end trigger ===") {
		t.Fatalf("trigger fences missing\n%s", prompt)
	}
	for _, w := range []string{
		"subject:     Confusing example in the docs",
		"from:        philwinder",
		`"event":"issues"`,
	} {
		if !strings.Contains(prompt, w) {
			t.Errorf("prompt missing %q", w)
		}
	}
}

// TestBuildPromptManualTrigger pins the operator-driven activation path.
// The prompt body must be operator-aware so the Worker doesn't treat the
// activation as either a hire (first-time setup) or an event (no event
// envelope to read). Standard wrapper still renders the mandate so the
// Worker re-reads its role / identity per the helix-specs branch.
func TestBuildPromptManualTrigger(t *testing.T) {
	t.Parallel()
	prompt := BuildPrompt("w-eng", "[mandate]", []activation.Trigger{{Kind: activation.TriggerManual}})
	if !strings.Contains(prompt, "operator manually woke you up") {
		t.Errorf("manual trigger body missing\n%s", prompt)
	}
	if !strings.Contains(prompt, "[mandate]") {
		t.Errorf("mandate wrapper missing\n%s", prompt)
	}
	if got := DescribeTrigger(activation.Trigger{Kind: activation.TriggerManual}); got != "manual" {
		t.Errorf("DescribeTrigger(manual) = %q, want %q", got, "manual")
	}
}
