package webservice

import (
	"testing"
)

func TestShellEscape(t *testing.T) {
	cases := map[string]string{
		"":                                "''",
		"simple":                          "'simple'",
		"with spaces":                     "'with spaces'",
		"path/to/file.txt":                "'path/to/file.txt'",
		"abc'def":                         `'abc'\''def'`,
		"; rm -rf / # injection attempt":  "'; rm -rf / # injection attempt'",
		"$(cat /etc/passwd)":              "'$(cat /etc/passwd)'",
		"`hostname`":                      "'`hostname`'",
		"https://user:p@ss@github.com/o/r": "'https://user:p@ss@github.com/o/r'",
	}
	for in, want := range cases {
		if got := shellEscape(in); got != want {
			t.Errorf("shellEscape(%q) = %q, want %q", in, got, want)
		}
	}
}
