// Package broadcast provides a tiny in-process pub/sub used by long-poll
// readers to wake when a new Event is published to a Stream they care
// about.
//
// Subscribers register interest in a set of Stream IDs and receive an
// empty struct through their wake-up channel when any matching event
// is notified. Multiple rapid-fire notifications coalesce into a single
// wake-up — subscribers are expected to re-query the Events store after
// waking, so "you missed one" cannot actually happen.
package broadcast

import (
	"sync"

	"github.com/helixml/helix/api/pkg/org/stream"
)

// Hub is safe for concurrent use. The zero value is not usable;
// use New.
type Hub struct {
	mu     sync.Mutex
	subs   map[stream.ID]map[chan struct{}]struct{}
	allSub map[chan struct{}]struct{} // SubscribeAll listeners — wake on any Notify
}

// New returns a ready-to-use Hub.
func New() *Hub {
	return &Hub{
		subs:   make(map[stream.ID]map[chan struct{}]struct{}),
		allSub: make(map[chan struct{}]struct{}),
	}
}

// SubscribeAll registers a wake-up channel that fires on every Notify
// regardless of stream ID. Used by the unified /ui/streams live view
// (no specific stream selected) to refresh whenever any worker writes
// anywhere. Caller MUST defer UnsubscribeAll.
func (b *Hub) SubscribeAll() chan struct{} {
	ch := make(chan struct{}, 1)
	b.mu.Lock()
	b.allSub[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

// UnsubscribeAll removes a SubscribeAll listener.
func (b *Hub) UnsubscribeAll(ch chan struct{}) {
	b.mu.Lock()
	delete(b.allSub, ch)
	b.mu.Unlock()
}

// Subscribe registers a wake-up channel for the given Stream IDs and
// returns it. The channel is buffered (size 1) so a notification never
// blocks Notify; coalesced notifications are deliberate.
//
// Callers MUST call Unsubscribe with the same channel and ID set when
// they are done, typically via defer.
func (b *Hub) Subscribe(streamIDs []stream.ID) chan struct{} {
	ch := make(chan struct{}, 1)
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, sid := range streamIDs {
		set, ok := b.subs[sid]
		if !ok {
			set = make(map[chan struct{}]struct{})
			b.subs[sid] = set
		}
		set[ch] = struct{}{}
	}
	return ch
}

// Unsubscribe removes the channel from all per-Stream subscriber sets.
// Safe to call with an empty streamIDs list.
func (b *Hub) Unsubscribe(streamIDs []stream.ID, ch chan struct{}) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, sid := range streamIDs {
		if set, ok := b.subs[sid]; ok {
			delete(set, ch)
			if len(set) == 0 {
				delete(b.subs, sid)
			}
		}
	}
}

// Notify wakes every subscriber that registered interest in streamID.
// Non-blocking: if a subscriber's wake-up channel is already full, the
// signal is coalesced. Subscribers are expected to re-query the store
// after waking.
func (b *Hub) Notify(streamID stream.ID) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs[streamID] {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	for ch := range b.allSub {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
