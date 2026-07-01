package orgchart

import (
	"testing"
	"time"
)

// TestNewBotDefaultsPreserveContextFalse pins that a freshly constructed
// Bot does not preserve context — the spawner wipes its session on every
// re-activation unless the operator opts in.
func TestNewBotDefaultsPreserveContextFalse(t *testing.T) {
	t.Parallel()
	b, err := NewBot("b-test", "content", nil, nil, time.Now(), "org-test")
	if err != nil {
		t.Fatalf("NewBot: %v", err)
	}
	if b.PreserveContext {
		t.Error("NewBot PreserveContext = true, want false (wipe-on-trigger is the default)")
	}
}

// TestWithPreserveContext checks the immutable builder flips the flag on a
// copy without mutating the receiver.
func TestWithPreserveContext(t *testing.T) {
	t.Parallel()
	b, err := NewBot("b-test", "content", nil, nil, time.Now(), "org-test")
	if err != nil {
		t.Fatalf("NewBot: %v", err)
	}
	on := b.WithPreserveContext(true)
	if !on.PreserveContext {
		t.Error("WithPreserveContext(true).PreserveContext = false, want true")
	}
	if b.PreserveContext {
		t.Error("WithPreserveContext mutated the receiver; builders must return a copy")
	}
	if on.WithPreserveContext(false).PreserveContext {
		t.Error("WithPreserveContext(false).PreserveContext = true, want false")
	}
}
