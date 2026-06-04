// Package streamhub is a small wake-only facade over api/pkg/pubsub
// that preserves the typed streaming.StreamID API the helix-org call sites used
// when they spoke to api/pkg/org/broadcast.
//
// Helix already runs a NATS pubsub (real in production, in-memory in
// tests) — the parallel broadcaster that used to live here was a
// duplicate of that infrastructure. This package is the thin adapter
// that keeps the call-site API (Notify / Subscribe / SubscribeAll)
// but routes the wake signal through pubsub.PubSub.
//
// Wake-only semantics, preserved verbatim from the deleted broadcast
// package:
//
//   - Subscribers register interest in zero-or-more stream.IDs (or
//     "all streams" via SubscribeAll) and receive an empty struct
//     through their wake-up channel when any matching Notify fires.
//   - The wake-up channel is buffered (size 1). A notification that
//     arrives while the buffer is full is dropped — bursty notifies
//     coalesce into a single wake.
//   - Notify is non-blocking. The underlying pubsub Publish is fire-
//     and-forget; the wake handler uses select/default so a full
//     subscriber channel never blocks the publisher.
//   - Subscribers are expected to re-query the store after waking; no
//     payload semantics. The pubsub payload is always nil.
package streamhub

import (
	"context"
	"sync"

	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/pubsub"
)

// topicPrefix is the NATS subject prefix every wake topic shares.
// Concrete topics are "<prefix>.<streamID>". The wildcard form
// "<prefix>.>" matches every per-stream topic, used by SubscribeAll.
//
// Stream IDs are kebab-case (`s-activations-w-alice` etc.) so they
// never contain NATS-special characters (`.`, `*`, `>`, whitespace);
// passing them as a single subject token is safe.
const topicPrefix = "helix-org.stream-updates"

// topicFor returns the NATS subject this stream's wake notifications
// publish to.
func topicFor(sid streaming.StreamID) string {
	return topicPrefix + "." + string(sid)
}

// wildcardTopic is the NATS-wildcard subscription that matches every
// per-stream topic — used by SubscribeAll.
const wildcardTopic = topicPrefix + ".>"

// Hub is safe for concurrent use. The zero value is not usable; use
// New.
type Hub struct {
	ps pubsub.PubSub

	mu sync.Mutex
	// subs tracks the underlying pubsub.Subscriptions that wake each
	// caller-visible channel. UnsubscribeAll / Unsubscribe iterate this
	// map to tear the right ones down.
	subs map[chan struct{}][]pubsub.Subscription
}

// New returns a ready-to-use Hub backed by the supplied pubsub.PubSub.
// The PubSub must outlive the Hub. Panics if ps is nil — there is no
// safe fallback (the broadcast nil-check at call sites is preserved
// at the call site, not here).
func New(ps pubsub.PubSub) *Hub {
	if ps == nil {
		panic("streamhub.New: pubsub.PubSub is nil")
	}
	return &Hub{
		ps:   ps,
		subs: make(map[chan struct{}][]pubsub.Subscription),
	}
}

// signal performs the same buffered-1 drop-on-full coalescing the old
// broadcast package's Notify did, so a slow subscriber never blocks
// the pubsub handler goroutine.
func signal(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// Subscribe registers a wake-up channel for the given stream IDs and
// returns it. The channel is buffered (size 1) so a notification never
// blocks the underlying pubsub delivery; coalesced notifications are
// deliberate.
//
// Callers MUST call Unsubscribe with the same channel and ID set when
// they are done, typically via defer. Subscribing with an empty list
// returns a never-woken channel — equivalent to the broadcast
// behaviour.
func (h *Hub) Subscribe(streamIDs []streaming.StreamID) chan struct{} {
	ch := make(chan struct{}, 1)
	if len(streamIDs) == 0 {
		// Track the channel so Unsubscribe is still a no-op (rather
		// than panicking on a missing key).
		h.mu.Lock()
		h.subs[ch] = nil
		h.mu.Unlock()
		return ch
	}
	handler := func(_ []byte) error {
		signal(ch)
		return nil
	}
	pubsubSubs := make([]pubsub.Subscription, 0, len(streamIDs))
	for _, sid := range streamIDs {
		sub, err := h.ps.Subscribe(context.Background(), topicFor(sid), handler)
		if err != nil {
			// Tear down partial state to avoid leaking subscriptions.
			for _, s := range pubsubSubs {
				_ = s.Unsubscribe()
			}
			// Mirror the broadcast package's never-fail contract by
			// returning a channel that simply never wakes. Callers do
			// not currently check for errors.
			h.mu.Lock()
			h.subs[ch] = nil
			h.mu.Unlock()
			return ch
		}
		pubsubSubs = append(pubsubSubs, sub)
	}
	h.mu.Lock()
	h.subs[ch] = pubsubSubs
	h.mu.Unlock()
	return ch
}

// Unsubscribe tears down the wake-channel's underlying pubsub
// subscriptions.
//
// Semantics-preserving notes vs the old broadcast.Hub.Unsubscribe:
//
//   - An empty or nil streamIDs list is a no-op (matches broadcast).
//     This is the contract the /ui live-view callers and the worker_log
//     defer-chain rely on.
//   - With a non-empty streamIDs list, EVERY pubsub subscription
//     attached to the wake-channel is torn down — irrespective of the
//     individual stream IDs passed in. Every in-tree caller passes
//     back the same slice it supplied to Subscribe, so the visible
//     behaviour is unchanged. (Tracking per-stream pubsub subs would
//     add bookkeeping with no observable benefit; revisit if a caller
//     ever wants partial-unsubscribe.)
func (h *Hub) Unsubscribe(streamIDs []streaming.StreamID, ch chan struct{}) {
	if len(streamIDs) == 0 {
		return
	}
	h.mu.Lock()
	pubsubSubs, ok := h.subs[ch]
	if ok {
		delete(h.subs, ch)
	}
	h.mu.Unlock()
	for _, s := range pubsubSubs {
		_ = s.Unsubscribe()
	}
}

// SubscribeAll registers a wake-up channel that fires on every Notify
// regardless of stream ID. Used by the unified streams live view (no
// specific stream selected) to refresh whenever any worker writes
// anywhere. Caller MUST defer UnsubscribeAll.
//
// Backed by a NATS wildcard subscription (`helix-org.stream-updates.>`)
// so it matches every per-stream topic published via Notify. Both the
// real NATS provider and the in-memory test provider honour wildcard
// subscriptions.
func (h *Hub) SubscribeAll() chan struct{} {
	ch := make(chan struct{}, 1)
	sub, err := h.ps.Subscribe(context.Background(), wildcardTopic, func(_ []byte) error {
		signal(ch)
		return nil
	})
	h.mu.Lock()
	defer h.mu.Unlock()
	if err != nil {
		// Channel that never wakes — preserves broadcast's never-fail
		// contract.
		h.subs[ch] = nil
		return ch
	}
	h.subs[ch] = []pubsub.Subscription{sub}
	return ch
}

// UnsubscribeAll removes a SubscribeAll listener. Safe to call on a
// channel returned by SubscribeAll or Subscribe — both are tracked the
// same way internally.
func (h *Hub) UnsubscribeAll(ch chan struct{}) {
	h.mu.Lock()
	pubsubSubs, ok := h.subs[ch]
	if ok {
		delete(h.subs, ch)
	}
	h.mu.Unlock()
	for _, s := range pubsubSubs {
		_ = s.Unsubscribe()
	}
}

// Notify wakes every subscriber that registered interest in streamID.
// Non-blocking: the underlying pubsub Publish is fire-and-forget and
// the wake handler uses select/default, so a subscriber whose channel
// buffer is full simply drops the signal (coalesced — the subscriber
// is expected to re-query after waking).
func (h *Hub) Notify(streamID streaming.StreamID) {
	// Publish payload is nil — this is a pure wake signal. Ignoring
	// publish errors matches broadcast's contract: Notify was a
	// best-effort wake-only call with no return value.
	_ = h.ps.Publish(context.Background(), topicFor(streamID), nil)
}
