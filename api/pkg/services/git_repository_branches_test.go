package services

import (
	"reflect"
	"testing"
)

func TestParseLsRemoteBranches(t *testing.T) {
	stdout := "aaaaaaaa\trefs/heads/main\n" +
		"bbbbbbbb\trefs/heads/feature/active\n" +
		"cccccccc\trefs/tags/v1.0.0\n" +
		"malformed\n"

	// A stale local branch is intentionally absent: only authoritative
	// upstream heads returned by ls-remote can enter this list.
	want := []string{"main", "feature/active"}
	if got := parseLsRemoteBranches(stdout); !reflect.DeepEqual(got, want) {
		t.Fatalf("parseLsRemoteBranches() = %v, want %v", got, want)
	}
}
