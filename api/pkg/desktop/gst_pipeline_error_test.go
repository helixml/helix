//go:build cgo && linux

package desktop

import (
	"errors"
	"strings"
	"testing"
)

// The zero-copy source's first-frame timeout must be recognisable (so the
// shared source can retry it) without changing what the user is shown.
func TestNoFirstFrameErrorIsRetryableAndKeepsMessage(t *testing.T) {
	g := &GstPipeline{}
	err := g.createUserFriendlyError(
		"no video frame received from the compositor within 10s — the desktop may not be rendering",
		"src/pipewiresrc/imp.rs(970)", "pipewirezerocopysrc0")
	if !errors.Is(err, ErrNoFirstFrame) {
		t.Fatalf("want ErrNoFirstFrame, got %v", err)
	}
	if !strings.HasPrefix(err.Error(), "Video streaming error.") {
		t.Fatalf("user-facing message changed: %q", err.Error())
	}

	other := g.createUserFriendlyError("Internal data stream error", "", "nvh264enc0")
	if errors.Is(other, ErrNoFirstFrame) {
		t.Fatalf("unrelated pipeline errors must stay terminal: %v", other)
	}
}
