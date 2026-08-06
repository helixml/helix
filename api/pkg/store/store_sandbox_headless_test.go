package store

import "testing"

func TestSandboxVersionKeyForHeadlessACP(t *testing.T) {
	if got := sandboxVersionKey("headless"); got != "headless-acp" {
		t.Fatalf("sandboxVersionKey(headless) = %q, want headless-acp", got)
	}
	if got := sandboxVersionKey("ubuntu"); got != "ubuntu" {
		t.Fatalf("sandboxVersionKey(ubuntu) = %q, want ubuntu", got)
	}
}
