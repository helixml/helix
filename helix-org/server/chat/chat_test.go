package chat

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/helixml/helix/helix-org/domain"
	"github.com/helixml/helix/helix-org/prompts"
)

// stubPrompt is a single-arg prompt used to exercise the bridge's
// slash-command expansion logic without coupling these tests to the
// real /role template content.
type stubPrompt struct {
	name Name
	arg  string
}

type Name = prompts.Name

func (s stubPrompt) Name() Name        { return s.name }
func (stubPrompt) Title() string       { return "stub" }
func (stubPrompt) Description() string { return "stub" }
func (s stubPrompt) Arguments() []prompts.Argument {
	if s.arg == "" {
		return nil
	}
	return []prompts.Argument{{Name: s.arg}}
}
func (stubPrompt) RequiresTool() domain.ToolName { return "" }
func (stubPrompt) Render(_ context.Context, args map[string]string) ([]prompts.Message, error) {
	body := "rendered:" + args["hint"]
	return []prompts.Message{{Role: "user", Text: body}}, nil
}

func newBridgeWithPrompts(t *testing.T, ps ...prompts.Prompt) *Bridge {
	t.Helper()
	reg := prompts.NewRegistry()
	for _, p := range ps {
		if err := reg.Register(p); err != nil {
			t.Fatalf("register %s: %v", p.Name(), err)
		}
	}
	return New("claude", t.TempDir(), "http://example/mcp", slog.Default()).WithPrompts(reg)
}

// TestExpandsSlashCommandIntoPromptText confirms that `/foo` is
// rewritten to the rendered prompt body when `foo` is registered.
func TestExpandsSlashCommandIntoPromptText(t *testing.T) {
	t.Parallel()
	b := newBridgeWithPrompts(t, stubPrompt{name: "foo", arg: "hint"})
	got, ok := b.expandSlashCommand(context.Background(), "/foo")
	if !ok {
		t.Fatal("expand = false, want true")
	}
	if !strings.HasPrefix(got, "rendered:") {
		t.Fatalf("got = %q, want it to start with 'rendered:'", got)
	}
}

// TestThreadsTailIntoFirstArgument is the contract that lets the user
// type `/role marketing director` and have "marketing director" land
// as the prompt's hint.
func TestThreadsTailIntoFirstArgument(t *testing.T) {
	t.Parallel()
	b := newBridgeWithPrompts(t, stubPrompt{name: "foo", arg: "hint"})
	got, _ := b.expandSlashCommand(context.Background(), "/foo marketing director")
	if !strings.Contains(got, "marketing director") {
		t.Fatalf("tail not threaded through: %q", got)
	}
}

// TestUnknownSlashCommandFallsThrough confirms that a slash the bridge
// doesn't recognise is not consumed — the literal text reaches claude,
// which can then handle it (e.g. surface its own "unknown command"
// message) instead of the bridge silently dropping the message.
func TestUnknownSlashCommandFallsThrough(t *testing.T) {
	t.Parallel()
	b := newBridgeWithPrompts(t, stubPrompt{name: "foo"})
	if _, ok := b.expandSlashCommand(context.Background(), "/missing"); ok {
		t.Fatal("expand = true for unknown command, want false")
	}
}

// TestNonSlashIsNotIntercepted is the simplest regression guard: a
// plain user message must not be rewritten just because the bridge has
// a registry attached.
func TestNonSlashIsNotIntercepted(t *testing.T) {
	t.Parallel()
	b := newBridgeWithPrompts(t, stubPrompt{name: "foo"})
	if _, ok := b.expandSlashCommand(context.Background(), "hello world"); ok {
		t.Fatal("expand = true for non-slash text, want false")
	}
}

// TestNilRegistryFallsThrough mirrors production wiring where bridges
// without a prompt registry attached should pass slash commands
// through to claude unchanged.
func TestNilRegistryFallsThrough(t *testing.T) {
	t.Parallel()
	b := New("claude", t.TempDir(), "http://example/mcp", slog.Default())
	if _, ok := b.expandSlashCommand(context.Background(), "/foo"); ok {
		t.Fatal("expand = true with nil registry, want false")
	}
}
