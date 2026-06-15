package streams

import (
	"context"

	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// Outbound delivers an Event out of a Stream whose Transport is
// configured for outbound (webhook POST, email send, …) — the
// send-side mirror of Inbound. One implementation per transport Kind
// lives in that transport's infrastructure package; the composition
// root registers them (on the dispatcher, which calls Emit for every
// appended Event). This keeps the provider-specific delivery mechanism
// (HTTP, email API) out of the application layer.
//
// Emit runs on its own goroutine with a background context (the send
// outlives the request that triggered it); the implementation bounds
// its own time and logs its own failures. A nil Config / absent
// outbound target is a no-op the implementation handles.
type Outbound interface {
	Emit(ctx context.Context, stream streaming.Stream, event streaming.Event) error
}
