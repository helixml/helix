package config_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/org/config"
	"github.com/helixml/helix/api/pkg/org/store/sqlite"
)

func newRegistry(t *testing.T) *config.Registry {
	t.Helper()
	s, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return config.New(s.Configs)
}

func TestRegistryRegisterAndSet(t *testing.T) {
	t.Parallel()
	r := newRegistry(t)
	r.Register(config.Spec{
		Key:         "claude.bin",
		Type:        config.TypeString,
		Default:     `"claude"`,
		Required:    true,
		Description: "Path to claude CLI.",
	})

	ctx := context.Background()

	// Default applies before any Set.
	got, err := r.GetString(ctx, "claude.bin")
	if err != nil {
		t.Fatalf("GetString default: %v", err)
	}
	if got != "claude" {
		t.Fatalf("default = %q", got)
	}

	// Set overrides default.
	if err := r.Set(ctx, "claude.bin", `"/usr/local/bin/claude"`, ""); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, _ = r.GetString(ctx, "claude.bin")
	if got != "/usr/local/bin/claude" {
		t.Fatalf("after set = %q", got)
	}

	// Delete falls back to default again.
	if err := r.Delete(ctx, "claude.bin"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, _ = r.GetString(ctx, "claude.bin")
	if got != "claude" {
		t.Fatalf("after delete = %q", got)
	}
}

func TestRegistryRequiredMissing(t *testing.T) {
	t.Parallel()
	r := newRegistry(t)
	r.Register(config.Spec{
		Key: "claude.public_url", Type: config.TypeString, Required: true,
	})

	ctx := context.Background()
	_, err := r.GetString(ctx, "claude.public_url")
	if !errors.Is(err, config.ErrRequired) {
		t.Fatalf("err = %v, want ErrRequired", err)
	}
}

func TestRegistryOptionalMissing(t *testing.T) {
	t.Parallel()
	r := newRegistry(t)
	r.Register(config.Spec{
		Key: "transport.postmark", Type: config.TypeObject,
	})

	ctx := context.Background()
	var pm struct {
		Token string `json:"token"`
	}
	err := r.GetObject(ctx, "transport.postmark", &pm)
	if !errors.Is(err, config.ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
}

func TestRegistryUnknownKey(t *testing.T) {
	t.Parallel()
	r := newRegistry(t)
	ctx := context.Background()

	if err := r.Set(ctx, "ghost.key", `"x"`, ""); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("Set unknown = %v", err)
	}
	if _, err := r.GetRaw(ctx, "ghost.key"); err == nil {
		t.Fatalf("GetRaw unknown = nil")
	}
}

func TestRegistryValidationRejectsBadShape(t *testing.T) {
	t.Parallel()
	r := newRegistry(t)
	r.Register(config.Spec{Key: "a.s", Type: config.TypeString})
	r.Register(config.Spec{Key: "a.i", Type: config.TypeInt})
	r.Register(config.Spec{Key: "a.o", Type: config.TypeObject})

	ctx := context.Background()
	cases := []struct{ key, val string }{
		{"a.s", `42`},       // not a string
		{"a.s", `{"x":1}`},  // not a string
		{"a.i", `"hi"`},     // not an int
		{"a.i", `1.5`},      // not an integer
		{"a.o", `42`},       // not an object
		{"a.o", `[1,2,3]`},  // not an object
		{"a.o", `not json`}, // not JSON
	}
	for _, tc := range cases {
		if err := r.Set(ctx, tc.key, tc.val, ""); err == nil {
			t.Errorf("Set(%q, %q) = nil, want validation error", tc.key, tc.val)
		}
	}
}

func TestRegistryRedaction(t *testing.T) {
	t.Parallel()
	r := newRegistry(t)
	r.Register(config.Spec{
		Key:     "transport.postmark",
		Type:    config.TypeObject,
		Secrets: []string{"token"},
	})

	ctx := context.Background()
	if err := r.Set(ctx, "transport.postmark", `{"token":"abc-xyz","from":"x@y.com"}`, ""); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Plaintext via GetRaw / GetObject — consumers see real values.
	raw, _ := r.GetRaw(ctx, "transport.postmark")
	if !strings.Contains(raw, "abc-xyz") {
		t.Fatalf("GetRaw should not redact: %q", raw)
	}

	// Redacted via GetRedacted — for CLI output.
	redacted, _ := r.GetRedacted(ctx, "transport.postmark")
	if strings.Contains(redacted, "abc-xyz") {
		t.Fatalf("GetRedacted leaked secret: %q", redacted)
	}
	if !strings.Contains(redacted, "x@y.com") {
		t.Fatalf("GetRedacted clobbered non-secret: %q", redacted)
	}
}

func TestRegistryRegisterTwicePanics(t *testing.T) {
	t.Parallel()
	r := newRegistry(t)
	r.Register(config.Spec{Key: "k", Type: config.TypeString})

	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic on double register")
		}
	}()
	r.Register(config.Spec{Key: "k", Type: config.TypeString})
}

func TestRegistryRegisterBadDefaultPanics(t *testing.T) {
	t.Parallel()
	r := newRegistry(t)
	defer func() {
		if recover() == nil {
			t.Fatalf("expected panic on bad default")
		}
	}()
	r.Register(config.Spec{Key: "k", Type: config.TypeInt, Default: `"hello"`})
}
