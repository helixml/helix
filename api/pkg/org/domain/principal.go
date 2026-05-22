package domain

import (
	"github.com/helixml/helix/api/pkg/org/principal"
)

// SourcePrincipal returns Event.Source as a typed
// principal.Principal. Today Source is a worker.ID typed string —
// every production write sets it to the publishing Worker (or
// leaves it empty for system-emitted events). The accessor wraps
// that in a principal so dispatcher / worker_log / UI consumers can
// read one type regardless of whether Source ever widens to carry
// transport-native senders.
//
// Inference rules:
//
//   - empty Source → zero-Principal (system-emitted / inbound
//     transport without a resolved sender)
//   - non-empty Source → KindWorker (today's only populated case)
//
// When future transports start emitting Events whose Source is a
// transport-native identifier rather than a Worker, the Source
// field type itself widens (or grows a sibling Kind column) — this
// accessor is the first step toward that.
func (e Event) SourcePrincipal() principal.Principal {
	if e.Source == "" {
		return principal.Principal{}
	}
	return principal.NewWorker(string(e.Source))
}
