package runtime

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
)

func TestNoopWorkspaceSyncIsAlwaysNil(t *testing.T) {
	t.Parallel()
	var ws WorkspaceSync = NoopWorkspaceSync{}
	if err := ws.MirrorFile(context.Background(), "org-test", "w-test", "role.md", "x", "msg"); err != nil {
		t.Errorf("NoopWorkspaceSync.MirrorFile: %v", err)
	}
}

func TestNoopHireHandlerIsAlwaysNil(t *testing.T) {
	t.Parallel()
	var h HireHook = NoopHireHook{}
	// Empty and non-empty user IDs both must succeed.
	if err := h.OnHire(context.Background(), "org-test", "w-test", ""); err != nil {
		t.Errorf("OnHire empty userID: %v", err)
	}
	if err := h.OnHire(context.Background(), "org-test", "w-test", "u-phil"); err != nil {
		t.Errorf("OnHire with userID: %v", err)
	}
}

func TestValidateWorkspaceName(t *testing.T) {
	t.Parallel()
	for _, ok := range []string{"role.md", "identity.md", "sub/file.md"} {
		if err := ValidateWorkspaceName(ok); err != nil {
			t.Errorf("name %q rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "/abs", "../up", "a/../b"} {
		if err := ValidateWorkspaceName(bad); err == nil {
			t.Errorf("name %q accepted (should fail)", bad)
		}
	}
}

var _ orgchart.BotID = orgchart.BotID("w-test") // import-pin
