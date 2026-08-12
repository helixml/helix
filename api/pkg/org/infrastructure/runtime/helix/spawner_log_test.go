package helix

import "testing"

func TestSanitizeLogValueRemovesLineBreaks(t *testing.T) {
	got := sanitizeLogValue("upstream\r\nforged\nentry\rtail")
	if got != "upstreamforgedentrytail" {
		t.Fatalf("sanitizeLogValue() = %q", got)
	}
}
