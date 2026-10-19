package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAnthropicAliasFamily(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  string
	}{
		{"subscription alias", "claude-subscription", "opus"},
		{"bare opus", "opus", "opus"},
		{"bare sonnet", "sonnet", "sonnet"},
		{"bare haiku", "haiku", "haiku"},
		{"uppercase", "SONNET", "sonnet"},
		{"context window suffix", "opus[1m]", "opus"},
		{"sonnet context window", "sonnet[1m]", "sonnet"},
		{"full model id", "claude-opus-4-7", ""},
		{"family prefix without bracket", "opusx", ""},
		{"empty", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := anthropicAliasFamily(tc.model)
			assert.Equal(t, anthropicAliasFamily(tc.model), got)
		})
	}
}

func TestAnthropicFamilyVersion(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		wantFam   string
		wantMajor int
		wantMinor int
		wantOK    bool
	}{
		{"opus 4.7", "claude-opus-4-7", "opus", 4, 7, true},
		{"sonnet 4.6 dated", "claude-sonnet-4-6-20260115", "sonnet", 4, 6, true},
		{"haiku 4.5 dated", "claude-haiku-4-5-20251001", "haiku", 4, 5, true},
		{"major only", "claude-opus-5", "opus", 5, 0, true},
		{"dotted minor", "claude-sonnet-4.5", "sonnet", 4, 5, true},
		{"uppercase", "CLAUDE-OPUS-4-8", "opus", 4, 8, true},
		{"fast variant excluded", "claude-opus-4-6-fast", "", 0, 0, false},
		{"legacy ordering", "claude-3-7-sonnet-20250219", "", 0, 0, false},
		{"non-anthropic", "gpt-6-sol", "", 0, 0, false},
		{"empty", "", "", 0, 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.NotPanics(t, func() {
				fam, major, minor, ok := anthropicFamilyVersion(tc.model)
				if ok {
					assert.NotEmpty(t, fam)
					assert.GreaterOrEqual(t, major, 0)
					assert.GreaterOrEqual(t, minor, 0)
				}
			})
		})
	}
}

func TestTrimAnthropicDateSuffix(t *testing.T) {
	tests := []struct {
		name string
		slug string
		want string
	}{
		{"dated sonnet", "claude-sonnet-4-5-20250929", "claude-sonnet-4-5"},
		{"dated haiku", "claude-haiku-4-5-20251001", "claude-haiku-4-5"},
		{"undated", "claude-opus-4-7", "claude-opus-4-7"},
		{"seven digit tail kept", "claude-opus-4-7-2025092", "claude-opus-4-7-2025092"},
		{"date not at end", "claude-sonnet-4-5-20250929-v1", "claude-sonnet-4-5-20250929-v1"},
		{"empty", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := trimAnthropicDateSuffix(tc.slug)
			assert.Equal(t, got, trimAnthropicDateSuffix(got))
		})
	}
}
