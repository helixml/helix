package prompts_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/helixml/helix-org/domain"
	"github.com/helixml/helix-org/prompts"
)

// stubPrompt is a minimal Prompt for exercising registry behaviour
// without coupling these tests to the new_role implementation.
type stubPrompt struct {
	name Name
	tool domain.ToolName
}

type Name = prompts.Name

func (s stubPrompt) Name() Name                    { return s.name }
func (stubPrompt) Title() string                   { return "stub" }
func (stubPrompt) Description() string             { return "stub" }
func (stubPrompt) Arguments() []prompts.Argument   { return nil }
func (s stubPrompt) RequiresTool() domain.ToolName { return s.tool }
func (stubPrompt) Render(_ context.Context, _ map[string]string) ([]prompts.Message, error) {
	return []prompts.Message{{Role: "user", Text: "stub"}}, nil
}

func TestRegistryRegisterAndGet(t *testing.T) {
	t.Parallel()
	reg := prompts.NewRegistry()
	if err := reg.Register(stubPrompt{name: "a"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	p, err := reg.Get("a")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if p.Name() != "a" {
		t.Fatalf("Name = %q, want a", p.Name())
	}
}

func TestRegistryRejectsEmptyName(t *testing.T) {
	t.Parallel()
	reg := prompts.NewRegistry()
	if err := reg.Register(stubPrompt{name: ""}); err == nil {
		t.Fatal("Register empty name = nil, want error")
	}
}

func TestRegistryRejectsDuplicate(t *testing.T) {
	t.Parallel()
	reg := prompts.NewRegistry()
	if err := reg.Register(stubPrompt{name: "a"}); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	err := reg.Register(stubPrompt{name: "a"})
	if err == nil {
		t.Fatal("duplicate Register = nil, want error")
	}
	if !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("err = %v, want 'already registered'", err)
	}
}

func TestRegistryGetUnknown(t *testing.T) {
	t.Parallel()
	reg := prompts.NewRegistry()
	_, err := reg.Get("missing")
	if err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Fatalf("err = %v, want 'not registered'", err)
	}
	// Stay friendly to callers that want to translate "missing" into
	// their own error type — should at least be a real error value.
	if errors.Unwrap(err) != nil {
		t.Fatalf("err.Unwrap() = %v, want nil sentinel-shaped error", errors.Unwrap(err))
	}
}

func TestRegistryAllReturnsEverything(t *testing.T) {
	t.Parallel()
	reg := prompts.NewRegistry()
	for _, n := range []Name{"a", "b", "c"} {
		if err := reg.Register(stubPrompt{name: n}); err != nil {
			t.Fatalf("Register %s: %v", n, err)
		}
	}
	all := reg.All()
	if len(all) != 3 {
		t.Fatalf("All() = %d, want 3", len(all))
	}
	got := map[Name]bool{}
	for _, p := range all {
		got[p.Name()] = true
	}
	for _, want := range []Name{"a", "b", "c"} {
		if !got[want] {
			t.Errorf("All() missing %q", want)
		}
	}
}
