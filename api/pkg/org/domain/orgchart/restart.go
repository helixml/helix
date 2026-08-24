package orgchart

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
)

// RestartFingerprint hashes exactly the Node config a running sandbox
// consumes at startup and that nothing hot-applies afterwards:
//
//   - Tools — the coding agent caches tools/list at MCP client init, and
//     nothing pushes an update (notifications/tools/list_changed is not
//     implemented on either side).
//   - Content — materialized into AGENTS.md/CLAUDE.md before the desktop
//     starts. SyncAgentProfile rewrites them in a live container, but only
//     during an activation, never on save.
//
// Deliberately excluded, because each already reaches a running sandbox
// without a restart: runtime/model/provider/effort (hot-switched through
// settings.json by the switch-agent path), worker secrets (fetched live
// through the get_secret MCP tool), and Name/PreserveContext/ProjectIDs/
// triggers (evaluated server-side per request or dispatch). Widening this
// set makes the banner fire on changes that need nothing, which teaches
// operators to ignore it.
//
// Tools are sorted so reordering is not a change. The 0x00 terminator per
// name and the 0x01 domain separator before Content stop a tool name and
// the instruction text from running together into the same bytes.
func RestartFingerprint(n Node) string {
	names := make([]string, 0, len(n.Tools))
	for _, t := range n.Tools {
		names = append(names, string(t))
	}
	sort.Strings(names)

	h := sha256.New()
	for _, name := range names {
		h.Write([]byte(name))
		h.Write([]byte{0x00})
	}
	h.Write([]byte{0x01})
	h.Write([]byte(n.Content))
	return hex.EncodeToString(h.Sum(nil))
}
