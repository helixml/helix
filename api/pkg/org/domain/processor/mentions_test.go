package processor

import "testing"

func TestMentionsWord(t *testing.T) {
	cases := []struct {
		name, text string
		want       bool
	}{
		{"alice", "hey alice can you help", true},
		{"alice", "Alice, please review", true},  // case-insensitive
		{"alice", "ALICE!", true},
		{"sam", "the salmon was sampled", false}, // word boundary
		{"sam", "ask Sam about it", true},
		{"data-bot", "ping data-bot now", true},      // hyphenated name
		{"w-jokebot", "hey w-jokebot tell a joke", true}, // full worker id (w- prefix)
		{"w-jokebot", "jokebot tell a joke", false},      // bare slug must NOT match the full-id route
		{"bob", "no mention here", false},
		{"", "anything at all", false},           // empty name never matches
		{"alice", "", false},
	}
	for _, c := range cases {
		if got := mentionsWord(c.name, c.text); got != c.want {
			t.Errorf("mentions(%q,%q)=%v want %v", c.name, c.text, got, c.want)
		}
	}
}
