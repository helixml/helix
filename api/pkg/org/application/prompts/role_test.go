package prompts_test

import (
	"context"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/org/application/prompts"
	"github.com/helixml/helix/api/pkg/org/application/tools"
)

func TestRoleRequiresCreateRoleGrant(t *testing.T) {
	t.Parallel()
	if got := (prompts.Role{}).RequiresTool(); got != tools.CreateRoleName {
		t.Fatalf("RequiresTool = %q, want %q", got, tools.CreateRoleName)
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
	// The template must mention the tool we're driving toward and the
	// canonical Role-markdown sections demonstrated in the demo Roles.
	// These assertions pin the *contract* of the prompt — that it tells
	// the LLM to call create_role and produces output the rest of the
	// org can read. They do not pin every word of the prose.
	for _, want := range []string{"create_role", "## Triggers", "## Streams", "## Constraints"} {
		if !strings.Contains(msgs[0].Text, want) {
			t.Errorf("template missing %q", want)
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
}

func TestRoleIgnoresWhitespaceHint(t *testing.T) {
	t.Parallel()
	withHint, _ := (prompts.Role{}).Render(context.Background(), map[string]string{"hint": "   "})
	withoutHint, _ := (prompts.Role{}).Render(context.Background(), nil)
	if withHint[0].Text != withoutHint[0].Text {
		t.Fatalf("whitespace-only hint changed output")
	}
}
