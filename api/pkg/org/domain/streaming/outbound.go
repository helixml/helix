package streaming

import "context"

// Outbound is the domain port for a Stream's outbound transport: delivering
// an appended Event OUT of the Stream (a webhook POST, an email send, …) —
// the send-side mirror of Inbound. One implementation per transport Kind
// lives in that transport's infrastructure package; the dispatcher calls
// Emit for every appended Event on a Stream whose Kind has a registered
// emitter. This keeps the provider-specific delivery mechanism (HTTP,
// email API) out of the application layer.
//
// Emit runs on its own goroutine with a background context (the send
// outlives the request that triggered it); the implementation bounds its
// own time and logs its own failures. A nil Config / absent outbound
// target is a no-op the implementation handles.
type Outbound interface {
	Emit(ctx context.Context, stream Stream, event Event) error
}
