package orgchart

import (
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/stretchr/testify/require"
)

func fpNode(t *testing.T, content string, tools []tool.Name) Node {
	t.Helper()
	n, err := NewNode("b-fp", content, tools, time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC), "org-fp")
	require.NoError(t, err)
	return n
}

// Reordering a Bot's tool list is not a config change — the MCP surface
// is a set. Without sorting, dragging a chip in the tool picker would
// nag every operator to restart.
func TestRestartFingerprint_StableAcrossToolReordering(t *testing.T) {
	a := fpNode(t, "# bot", []tool.Name{"chat", "ask_human", "reports"})
	b := fpNode(t, "# bot", []tool.Name{"reports", "chat", "ask_human"})

	require.Equal(t, RestartFingerprint(a), RestartFingerprint(b))
}

func TestRestartFingerprint_ChangesOnToolAdd(t *testing.T) {
	before := fpNode(t, "# bot", []tool.Name{"chat"})
	after := fpNode(t, "# bot", []tool.Name{"chat", "get_secret"})

	require.NotEqual(t, RestartFingerprint(before), RestartFingerprint(after))
}

func TestRestartFingerprint_ChangesOnToolRemove(t *testing.T) {
	before := fpNode(t, "# bot", []tool.Name{"chat", "get_secret"})
	after := fpNode(t, "# bot", []tool.Name{"chat"})

	require.NotEqual(t, RestartFingerprint(before), RestartFingerprint(after))
}

func TestRestartFingerprint_ChangesOnContentEdit(t *testing.T) {
	before := fpNode(t, "# bot", []tool.Name{"chat"})
	after := fpNode(t, "# bot, now with feeling", []tool.Name{"chat"})

	require.NotEqual(t, RestartFingerprint(before), RestartFingerprint(after))
}

// These reach a running sandbox without a restart, so editing them must
// not raise the banner. This test is the guard against the fingerprint
// quietly widening into every Node field.
func TestRestartFingerprint_IgnoresFieldsThatHotApply(t *testing.T) {
	base := fpNode(t, "# bot", []tool.Name{"chat"})

	want := RestartFingerprint(base)
	require.Equal(t, want, RestartFingerprint(base.WithName("Chief of Staff")))
	require.Equal(t, want, RestartFingerprint(base.WithPreserveContext(true)))
	require.Equal(t, want, RestartFingerprint(base.WithProjectIDs([]string{"prj_01", "prj_02"})))
}

// Guards the per-name 0x00 terminator. Concatenated without it, tools
// ["a","b"] and ["ab"] both hash the bytes "ab" — so revoking "b" from a
// bot that also has "a" would look like no change at all.
func TestRestartFingerprint_ToolNamesCannotRunTogether(t *testing.T) {
	a := fpNode(t, "c", []tool.Name{"a", "b"})
	b := fpNode(t, "c", []tool.Name{"ab"})

	require.NotEqual(t, RestartFingerprint(a), RestartFingerprint(b))
}

// Guards the 0x01 domain separator between the tool list and the content.
// The per-name 0x00 terminator alone cannot disambiguate these two: with
// no separator, one tool "a" plus content "b" and no tools at all plus
// content "a\x00b" both hash the bytes 61 00 62. Content is free-form
// markdown, so a NUL in it is reachable.
func TestRestartFingerprint_ToolsCannotRunIntoContent(t *testing.T) {
	a := fpNode(t, "b", []tool.Name{"a"})
	b := fpNode(t, "a\x00b", nil)

	require.NotEqual(t, RestartFingerprint(a), RestartFingerprint(b))
}
