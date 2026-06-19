package processor_test

import (
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/processor"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

func newTruncateProc(t *testing.T, maxBytes int) processor.Processor {
	t.Helper()
	p, err := processor.NewProcessor(
		"p-cap", "Capper", "s-in", processor.KindTruncate,
		cfg(t, map[string]int{"max_bytes": maxBytes}), out("s-out"),
		"", time.Now(), "org-1",
	)
	if err != nil {
		t.Fatalf("NewProcessor: %v", err)
	}
	return p
}

func TestTruncateCapsBody(t *testing.T) {
	p := newTruncateProc(t, 5)
	res, err := p.Process(streaming.Message{Body: "abcdefghij"})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if got := res[0].Message.Body; got != "abcde" {
		t.Errorf("body = %q, want abcde", got)
	}
}

func TestTruncateShortBodyUnchanged(t *testing.T) {
	p := newTruncateProc(t, 100)
	res, _ := p.Process(streaming.Message{Body: "short"})
	if got := res[0].Message.Body; got != "short" {
		t.Errorf("body = %q, want short", got)
	}
}

func TestTruncateRuneSafe(t *testing.T) {
	// "é" is 2 bytes (0xC3 0xA9). Capping a string of three "é" at 5 bytes
	// must back up to a rune boundary (4 bytes = two "é"), never split one.
	body := strings.Repeat("é", 3) // 6 bytes
	p := newTruncateProc(t, 5)
	res, _ := p.Process(streaming.Message{Body: body})
	got := res[0].Message.Body
	if got != "éé" {
		t.Errorf("body = %q (% x), want éé", got, got)
	}
	if !utf8ValidWhole(got) {
		t.Errorf("truncation split a rune: % x", got)
	}
}

func utf8ValidWhole(s string) bool {
	return strings.ToValidUTF8(s, "�") == s
}

func TestTruncateRejectsNonPositiveMax(t *testing.T) {
	_, err := processor.NewProcessor(
		"p-zero", "Zero", "s-in", processor.KindTruncate,
		cfg(t, map[string]int{"max_bytes": 0}), out("s-out"),
		"", time.Now(), "org-1",
	)
	if err == nil {
		t.Fatal("want error for max_bytes=0, got nil")
	}
}
