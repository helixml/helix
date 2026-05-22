package message

import (
	"strings"

	"github.com/helixml/helix/api/pkg/org/principal"
)

// workerIDPrefix is the canonical prefix every internal Worker ID
// carries (per ADR-0001 / domain.NewWorker conventions). The
// FromPrincipal accessor uses it to disambiguate Worker IDs from
// transport-native sender strings without consulting any external
// store.
const workerIDPrefix = "w-"

// FromPrincipal returns the Message's From field as a typed
// principal.Principal. Inference rules (per 06 §5 / 08 §M6):
//
//   - empty From → zero-Principal (no sender, system-emitted)
//   - From starts with "w-" → KindWorker
//   - anything else → KindTransport (transport-native identifier:
//     email, Slack ID, phone number, IoT device, …)
//
// The typed view lifts the loose-string From into the same Principal
// VO Event.SourcePrincipal returns, so dispatcher / worker_log / UI
// consumers see one type. The underlying From field stays a string
// during the B6 transition; the field-type swap is a later move.
func (m Message) FromPrincipal() principal.Principal {
	if m.From == "" {
		return principal.Principal{}
	}
	if strings.HasPrefix(m.From, workerIDPrefix) {
		return principal.NewWorker(m.From)
	}
	return principal.NewTransport(m.From)
}
