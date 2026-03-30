package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultTmuxConfig(t *testing.T) {
	cfg := DefaultTmuxConfig()

	if cfg.Prefix != "ctrl+b" {
		t.Errorf("expected prefix ctrl+b, got %s", cfg.Prefix)
	}
	if cfg.SplitH != "\"" {
		t.Errorf("expected splitH '\"', got %s", cfg.SplitH)
	}
	if cfg.SplitV != "%" {
		t.Errorf("expected splitV '%%', got %s", cfg.SplitV)
	}
	if cfg.PaneNext != "o" {
		t.Errorf("expected paneNext 'o', got %s", cfg.PaneNext)
	}
	if cfg.ClosePane != "x" {
		t.Errorf("expected closePane 'x', got %s", cfg.ClosePane)
	}
	if cfg.Detach != "d" {
		t.Errorf("expected detach 'd', got %s", cfg.Detach)
	}
}

func TestParseTmuxConfig(t *testing.T) {
	// Write a test tmux.conf
	dir := t.TempDir()
	confPath := filepath.Join(dir, ".tmux.conf")

	content := `# act like vim
setw -g mode-keys vi
bind h select-pane -L
bind j select-pane -D
bind k select-pane -U
bind l select-pane -R

# act like GNU screen
unbind C-b
set -g prefix C-a
bind-key C-a send-prefix

# splits
unbind %
bind | split-window -h
unbind '"'
bind - split-window -v
`
	if err := os.WriteFile(confPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultTmuxConfig()
	if err := parseTmuxFile(confPath, cfg); err != nil {
		t.Fatal(err)
	}

	if cfg.Prefix != "ctrl+a" {
		t.Errorf("expected prefix ctrl+a, got %s", cfg.Prefix)
	}
	if cfg.SplitV != "|" {
		t.Errorf("expected splitV '|', got %s", cfg.SplitV)
	}
	if cfg.SplitH != "-" {
		t.Errorf("expected splitH '-', got %s", cfg.SplitH)
	}
	if cfg.PaneLeft != "h" {
		t.Errorf("expected paneLeft 'h', got %s", cfg.PaneLeft)
	}
	if cfg.PaneDown != "j" {
		t.Errorf("expected paneDown 'j', got %s", cfg.PaneDown)
	}
	if cfg.PaneUp != "k" {
		t.Errorf("expected paneUp 'k', got %s", cfg.PaneUp)
	}
	if cfg.PaneRight != "l" {
		t.Errorf("expected paneRight 'l', got %s", cfg.PaneRight)
	}
}

func TestTmuxKeyConversion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"C-a", "ctrl+a"},
		{"C-b", "ctrl+b"},
		{"M-x", "alt+x"},
		{"F1", "F1"},
	}

	for _, tt := range tests {
		got := tmuxKeyToTeaKey(tt.input)
		if got != tt.expected {
			t.Errorf("tmuxKeyToTeaKey(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
