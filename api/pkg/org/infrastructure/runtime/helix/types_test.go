package helix

import (
	"testing"

	"github.com/helixml/helix/api/pkg/types"
)

func TestIsTerminalOutput(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"complete", "error", "interrupted"} {
		if !IsTerminalOutput(types.SessionOutputResponse{Status: status}) {
			t.Errorf("status %q must be terminal", status)
		}
	}
	for _, status := range []string{"", "waiting", "editing"} {
		if IsTerminalOutput(types.SessionOutputResponse{Status: status}) {
			t.Errorf("status %q must not be terminal", status)
		}
	}
}
