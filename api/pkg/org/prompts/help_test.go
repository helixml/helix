package prompts_test

import (
	"context"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/org/prompts"
)

// TestHelpListsRegisteredPrompts pins the core /help contract: every
// prompt registered on the registry shows up in the rendered output
// with its name and description.
func TestHelpListsRegisteredPrompts(t *testing.T) {
	t.Parallel()
	reg := prompts.NewRegistry()
	if err := reg.Register(stubPrompt{name: "alpha"}); err != nil {
		t.Fatalf("register alpha: %v", err)
	}
	if err := reg.Register(stubPrompt{name: "beta"}); err != nil {
		t.Fatalf("register beta: %v", err)
	}
	help := prompts.NewHelp(reg)
	if err := reg.Register(help); err != nil {
		t.Fatalf("register help: %v", err)
	}

	msgs, err := help.Render(context.Background(), nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("messages = %d, want 1", len(msgs))
	}
	for _, want := range []string{"`/alpha`", "`/beta`", "`/help`"} {
		if !strings.Contains(msgs[0].Text, want) {
			t.Errorf("output missing %q\n%s", want, msgs[0].Text)
		}
	}
}

// TestHelpAutoGeneratesNewPrompts is the regression guard for the
// design promise: adding a new prompt must NOT require touching
// help.go. We register Help, render once, then register a new prompt
// and render again — the second render must include the new prompt.
//
// If this ever fails, someone has snapshotted the prompt list at
// construction time instead of resolving lazily on each Render call.
func TestHelpAutoGeneratesNewPrompts(t *testing.T) {
	t.Parallel()
	reg := prompts.NewRegistry()
	help := prompts.NewHelp(reg)
	_ = reg.Register(help)

	first, _ := help.Render(context.Background(), nil)
	if strings.Contains(first[0].Text, "`/late-arrival`") {
		t.Fatal("output unexpectedly contains a prompt that hasn't been registered yet")
	}

	if err := reg.Register(stubPrompt{name: "late-arrival"}); err != nil {
		t.Fatalf("register late-arrival: %v", err)
	}
	second, _ := help.Render(context.Background(), nil)
	if !strings.Contains(second[0].Text, "`/late-arrival`") {
		t.Fatalf("output missed prompt registered after Help: %s", second[0].Text)
	}
}

// TestHelpAlphabeticallySorted: the listing is deterministic so a
// future change that flipped to map-order randomness is caught.
func TestHelpAlphabeticallySorted(t *testing.T) {
	t.Parallel()
	reg := prompts.NewRegistry()
	for _, n := range []string{"zebra", "alpha", "mango"} {
		_ = reg.Register(stubPrompt{name: prompts.Name(n)})
	}
	help := prompts.NewHelp(reg)
	_ = reg.Register(help)

	msgs, _ := help.Render(context.Background(), nil)
	// The body has a stage-direction preamble that itself mentions
	// `/help`; if we searched the whole string, /help would always
	// appear first. Skip past the listing header so we're only
	// matching the actual sorted list.
	_, list, ok := strings.Cut(msgs[0].Text, "Available slash commands")
	if !ok {
		t.Fatalf("listing header missing from output:\n%s", msgs[0].Text)
	}
	a := strings.Index(list, "`/alpha`")
	h := strings.Index(list, "`/help`")
	m := strings.Index(list, "`/mango`")
	z := strings.Index(list, "`/zebra`")
	if a >= h || h >= m || m >= z {
		t.Fatalf("not alphabetical: alpha=%d help=%d mango=%d zebra=%d\n%s", a, h, m, z, list)
	}
}

func TestHelpNoToolGate(t *testing.T) {
	t.Parallel()
	if got := (prompts.Help{}).RequiresTool(); got != "" {
		t.Errorf("RequiresTool = %q, want empty (universal visibility)", got)
	}
}

// TestRegisterBuiltinsIncludesHelp covers the wiring promise: serve.go
// calls prompts.RegisterBuiltins, and the result must include Help so
// users see it in the autocomplete and can invoke it.
func TestRegisterBuiltinsIncludesHelp(t *testing.T) {
	t.Parallel()
	reg := prompts.NewRegistry()
	if err := prompts.RegisterBuiltins(reg); err != nil {
		t.Fatalf("RegisterBuiltins: %v", err)
	}
	if _, err := reg.Get(prompts.HelpName); err != nil {
		t.Errorf("help missing from builtins: %v", err)
	}
	if _, err := reg.Get(prompts.RoleName); err != nil {
		t.Errorf("role missing from builtins: %v", err)
	}
}
