package desktop

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCatInvocationAllowed(t *testing.T) {
	cases := []struct {
		name string
		cmd  []string
		want bool
	}{
		{"allow Codex credentials", []string{"cat", "/home/retro/.codex/auth.json"}, true},
		{"allow Codex stdout", []string{"cat", "/tmp/codex-auth-stdout.txt"}, true},
		{"allow Codex error", []string{"cat", "/tmp/codex-auth-error.txt"}, true},
		{"reject process environment", []string{"cat", "/proc/1/environ"}, false},
		{"reject traversal", []string{"cat", "/tmp/../proc/1/environ"}, false},
		{"reject flags", []string{"cat", "--", "/home/retro/.codex/auth.json"}, false},
		{"reject multiple files", []string{"cat", "/tmp/codex-auth-stdout.txt", "/etc/passwd"}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := catInvocationAllowed(tc.cmd); got != tc.want {
				t.Fatalf("catInvocationAllowed(%v) = %v, want %v", tc.cmd, got, tc.want)
			}
		})
	}
}

func TestExecRejectsUnsafeCommands(t *testing.T) {
	server := NewServer(Config{}, slog.Default())
	for _, body := range []string{
		`{"command":["cat","/proc/1/environ"]}`,
		`{"command":["claude","--version"]}`,
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/exec", strings.NewReader(body))
		server.handleExec(recorder, request)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("body %s returned %d, want %d", body, recorder.Code, http.StatusForbidden)
		}
	}
}

func TestGitInvocationAllowed(t *testing.T) {
	cases := []struct {
		name string
		cmd  []string
		want bool
	}{
		{"allow user.name", []string{"git", "config", "--global", "user.name", "Alice"}, true},
		{"allow user.email", []string{"git", "config", "--global", "user.email", "alice@example.com"}, true},
		{"reject missing subcommand", []string{"git"}, false},
		{"reject non-config subcommand", []string{"git", "clone", "--global", "user.name", "x"}, false},
		{"reject non-global scope", []string{"git", "config", "--system", "user.name", "x"}, false},
		{"reject other key", []string{"git", "config", "--global", "credential.helper", "store"}, false},
		{"reject extra args", []string{"git", "config", "--global", "user.name", "Alice", "extra"}, false},
		{"reject flag as value", []string{"git", "config", "--global", "user.name", "--some-flag"}, false},
		{"reject short flag as value", []string{"git", "config", "--global", "user.email", "-x"}, false},
		{"reject wrong binary", []string{"not-git", "config", "--global", "user.name", "x"}, false},
		{"reject empty slice", []string{}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := gitInvocationAllowed(tc.cmd)
			if got != tc.want {
				t.Fatalf("gitInvocationAllowed(%v) = %v, want %v", tc.cmd, got, tc.want)
			}
		})
	}
}
