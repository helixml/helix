package prompts_test

import (
	"context"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/org/application/prompts"
)

func TestRoleRequiresCreateBotTool(t *testing.T) {
	t.Parallel()
	// "create_bot" is the stable public tool name; RegisterBuiltins
	// fails fast at boot if the registered name ever drifts from it.
	if got := (prompts.Role{}).RequiresTool(); got != "create_bot" {
		t.Fatalf("RequiresTool = %q, want %q", got, "create_bot")
	}
}

func TestRoleRendersTemplate(t *testing.T) {
	t.Parallel()
	msgs, err := (prompts.Role{}).Render(context.Background(), nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("messages = %d, want 1", len(msgs))
	}
	if msgs[0].Role != "user" {
		t.Fatalf("role = %q, want user", msgs[0].Role)
	}
	text := strings.ToLower(msgs[0].Text)
	// The template must mention the tool we're driving toward and the
	// canonical Role-markdown sections demonstrated in the demo Roles.
	// These assertions pin the *contract* of the prompt — that it tells
	// the LLM to call create_bot and produces output the rest of the
	// org can read. They do not pin every word of the prose.
	for _, want := range []string{
		"create_bot",
		"## Behaviour",
		"## Starts when",
		"## Constraints",
		"human-readable **name or role title**",
		"concrete **purpose**",
		"stop and wait for the answer",
		"generic placeholder",
		"keel-hq/keel",
		"keel-maintainer",
		"proceed without a question",
		"full standard worker tool set",
		"match the named owner/repository exactly",
		"finish that scope in the same turn",
		"POST /api/v1/git/repositories",
		"substitute a similarly named repository",
		"do not ask a routine follow-up",
		"set_bot_content",
	} {
		if !strings.Contains(text, strings.ToLower(want)) {
			t.Errorf("template missing %q", want)
		}
	}
	for _, forbidden := range []string{"Don't interview me", "A good guess beats a question", "Draft from this directly — no interview", "Want to change anything?"} {
		if strings.Contains(msgs[0].Text, forbidden) {
			t.Errorf("template still contains unsafe instruction %q", forbidden)
		}
	}
}

func TestRoleAppendsHint(t *testing.T) {
	t.Parallel()
	msgs, err := (prompts.Role{}).Render(context.Background(), map[string]string{"hint": "marketing director"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(msgs[0].Text, "marketing director") {
		t.Fatalf("hint not in output: %s", msgs[0].Text)
	}
	if !strings.Contains(msgs[0].Text, "Otherwise ask for the missing part and wait") {
		t.Fatalf("hint bypasses the required brief: %s", msgs[0].Text)
	}
}

func TestRoleIgnoresWhitespaceHint(t *testing.T) {
	t.Parallel()
	withHint, _ := (prompts.Role{}).Render(context.Background(), map[string]string{"hint": "   "})
	withoutHint, _ := (prompts.Role{}).Render(context.Background(), nil)
	if withHint[0].Text != withoutHint[0].Text {
		t.Fatalf("whitespace-only hint changed output")
	}
}
