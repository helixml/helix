package tui

import (
	"testing"
)

func TestSlashCommandMatch(t *testing.T) {
	reg := NewSlashCommandRegistry()

	// Empty prefix returns all
	all := reg.Match("")
	if len(all) == 0 {
		t.Error("expected all commands for empty prefix")
	}

	// "m" should match "mcp" and "model"
	matches := reg.Match("m")
	if len(matches) != 2 {
		t.Errorf("expected 2 matches for 'm', got %d", len(matches))
	}

	// "mcp" exact match
	matches = reg.Match("mcp")
	if len(matches) != 1 || matches[0].Name != "mcp" {
		t.Error("expected exact match for 'mcp'")
	}

	// No match
	matches = reg.Match("zzz")
	if len(matches) != 0 {
		t.Error("expected no matches for 'zzz'")
	}
}

func TestParseSlashCommand(t *testing.T) {
	tests := []struct {
		input   string
		cmd     string
		args    string
	}{
		{"/mcp", "mcp", ""},
		{"/model gpt-4", "model", "gpt-4"},
		{"/approve", "approve", ""},
		{"hello", "", "hello"},
		{"/ ", "", ""},
	}

	for _, tt := range tests {
		cmd, args := ParseSlashCommand(tt.input)
		if cmd != tt.cmd {
			t.Errorf("ParseSlashCommand(%q) cmd = %q, want %q", tt.input, cmd, tt.cmd)
		}
		if args != tt.args {
			t.Errorf("ParseSlashCommand(%q) args = %q, want %q", tt.input, args, tt.args)
		}
	}
}

func TestIsSlashCommand(t *testing.T) {
	if !IsSlashCommand("/mcp") {
		t.Error("expected /mcp to be a slash command")
	}
	if IsSlashCommand("hello") {
		t.Error("expected 'hello' to not be a slash command")
	}
	if !IsSlashCommand("  /test") {
		t.Error("expected '  /test' to be a slash command (with leading spaces)")
	}
}
