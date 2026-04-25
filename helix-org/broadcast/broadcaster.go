// Package broadcast provides a tiny in-process pub/sub used by the
// long-poll feed handler to wake blocked readers when a new Event is
// published to a Channel they care about.
//
// Subscribers register interest in a set of Channel IDs and receive an
// empty struct through their wake-up channel when any matching event
// is notified. Multiple rapid-fire notifications coalesce into a single
// wake-up — subscribers are expected to re-query the Events store after
// waking, so "you missed one" cannot actually happen.
package broadcast

import (
	"sync"

	"github.com/helixml/helix-org/domain"
)

// Broadcaster is safe for concurrent use. The zero value is not usable;
// use New.
type Broadcaster struct {
	mu   sync.Mutex
	subs map[domain.ChannelID]map[chan struct{}]struct{}
}

// New returns a ready-to-use Broadcaster.
func New() *Broadcaster {
	return &Broadcaster{subs: make(map[domain.ChannelID]map[chan struct{}]struct{})}
}

// Subscribe registers a wake-up channel for the given Channel IDs and
// returns it. The channel is buffered (size 1) so a notification never
// blocks Notify; coalesced notifications are deliberate.
//
// Callers MUST call Unsubscribe with the same channel and ID set when
// they are done, typically via defer.
func (b *Broadcaster) Subscribe(channelIDs []domain.ChannelID) chan struct{} {
	ch := make(chan struct{}, 1)
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, cid := range channelIDs {
		set, ok := b.subs[cid]
		if !ok {
			set = make(map[chan struct{}]struct{})
			b.subs[cid] = set
		}
		set[ch] = struct{}{}
	}
	return ch
}

// Unsubscribe removes the channel from all per-Channel subscriber sets.
// Safe to call with an empty channelIDs list.
func (b *Broadcaster) Unsubscribe(channelIDs []domain.ChannelID, ch chan struct{}) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, cid := range channelIDs {
		if set, ok := b.subs[cid]; ok {
			delete(set, ch)
			if len(set) == 0 {
				delete(b.subs, cid)
			}
		}
	}
}

// Notify wakes every subscriber that registered interest in channelID.
// Non-blocking: if a subscriber's wake-up channel is already full, the
// signal is coalesced. Subscribers are expected to re-query the store
// after waking.
func (b *Broadcaster) Notify(channelID domain.ChannelID) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs[channelID] {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
