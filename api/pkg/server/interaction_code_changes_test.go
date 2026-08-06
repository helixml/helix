package server

import "testing"

func TestCheckpointRefSanitizesIdentifiers(t *testing.T) {
	got := checkpointRef("ses_01/../../main", "int_01:two", "before")
	want := "refs/helix/checkpoints/ses_01-----main/int_01-two/before"
	if got != want {
		t.Fatalf("checkpointRef() = %q, want %q", got, want)
	}
}
