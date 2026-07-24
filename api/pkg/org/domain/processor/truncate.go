package processor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// KindTruncate caps a Message body to a maximum byte length. It exists
// mainly to prove the registry is generic — a second Kind is one file +
// one constant + one map entry, with no edits to the umbrella. Always
// passes (1 input → 1 output).
const KindTruncate Kind = "truncate"

// truncateConfig is the parsed config for KindTruncate.
type truncateConfig struct {
	MaxBytes int `json:"max_bytes"`
}

// Validate enforces a single unconditional output and a positive cap.
func (c truncateConfig) Validate(out []Output) error {
	if len(out) != 1 {
		return fmt.Errorf("truncate processor needs exactly 1 output, got %d", len(out))
	}
	if out[0].Match != "" {
		return errors.New("truncate output must not carry a match predicate (transform is unconditional)")
	}
	if c.MaxBytes <= 0 {
		return fmt.Errorf("truncate max_bytes must be > 0, got %d", c.MaxBytes)
	}
	return nil
}

// Process returns the Message with its body capped to MaxBytes,
// rune-safely (never splitting a multi-byte rune).
func (c truncateConfig) Process(_ context.Context, in streaming.Message, out []Output) ([]Result, error) {
	m := in
	m.Body = runeSafeTruncate(in.Body, c.MaxBytes)
	return []Result{{TopicID: out[0].TopicID, Message: m}}, nil
}

// truncate is the Strategy for KindTruncate.
type truncate struct{}

// ParseConfig decodes the raw blob into a truncateConfig. An empty blob
// yields MaxBytes == 0, which Validate rejects.
func (truncate) ParseConfig(raw json.RawMessage) (Config, error) {
	if len(raw) == 0 {
		return truncateConfig{}, nil
	}
	var c truncateConfig
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("parse truncate config: %w", err)
	}
	return c, nil
}

// runeSafeTruncate returns s capped to at most maxBytes bytes without
// splitting a UTF-8 rune. If the byte boundary lands in the middle of a
// rune, it backs up to the previous rune boundary. maxBytes <= 0
// returns the empty string. Shared with the template Kind's `trunc`
// FuncMap helper.
func runeSafeTruncate(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}
