package server

import (
	"net/http"

	"github.com/helixml/helix/api/pkg/types"
)

// Read-only desktop streaming.
//
// /ws/stream is one socket in both directions: H.264 frames out, and input IN —
// keyboard (0x10), mouse click/absolute/relative (0x11-0x13) and touch (0x14),
// dispatched by desktop.handleStreamInputMessageWithClient.
//
// That is right for the operator of a spec task, who owns the desktop. It is
// wrong for an embed viewer, who is a member of the public watching their own
// agent work on someone else's public site. Letting them stream video is
// reasonable; letting them type into a container that holds a terminal, a
// browser and git credentials is not.
//
// So the stream is allowed for embed keys and the input half is dropped.
//
// WHERE THIS IS ENFORCED, AND WHY THERE. The proxy between the API and the
// sandbox is a raw byte pump — it does not parse WebSocket frames, so it cannot
// filter message types without teaching it the framing. But the API *constructs*
// the upgrade request it sends to the sandbox rather than forwarding the
// client's, so it can state the caller's privilege in a header the browser has
// no way to set or clear. The sandbox then simply ignores input on connections
// marked read-only.
//
// This is a capability decided from the credential, not from anything the client
// says — the same principle as the embed path allowlist.

// desktopReadOnlyHeader marks a desktop stream connection as view-only. Set by
// this server on the upgrade it sends to the sandbox; never accepted from a
// client.
const desktopReadOnlyHeader = "X-Helix-Readonly"

// desktopStreamIsReadOnly reports whether this caller may only watch.
//
// Fails closed for the credential type that is handed to a browser: an embed key
// is the only credential we deliberately put on an untrusted page, so it is the
// one that must never carry input. Every other caller reaches the desktop
// through a credential that already implies ownership of the session.
func desktopStreamIsReadOnly(r *http.Request) bool {
	user := getRequestUser(r)
	if user == nil {
		// No resolved user should not reach here (authz runs first), but if it
		// ever does, watching is the safe interpretation.
		return true
	}
	return user.APIKeyType == types.APIkeytypeEmbed
}
