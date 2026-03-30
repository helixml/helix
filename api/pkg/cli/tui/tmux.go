package tui

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// TmuxConfig holds parsed keybindings from the user's tmux.conf.
type TmuxConfig struct {
	Prefix     string // e.g. "ctrl+b", "ctrl+a"
	SplitH     string // horizontal split key (default: `"`)
	SplitV     string // vertical split key (default: `%`)
	PaneNext   string // next pane (default: `o`)
	PanePrev   string // previous pane (default: `;`)
	PaneLeft   string // select pane left (default: unset)
	PaneDown   string // select pane down (default: unset)
	PaneUp     string // select pane up (default: unset)
	PaneRight  string // select pane right (default: unset)
	ClosePane  string // close pane (default: `x`)
	Detach     string // detach (default: `d`)
}

// DefaultTmuxConfig returns exact tmux defaults.
func DefaultTmuxConfig() *TmuxConfig {
	return &TmuxConfig{
		Prefix:    "ctrl+b",
		SplitH:    "\"",
		SplitV:    "%",
		PaneNext:  "o",
		PanePrev:  ";",
		ClosePane: "x",
		Detach:    "d",
	}
}

// LoadTmuxConfig parses the user's tmux.conf and overlays onto defaults.
func LoadTmuxConfig() *TmuxConfig {
	cfg := DefaultTmuxConfig()

	// Try standard locations
	home, err := os.UserHomeDir()
	if err != nil {
		return cfg
	}

	paths := []string{
		filepath.Join(home, ".tmux.conf"),
		filepath.Join(home, ".config", "tmux", "tmux.conf"),
	}

	for _, p := range paths {
		if err := parseTmuxFile(p, cfg); err == nil {
			break // use the first one found
		}
	}

	return cfg
}

func parseTmuxFile(path string, cfg *TmuxConfig) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parseTmuxLine(line, cfg)
	}
	return scanner.Err()
}

func parseTmuxLine(line string, cfg *TmuxConfig) {
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return
	}

	// Parse: set -g prefix C-a
	// Also: set-option -g prefix C-a
	if (fields[0] == "set" || fields[0] == "set-option") && len(fields) >= 4 {
		if fields[1] == "-g" && fields[2] == "prefix" {
			cfg.Prefix = tmuxKeyToTeaKey(fields[3])
		}
		return
	}

	// Parse: bind X split-window -h/-v
	// Also: bind-key X split-window -h/-v
	if fields[0] == "bind" || fields[0] == "bind-key" {
		parseTmuxBind(fields[1:], cfg)
		return
	}
}

func parseTmuxBind(args []string, cfg *TmuxConfig) {
	if len(args) < 2 {
		return
	}

	key := args[0]
	// Skip -r flag (repeat)
	if key == "-r" {
		if len(args) < 3 {
			return
		}
		key = args[1]
		args = args[2:]
	} else {
		args = args[1:]
	}

	cmd := args[0]

	switch cmd {
	case "split-window":
		// split-window -h = vertical split (new pane to the right)
		// split-window -v = horizontal split (new pane below)
		// split-window (no flag) = same as -v
		for _, arg := range args[1:] {
			if arg == "-h" {
				cfg.SplitV = key // -h is vertical in tmux terminology
				return
			}
		}
		cfg.SplitH = key

	case "select-pane":
		for _, arg := range args[1:] {
			switch arg {
			case "-L":
				cfg.PaneLeft = key
			case "-D":
				cfg.PaneDown = key
			case "-R":
				cfg.PaneRight = key
			case "-U":
				cfg.PaneUp = key
			}
		}

	case "kill-pane":
		cfg.ClosePane = key

	case "detach-client", "detach":
		cfg.Detach = key
	}
}

// tmuxKeyToTeaKey converts tmux key notation (e.g. "C-a") to bubbletea
// key notation (e.g. "ctrl+a").
func tmuxKeyToTeaKey(tmuxKey string) string {
	if strings.HasPrefix(tmuxKey, "C-") {
		return "ctrl+" + strings.ToLower(tmuxKey[2:])
	}
	if strings.HasPrefix(tmuxKey, "M-") {
		return "alt+" + strings.ToLower(tmuxKey[2:])
	}
	return tmuxKey
}

// IsInTmux returns true if we're running inside tmux.
func IsInTmux() bool {
	return os.Getenv("TMUX") != ""
}
