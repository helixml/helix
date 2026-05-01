// Package config holds the registry of operational-config keys and
// the typed accessors subsystems use to read them at runtime.
//
// Configuration is stored in the SQLite `configs` table and mutated
// only through the helix-org config CLI — never via MCP. Each
// subsystem (spawner, dispatcher, future transports, etc.) registers
// the keys it owns at startup, declaring schema, default, and which
// JSON paths are secret. The CLI's set/get/list go through this
// registry for validation and redaction; consumers go through it for
// typed reads.
//
// See design/config.md for the full spec and the rationale behind
// the org-graph-vs-ops split.
package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/helixml/helix-org/domain"
	"github.com/helixml/helix-org/store"
)

// Spec describes a single configurable key. Subsystems register
// their Specs at startup; the registry uses them to validate set
// operations and redact secrets on read.
//
// Schema validation is intentionally done by a small custom checker
// (TypeOf) rather than full JSON Schema — the surface is narrow
// (string, int, object), the dependency footprint stays small, and
// the error messages are clearer for operators.
type Spec struct {
	// Key is the dot-namespaced identifier (e.g. "claude.bin",
	// "transport.postmark"). Owned by exactly one subsystem.
	Key string

	// Type is what shape the JSON value must be. The CLI rejects
	// values that don't match.
	Type ValueType

	// Default is the JSON value used when the row is missing.
	// Empty string means no default — Required keys without a row
	// and without a default error on read.
	Default string

	// Required means consumer reads error if no row exists and no
	// default is set. False means the key is optional (subsystem
	// boots dormant when missing).
	Required bool

	// Secrets lists JSON paths within the value (object kind only)
	// that should be redacted on get/list. e.g. ["token"] for
	// transport.postmark redacts only the "token" field.
	Secrets []string

	// Description is a one-line summary shown by `config list` so
	// operators can discover what's settable.
	Description string
}

// ValueType is the small set of value shapes the registry validates.
type ValueType string

const (
	TypeString ValueType = "string"
	TypeInt    ValueType = "int"
	TypeObject ValueType = "object"
)

// Registry is the central coordinator: it holds Specs and reads/writes
// configs through the store.
//
// Specs are registered once at startup; reads happen on every
// operation. There's no in-memory cache — SQLite is fast enough and
// live updates Just Work. If a hot path later proves cache-worthy,
// add a TTL cache layered on top.
type Registry struct {
	store store.Configs

	mu    sync.RWMutex
	specs map[string]Spec
}

// New returns a Registry bound to the given Configs store.
func New(s store.Configs) *Registry {
	return &Registry{store: s, specs: make(map[string]Spec)}
}

// Register declares a config key. Re-registering the same key panics;
// each key has exactly one owner.
func (r *Registry) Register(spec Spec) {
	if spec.Key == "" {
		panic("config: register with empty key")
	}
	if spec.Type == "" {
		panic(fmt.Sprintf("config: register %q without type", spec.Key))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.specs[spec.Key]; exists {
		panic(fmt.Sprintf("config: key %q already registered", spec.Key))
	}
	if spec.Default != "" {
		if err := validateValue(spec, spec.Default); err != nil {
			panic(fmt.Sprintf("config: register %q with invalid default: %v", spec.Key, err))
		}
	}
	r.specs[spec.Key] = spec
}

// Spec returns the registered spec for a key, ok=false if not registered.
func (r *Registry) Spec(key string) (Spec, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.specs[key]
	return s, ok
}

// Specs returns every registered Spec, sorted by key. Used by `config list`.
func (r *Registry) Specs() []Spec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Spec, 0, len(r.specs))
	for _, s := range r.specs {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// Set validates the value against the registered Spec and upserts the
// row. Unknown keys (not registered) are rejected — the registry is
// the source of truth for what's settable.
//
// updatedBy is the WorkerID for the audit column; empty is allowed
// today (auth not yet wired) but reserved.
func (r *Registry) Set(ctx context.Context, key, value string, updatedBy domain.WorkerID) error {
	spec, ok := r.Spec(key)
	if !ok {
		return fmt.Errorf("unknown config key %q (no subsystem has registered it)", key)
	}
	if err := validateValue(spec, value); err != nil {
		return fmt.Errorf("validate %q: %w", key, err)
	}
	cfg, err := domain.NewConfig(key, value, time.Now().UTC(), updatedBy)
	if err != nil {
		return err
	}
	return r.store.Set(ctx, cfg)
}

// Delete removes the row. Subsequent reads fall back to the registered
// default (if any), or error (if Required).
func (r *Registry) Delete(ctx context.Context, key string) error {
	if _, ok := r.Spec(key); !ok {
		return fmt.Errorf("unknown config key %q", key)
	}
	return r.store.Delete(ctx, key)
}

// GetRaw returns the raw JSON value — the row's value if set,
// otherwise the registered default. Returns ErrNotConfigured when no
// row exists, no default is set, and the spec is not Required (caller
// can treat as "feature disabled"). Returns a wrapped error when
// Required and missing.
func (r *Registry) GetRaw(ctx context.Context, key string) (string, error) {
	spec, ok := r.Spec(key)
	if !ok {
		return "", fmt.Errorf("unknown config key %q", key)
	}
	cfg, err := r.store.Get(ctx, key)
	if err == nil {
		return cfg.Value, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return "", err
	}
	if spec.Default != "" {
		return spec.Default, nil
	}
	if spec.Required {
		return "", fmt.Errorf("config %q: %w", key, ErrRequired)
	}
	return "", ErrNotConfigured
}

// GetRedacted returns the value with secret JSON paths replaced by
// "..." — for `config get` and `config list` output. For object
// values where Secrets is set, returns valid JSON with the redacted
// fields. For non-object values or empty Secrets, returns the raw
// value.
func (r *Registry) GetRedacted(ctx context.Context, key string) (string, error) {
	raw, err := r.GetRaw(ctx, key)
	if err != nil {
		return "", err
	}
	spec, _ := r.Spec(key)
	return redact(spec, raw)
}

// GetString reads a string-typed config and returns its Go value.
// Errors if the spec isn't string-typed or the value doesn't parse.
func (r *Registry) GetString(ctx context.Context, key string) (string, error) {
	raw, err := r.GetRaw(ctx, key)
	if err != nil {
		return "", err
	}
	spec, _ := r.Spec(key)
	if spec.Type != TypeString {
		return "", fmt.Errorf("config %q: spec type is %s, not string", key, spec.Type)
	}
	var s string
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return "", fmt.Errorf("decode string for %q: %w", key, err)
	}
	return s, nil
}

// GetInt reads an int-typed config and returns its Go value.
func (r *Registry) GetInt(ctx context.Context, key string) (int64, error) {
	raw, err := r.GetRaw(ctx, key)
	if err != nil {
		return 0, err
	}
	spec, _ := r.Spec(key)
	if spec.Type != TypeInt {
		return 0, fmt.Errorf("config %q: spec type is %s, not int", key, spec.Type)
	}
	var n int64
	if err := json.Unmarshal([]byte(raw), &n); err != nil {
		return 0, fmt.Errorf("decode int for %q: %w", key, err)
	}
	return n, nil
}

// GetObject decodes the value into the given destination, which must
// be a pointer. Errors if the spec isn't object-typed.
func (r *Registry) GetObject(ctx context.Context, key string, dst any) error {
	raw, err := r.GetRaw(ctx, key)
	if err != nil {
		return err
	}
	spec, _ := r.Spec(key)
	if spec.Type != TypeObject {
		return fmt.Errorf("config %q: spec type is %s, not object", key, spec.Type)
	}
	if err := json.Unmarshal([]byte(raw), dst); err != nil {
		return fmt.Errorf("decode object for %q: %w", key, err)
	}
	return nil
}

// Sentinel errors for callers that want to distinguish "not yet
// configured" from "required but missing" from "doesn't exist".
var (
	// ErrNotConfigured indicates the key has no row and no default,
	// and isn't required. Subsystems treat this as "feature dormant".
	ErrNotConfigured = errors.New("not configured")
	// ErrRequired indicates a required key is missing.
	ErrRequired = errors.New("required config key not set")
)

func validateValue(spec Spec, raw string) error {
	if raw == "" {
		return errors.New("value is empty")
	}
	switch spec.Type {
	case TypeString:
		var s string
		if err := json.Unmarshal([]byte(raw), &s); err != nil {
			return fmt.Errorf("not a JSON string: %w", err)
		}
	case TypeInt:
		var n json.Number
		dec := json.NewDecoder(strings.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&n); err != nil {
			return fmt.Errorf("not a number: %w", err)
		}
		if _, err := n.Int64(); err != nil {
			return fmt.Errorf("not an integer: %w", err)
		}
	case TypeObject:
		var m map[string]json.RawMessage
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			return fmt.Errorf("not a JSON object: %w", err)
		}
	default:
		return fmt.Errorf("unknown spec type %q", spec.Type)
	}
	return nil
}

// redact replaces JSON paths listed in spec.Secrets with "..." in the
// value's JSON representation. Only object values support secret
// fields; for other types or empty Secrets, raw is returned unchanged.
func redact(spec Spec, raw string) (string, error) {
	if len(spec.Secrets) == 0 || spec.Type != TypeObject {
		return raw, nil
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return raw, nil // already malformed; let the consumer's typed Get error
	}
	for _, path := range spec.Secrets {
		// Path is a top-level field name today. If we ever need
		// nested paths (a.b.c), split on "." and walk. Keeping it
		// flat for Phase 1 — no real config has nested secrets yet.
		if _, exists := obj[path]; exists {
			obj[path] = "..."
		}
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return "", fmt.Errorf("re-encode redacted value: %w", err)
	}
	return string(out), nil
}
