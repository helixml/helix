package desktop

import "testing"

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
